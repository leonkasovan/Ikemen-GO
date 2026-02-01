//go:build lite

package main

import (
	"context"
	"fmt"
	"io"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/effects"
	"github.com/gopxl/beep/v2/flac"
	"github.com/gopxl/beep/v2/midi"
	"github.com/gopxl/beep/v2/mp3"
	"github.com/gopxl/beep/v2/vorbis"
	"github.com/gopxl/beep/v2/wav"
)

func (bgm *Bgm) Open(filename string, loop, bgmVolume, bgmLoopStart, bgmLoopEnd, startPosition int, freqmul float32, loopcount int) {
	// Right away, cancel any running goroutines.
	bgm.mu.Lock()
	if bgm.cancel != nil {
		bgm.cancel()
	}
	var ctx context.Context
	ctx, bgm.cancel = context.WithCancel(context.Background())
	bgm.mu.Unlock()

	bgm.filename = filename
	bgm.loop = loop
	bgm.bgmVolume = bgmVolume
	bgm.freqmul = freqmul
	// Starve the current music streamer
	if bgm.ctrl != nil {
		speaker.Lock()
		bgm.ctrl.Streamer = nil
		speaker.Unlock()
	}
	// Special value "" is used to stop music
	if filename == "" {
		return
	}

	f, err := OpenFile(bgm.filename)
	if err != nil {
		// sys.bgm = *newBgm() // removing this gets pause step playsnd to work correctly 100% of the time
		sys.errLog.Printf("Failed to open bgm: %v", err)
		return
	}
	var format beep.Format
	if HasExtension(bgm.filename, ".ogg") {
		bgm.streamer, format, err = vorbis.Decode(f)
		bgm.format = "ogg"
	} else if HasExtension(bgm.filename, ".mp3") {
		bgm.streamer, format, err = mp3.Decode(f)
		bgm.format = "mp3"
	} else if HasExtension(bgm.filename, ".wav") {
		bgm.streamer, format, err = wav.Decode(f)
		bgm.format = "wav"
	} else if HasExtension(bgm.filename, ".flac") {
		bgm.streamer, format, err = flac.Decode(f)
		bgm.format = "flac"
	} else if HasExtension(bgm.filename, ".mid") || HasExtension(bgm.filename, ".midi") {
		if sf, sferr := loadSoundFont(sys.cfg.Sound.SoundFont); sferr != nil {
			err = sferr
		} else {
			bgm.streamer, format, err = midi.Decode(f, sf, beep.SampleRate(int(sys.cfg.Sound.SampleRate)))
			bgm.format = "midi"
		}
	} else {
		err = Error(fmt.Sprintf("unsupported file extension: %v", bgm.filename))
	}
	if err != nil {
		f.Close()
		sys.errLog.Printf("Failed to load bgm: %v\n%v", bgm.filename, err)
		return
	}
	lc := 0
	if loop != 0 {
		if loopcount >= 0 {
			lc = MaxI(0, loopcount-1)
		} else {
			lc = -1
		}

		// Clamp loop end
		if bgmLoopEnd <= 0 || bgmLoopEnd > bgm.streamer.Len() {
			bgmLoopEnd = bgm.streamer.Len()
		}
	}
	// Don't do anything if we have the nomusic command line flag
	if _, ok := sys.cmdFlags["-nomusic"]; ok {
		return
	}
	// Don't do anything if we have the nosound command line flag
	if _, ok := sys.cmdFlags["-nosound"]; ok {
		return
	}
	bgm.startPos = startPosition
	sw := newSwapSeeker(bgm.streamer)
	bgm.streamer = sw // fix pos not updating after swapping
	streamer := newStreamLooper(sw, lc, bgmLoopStart, bgmLoopEnd)
	// we're going to continue to use our own modified streamLooper because beep doesn't allow
	// negative values for loopcount (no forever case)
	bgm.volctrl = &effects.Volume{Streamer: streamer, Base: 2, Volume: 0, Silent: true}
	bgm.sampleRate = format.SampleRate
	dstFreq := beep.SampleRate(float32(sys.cfg.Sound.SampleRate) / bgm.freqmul)
	resampler := beep.Resample(audioResampleQuality, bgm.sampleRate, dstFreq, bgm.volctrl)
	bgm.ctrl = &beep.Ctrl{Streamer: resampler}
	bgm.volRestore = 0 // need this to prevent paused BGM volume from overwriting the new BGM volume
	bgm.UpdateVolume()
	bgm.streamer.Seek(startPosition)
	speaker.Play(bgm.ctrl)

	// Handle the RAM swap in the background (only for looped BGM and only if the user enabled it)
	if lc != 0 && sys.cfg.Sound.BGMRAMBuffer && bgm.format != "xmp" {
		go func(ctx context.Context) {
			// Call the cancel function when the goroutine exits
			// to ensure cleanup.
			defer func() {
				bgm.mu.Lock()
				bgm.cancel()
				bgm.mu.Unlock()
			}()

			// We're gonna be using this a lot to check cancellation.
			isCancelled := func() bool {
				select {
				case <-ctx.Done():
					sys.errLog.Println("New BGM queued up - skipping RAM swap for this BGM")
					return true
				default:
					// Continue
					return false
				}
			}

			// New BGM was queued up before opening file, return NOW
			if isCancelled() {
				return
			}

			lf, err := OpenFile(bgm.filename)
			if err != nil {
				sys.errLog.Println(err)
				return
			}

			// New BGM was queued up after opening file, close & return NOW
			if isCancelled() {
				lf.Close()
				return
			}

			// We gotta re-decode this crap again (but it won't matter since we're on a different thread)
			var dec beep.StreamSeeker
			switch bgm.format {
			case "ogg":
				dec, _, err = vorbis.Decode(lf)
			case "mp3":
				dec, _, err = mp3.Decode(lf)
			case "wav":
				dec, _, err = wav.Decode(lf)
			case "flac":
				dec, _, err = flac.Decode(lf)
			case "midi":
				sf, e := loadSoundFont(sys.cfg.Sound.SoundFont)
				if e != nil {
					sys.errLog.Println(e)
					return
				}
				dec, _, err = midi.Decode(lf, sf, bgm.sampleRate)
			}
			if err != nil {
				sys.errLog.Println(err)
				return
			}

			// build RAM buffer with loop span
			dec.Seek(0)
			buf := beep.NewBuffer(format)
			buf.Append(beep.Take(dec.Len(), dec))

			// close if this decoder supports it (all but MIDI)
			if c, ok := dec.(io.Closer); ok {
				c.Close()
			}
			lf.Close() // doing this out of paranoia, harmless to call

			// Now create the new bufferSeeker
			memSeeker := newBufferSeeker(buf)

			// just swap it now so we're not keeping threads open
			if memSeeker.Len() > 0 {
				// There can sometimes be a sample mismatch with re-decoding MIDI so this takes care of that
				if sl, ok := streamer.(*StreamLooper); ok {
					sl.loopend = MinI(memSeeker.Len(), sl.loopend)
				}

				for {
					select {
					case <-ctx.Done():
						// Cancelled by a new call to bgm.Open()
						return
					default:
						sw.Swap(memSeeker)
						return
					}
				}
			}
		}(ctx)
	}
}
