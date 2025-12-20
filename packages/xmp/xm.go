package xmp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	XmEventNoteFollows       = 0x01
	XmEventInstrumentFollows = 0x02
	XmEventVolumeFollows     = 0x04
	XmEventFxTypeFollows     = 0x08
	XmEventFxParmFollows     = 0x10
)

type Module struct {
	Header      FileHeader
	Patterns    []Pattern
	Instruments []Instrument
}

type FileHeader struct {
	ID          [17]byte
	Name        [20]byte
	DosEOF      byte
	Tracker     [20]byte
	Version     uint16
	HeaderSize  uint32
	SongLength  uint16
	RestartPos  uint16
	Channels    uint16
	Patterns    uint16
	Instruments uint16
	Flags       uint16
	Tempo       uint16
	BPM         uint16
	Order       [256]byte
}

type Pattern struct {
	Rows [][]*Event
}

type Event struct {
	Note       byte
	Instrument byte
	Volume     byte
	FxType     byte
	FxParam    byte
}

type Instrument struct {
	Name      string
	SampleMap [96]byte // Key 0-95 -> Sample Index
	Samples   []Sample
}

type Sample struct {
	Name       string
	Length     uint32
	LoopStart  uint32
	LoopLength uint32
	Volume     byte
	Finetune   int8
	Type       byte
	Pan        byte
	RelNote    int8
	Data       []byte
}

// Internal headers
type xmPatternHeader struct {
	Length   uint32
	Packing  byte
	Rows     uint16
	DataSize uint16
}

type xmInstrumentHeader struct {
	Size       uint32
	Name       [22]byte
	Type       byte
	NumSamples uint16
	// SampleHeaderSize is NOT part of the standard 29-byte header read
}

type xmSampleHeader struct {
	Length     uint32
	LoopStart  uint32
	LoopLength uint32
	Volume     byte
	Finetune   int8
	Type       byte
	Pan        byte
	RelNote    int8
	Reserved   byte
	Name       [22]byte
}

func LoadXM(path string) (*Module, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	mod := &Module{}

	// 1. Header
	if err := binary.Read(f, binary.LittleEndian, &mod.Header.ID); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(string(mod.Header.ID[:]), "Extended Module:") {
		return nil, errors.New("invalid XM header ID")
	}

	// Read rest of header fields individually
	readFields := []interface{}{
		&mod.Header.Name, &mod.Header.DosEOF, &mod.Header.Tracker, &mod.Header.Version,
		&mod.Header.HeaderSize, &mod.Header.SongLength, &mod.Header.RestartPos,
		&mod.Header.Channels, &mod.Header.Patterns, &mod.Header.Instruments,
		&mod.Header.Flags, &mod.Header.Tempo, &mod.Header.BPM,
	}
	for _, field := range readFields {
		if err := binary.Read(f, binary.LittleEndian, field); err != nil {
			return nil, err
		}
	}

	// Order Table
	orderLen := int(mod.Header.HeaderSize) - 20
	if orderLen > 0 {
		if orderLen > 256 {
			orderLen = 256
		}
		if _, err := io.ReadFull(f, mod.Header.Order[:orderLen]); err != nil {
			return nil, err
		}
	}

	// Seek to patterns (Header Start + 60 + HeaderSize)
	patOffset := int64(60) + int64(mod.Header.HeaderSize)
	f.Seek(patOffset, io.SeekStart)

	// 2. Patterns
	mod.Patterns = make([]Pattern, mod.Header.Patterns)
	for i := 0; i < int(mod.Header.Patterns); i++ {
		p, err := loadPattern(f, mod.Header.Channels)
		if err != nil {
			return nil, fmt.Errorf("pattern %d: %v", i, err)
		}
		mod.Patterns[i] = p
	}

	// 3. Instruments
	mod.Instruments = make([]Instrument, mod.Header.Instruments)
	for i := 0; i < int(mod.Header.Instruments); i++ {
		inst, err := loadInstrument(f)
		if err != nil {
			return nil, fmt.Errorf("instrument %d: %v", i, err)
		}
		mod.Instruments[i] = inst
	}

	return mod, nil
}

func loadPattern(r io.Reader, chn uint16) (Pattern, error) {
	var ph xmPatternHeader
	if err := binary.Read(r, binary.LittleEndian, &ph.Length); err != nil {
		return Pattern{}, err
	}
	if err := binary.Read(r, binary.LittleEndian, &ph.Packing); err != nil {
		return Pattern{}, err
	}
	if err := binary.Read(r, binary.LittleEndian, &ph.Rows); err != nil {
		return Pattern{}, err
	}
	if err := binary.Read(r, binary.LittleEndian, &ph.DataSize); err != nil {
		return Pattern{}, err
	}

	// Pattern header length usually 9. Skip extra if any.
	if ph.Length > 9 {
		io.CopyN(io.Discard, r, int64(ph.Length-9))
	}

	pat := Pattern{Rows: make([][]*Event, ph.Rows)}
	for i := range pat.Rows {
		pat.Rows[i] = make([]*Event, chn)
		for j := range pat.Rows[i] {
			pat.Rows[i][j] = &Event{}
		}
	}

	if ph.DataSize > 0 {
		data := make([]byte, ph.DataSize)
		if _, err := io.ReadFull(r, data); err != nil {
			return Pattern{}, err
		}
		buf := bytes.NewReader(data)

		for r := 0; r < int(ph.Rows); r++ {
			for c := 0; c < int(chn); c++ {
				evt := pat.Rows[r][c]
				b, err := buf.ReadByte()
				if err != nil {
					break
				}

				if b&0x80 != 0 {
					if b&XmEventNoteFollows != 0 {
						evt.Note, _ = buf.ReadByte()
					}
					if b&XmEventInstrumentFollows != 0 {
						evt.Instrument, _ = buf.ReadByte()
					}
					if b&XmEventVolumeFollows != 0 {
						evt.Volume, _ = buf.ReadByte()
					}
					if b&XmEventFxTypeFollows != 0 {
						evt.FxType, _ = buf.ReadByte()
					}
					if b&XmEventFxParmFollows != 0 {
						evt.FxParam, _ = buf.ReadByte()
					}
				} else {
					evt.Note = b
					evt.Instrument, _ = buf.ReadByte()
					evt.Volume, _ = buf.ReadByte()
					evt.FxType, _ = buf.ReadByte()
					evt.FxParam, _ = buf.ReadByte()
				}
			}
		}
	}
	return pat, nil
}

func loadInstrument(f io.ReadSeeker) (Instrument, error) {
	inst := Instrument{}
	startPos, _ := f.Seek(0, io.SeekCurrent)

	var ih xmInstrumentHeader
	// Read standard 29 bytes
	if err := binary.Read(f, binary.LittleEndian, &ih.Size); err != nil {
		return inst, err
	}
	if err := binary.Read(f, binary.LittleEndian, &ih.Name); err != nil {
		return inst, err
	}
	if err := binary.Read(f, binary.LittleEndian, &ih.Type); err != nil {
		return inst, err
	}
	if err := binary.Read(f, binary.LittleEndian, &ih.NumSamples); err != nil {
		return inst, err
	}

	// FIX: Use blank identifier to consume the 4 bytes without causing "unused variable" error
	var trash uint32
	if err := binary.Read(f, binary.LittleEndian, &trash); err != nil {
		return inst, err
	}

	inst.Name = string(bytes.TrimRight(ih.Name[:], "\x00"))
	inst.Samples = make([]Sample, ih.NumSamples)

	var totalSampleData int64 = 0

	if ih.NumSamples > 0 {
		// Calculate if we need to read the Sample Map (96 bytes)
		// We have effectively read 33 bytes (29 header + 4 extra).
		// If ih.Size is large (>= 29 + 4 + 96), the map is present.

		bytesRead := 33
		headerTarget := int(ih.Size)

		if headerTarget >= 129 { // 29 header + 4 trash + 96 map
			if err := binary.Read(f, binary.LittleEndian, &inst.SampleMap); err != nil {
				return inst, err
			}
			bytesRead += 96
		}

		// CRITICAL FIX: Ensure we are exactly at the end of the header before reading Sample Headers
		seekToSamples := startPos + int64(ih.Size)
		f.Seek(seekToSamples, io.SeekStart)

		// Read Sample Headers
		sampleHeaders := make([]xmSampleHeader, ih.NumSamples)
		for i := 0; i < int(ih.NumSamples); i++ {
			binary.Read(f, binary.LittleEndian, &sampleHeaders[i])
			s := &inst.Samples[i]
			s.Name = string(bytes.TrimRight(sampleHeaders[i].Name[:], "\x00"))
			s.Length = sampleHeaders[i].Length
			s.LoopStart = sampleHeaders[i].LoopStart
			s.LoopLength = sampleHeaders[i].LoopLength
			s.Volume = sampleHeaders[i].Volume
			s.Finetune = sampleHeaders[i].Finetune
			s.Type = sampleHeaders[i].Type
			s.Pan = sampleHeaders[i].Pan
			s.RelNote = sampleHeaders[i].RelNote
			totalSampleData += int64(s.Length)
		}

		// Read Sample Data
		for i := 0; i < int(ih.NumSamples); i++ {
			if inst.Samples[i].Length == 0 {
				continue
			}
			data := make([]byte, inst.Samples[i].Length)
			io.ReadFull(f, data)

			// Delta Decode
			is16Bit := (inst.Samples[i].Type & 0x10) != 0
			if !is16Bit {
				var old byte
				for k := 0; k < len(data); k++ {
					data[k] += old
					old = data[k]
				}
			} else {
				var old uint16
				for k := 0; k < len(data); k += 2 {
					val := uint16(data[k]) | uint16(data[k+1])<<8
					val += old
					old = val
					data[k] = byte(val)
					data[k+1] = byte(val >> 8)
				}
			}
			inst.Samples[i].Data = data
		}
	} else {
		// No samples, skip to end of declared header size
		f.Seek(startPos+int64(ih.Size), io.SeekStart)
	}

	// SAFETY NET: Force alignment for the next instrument
	finalPos := startPos + int64(ih.Size) + (int64(ih.NumSamples) * 40) + totalSampleData
	if _, err := f.Seek(finalPos, io.SeekStart); err != nil {
		return inst, err
	}

	return inst, nil
}
