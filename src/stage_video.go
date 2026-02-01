//go:build !lite

package main

import (
	"encoding/binary"
	"fmt"
	"image"
	"math"
	"strings"
	"time"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/effects"
	"github.com/ikemen-engine/reisen"
)

type bgVideo struct {
	errs            chan error
	frameBuffer     chan *image.RGBA
	audioBuffer     chan []float64 // interleaved L,R float64 samples
	quit            chan struct{}  // signals the decode goroutine to exit
	done            chan struct{}  // closed when the goroutine exits
	audioStream     *reisen.AudioStream
	videoStream     *reisen.VideoStream
	media           *reisen.Media
	loop            bool
	texture         Texture
	startWall       time.Time
	basePTS         time.Duration
	haveBasePTS     bool
	elapsedPTS      time.Duration
	lastFrame       *image.RGBA
	volume          int
	scaleMode       BgVideoScaleMode
	flag            BgVideoScaleFilter
	playing         bool
	visible         bool
	videoCtrl       *beep.Ctrl
	videoVol        *effects.Volume
	audioSampleRate beep.SampleRate
	inMixer         bool
}

const (
	frameBufferSize = 10
)

// reisenAudioStreamer adapts chunks of interleaved float64 samples from a channel
// into a Beep Streamer. It never blocks the audio callback: when starved, it emits
// silence to keep clocking; when the channel is closed and pending is drained, it ends.
type reisenAudioStreamer struct {
	ch      <-chan []float64
	pending []float64
	closed  bool
}

func (rs *reisenAudioStreamer) Err() error {
	return nil
}

func (rs *reisenAudioStreamer) Stream(out [][2]float64) (n int, ok bool) {
	// Fill out as much as we can. If we run out of pending data and the channel
	// is closed, we end; otherwise we output silence for the remainder to avoid XRuns.
	for i := 0; i < len(out); i++ {
		if len(rs.pending) < 2 {
			if !rs.closed {
				select {
				case chunk, okc := <-rs.ch:
					if okc {
						rs.pending = append(rs.pending, chunk...)
					} else {
						rs.closed = true
					}
				default:
					// No data right now. Emit silence this sample to keep the callback brief.
					out[i][0], out[i][1] = 0, 0
					n++
					continue
				}
			}
			if rs.closed && len(rs.pending) < 2 {
				// No more data and nothing pending: end of stream.
				return n, n > 0
			}
		}
		out[i][0] = rs.pending[0]
		out[i][1] = rs.pending[1]
		rs.pending = rs.pending[2:]
		n++
	}
	return n, true
}

// Non-blocking, close-safe drains (return immediately if channel is closed/empty).
func drainAudio(ch <-chan []float64) {
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		default:
			return
		}
	}
}

func drainFrames(ch <-chan *image.RGBA) {
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		default:
			return
		}
	}
}

func (bgv *bgVideo) Open(filename string, volume int, sm BgVideoScaleMode, sf BgVideoScaleFilter, loop bool) error {
	// fmt.Println("Opening media file:", filename)
	m, err := reisen.NewMedia(filename)
	if err != nil {
		return err
	}

	//bgv.describe()

	bgv.volume = volume
	bgv.scaleMode = sm
	bgv.flag = sf
	bgv.loop = loop
	bgv.media = m
	bgv.playing = false
	bgv.visible = true
	bgv.elapsedPTS = 0
	bgv.haveBasePTS = false

	bgv.frameBuffer = make(chan *image.RGBA, frameBufferSize)
	bgv.audioBuffer = make(chan []float64, 128)
	bgv.errs = make(chan error)
	bgv.quit = make(chan struct{})
	bgv.done = make(chan struct{})

	err = bgv.media.OpenDecode()
	if err != nil {
		return err
	}

	videoStreams := bgv.media.VideoStreams()
	if len(videoStreams) == 0 {
		return fmt.Errorf("No decodable video streams in %s (check codecs in your FFmpeg build)", filename)
	}

	err = videoStreams[0].Open()
	if err != nil {
		return err
	}

	// Configure FFmpeg scaling/padding via AVFilter (scale+pad) for video.
	// We compute the desired target based on window size and policy.
	if v := videoStreams[0]; v != nil {
		srcW, srcH := v.Width(), v.Height()
		winW, winH := int(sys.scrrect[2]), int(sys.scrrect[3])

		if fg := buildFFFilterGraph(srcW, srcH, winW, winH, sm, sf); fg != "" {
			if err := v.ApplyVideoFilterGraph(fg); err != nil {
				// Don't fail playback if filter graph can't be applied; fall back to sws_scale path.
				sys.errLog.Printf("video: ApplyVideoFilterGraph failed (%v) for graph '%s', using sws_scale fallback", err, fg)
			}
		}
	}
	bgv.videoStream = videoStreams[0]

	// Try to open the first audio stream, if any
	audioStreams := bgv.media.AudioStreams()
	if len(audioStreams) > 0 {
		if err := audioStreams[0].Open(); err == nil {
			bgv.audioStream = audioStreams[0]
			// Build an independent chain mixed into sys.soundMixer
			bgv.audioSampleRate = beep.SampleRate(audioStreams[0].SampleRate())
			rs := &reisenAudioStreamer{ch: bgv.audioBuffer}
			bgv.videoVol = &effects.Volume{Streamer: rs, Base: 2}
			// Resample video audio to match engine output sample rate
			dst := beep.SampleRate(sys.cfg.Sound.SampleRate)
			resampler := beep.Resample(audioResampleQuality, bgv.audioSampleRate, dst, bgv.videoVol)
			bgv.videoCtrl = &beep.Ctrl{Streamer: resampler, Paused: true} // start paused until SetPlaying(true)
			sys.soundMixer.Add(bgv.videoCtrl)
			bgv.inMixer = true
			bgv.volume = volume
			bgv.updateAudioVolume()
		} else {
			return fmt.Errorf("Audio stream open failed: %v", err)
		}
	}

	// Normalize timeline
	if err := videoStreams[0].Rewind(0); err != nil {
		return fmt.Errorf("Rewind(0) failed: %v", err)
	}
	if bgv.audioStream != nil {
		_ = bgv.audioStream.Rewind(0)
	}

	bgv.haveBasePTS = false

	// Decode loop. When EOF is reached and loop==true, rewind to t=0 and continue.
	go func() {
		defer close(bgv.done)
		for {
			// Allow external shutdown.
			select {
			case <-bgv.quit:
				goto finish
			default:
			}
			// Do not demux/decode while paused: that keeps A/V frozen and avoids audio blips.
			if !bgv.playing {
				time.Sleep(5 * time.Millisecond)
				continue
			}
			gotPacket := bgv.processPacket()
			if gotPacket {
				continue
			}
			// No packet => demuxer exhausted or error already reported via errs.
			// If looping is requested, rewind to the beginning and reset pacing.
			if bgv.loop {
				// Prefer rewinding the VIDEO stream to keep A/V in sync per reisen docs.
				if err := bgv.videoStream.Rewind(0); err != nil {
					bgv.errs <- fmt.Errorf("loop rewind video failed: %v", err)
					break
				}
				// Optional: also rewind audio if present; safe no-op if demux-only.
				if bgv.audioStream != nil {
					_ = bgv.audioStream.Rewind(0)
				}
				// Drop any queued audio/video so the new loop starts clean.
				drainAudio(bgv.audioBuffer)
				drainFrames(bgv.frameBuffer)
				// Also clear any pending samples in the Beep adapter.
				if bgv.videoVol != nil {
					if rs, ok := bgv.videoVol.Streamer.(*reisenAudioStreamer); ok {
						rs.pending = nil
						rs.closed = false
					}
				}
				// Reset pacing baseline so PresentationOffset() maps to a fresh wall clock.
				bgv.haveBasePTS = false
				bgv.elapsedPTS = 0
				bgv.startWall = time.Now() // re-anchor wall clock for producer-side sleep
				continue
			}
			// Not looping -> finish and clean up.
			break
		}
	finish:
		// Cleanup only when we actually finish (i.e., not looping forever).
		bgv.videoStream.Close()
		if bgv.audioStream != nil {
			bgv.audioStream.Close()
		}
		if bgv.media != nil {
			bgv.media.CloseDecode()
		}
		// Detach from mixer
		if bgv.videoCtrl != nil {
			speaker.Lock()
			bgv.videoCtrl.Streamer = nil
			speaker.Unlock()
		}
		bgv.inMixer = false
		close(bgv.frameBuffer)
		close(bgv.audioBuffer)
		close(bgv.errs)
	}()

	return nil
}

func (bgv *bgVideo) describe() error {
	// Print the media properties.
	dur, err := bgv.media.Duration()
	if err != nil {
		return err
	}
	fmt.Println("Duration:", dur)
	fmt.Println("Format name:", bgv.media.FormatName())
	fmt.Println("Format long name:", bgv.media.FormatLongName())
	fmt.Println("MIME type:", bgv.media.FormatMIMEType())
	fmt.Println("Number of streams:", bgv.media.StreamCount())
	fmt.Println()

	// Enumerate the media file streams.
	for _, stream := range bgv.media.Streams() {
		dur, err := stream.Duration()
		if err != nil {
			return err
		}
		tbNum, tbDen := stream.TimeBase()
		fpsNum, fpsDen := stream.FrameRate()
		fmt.Println("Index:", stream.Index())
		fmt.Println("Stream type:", stream.Type())
		fmt.Println("Codec name:", stream.CodecName())
		fmt.Println("Codec long name:", stream.CodecLongName())
		fmt.Println("Stream duration:", dur)
		fmt.Println("Stream bit rate:", stream.BitRate())
		fmt.Printf("Time base: %d/%d\n", tbNum, tbDen)
		fmt.Printf("Frame rate: %d/%d\n", fpsNum, fpsDen)
		fmt.Println("Frame count:", stream.FrameCount())
		fmt.Println()
	}
	return nil
}

func (bgv *bgVideo) processPacket() bool {
	if bgv.media == nil {
		// No media yet; nothing to do.
		return false
	}
	packet, gotPacket, err := bgv.media.ReadPacket()
	if err != nil {
		bgv.errs <- err
	}

	if !gotPacket {
		return false
	}

	switch packet.Type() {
	case reisen.StreamVideo:
		s := bgv.media.Streams()[packet.StreamIndex()].(*reisen.VideoStream)
		vf, gotFrame, err := s.ReadVideoFrame()

		if err != nil {
			bgv.errs <- err
		}

		// Keep decoding even if this packet didn't yield a frame.
		if gotFrame && vf != nil {
			// Pace on producer side: sleep until the frame's presentation time.
			if off, err := vf.PresentationOffset(); err == nil {
				// Rebase to first frame
				if !bgv.haveBasePTS {
					bgv.basePTS = off
					bgv.haveBasePTS = true
				}
				elapsed := off - bgv.basePTS
				if elapsed < 0 {
					elapsed = 0
				}
				bgv.elapsedPTS = elapsed
				sleepUntil := bgv.startWall.Add(elapsed)
				d := time.Until(sleepUntil)
				if d > 0 {
					time.Sleep(d)
				}
			}
			// Deliver only when visible; while hidden we still pace timers but drop frames.
			if bgv.visible {
				bgv.frameBuffer <- vf.Image()
				bgv.lastFrame = vf.Image() // remember last for sticky reupload
			}
		}

	case reisen.StreamAudio:
		// Only push audio while playing; otherwise streamer emits silence.
		if !bgv.playing {
			return true
		}
		// Decode to float64 interleaved samples and push to audioBuffer.
		// Reisen delivers stereo float64 as little-endian bytes (L,R,L,R,...) per frame.
		// We do NOT sleep here; the Beep speaker drives timing and back-pressures via the channel.
		s := bgv.media.Streams()[packet.StreamIndex()].(*reisen.AudioStream)
		af, gotFrame, err := s.ReadAudioFrame()
		if err != nil {
			bgv.errs <- err
		}
		if gotFrame && af != nil {
			raw := af.Data()
			if len(raw) >= 16 { // at least one stereo sample (2 * float64)
				count := len(raw) / 8
				samples := make([]float64, 0, count)
				for i := 0; i+8 <= len(raw); i += 8 {
					u := binary.LittleEndian.Uint64(raw[i : i+8])
					samples = append(samples, math.Float64frombits(u))
				}
				if bgv.visible {
					// Never block the decode loop on audio delivery.
					// If speaker can't keep up, drop this chunk to keep video moving.
					select {
					case bgv.audioBuffer <- samples:
					default:
						// drop
					}
				} else {
					// Hidden: drop audio to remain silent while timers continue to run.
				}
			}
		}
	}

	return true
}

func (bgv *bgVideo) Tick() error {
	select {
	case err, ok := <-bgv.errs:
		if ok {
			return err
		}

	default:
	}

	// Non-blocking receive so render loop never stalls
	select {
	case frame, ok := <-bgv.frameBuffer:
		if !ok {
			// Decoder finished (loop=0). Do not keep rendering a stale/blank texture.
			bgv.texture = nil
			bgv.lastFrame = nil
			// Keep audio path paused if it exists.
			if bgv.videoCtrl != nil {
				speaker.Lock()
				bgv.videoCtrl.Paused = true
				speaker.Unlock()
			}
			return nil
		}
		// Upload the (possibly FFmpeg-scaled/padded) frame as-is.
		rect := frame.Bounds()
		w := int32(rect.Dx())
		h := int32(rect.Dy())
		if bgv.texture == nil || w != bgv.texture.GetWidth() || h != bgv.texture.GetHeight() {
			bgv.texture = gfx.newTexture(w, h, 32, true)
		}
		bgv.texture.SetData(frame.Pix)
		bgv.lastFrame = frame
	default:
		// No new frame right now. Re-upload last to keep video visible.
		if bgv.lastFrame != nil {
			rect := bgv.lastFrame.Bounds()
			w := int32(rect.Dx())
			h := int32(rect.Dy())
			if bgv.texture == nil || w != bgv.texture.GetWidth() || h != bgv.texture.GetHeight() {
				bgv.texture = gfx.newTexture(w, h, 32, true)
			}
			bgv.texture.SetData(bgv.lastFrame.Pix)
		}
	}

	return nil
}

// SetPlaying toggles decode/audio production and (re)anchors the pacing clock.
func (bgv *bgVideo) SetPlaying(on bool) {
	// If the decode goroutine has already exited (EOF or Close), do not try to
	// "resume" a dead stream (can happen on pause/unpause or BGCtrl toggles).
	if bgv.done != nil {
		select {
		case <-bgv.done:
			bgv.playing = false
			if bgv.videoCtrl != nil {
				speaker.Lock()
				bgv.videoCtrl.Paused = true
				speaker.Unlock()
			}
			return
		default:
		}
	}
	if on == bgv.playing {
		return
	}
	if on {
		// Ensure we're attached to the mixer (it may have been cleared).
		if bgv.videoCtrl != nil && !bgv.inMixer {
			sys.soundMixer.Add(bgv.videoCtrl)
			bgv.inMixer = true
		}
		// If we have not established a baseline PTS in this decode epoch,
		// rewind to t=0 so the first decoded frame is a keyframe.
		if !bgv.haveBasePTS && bgv.videoStream != nil {
			_ = bgv.videoStream.Rewind(0)
		}
		// Anchor wall clock so that current elapsedPTS displays "now".
		bgv.startWall = time.Now().Add(-bgv.elapsedPTS)
		bgv.playing = true
	} else {
		// Freeze: decode loop will idle until re-enabled.
		bgv.playing = false
	}
	// Also pause/unpause the mixer-side ctrl for CPU savings.
	if bgv.videoCtrl != nil {
		speaker.Lock()
		bgv.videoCtrl.Paused = !on
		speaker.Unlock()
	}
}

// SetVisible controls whether decoded A/V is presented.
// When turning invisible, drain any queued audio so the mixer goes silent immediately.
func (bgv *bgVideo) SetVisible(on bool) {
	if on == bgv.visible {
		return
	}
	bgv.visible = on
	if !on {
		// Drop queued audio so the streamer outputs silence right away.
		drainAudio(bgv.audioBuffer)
		// Drop queued video frames so we don't re-upload a stale image later.
		drainFrames(bgv.frameBuffer)
	}
}

// Reset rewinds streams to t=0 and clears pacing; playback remains paused.
func (bgv *bgVideo) Reset() {
	if bgv.videoStream != nil {
		_ = bgv.videoStream.Rewind(0)
	}
	if bgv.audioStream != nil {
		_ = bgv.audioStream.Rewind(0)
	}
	// Purge any queued samples/frames (close-safe).
	drainAudio(bgv.audioBuffer)
	drainFrames(bgv.frameBuffer)
	// Clear any pending samples in the Beep adapter (if present).
	if bgv.videoVol != nil {
		if rs, ok := bgv.videoVol.Streamer.(*reisenAudioStreamer); ok {
			rs.pending = nil
			rs.closed = false
		}
	}
	// Drop previously uploaded GPU texture and sticky frame to prevent flashes.
	bgv.texture = nil
	bgv.lastFrame = nil
	bgv.haveBasePTS = false
	bgv.elapsedPTS = 0
}

// buildFFFilterGraph builds a scale(+optional crop/pad)+format filtergraph string for FFmpeg.
func buildFFFilterGraph(sw, sh, ww, wh int, sm BgVideoScaleMode, sf BgVideoScaleFilter) string {
	if ww <= 0 || wh <= 0 || sw <= 0 || sh <= 0 {
		return ""
	}

	// Map our filter enum to FFmpeg scale flags.
	flag := "bicubic"
	switch sf {
	case SF_FastBilinear:
		flag = "fast_bilinear"
	case SF_Bilinear:
		flag = "bilinear"
	case SF_Bicubic:
		flag = "bicubic"
	case SF_Experimental:
		flag = "experimental"
	case SF_Neighbor:
		flag = "neighbor"
	case SF_Area:
		flag = "area"
	case SF_Bicublin:
		flag = "bicublin"
	case SF_Gauss:
		flag = "gauss"
	case SF_Sinc:
		flag = "sinc"
	case SF_Lanczos:
		flag = "lanczos"
	case SF_Spline:
		flag = "spline"
	}

	// Helpers
	scaleExact := func(w, h int) string {
		if w < 1 {
			w = 1
		}
		if h < 1 {
			h = 1
		}
		return fmt.Sprintf("scale=%d:%d:flags=%s", w, h, flag)
	}
	// Ask FFmpeg to keep AR and compute the other dimension; -2 keeps it even when needed.
	scaleToW := func(w int) string {
		if w < 1 {
			w = 1
		}
		return fmt.Sprintf("scale=%d:-2:flags=%s:force_divisible_by=2", w, flag)
	}
	scaleToH := func(h int) string {
		if h < 1 {
			h = 1
		}
		return fmt.Sprintf("scale=-2:%d:flags=%s:force_divisible_by=2", h, flag)
	}
	padCenter := func(w, h int) string {
		return fmt.Sprintf("pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black", w, h)
	}
	// Center-crop vertically to a maximum height; no-op if ih<=h.
	cropCenterH := func(w, h int) string {
		// After width-constrained scale, iw≈w; keep width w, clamp height to h.
		return fmt.Sprintf("crop=%d:min(ih\\,%d):0:floor((ih-min(ih\\,%d))/2)", w, h, h)
	}
	// Center-crop horizontally to a maximum width; no-op if iw<=w.
	cropCenterW := func(w, h int) string {
		// After height-constrained scale, ih≈h; keep height h, clamp width to w.
		return fmt.Sprintf("crop=min(iw\\,%d):%d:floor((iw-min(iw\\,%d))/2):0", w, h, w)
	}

	var parts []string
	switch sm {
	case SM_None:
		// None: draw at native resolution, no scaling or padding.
		return ""

	case SM_Stretch:
		// Stretch: fill window exactly, distorting aspect ratio (no bars, no crop).
		parts = append(parts, scaleExact(ww, wh), "format=rgba")

	case SM_Fit:
		// Fit (contain): uniform scale so entire video fits inside window; add bars if needed.
		parts = append(parts,
			fmt.Sprintf("scale=%d:%d:flags=%s:force_original_aspect_ratio=decrease:force_divisible_by=2", ww, wh, flag),
			padCenter(ww, wh),
			"format=rgba",
		)

	case SM_FitWidth:
		// FitWidth: match window width; keep AR.
		// If height exceeds window -> center-crop vertically; if smaller -> pad vertically.
		parts = append(parts,
			scaleToW(ww),
			cropCenterH(ww, wh), // crops when ih>wh; no-op when ih<=wh
			padCenter(ww, wh),   // pads when ih<wh; no-op when ih>=wh
			"format=rgba",
		)

	case SM_FitHeight:
		// FitHeight: match window height; keep AR.
		// If width exceeds window -> center-crop horizontally; if smaller -> pad horizontally.
		parts = append(parts,
			scaleToH(wh),
			cropCenterW(ww, wh), // crops when iw>ww; no-op when iw<=ww
			padCenter(ww, wh),   // pads when iw<ww; no-op when iw>=ww
			"format=rgba",
		)

	case SM_ZoomFill:
		// ZoomFill (cover): uniform scale until content covers the window; center-crop overflow.
		parts = append(parts,
			fmt.Sprintf("scale=%d:%d:flags=%s:force_original_aspect_ratio=increase:force_divisible_by=2", ww, wh, flag),
			// Now both iw>=ww and ih>=wh, so crop center to the window.
			fmt.Sprintf("crop=%d:%d:floor((iw-%d)/2):floor((ih-%d)/2)", ww, wh, ww, wh),
			"format=rgba",
		)
	}

	return strings.Join(parts, ",")
}

// updateAudioVolume applies the same dB mapping as BGM so video audio
// behaves like music but on a separate mixer path.
func (bgv *bgVideo) updateAudioVolume() {
	if bgv.videoVol == nil {
		return
	}
	// Match BGM.UpdateVolume logic:
	vol := -5 + float64(sys.cfg.Sound.BGMVolume)*0.06*
		(float64(sys.cfg.Sound.MasterVolume)/100.0)*
		(float64(bgv.volume)/100.0)
	if vol >= 1 {
		vol = 1
	}
	silent := vol <= -5
	speaker.Lock()
	bgv.videoVol.Volume = vol
	bgv.videoVol.Silent = silent
	speaker.Unlock()
}

// MixerCleared should be called when sys.soundMixer.Clear() is executed,
// so the next SetPlaying(true) can re-attach this stream.
func (bgv *bgVideo) MixerCleared() {
	bgv.inMixer = false
}

// Close stops decoding and frees resources. Safe to call multiple times.
func (bgv *bgVideo) Close() {
	// Quiesce producers and renderer first.
	bgv.SetPlaying(false)
	bgv.SetVisible(false)
	// If already closed, return.
	select {
	case <-bgv.done:
		return
	default:
	}
	// Signal goroutine to exit; cleanup happens there.
	if bgv.quit != nil {
		close(bgv.quit)
	}
}

type backGround struct {
	_type                 BgType
	palfx                 *PalFX
	anim                  *Animation
	bga                   bgAction
	video                 *bgVideo
	id                    int32
	start                 [2]float32
	xofs                  float32
	delta                 [2]float32
	width                 [2]int32
	xscale                [2]float32
	rasterx               [2]float32
	yscalestart           float32
	yscaledelta           float32
	actionno              int32
	startv                [2]float32
	startrad              [2]float32
	startsint             [2]int32
	startsinlt            [2]int32
	visible               bool
	enabled               bool
	positionlink          bool
	layerno               int32
	autoresizeparallax    bool
	autoresizeparallaxSet bool
	notmaskwindow         int32
	startrect             [4]int32
	windowdelta           [2]float32
	scalestart            [2]float32
	scaledelta            [2]float32
	zoomdelta             [2]float32
	zoomdeltaSet          bool
	zoomscaledelta        [2]float32
	xbottomzoomdelta      float32
	roundpos              bool
	rot                   Rotation
	fLength               float32
	projection            Projection
	xshear                float32
}

func (s *BGDef) action() {
	// TODO: We could merge stage and motif BGCtrl's further. A lot of it is the same
	for i := range s.bgc {
		bgc := &s.bgc[i]
		if bgc.starttime < 0 || (bgc.looptime >= 0 && bgc.starttime >= bgc.looptime) {
			continue
		}

		if bgc.looptime > 0 && bgc.endtime > bgc.looptime {
			bgc.endtime = bgc.looptime
		}

		active := false
		if s.time >= bgc.starttime {
			if bgc.looptime > 0 {
				duration := bgc.endtime - bgc.starttime
				if (s.time-bgc.starttime)%bgc.looptime <= duration {
					active = true
				}
			} else {
				if s.time <= bgc.endtime {
					active = true
				}
			}
		}

		if active {
			s.runBgCtrl(bgc)
		}
	}

	// After BGCtrl mutates states, align video play/pause to "active"
	for i := range s.bg {
		if s.bg[i].video != nil {
			// Apply visibility first to avoid initial audio blip at t=0 when Visible=0.
			s.bg[i].video.SetVisible(s.bg[i].visible)
			s.bg[i].video.SetPlaying(s.bg[i].enabled)
		}
	}

	s.bga.action(true)
	if s.model != nil {
		s.model.step(1)
	}

	// Global time must be incremented after updating BGCtrl
	// https://github.com/ikemen-engine/Ikemen-GO/issues/2656
	s.time++

	link := 0
	for i, b := range s.bg {
		s.bg[i].bga.action(b.enabled)
		if i > 0 && b.positionlink {
			s.bg[i].bga.offset[0] += s.bg[link].bga.sinoffset[0]
			s.bg[i].bga.offset[1] += s.bg[link].bga.sinoffset[1]
		} else {
			link = i
		}
		if b.enabled {
			s.bg[i].anim.Action()
		}
	}
}

func readBackGround(is IniSection, link *backGround,
	sff *Sff, at AnimationTable, sProps StageProps, def string, startlayer int32) (*backGround, error) {
	bg := newBackGround(sff)
	typ := is["type"]
	if len(typ) == 0 {
		return bg, nil
	}
	switch typ[0] {
	case 'N', 'n':
		bg._type = BG_Normal
	case 'A', 'a':
		bg._type = BG_Anim
	case 'P', 'p':
		bg._type = BG_Parallax
	case 'V', 'v':
		bg._type = BG_Video
		bg.video = &bgVideo{}
	case 'D', 'd':
		bg._type = BG_Dummy
	default:
		return bg, nil
	}
	var tmp int32
	is.ReadI32("layerno", &bg.layerno)
	bg.layerno += startlayer

	if bg._type == BG_Video {
		if bg.video == nil {
			bg.video = &bgVideo{}
		}
		path := is["path"]
		LoadFile(&path, []string{def, "", sys.motif.Def, "data/", "video/"}, func(filename string) error {
			path = filename
			return nil
		})
		if len(path) != 0 {
			volume := 100
			if v, ok := is["volume"]; ok {
				volume = int(Atoi(v))
			}

			var sm BgVideoScaleMode
			if v, ok := is["scalemode"]; ok {
				switch strings.ToLower(strings.TrimSpace(v)) {
				case "none":
					sm = SM_None
				case "stretch":
					sm = SM_Stretch
				case "fit":
					sm = SM_Fit
				case "fitwidth":
					sm = SM_FitWidth
				case "fitheight":
					sm = SM_FitHeight
				case "zoomfill":
					sm = SM_ZoomFill
				default:
					return nil, Error("Invalid BG Video scale mode: " + v)
				}
			}

			var sf BgVideoScaleFilter
			if v, ok := is["scalefilter"]; ok {
				switch strings.ToLower(strings.TrimSpace(v)) {
				case "fastbilinear":
					sf = SF_FastBilinear
				case "bilinear":
					sf = SF_Bilinear
				case "bicubic":
					sf = SF_Bicubic
				case "experimental":
					sf = SF_Experimental
				case "neighbor":
					sf = SF_Neighbor
				case "area":
					sf = SF_Area
				case "bicublin":
					sf = SF_Bicublin
				case "gauss":
					sf = SF_Gauss
				case "sinc":
					sf = SF_Sinc
				case "lanczos":
					sf = SF_Lanczos
				case "spline":
					sf = SF_Spline
				default:
					return nil, Error("Invalid BG Video scale filter: " + v)
				}
			}

			var loop bool
			is.ReadBool("loop", &loop)

			if err := bg.video.Open(path, volume, sm, sf, loop); err != nil {
				return nil, err
			}
		}
	} else if bg._type != BG_Dummy {
		var hasAnim bool
		if (bg._type != BG_Normal || len(is["spriteno"]) == 0) &&
			is.ReadI32("actionno", &bg.actionno) {
			if a := at.get(bg.actionno); a != nil {
				bg.anim = a
				hasAnim = true
			}
		}
		if hasAnim {
			if bg._type == BG_Normal {
				bg._type = BG_Anim
			}
		} else {
			var g, n int32
			if is.readI32ForStage("spriteno", &g, &n) {
				bg.anim.frames = []AnimFrame{*newAnimFrame()}
				bg.anim.frames[0].Group, bg.anim.frames[0].Number = g, n
			}
			if is.ReadI32("mask", &tmp) {
				if tmp != 0 {
					bg.anim.mask = 0
				} else {
					bg.anim.mask = -1
				}
			}
		}
	}
	is.ReadBool("positionlink", &bg.positionlink)
	if bg.positionlink && link != nil {
		bg.startv = link.startv
		bg.delta = link.delta
	}
	if _, ok := is["autoresizeparallax"]; ok {
		bg.autoresizeparallaxSet = true
		is.ReadBool("autoresizeparallax", &bg.autoresizeparallax)
	}
	is.readF32ForStage("start", &bg.start[0], &bg.start[1])
	if !bg.positionlink {
		is.readF32ForStage("delta", &bg.delta[0], &bg.delta[1])
	}
	is.readF32ForStage("scalestart", &bg.scalestart[0], &bg.scalestart[1])
	is.readF32ForStage("scaledelta", &bg.scaledelta[0], &bg.scaledelta[1])
	is.readF32ForStage("xshear", &bg.xshear)
	is.readF32ForStage("angle", &bg.rot.angle)
	is.readF32ForStage("xangle", &bg.rot.xangle)
	is.readF32ForStage("yangle", &bg.rot.yangle)
	is.readF32ForStage("focallength", &bg.fLength)
	if str, ok := is["projection"]; ok {
		switch strings.ToLower(strings.TrimSpace(str)) {
		case "orthographic":
			bg.projection = Projection_Orthographic
		case "perspective":
			bg.projection = Projection_Perspective
		case "perspective2":
			bg.projection = Projection_Perspective2
		}
	}
	is.readF32ForStage("xbottomzoomdelta", &bg.xbottomzoomdelta)
	is.readF32ForStage("zoomscaledelta", &bg.zoomscaledelta[0], &bg.zoomscaledelta[1])
	if is.readF32ForStage("zoomdelta", &bg.zoomdelta[0], &bg.zoomdelta[1]) {
		bg.zoomdeltaSet = true
	}
	if bg.zoomdelta[0] != math.MaxFloat32 && bg.zoomdelta[1] == math.MaxFloat32 {
		bg.zoomdelta[1] = bg.zoomdelta[0]
	}

	// Read transparency
	if data, ok := is["trans"]; ok {
		switch strings.ToLower(data) {
		case "add":
			bg.anim.mask = 0
			bg.anim.transType = TT_add
			bg.anim.srcAlpha = 255
			bg.anim.dstAlpha = 255
		case "add1":
			bg.anim.mask = 0
			bg.anim.transType = TT_add
			bg.anim.srcAlpha = 255
			bg.anim.dstAlpha = 128
		case "addalpha":
			bg.anim.mask = 0
			bg.anim.transType = TT_add
			bg.anim.srcAlpha = 255
			bg.anim.dstAlpha = 0 // In Mugen it defaults to this before reading the alpha
		case "sub":
			bg.anim.mask = 0
			bg.anim.transType = TT_sub
			bg.anim.srcAlpha = 255
			bg.anim.dstAlpha = 255
		case "none":
			// In Mugen this does the same as Default
			// TODO: Make ikemenversion fix it
			//bg.anim.transType = TT_none
			bg.anim.transType = TT_default
			bg.anim.srcAlpha = 255
			bg.anim.dstAlpha = 0
		case "default":
			bg.anim.transType = TT_default
			bg.anim.srcAlpha = 255
			bg.anim.dstAlpha = 0
		default:
			return nil, Error("Invalid trans type: " + data)
		}
	}

	// Read alpha if applicable
	if bg.anim.transType == TT_add || bg.anim.transType == TT_sub {
		if _, ok := is["alpha"]; ok {
			s, d := int32(bg.anim.srcAlpha), int32(bg.anim.dstAlpha)
			if is.readI32ForStage("alpha", &s, &d) {
				bg.anim.srcAlpha = int16(Clamp(s, 0, 255))
				bg.anim.dstAlpha = int16(Clamp(d, 0, 255))
			}
		}
	}

	if is.readI32ForStage("tile", &bg.anim.tile.xflag, &bg.anim.tile.yflag) {
		if bg._type == BG_Parallax {
			bg.anim.tile.yflag = 0
		}
		if bg.anim.tile.xflag < 0 {
			bg.anim.tile.xflag = math.MaxInt32
		}
	}
	if bg._type == BG_Parallax {
		if !is.readI32ForStage("width", &bg.width[0], &bg.width[1]) {
			is.readF32ForStage("xscale", &bg.rasterx[0], &bg.rasterx[1])
		}
		is.ReadF32("yscalestart", &bg.yscalestart)
		is.ReadF32("yscaledelta", &bg.yscaledelta)
	} else {
		is.ReadI32("tilespacing", &bg.anim.tile.xspacing, &bg.anim.tile.yspacing)
		//bg.anim.tile.yspacing = bg.anim.tile.xspacing
		if bg.actionno < 0 && len(bg.anim.frames) > 0 {
			group := bg.anim.frames[0].Group
			number := bg.anim.frames[0].Number
			if group >= 0 && number >= 0 {
				if spr := sff.GetSprite(uint16(group), uint16(number)); spr != nil {
					bg.anim.tile.xspacing += int32(spr.Size[0])
					bg.anim.tile.yspacing += int32(spr.Size[1])
				}
			}
		} else {
			if bg.anim.tile.xspacing == 0 {
				bg.anim.tile.xflag = 0
			}
			if bg.anim.tile.yspacing == 0 {
				bg.anim.tile.yflag = 0
			}
		}
	}
	if is.readI32ForStage("window", &bg.startrect[0], &bg.startrect[1],
		&bg.startrect[2], &bg.startrect[3]) {
		bg.startrect[2] = Max(0, bg.startrect[2]+1-bg.startrect[0])
		bg.startrect[3] = Max(0, bg.startrect[3]+1-bg.startrect[1])
		bg.notmaskwindow = 1
	}
	if is.readI32ForStage("maskwindow", &bg.startrect[0], &bg.startrect[1],
		&bg.startrect[2], &bg.startrect[3]) {
		bg.startrect[2] = Max(0, bg.startrect[2]-bg.startrect[0])
		bg.startrect[3] = Max(0, bg.startrect[3]-bg.startrect[1])
		bg.notmaskwindow = 0
	}
	is.readF32ForStage("windowdelta", &bg.windowdelta[0], &bg.windowdelta[1])
	is.ReadI32("id", &bg.id)
	is.readF32ForStage("velocity", &bg.startv[0], &bg.startv[1])
	for i := 0; i < 2; i++ {
		var name string
		if i == 0 {
			name = "sin.x"
		} else {
			name = "sin.y"
		}
		r, slt, st := float32(math.NaN()), float32(math.NaN()), float32(math.NaN())
		if is.readF32ForStage(name, &r, &slt, &st) {
			if !math.IsNaN(float64(r)) {
				bg.startrad[i], bg.bga.radius[i] = r, r
			}
			if !math.IsNaN(float64(slt)) {
				var slti int32
				is.readI32ForStage(name, &tmp, &slti)
				bg.startsinlt[i], bg.bga.sinlooptime[i] = slti, slti
			}
			if bg.bga.sinlooptime[i] > 0 && !math.IsNaN(float64(st)) {
				bg.bga.sintime[i] = int32(st*float32(bg.bga.sinlooptime[i])/360) %
					bg.bga.sinlooptime[i]
				if bg.bga.sintime[i] < 0 {
					bg.bga.sintime[i] += bg.bga.sinlooptime[i]
				}
				bg.startsint[i] = bg.bga.sintime[i]
			}
		}
	}
	if !is.ReadBool("roundpos", &bg.roundpos) {
		bg.roundpos = sProps.roundpos
	}
	return bg, nil
}

func (bg backGround) draw(pos [2]float32, drawscl, bgscl, stglscl float32,
	stgscl [2]float32, shakeY float32, isStage bool) {

	// Handle parallax scaling (type = 2)
	scalestartX := bg.scalestart[0]
	if bg._type == BG_Parallax && (bg.width[0] != 0 || bg.width[1] != 0) && bg.anim.spr != nil {
		bg.xscale[0] = float32(bg.width[0]) / float32(bg.anim.spr.Size[0])
		bg.xscale[1] = float32(bg.width[1]) / float32(bg.anim.spr.Size[0])
		scalestartX = AbsF(scalestartX)
		bg.xofs = scalestartX * ((-float32(bg.width[0]) / 2) + float32(bg.anim.spr.Offset[0])*bg.xscale[0])
		bg.anim.isParallax = true
	}

	// Calculate raster x ratio and base x scale
	xras := (bg.rasterx[1] - bg.rasterx[0]) / bg.rasterx[0]
	xbs, dx := bg.xscale[1], MaxF(0, bg.delta[0]*bgscl)

	// Initialize local scaling factors
	var sclx_recip, sclx, scly float32 = 1, 1, 1
	lscl := [...]float32{stglscl * stgscl[0], stglscl * stgscl[1]}

	// Handle zoom scaling if zoomdelta is specified
	var Yzoomdelta float32 = 1
	if bg.zoomdelta[0] != math.MaxFloat32 {
		sclx = drawscl + (1-drawscl)*(1-bg.zoomdelta[0])
		scly = drawscl + (1-drawscl)*(1-bg.zoomdelta[1])
		Yzoomdelta = bg.zoomdelta[1]
		if !bg.autoresizeparallax {
			sclx_recip = 1 + bg.zoomdelta[0]*((1/(sclx*lscl[0])*lscl[0])-1)
		}
	} else {
		sclx = MaxF(0, drawscl+(1-drawscl)*(1-dx))
		scly = MaxF(0, drawscl+(1-drawscl)*(1-MaxF(0, bg.delta[1]*bgscl)))
		Yzoomdelta = MaxF(0, bg.delta[1]*bgscl)
	}

	// Adjust x scale and x bottom zoom if autoresizeparallax is enabled
	if sclx != 0 && bg.autoresizeparallax {
		tmp := 1 / sclx
		if bg.xbottomzoomdelta != math.MaxFloat32 {
			xbs *= MaxF(0, drawscl+(1-drawscl)*(1-bg.xbottomzoomdelta*(xbs/bg.xscale[0]))) * tmp
		} else {
			xbs *= MaxF(0, drawscl+(1-drawscl)*(1-dx*(xbs/bg.xscale[0]))) * tmp
		}
		tmp *= MaxF(0, drawscl+(1-drawscl)*(1-dx*(xras+1)))
		xras -= tmp - 1
		xbs *= tmp
	}

	// Adjust scaling based on zoomscaledelta if available
	var xs3, ys3 float32 = 1, 1
	if bg.zoomscaledelta[0] != math.MaxFloat32 {
		xs3 = (drawscl + (1-drawscl)*(1-bg.zoomscaledelta[0])) / sclx
	}
	if bg.zoomscaledelta[1] != math.MaxFloat32 {
		ys3 = (drawscl + (1-drawscl)*(1-bg.zoomscaledelta[1])) / scly
	}

	// This handles the flooring of the camera position in MUGEN versions earlier than 1.0.
	var x, yScrollPos float32
	if bg.roundpos {
		x = bg.start[0] + bg.xofs - float32(Floor(pos[0]/stgscl[0]))*bg.delta[0] + bg.bga.offset[0]
		yScrollPos = float32(Floor(pos[1]/drawscl/stgscl[1])) * bg.delta[1]
		for i := 0; i < 2; i++ {
			pos[i] = float32(math.Floor(float64(pos[i])))
		}
	} else {
		x = bg.start[0] + bg.xofs - pos[0]/stgscl[0]*bg.delta[0] + bg.bga.offset[0]
		// Hires breaks ydelta scrolling vel, so bgscl was commented from here.
		yScrollPos = (pos[1] / drawscl / stgscl[1]) * bg.delta[1] // * bgscl
	}

	y := bg.start[1] - yScrollPos + bg.bga.offset[1]

	//In MUGEN, if Boundhigh is a positive value, the reference pos of yscaledelta will change
	var positiveBoundhigh float32
	if isStage && sys.cam.boundhigh > 0 {
		positiveBoundhigh = float32(sys.cam.boundhigh) * bg.delta[1] * bgscl / drawscl / stgscl[1]
	}
	// Calculate Y scaling based on vertical scroll position and delta
	ys2 := bg.scaledelta[1] * pos[1] * bg.delta[1] * bgscl / drawscl / stgscl[1]
	ys := ((100-(pos[1]-positiveBoundhigh)*bg.yscaledelta)*bgscl/bg.yscalestart)*bg.scalestart[1] + ys2
	xs := bg.scaledelta[0] * pos[0] * bg.delta[0] * bgscl / stgscl[0]
	x *= bgscl

	// Apply stage logic if BG is part of a stage
	if isStage {
		zoff := float32(sys.cam.zoffset) * stglscl
		y = y*bgscl + ((zoff-shakeY)/scly-zoff)/stglscl/stgscl[1]
		y -= sys.cam.aspectcorrection / (scly * stglscl * stgscl[1])
		y -= (sys.cam.zoomanchorcorrection / (scly * stglscl * stgscl[1])) * Yzoomdelta
	} else {
		y = y*bgscl + ((float32(sys.gameHeight)-shakeY)/stglscl/scly-240)/stgscl[1]
	}

	// Final scaling factors
	sclx *= lscl[0]
	scly *= stglscl * stgscl[1]

	// Calculate window scale
	var wscl [2]float32
	for i := range wscl {
		if bg.zoomdelta[i] != math.MaxFloat32 {
			wscl[i] = MaxF(0, drawscl+(1-drawscl)*(1-MaxF(0, bg.zoomdelta[i]))) * bgscl * lscl[i]
		} else {
			wscl[i] = MaxF(0, drawscl+(1-drawscl)*(1-MaxF(0, bg.windowdelta[i]*bgscl))) * bgscl * lscl[i]
		}
	}

	// Calculate window top left corner position
	rect := bg.startrect

	startrect0 := float32(rect[0]) - (pos[0])/stgscl[0]*bg.windowdelta[0] +
		(float32(sys.gameWidth)/2/sclx - float32(bg.notmaskwindow)*(float32(sys.gameWidth)/2)*(1/lscl[0]))
	startrect0 *= sys.widthScale * wscl[0]
	if !isStage && wscl[0] == 1 {
		// Screenpacks X coordinates start from left edge of screen
		startrect0 += float32(sys.gameWidth-320) / 2 * sys.widthScale
	}

	startrect1 := float32(rect[1]) - pos[1]/drawscl/stgscl[1]*bg.windowdelta[1]
	if isStage {
		zoff := float32(sys.cam.zoffset) * stglscl
		startrect1 += (zoff-shakeY)/scly - zoff/stglscl/stgscl[1]
		startrect1 -= sys.cam.aspectcorrection / scly
		startrect1 -= (sys.cam.zoomanchorcorrection / scly) * Yzoomdelta
	}
	startrect1 *= sys.heightScale * wscl[1]
	startrect1 -= shakeY

	// Determine final window
	rect[0] = int32(math.Floor(float64(startrect0)))
	rect[1] = int32(math.Floor(float64(startrect1)))
	rect[2] = int32(math.Floor(float64(startrect0 + (float32(rect[2]) * sys.widthScale * wscl[0]) - float32(rect[0]))))
	rect[3] = int32(math.Floor(float64(startrect1 + (float32(rect[3]) * sys.heightScale * wscl[1]) - float32(rect[1]))))

	// Render background if it's within the screen area
	if rect[0] < sys.scrrect[2] && rect[1] < sys.scrrect[3] && rect[0]+rect[2] > 0 && rect[1]+rect[3] > 0 {
		if bg._type == BG_Video {
			if bg.video == nil {
				return
			}
			bg.video.Tick()
			if bg.video.texture == nil {
				return
			}

			bg.anim.isVideo = true
			bg.anim.spr = newSprite()
			bg.anim.spr.Tex = bg.video.texture

			w := float32(bg.video.texture.GetWidth())
			h := float32(bg.video.texture.GetHeight())
			//if bg.video.scaleMode != SM_None {
			//	w /= sys.widthScale
			//	h /= sys.heightScale
			//}
			bg.anim.spr.Size = [2]uint16{
				uint16(math.Ceil(float64(w))),
				uint16(math.Ceil(float64(h))),
			}

			bg.anim.scale_x = 1
			bg.anim.scale_y = 1

		}

		// Xshear offset correction
		xsoffset := -bg.xshear * SignF(bg.scalestart[1]) * (float32(bg.anim.spr.Offset[1]) * scly)

		if bg.rot.angle != 0 {
			xsoffset /= bg.rot.angle
		}

		// Choose render origin: top-left for screenpack/storyboard videos, center for everything else
		var rcx float32
		if bg._type != BG_Video || isStage {
			rcx = float32(sys.gameWidth) / 2
		}

		bg.anim.Draw(&rect, x-xsoffset, y, sclx, scly,
			bg.xscale[0]*bgscl*(scalestartX+xs)*xs3,
			xbs*bgscl*(scalestartX+xs)*xs3,
			ys*ys3, xras*x/(AbsF(ys*ys3)*lscl[1]*float32(bg.anim.spr.Size[1])*bg.scalestart[1])*sclx_recip*bg.scalestart[1]-bg.xshear,
			bg.rot, rcx, bg.palfx, 1, [2]float32{1, 1}, int32(bg.projection), bg.fLength, 0, false)
	}
}

func (s *Stage) action() {
	// Handle Music
	s.music.act()

	link, zlink := 0, -1
	canStep := sys.tickFrame() && !s.paused()

	// Update animations and controllers
	if canStep {
		s.bgCtrlAction()
		s.bga.action(true)

		if s.model != nil {
			s.model.step(sys.turbo)
		}
	}

	// Always (every frame) sync decoder run state to global pause + Enable.
	// This prevents the decoder clock from advancing during pause.
	for i := range s.bg {
		if s.bg[i]._type == BG_Video && s.bg[i].video != nil {
			shouldPlay := s.bg[i].enabled && canStep
			// Apply visibility first so there's no frame-0 audio when Visible=0.
			s.bg[i].video.SetVisible(s.bg[i].visible)
			s.bg[i].video.SetPlaying(shouldPlay)
		}
	}

	// Update BG elements
	for i, b := range s.bg {
		b.palfx.step()

		// BGPalFX can step even if the stage is paused
		if sys.bgPalFX.enable {
			// TODO: Finish proper synthesization of bgPalFX into PalFX from bg element
			// (Right now, bgPalFX just overrides all unique parameters from BG Elements' PalFX)
			// for j := 0; j < 3; j++ {
			// if sys.bgPalFX.invertall {
			// b.palfx.eAdd[j] = -b.palfx.add[j] * (b.palfx.mul[j]/256) + 256 * (1-(b.palfx.mul[j]/256))
			// b.palfx.eMul[j] = 256
			// }
			// b.palfx.eAdd[j] = int32((float32(b.palfx.eAdd[j])) * sys.bgPalFX.eColor)
			// b.palfx.eMul[j] = int32(float32(b.palfx.eMul[j]) * sys.bgPalFX.eColor + 256*(1-sys.bgPalFX.eColor))
			// }
			// b.palfx.synthesize(sys.bgPalFX)
			b.palfx.eAdd = sys.bgPalFX.eAdd
			b.palfx.eMul = sys.bgPalFX.eMul
			b.palfx.eColor = sys.bgPalFX.eColor
			b.palfx.eHue = sys.bgPalFX.eHue
			b.palfx.eInvertall = sys.bgPalFX.eInvertall
			b.palfx.eInvertblend = sys.bgPalFX.eInvertblend
			b.palfx.eAllowNeg = sys.bgPalFX.eAllowNeg
		}

		if canStep {
			s.bg[i].bga.action(b.enabled)
			if i > 0 && b.positionlink {
				bgasinoffset0 := s.bg[link].bga.sinoffset[0]
				bgasinoffset1 := s.bg[link].bga.sinoffset[1]
				if s.hires {
					bgasinoffset0 = bgasinoffset0 / 2
					bgasinoffset1 = bgasinoffset1 / 2
				}
				s.bg[i].bga.offset[0] += bgasinoffset0
				s.bg[i].bga.offset[1] += bgasinoffset1
			} else {
				link = i
			}
			if s.zoffsetlink >= 0 && zlink < 0 && b.id == s.zoffsetlink {
				zlink = i
				s.bga.offset[1] += b.bga.offset[1]
			}
		}

		if b.enabled && canStep {
			s.bg[i].anim.Action()
		}
	}

	// Update model PalFX
	if s.model != nil {
		s.model.pfx.step()
		if sys.bgPalFX.enable {
			s.model.pfx.eAdd = sys.bgPalFX.eAdd
			s.model.pfx.eMul = sys.bgPalFX.eMul
			s.model.pfx.eColor = sys.bgPalFX.eColor
			s.model.pfx.eHue = sys.bgPalFX.eHue
			s.model.pfx.eInvertall = sys.bgPalFX.eInvertall
			s.model.pfx.eInvertblend = sys.bgPalFX.eInvertblend
			s.model.pfx.eAllowNeg = sys.bgPalFX.eAllowNeg
		}
	}
}

func (s *Stage) reset() {
	s.stageTime = 0
	s.sff.palList.ResetRemap()
	s.bga.clear()
	for i := range s.bg {
		s.bg[i].reset()
		// Ensure videos start paused, then rewind.
		if s.bg[i]._type == BG_Video && s.bg[i].video != nil {
			s.bg[i].video.SetPlaying(false)
			s.bg[i].video.Reset()
		}
	}
	if s.model != nil {
		s.model.reset()
	}
	// No need to reset BGCtrl at the moment. Tied to stagetime
}

// destroy stops any background video media so the stage can be safely discarded.
func (s *Stage) destroy() {
	for _, b := range s.bg {
		if b != nil && b._type == BG_Video && b.video != nil {
			b.video.Close()
		}
	}
}

func (s *System) clearMatchSound() {
	s.stopAllCharSound()
	// Quiesce stage videos so no background decoding continues while mixer is empty,
	// and mark them as detached so SetPlaying(true) can re-attach next frame.
	if s.stage != nil {
		for _, b := range s.stage.bg {
			if b != nil && b._type == BG_Video {
				b.video.SetPlaying(false)
				b.video.SetVisible(false)
				b.video.MixerCleared()
			}
		}
	}
}
