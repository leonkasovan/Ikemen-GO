package xmp

import (
	"errors"
	"io"
	"math"
	"os"
	"strings"

	"github.com/gopxl/beep/v2"
)

// --- Mixer Engine ---

type Channel struct {
	Instrument *Instrument
	Sample     *Sample
	Note       byte
	Volume     float64
	Pan        float64

	// Playback state
	Pos    float64
	Rate   float64
	Active bool
}

type Streamer struct {
	mod      *Module
	channels []Channel

	// Sequencing
	ord, row, tick int
	speed, bpm     int

	// Timing (Float for precision)
	samplesPerTick float64
	tickTimer      float64

	sampleRate float64
	err        error
}

func Decode(f io.ReadSeekCloser) (beep.StreamSeekCloser, beep.Format, error) {
	file, ok := f.(*os.File)
	if !ok {
		return nil, beep.Format{}, errors.New("XMPDecode: expected *os.File")
	}

	// Peek Header
	var magic [17]byte
	if _, err := file.ReadAt(magic[:], 0); err != nil {
		return nil, beep.Format{}, err
	}

	var mod *Module
	var err error

	if strings.HasPrefix(string(magic[:]), "Extended Module:") {
		mod, err = LoadXM(file.Name())
	} else {
		err = errors.New("Format not supported.")
	}

	if err != nil {
		return nil, beep.Format{}, err
	}

	s := &Streamer{
		mod:        mod,
		channels:   make([]Channel, mod.Header.Channels),
		speed:      int(mod.Header.Tempo),
		bpm:        int(mod.Header.BPM),
		sampleRate: 44100,
	}
	if s.speed == 0 {
		s.speed = 6
	}
	if s.bpm == 0 {
		s.bpm = 125
	}

	s.updateTickLen()

	format := beep.Format{
		SampleRate:  beep.SampleRate(44100),
		NumChannels: 2,
		Precision:   2,
	}
	return s, format, nil
}

func (s *Streamer) updateTickLen() {
	// Samples per tick = (SampleRate * 2.5) / BPM
	// Use float to prevent drift over time
	s.samplesPerTick = (s.sampleRate * 2.5) / float64(s.bpm)
}

func (s *Streamer) Stream(samples [][2]float64) (int, bool) {
	filled := 0

	for filled < len(samples) {
		// Process new tick if timer expired
		if s.tickTimer <= 0 {
			s.processTick()
			// Accumulate precise float time
			s.tickTimer += s.samplesPerTick
		}

		// Determine how many samples to mix for this iteration
		// Cast to int for array indexing, but keep timer fractional
		samplesToMix := int(math.Ceil(s.tickTimer))
		if filled+samplesToMix > len(samples) {
			samplesToMix = len(samples) - filled
		}

		// Sanity check
		if samplesToMix <= 0 {
			samplesToMix = 1 // Force advance if stuck
			s.tickTimer = 0
		}

		// Mix channels
		for i := 0; i < samplesToMix; i++ {
			var l, r float64

			for chIdx := range s.channels {
				ch := &s.channels[chIdx]
				if !ch.Active || ch.Sample == nil || len(ch.Sample.Data) == 0 {
					continue
				}

				idx := int(ch.Pos)

				// Loop handling
				if idx >= int(ch.Sample.Length) {
					hasLoop := (ch.Sample.Type & 0x03) != 0
					if hasLoop && ch.Sample.LoopLength > 2 {
						for idx >= int(ch.Sample.LoopStart+ch.Sample.LoopLength) {
							idx -= int(ch.Sample.LoopLength)
						}
						ch.Pos = float64(idx)
					} else {
						ch.Active = false
						continue
					}
				}

				var sampleVal float64
				is16 := (ch.Sample.Type & 0x10) != 0

				if is16 {
					offset := idx * 2
					if offset+1 < len(ch.Sample.Data) {
						val := int16(uint16(ch.Sample.Data[offset]) | uint16(ch.Sample.Data[offset+1])<<8)
						sampleVal = float64(val) / 32768.0
					}
				} else {
					// 8-bit
					if idx < len(ch.Sample.Data) {
						// MOD/XM 8-bit are typically signed logic in mixing
						val := int8(ch.Sample.Data[idx])
						sampleVal = float64(val) / 128.0
					}
				}

				vol := ch.Volume / 64.0
				pan := ch.Pan / 255.0

				l += sampleVal * vol * (1.0 - pan)
				r += sampleVal * vol * pan

				ch.Pos += ch.Rate
			}

			if l > 1.0 {
				l = 1.0
			} else if l < -1.0 {
				l = -1.0
			}
			if r > 1.0 {
				r = 1.0
			} else if r < -1.0 {
				r = -1.0
			}

			samples[filled+i][0] = l
			samples[filled+i][1] = r
		}

		filled += samplesToMix
		s.tickTimer -= float64(samplesToMix)
	}

	return filled, true
}

func (s *Streamer) processTick() {
	if s.tick == 0 {
		if s.row >= len(s.mod.Patterns[s.mod.Header.Order[s.ord]].Rows) {
			s.row = 0
			s.ord++
			if s.ord >= int(s.mod.Header.SongLength) {
				s.ord = int(s.mod.Header.RestartPos)
			}
		}

		patIdx := s.mod.Header.Order[s.ord]
		if int(patIdx) < len(s.mod.Patterns) {
			row := s.mod.Patterns[patIdx].Rows[s.row]
			for i := 0; i < len(s.channels); i++ {
				evt := row[i]
				s.processEvent(i, evt)
			}
		}
		s.row++
	}

	s.tick++
	if s.tick >= s.speed {
		s.tick = 0
	}
}

func (s *Streamer) processEvent(chIdx int, e *Event) {
	ch := &s.channels[chIdx]

	if e.Instrument > 0 {
		if int(e.Instrument)-1 < len(s.mod.Instruments) {
			ch.Instrument = &s.mod.Instruments[e.Instrument-1]
			ch.Volume = 64
			ch.Pan = 128
			// Amiga hard pan:
			if (chIdx+1)%4 == 1 || (chIdx+1)%4 == 4 {
				ch.Pan = 0
			} else {
				ch.Pan = 255
			}
		}
	}

	if e.Note > 0 && e.Note < 97 {
		if ch.Instrument != nil {
			sampIdx := ch.Instrument.SampleMap[e.Note-1]
			if int(sampIdx) < len(ch.Instrument.Samples) {
				ch.Sample = &ch.Instrument.Samples[sampIdx]
				ch.Note = e.Note
				ch.Pos = 0
				ch.Active = true

				baseNote := float64(e.Note) + float64(ch.Sample.RelNote)
				finetune := float64(ch.Sample.Finetune) / 128.0
				noteDiff := baseNote - 49.0 + finetune
				freq := 8363.0 * math.Pow(2, noteDiff/12.0)

				ch.Rate = freq / s.sampleRate
				ch.Volume = float64(ch.Sample.Volume)
			}
		}
	}

	if e.Volume >= 0x10 && e.Volume <= 0x50 {
		ch.Volume = float64(e.Volume - 0x10)
	}

	if e.FxType == 0x0F {
		if e.FxParam < 0x20 {
			s.speed = int(e.FxParam)
		} else {
			s.bpm = int(e.FxParam)
			s.updateTickLen()
		}
	}
}

func (s *Streamer) Err() error       { return s.err }
func (s *Streamer) Close() error     { return nil }
func (s *Streamer) Len() int         { return 10000000 }
func (s *Streamer) Position() int    { return 0 }
func (s *Streamer) Seek(p int) error { return nil }
