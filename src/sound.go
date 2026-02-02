package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"sync"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/effects"
	"github.com/gopxl/beep/v2/midi"
	"github.com/gopxl/beep/v2/wav"
	"github.com/gopxl/beep/v2/vorbis"
)

const (
	audioOutLen          = 2048
	audioFrequency       = 44100
	audioPrecision       = 4
	audioResampleQuality = 1
)

// ------------------------------------------------------------------
// Normalizer

type Normalizer struct {
	streamer beep.Streamer
	mul      float64
	l, r     *NormalizerLR
}

func NewNormalizer(st beep.Streamer) *Normalizer {
	return &Normalizer{streamer: st, mul: 4,
		l: &NormalizerLR{1, 0, 1, 1 / 32.0, 0, 0},
		r: &NormalizerLR{1, 0, 1, 1 / 32.0, 0, 0}}
}

func (n *Normalizer) Stream(samples [][2]float64) (s int, ok bool) {
	if len(samples) <= 0 {
		return 0, false
	}
	s, ok = n.streamer.Stream(samples)
	for i := range samples[:s] {
		lmul := n.l.process(n.mul, &samples[i][0])
		rmul := n.r.process(n.mul, &samples[i][1])
		if sys.cfg.Sound.AudioDucking {
			n.mul = math.Min(16.0, math.Min(lmul, rmul))
		} else {
			n.mul = 0.5 * (float64(sys.cfg.Sound.WavVolume) * float64(sys.cfg.Sound.MasterVolume) * 0.0001)
		}
	}
	return s, ok
}

func (n *Normalizer) Err() error {
	return n.streamer.Err()
}

type NormalizerLR struct {
	edge, edgeDelta, gain, average, bias, bias2 float64
}

func (n *NormalizerLR) process(mul float64, sam *float64) float64 {
	n.bias += (*sam - n.bias) / (float64(sys.cfg.Sound.SampleRate)/110.0 + 1)
	n.bias2 += (*sam - n.bias2) / (float64(sys.cfg.Sound.SampleRate)/112640.0 + 1)
	s := (n.bias2 - n.bias) * mul
	if math.Abs(s) > 1 {
		mul *= math.Pow(math.Abs(s), -n.edge)
		n.edgeDelta += 32 * (1 - n.edge) / float64(sys.cfg.Sound.SampleRate+32)
		s = math.Copysign(1.0, s)
	} else {
		tmp := (1 - math.Pow(1-math.Abs(s), 64)) * math.Pow(0.5-math.Abs(s), 3)
		mul += mul * (n.edge*(1/32.0-n.average)/n.gain + tmp*n.gain*(1-n.edge)/32) /
			(float64(sys.cfg.Sound.SampleRate)*2/8.0 + 1)
		n.edgeDelta -= (0.5 - n.average) * n.edge / (float64(sys.cfg.Sound.SampleRate) * 2)
	}
	n.gain += (1.0 - n.gain*(math.Abs(s)+1/32.0)) / (float64(sys.cfg.Sound.SampleRate) * 2)
	n.average += (math.Abs(s) - n.average) / (float64(sys.cfg.Sound.SampleRate) * 2)
	n.edge = float64(ClampF(float32(n.edge+n.edgeDelta), 0, 1))
	*sam = s
	return mul
}

// ------------------------------------------------------------------
// SwapSeeker - beep.StreamSeeker that can be swapped to in-memory
// copy at runtime.
type SwapSeeker struct {
	mu sync.RWMutex      // read/write mutex
	ss beep.StreamSeeker // current source
}

func newSwapSeeker(ss beep.StreamSeeker) *SwapSeeker {
	return &SwapSeeker{ss: ss}
}

// Swap(next, absStart): next - new seeker
func (sw *SwapSeeker) Swap(next beep.StreamSeeker) {
	speaker.Lock()
	sw.mu.Lock()
	pos := sw.ss.Position()

	if ln := next.Len(); ln <= 0 { // guard against 0
		// empty buffer, don't try to swap this
		sys.errLog.Println("Empty BGM RAM swap buffer somehow. Aborting swap at the absolute last possible moment!")
		sw.mu.Unlock()
		speaker.Unlock()
		return
	}

	next.Seek(pos)
	sw.ss = next
	sw.mu.Unlock()
	speaker.Unlock()
}

func (sw *SwapSeeker) Stream(out [][2]float64) (int, bool) {
	sw.mu.RLock()
	ss := sw.ss
	sw.mu.RUnlock()
	return ss.Stream(out)
}

func (sw *SwapSeeker) Seek(p int) error {
	sw.mu.RLock()
	ss := sw.ss
	err := ss.Seek(p)
	sw.mu.RUnlock()
	return err
}

func (sw *SwapSeeker) Position() int {
	sw.mu.RLock()
	pos := sw.ss.Position()
	sw.mu.RUnlock()
	return pos
}

func (sw *SwapSeeker) Len() int {
	sw.mu.RLock()
	l := sw.ss.Len()
	sw.mu.RUnlock()
	return l
}

func (sw *SwapSeeker) Err() error {
	sw.mu.RLock()
	e := sw.ss.Err()
	sw.mu.RUnlock()
	return e
}

// ------------------------------------------------------------------
// BufferSeeker ‒ wraps *beep.Buffer and gives an in-memory
// StreamSeeker
type BufferSeeker struct {
	buf *beep.Buffer  // the decoded audio buffer
	pos int           // absolute position in samples
	str beep.Streamer // current streamer
}

func newBufferSeeker(buf *beep.Buffer) *BufferSeeker {
	return &BufferSeeker{
		buf: buf,
		str: buf.Streamer(0, buf.Len()),
	}
}

func (b *BufferSeeker) Stream(out [][2]float64) (n int, ok bool) {
	// Pull samples from the current streamer
	n, ok = b.str.Stream(out)
	b.pos += n
	return n, ok
}

func (b *BufferSeeker) Seek(p int) error {
	// Clamp p so we don't panic on bogus values
	if p < 0 {
		p = 0
	} else if p > b.buf.Len() {
		p = b.buf.Len()
	}
	b.pos = p
	// Reset streamer so the next Stream() call starts at p
	b.str = b.buf.Streamer(p, b.buf.Len())
	return nil
}

func (b *BufferSeeker) Position() int { return b.pos }

func (b *BufferSeeker) Len() int { return b.buf.Len() }

func (b *BufferSeeker) Err() error { return nil }

// ------------------------------------------------------------------
// Loop Streamer

// Based on Loop() from Beep package. It adds support for loop points.
type StreamLooper struct {
	s         beep.StreamSeeker
	loopcount int
	loopstart int
	loopend   int
	err       error
}

func newStreamLooper(s beep.StreamSeeker, loopcount, loopstart, loopend int) beep.StreamSeeker {
	sl := &StreamLooper{
		s:         s,
		loopcount: loopcount,
		loopstart: loopstart,
		loopend:   loopend,
	}
	if sl.loopstart < 0 || sl.loopstart >= s.Len() {
		sl.loopstart = 0
	}
	if sl.loopend <= sl.loopstart || sl.loopend >= s.Len() {
		sl.loopend = s.Len()
	}
	return sl
}

// Adapted from beep.Loop2 (for dynamic modification)
func (l *StreamLooper) Stream(samples [][2]float64) (n int, ok bool) {
	if l.err != nil {
		return 0, false
	}
	for len(samples) > 0 {
		toStream := len(samples)
		if l.loopcount != 0 {
			samplesUntilEnd := l.loopend - l.s.Position()
			if samplesUntilEnd <= 0 {
				// End of loop, reset the position and decrease the loop count.
				if l.loopcount > 0 {
					l.loopcount--
				}
				if err := l.s.Seek(l.loopstart); err != nil {
					l.err = err
					return n, true
				}
				continue
			}
			// Stream only up to the end of the loop.
			toStream = MinI(samplesUntilEnd, toStream)
		}

		sn, sok := l.s.Stream(samples[:toStream])
		n += sn
		if sn < toStream || !sok {
			l.err = l.s.Err()
			return n, n > 0
		}
		samples = samples[sn:]
	}
	return n, true
}

func (b *StreamLooper) Err() error {
	return b.s.Err()
}

func (b *StreamLooper) Len() int {
	return b.s.Len()
}

func (b *StreamLooper) Position() int {
	return b.s.Position()
}

func (b *StreamLooper) Seek(p int) error {
	return b.s.Seek(p)
}

// ------------------------------------------------------------------
// Bgm

type Bgm struct {
	filename   string
	bgmVolume  int
	volRestore int
	loop       int
	streamer   beep.StreamSeeker
	ctrl       *beep.Ctrl
	volctrl    *effects.Volume
	format     string
	freqmul    float32
	sampleRate beep.SampleRate
	startPos   int
	mu         sync.Mutex
	cancel     context.CancelFunc
}

func newBgm() *Bgm {
	return &Bgm{}
}

func (bgm *Bgm) Stop() {
	if bgm.ctrl != nil {
		speaker.Lock()
		bgm.ctrl.Streamer = nil
		speaker.Unlock()
	}
	bgm.filename = ""
}

func loadSoundFont(filename string) (*midi.SoundFont, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	soundfont, err := midi.NewSoundFont(f)
	if err != nil {
		f.Close()
		return nil, err
	}
	return soundfont, nil
}

func (bgm *Bgm) SetPaused(pause bool) {
	if bgm.ctrl == nil || bgm.ctrl.Paused == pause {
		return
	}
	speaker.Lock()
	bgm.ctrl.Paused = pause
	speaker.Unlock()
}

func (bgm *Bgm) UpdateVolume() {
	if bgm.volctrl == nil {
		return
	}
	// TODO: Throw a debug warning if this triggers
	if bgm.bgmVolume > sys.cfg.Sound.MaxBGMVolume {
		sys.errLog.Printf("WARNING: BGM volume set beyond expected range (value: %v). Clamped to MaxBgmVolume", bgm.bgmVolume)
		bgm.bgmVolume = sys.cfg.Sound.MaxBGMVolume
	}

	// NOTE: This is what we're going to do, no matter the complaints, because BGMVolume is handled differently
	// than WAV volume anyway.  We've had problems changing this in the past so it's best to keep it as-is.
	volume := -5 + float64(sys.cfg.Sound.BGMVolume)*0.06*(float64(sys.cfg.Sound.MasterVolume)/100)*(float64(bgm.bgmVolume)/100)

	// clamp to 1
	if volume >= 1 {
		volume = 1
	}
	silent := volume <= -5
	speaker.Lock()
	bgm.volctrl.Volume = volume
	bgm.volctrl.Silent = silent
	speaker.Unlock()
}

func (bgm *Bgm) SetFreqMul(freqmul float32) {
	if bgm.freqmul != freqmul {
		if bgm.ctrl != nil {
			// Special case: freqmul == 0 pauses
			if freqmul == 0 {
				// Taken care of in system.go in tickSound
				bgm.freqmul = freqmul
				return
			}
			srcRate := bgm.sampleRate
			dstRate := beep.SampleRate(float32(sys.cfg.Sound.SampleRate) / freqmul)
			if resampler, ok := bgm.ctrl.Streamer.(*beep.Resampler); ok {
				speaker.Lock()
				resampler.SetRatio(float64(srcRate) / float64(dstRate))
				bgm.freqmul = freqmul
				speaker.Unlock()
			}
		}
	}
}

// OpenFromStreamer wires an arbitrary Beep streamer (e.g. Reisen-backed audio)
// into the existing BGM path so the video BGM replaces/uses the same channel.
func (bgm *Bgm) OpenFromStreamer(stream beep.Streamer, srcSampleRate beep.SampleRate, bgmVolume int) {
	// Right away, cancel any running goroutines.
	bgm.mu.Lock()
	if bgm.cancel != nil {
		bgm.cancel()
	}
	var ctx context.Context
	ctx, bgm.cancel = context.WithCancel(context.Background())
	_ = ctx // reserved for future use (mirrors Open)
	bgm.mu.Unlock()

	bgm.filename = "<video-stream>"
	bgm.loop = 0
	bgm.bgmVolume = bgmVolume
	bgm.freqmul = 1

	// Starve the current music streamer
	if bgm.ctrl != nil {
		speaker.Lock()
		bgm.ctrl.Streamer = nil
		speaker.Unlock()
	}
	// Honor CLI flags just like normal Open()
	if _, ok := sys.cmdFlags["-nomusic"]; ok {
		return
	}
	if _, ok := sys.cmdFlags["-nosound"]; ok {
		return
	}

	// Build the standard BGM chain: Volume -> Resample -> Ctrl -> Mixer
	bgm.sampleRate = srcSampleRate
	bgm.volctrl = &effects.Volume{Streamer: stream, Base: 2, Volume: 0, Silent: true}
	dstFreq := beep.SampleRate(float32(sys.cfg.Sound.SampleRate) / bgm.freqmul)
	resampler := beep.Resample(audioResampleQuality, bgm.sampleRate, dstFreq, bgm.volctrl)
	bgm.ctrl = &beep.Ctrl{Streamer: resampler}
	bgm.volRestore = 0
	bgm.UpdateVolume()
	speaker.Play(bgm.ctrl)
}

func (bgm *Bgm) SetLoopPoints(bgmLoopStart int, bgmLoopEnd int) {
	// Set both at once, why not
	if sl, ok := bgm.volctrl.Streamer.(*StreamLooper); ok {
		if sl.loopstart != bgmLoopStart && sl.loopend != bgmLoopEnd {
			speaker.Lock()
			sl.loopstart = bgmLoopStart
			sl.loopend = bgmLoopEnd
			speaker.Unlock()
			// Set one at a time
		} else {
			if sl.loopstart != bgmLoopStart {
				speaker.Lock()
				sl.loopstart = bgmLoopStart
				speaker.Unlock()
			} else if sl.loopend != bgmLoopEnd {
				speaker.Lock()
				sl.loopend = bgmLoopEnd
				speaker.Unlock()
			}
		}
	}
}

func (bgm *Bgm) Seek(positionSample int) {
	// For stream-only sources (e.g., video audio) we don't support seeking; ignore safely.
	if bgm.streamer == nil {
		return
	}
	speaker.Lock()
	// Reset to 0 if out of range
	if positionSample < 0 || positionSample > bgm.streamer.Len() {
		positionSample = 0
	}
	_ = bgm.streamer.Seek(positionSample)
	speaker.Unlock()
}

// ------------------------------------------------------------------
// Sound

type Sound struct {
	soundData   []byte
	format      beep.Format
	length      int
	soundFormat byte
}

func readSound(f io.ReadSeekCloser, size uint32) (*Sound, error) {
	var soundFormat byte = 0
	var s beep.StreamSeekCloser
	var beepFormat beep.Format
	var err error

	if size < 128 {
		return nil, fmt.Errorf("wav size is too small")
	}
	soundData := make([]byte, size)
	if _, err := f.Read(soundData); err != nil {
		return nil, err
	}
	// Decode the sound at least once, so that we know the format is OK
	s, beepFormat, err = wav.Decode(bytes.NewReader(soundData))
	if err == nil {
		soundFormat = 1
	} else {
		s, beepFormat, err = vorbis.Decode(io.NopCloser(bytes.NewReader(soundData)))
		if err == nil {
			soundFormat = 2
		}	
	}
	// Check if the file can be fully played
	// Run a decode test and catch any panics.
	var recovered interface{}
	func() {
		defer func() {
			// Catch any panic that occurs inside this anonymous function.
			if r := recover(); r != nil {
				recovered = r
			}
		}()

		// Try streaming until the end of the file.
		var samples [512][2]float64
		for {
			n, ok := s.Stream(samples[:])
			if n == 0 || !ok {
				// When the end is reached normally.
				if s.Err() == nil && s.Position() >= s.Len() {
					break
				}
				// Other errors.
				if s.Err() != nil {
					// Errors not caught by recover() are stored here.
					recovered = s.Err()
				}
				break
			}
		}
	}()
	// If a panic was caught.
	if recovered != nil {
		return nil, nil // If sound wasn't able to be fully played, we disable it to avoid engine freezing
	}
	return &Sound{soundData, beepFormat, s.Len(), soundFormat}, nil
}

func (s *Sound) GetStreamer() beep.StreamSeeker {
	switch s.soundFormat {
	case 1:
		streamer, _, _ := wav.Decode(bytes.NewReader(s.soundData))
		return streamer
	case 2:
		streamer, _, _ := vorbis.Decode(io.NopCloser(bytes.NewReader(s.soundData)))
		return streamer
	default:
		return nil
	}
}

// ------------------------------------------------------------------
// Snd

type Snd struct {
	table     map[[2]int32]*Sound
	ver, ver2 uint16
}

func newSnd() *Snd {
	return &Snd{table: make(map[[2]int32]*Sound)}
}

func LoadSnd(filename string) (*Snd, error) {
	s, err := LoadSndFiltered(filename, func(gn [2]int32) bool { return gn[0] >= 0 && gn[1] >= 0 }, 0)
	if err != nil {
		return nil, Error(fmt.Sprintf("LoadSnd failed: %v\n%v", filename, err))
	}
	return s, nil
}

// Parse a .snd file and return an Snd structure with its contents
// The "keepItem" function allows to filter out unwanted waves.
// If max > 0, the function returns immediately when a matching entry is found. It also gives up after "max" non-matching entries.
func LoadSndFiltered(filename string, keepItem func([2]int32) bool, max uint32) (*Snd, error) {
	s := newSnd()
	f, err := OpenFile(filename)
	if err != nil {
		return nil, err
	}
	defer func() { chk(f.Close()) }()
	buf := make([]byte, 12)
	var n int
	if n, err = f.Read(buf); err != nil {
		return nil, err
	}
	if string(buf[:n]) != "ElecbyteSnd\x00" {
		return nil, Error("Unrecognized SND file, invalid header")
	}
	read := func(x interface{}) error {
		return binary.Read(f, binary.LittleEndian, x)
	}
	if err := read(&s.ver); err != nil {
		return nil, err
	}
	if err := read(&s.ver2); err != nil {
		return nil, err
	}
	var numberOfSounds uint32
	if err := read(&numberOfSounds); err != nil {
		return nil, err
	}
	var subHeaderOffset uint32
	if err := read(&subHeaderOffset); err != nil {
		return nil, err
	}
	loops := numberOfSounds
	if max > 0 && max < numberOfSounds {
		loops = max
	}
	for i := uint32(0); i < loops; i++ {
		f.Seek(int64(subHeaderOffset), 0)
		var nextSubHeaderOffset uint32
		if err := read(&nextSubHeaderOffset); err != nil {
			return nil, err
		}
		var subFileLength uint32
		if err := read(&subFileLength); err != nil {
			return nil, err
		}
		var num [2]int32
		if err := read(&num); err != nil {
			return nil, err
		}
		if keepItem(num) {
			_, ok := s.table[num]
			if !ok {
				tmp, err := readSound(f, subFileLength)
				if err != nil {
					sys.errLog.Printf("%v sound %v,%v can't be read: %v\n", filename, num[0], num[1], err)
					if max > 0 {
						return nil, err
					}
				} else {
					// Sound is corrupted and can't be played, so we export a warning message to the console
					if tmp == nil {
						sys.appendToConsole(fmt.Sprintf("WARNING: %v sound %v,%v is corrupted and can't be played, so it was disabled", filename, num[0], num[1]))
					}
					s.table[num] = tmp
					if max > 0 {
						break
					}
				}
			}
		}
		subHeaderOffset = nextSubHeaderOffset
	}
	return s, nil
}

func (s *Snd) Get(gn [2]int32) *Sound {
	return s.table[gn]
}

func (s *Snd) play(gn [2]int32, volumescale int32, pan float32, loopstart, loopend, startposition int) bool {
	sound := s.Get(gn)
	return sys.soundChannels.Play(sound, gn[0], gn[1], volumescale, pan, loopstart, loopend, startposition)
}

func (s *Snd) stop(gn [2]int32) {
	sound := s.Get(gn)
	sys.soundChannels.Stop(sound)
}

func loadFromSnd(filename string, g, s int32, max uint32) (*Sound, error) {
	// Load the snd file
	snd, err := LoadSndFiltered(filename, func(gn [2]int32) bool { return gn[0] == g && gn[1] == s }, max)
	if err != nil {
		return nil, err
	}
	tmp, ok := snd.table[[2]int32{g, s}]
	if !ok {
		return nil, nil
	}
	return tmp, nil
}

// ------------------------------------------------------------------
// SoundEffect (handles volume and panning)

type SoundEffect struct {
	streamer beep.Streamer
	volume   float32
	ls, p    float32
	x        *float32
	priority int32
	channel  int32
	loop     int32
	freqmul  float32
	startPos int
}

func (s *SoundEffect) Stream(samples [][2]float64) (n int, ok bool) {
	// TODO: Test mugen panning in relation to PanningWidth and zoom settings
	lv, rv := s.volume, s.volume
	if sys.cfg.Sound.StereoEffects && (s.x != nil || s.p != 0) {
		var r float32
		if s.x != nil { // pan
			r = ((sys.xmax - s.ls**s.x) - s.p) / (sys.xmax - sys.xmin)
		} else { // abspan
			r = ((sys.xmax-sys.xmin)/2 - s.p) / (sys.xmax - sys.xmin)
		}
		sc := sys.cfg.Sound.PanningRange / 100
		of := (100 - sys.cfg.Sound.PanningRange) / 200
		lv = ClampF(s.volume*2*(r*sc+of), 0, 512)
		rv = ClampF(s.volume*2*((1-r)*sc+of), 0, 512)
	}

	n, ok = s.streamer.Stream(samples)
	for i := range samples[:n] {
		samples[i][0] *= float64(lv / 256)
		samples[i][1] *= float64(rv / 256)
	}
	return n, ok
}

func (s *SoundEffect) Err() error {
	return s.streamer.Err()
}

// ------------------------------------------------------------------
// SoundChannel

type SoundChannel struct {
	streamer          beep.StreamSeeker
	sfx               *SoundEffect
	ctrl              *beep.Ctrl
	sound             *Sound
	stopOnGetHit      bool
	stopOnChangeState bool
	group             int32
	number            int32
	timeStamp         int32
}

func (s *SoundChannel) Play(sound *Sound, group, number, loop int32, freqmul float32, loopStart, loopEnd, startPosition int) {
	if sound == nil {
		return
	}

	s.sound = sound
	s.group = group
	s.number = number
	s.timeStamp = sys.gameTime()
	s.streamer = s.sound.GetStreamer()

	loopCount := int(0)
	if loop < 0 {
		loopCount = -1
	} else {
		loopCount = MaxI(0, int(loop-1))
	}

	// going to continue using our streamLooper which is now modified from beep.Loop2
	looper := newStreamLooper(s.streamer, loopCount, loopStart, loopEnd)
	s.sfx = &SoundEffect{streamer: looper, volume: 256, priority: 0, channel: -1, loop: int32(loopCount), freqmul: freqmul, startPos: startPosition}
	srcRate := s.sound.format.SampleRate
	dstRate := beep.SampleRate(float32(sys.cfg.Sound.SampleRate) / s.sfx.freqmul)
	resampler := beep.Resample(audioResampleQuality, srcRate, dstRate, s.sfx)
	s.ctrl = &beep.Ctrl{Streamer: resampler}
	s.streamer.Seek(startPosition)
	sys.soundMixer.Add(s.ctrl)
}

func (s *SoundChannel) IsPlaying() bool {
	return s.sound != nil
}

func (s *SoundChannel) SetPaused(pause bool) {
	if s.ctrl == nil || s.ctrl.Paused == pause {
		return
	}
	speaker.Lock()
	s.ctrl.Paused = pause
	speaker.Unlock()
}

func (s *SoundChannel) Stop() {
	if s.ctrl != nil {
		speaker.Lock()
		s.ctrl.Streamer = nil
		speaker.Unlock()
	}
	s.sound = nil
}

func (s *SoundChannel) SetVolume(vol float32) {
	if s.ctrl != nil {
		s.sfx.volume = ClampF(vol, 0, 512)
	}
}

func (s *SoundChannel) SetPan(p, ls float32, x *float32) {
	if s.ctrl != nil {
		s.sfx.ls = ls
		s.sfx.x = x
		s.sfx.p = p * ls
	}
}

func (s *SoundChannel) SetPriority(priority int32) {
	if s.ctrl != nil {
		s.sfx.priority = priority
	}
}

func (s *SoundChannel) SetChannel(channel int32) {
	if s.ctrl != nil {
		s.sfx.channel = channel
	}
}

func (s *SoundChannel) SetFreqMul(freqmul float32) {
	if s.ctrl != nil {
		if s.sound != nil {
			// Special case: freqmul == 0 pauses
			if freqmul == 0 {
				s.sfx.freqmul = freqmul
				s.SetPaused(true)
				return
			}
			srcRate := s.sound.format.SampleRate
			dstRate := beep.SampleRate(float32(sys.cfg.Sound.SampleRate) / freqmul)
			if resampler, ok := s.ctrl.Streamer.(*beep.Resampler); ok {
				speaker.Lock()
				resampler.SetRatio(float64(srcRate) / float64(dstRate))
				s.sfx.freqmul = freqmul
				speaker.Unlock()
			}
		}
	}
}

func (s *SoundChannel) SetLoopPoints(loopstart, loopend int) {
	// Set both at once, why not
	if sl, ok := s.sfx.streamer.(*StreamLooper); ok {
		if sl.loopstart != loopstart && sl.loopend != loopend {
			speaker.Lock()
			sl.loopstart = loopstart
			sl.loopend = loopend
			speaker.Unlock()
			// Set one at a time
		} else {
			if sl.loopstart != loopstart {
				speaker.Lock()
				sl.loopstart = loopstart
				speaker.Unlock()
			} else if sl.loopend != loopend {
				speaker.Lock()
				sl.loopend = loopend
				speaker.Unlock()
			}
		}
	}
}

// ------------------------------------------------------------------
// SoundChannels (collection of prioritised sound channels)

type SoundChannels struct {
	channels  []SoundChannel
	volResume []float32
}

func newSoundChannels(size int32) *SoundChannels {
	s := &SoundChannels{}
	s.SetSize(size)
	return s
}

func (s *SoundChannels) SetSize(size int32) {
	if size > s.count() {
		c := make([]SoundChannel, size-s.count())
		v := make([]float32, size-s.count())
		s.channels = append(s.channels, c...)
		s.volResume = append(s.volResume, v...)
	} else if size < s.count() {
		for i := s.count() - 1; i >= size; i-- {
			s.channels[i].Stop()
		}
		s.channels = s.channels[:size]
		s.volResume = s.volResume[:size]
	}
}

func (s *SoundChannels) count() int32 {
	return int32(len(s.channels))
}

func (s *SoundChannels) New(ch int32, lowpriority bool, priority int32) *SoundChannel {
	if ch >= 0 && ch < sys.cfg.Sound.WavChannels {
		for i := s.count() - 1; i >= 0; i-- {
			if s.channels[i].IsPlaying() && s.channels[i].sfx.channel == ch {
				if (lowpriority && priority <= s.channels[i].sfx.priority) || priority < s.channels[i].sfx.priority {
					return nil
				}
				s.channels[i].Stop()
				return &s.channels[i]
			}
		}
	}
	if s.count() < sys.cfg.Sound.WavChannels {
		s.SetSize(sys.cfg.Sound.WavChannels)
	}
	for i := sys.cfg.Sound.WavChannels - 1; i >= 0; i-- {
		if !s.channels[i].IsPlaying() {
			return &s.channels[i]
		}
	}
	return nil
}

func (s *SoundChannels) reserveChannel() *SoundChannel {
	for i := range s.channels {
		if !s.channels[i].IsPlaying() {
			return &s.channels[i]
		}
	}
	return nil
}

func (s *SoundChannels) Get(ch int32) *SoundChannel {
	if ch >= 0 && ch < s.count() {
		for i := range s.channels {
			if s.channels[i].IsPlaying() && s.channels[i].sfx != nil && s.channels[i].sfx.channel == ch {
				return &s.channels[i]
			}
		}
		//return &s.channels[ch]
	}
	return nil
}

func (s *SoundChannels) Play(sound *Sound, group, number, volumescale int32, pan float32, loopStart, loopEnd, startPosition int) bool {
	if sound == nil {
		return false
	}
	c := s.reserveChannel()
	if c == nil {
		return false
	}
	c.Play(sound, group, number, 0, 1.0, loopStart, loopEnd, startPosition)
	c.SetVolume(float32(volumescale * 64 / 25))
	c.SetPan(pan, 0, nil)
	return true
}

func (s *SoundChannels) IsPlaying(sound *Sound) bool {
	for i := range s.channels {
		v := &s.channels[i]
		if v.sound != nil && v.sound == sound {
			return true
		}
	}
	return false
}

func (s *SoundChannels) Stop(sound *Sound) {
	for i := range s.channels {
		v := &s.channels[i]
		if v.sound != nil && v.sound == sound {
			v.Stop()
		}
	}
}

func (s *SoundChannels) StopAll() {
	for i := range s.channels {
		if s.channels[i].sound != nil {
			s.channels[i].Stop()
		}
	}
}

func (s *SoundChannels) Tick() {
	for i := range s.channels {
		v := &s.channels[i]
		if v.IsPlaying() {
			if v.streamer.Position() >= v.sound.length && v.sfx.loop != -1 { // End the sound
				v.sound = nil
			}
		}
	}
}
