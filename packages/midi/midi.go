/*
mkdir build && cd build
cmake .. \
  -DBUILD_SHARED_LIBS=OFF \
  -Denable-ipv6=OFF \
  -Denable-sdl2=OFF \
  -Denable-sdl3=OFF \
  -Denable-network=OFF \
  -Denable-dbus=OFF \
  -Denable-systemd=OFF \
  -Denable-pulseaudio=OFF \
  -Denable-alsa=OFF \
  -Denable-jack=OFF \
  -Denable-pipewire=OFF \
  -Denable-libsndfile=OFF \
  -Denable-ladspa=OFF \
  -Denable-readline=OFF \
  -Denable-openmp=OFF \
  -Denable-oss=OFF \
  -DCMAKE_BUILD_TYPE=Release
*/
package midi

/*
#cgo CFLAGS: -Iinclude
#cgo LDFLAGS: -L. -lfluidsynth -lglib-2.0

// Windows Build Tags
#cgo windows CFLAGS: -D_WIN32

// Linux Build Tags
#cgo linux CFLAGS: -D__linux -D__linux__

#include "fluidsynth.h"
#include <stdlib.h>
*/
import "C"
import (
	"errors"
	"os"
	"runtime"
	"unsafe"

	"github.com/gopxl/beep/v2"
	"github.com/ikemen-engine/Ikemen-GO/packages/physfs"
)

const (
	audioOutLen    = 2048               // frames per call (stereo frames)
	audioFrequency = 44100              // output samplerate (Hz)
)

// midiStreamer uses FluidSynth to synthesize MIDI into PCM for beep.
type midiStreamer struct {
	synth   *C.fluid_synth_t
	player  *C.fluid_player_t
	settings *C.fluid_settings_t

	// scratch buffers for synthesis (left/right int16)
	lbuf []int16
	rbuf []int16

	closed bool
	err    error

	// runtime tracking
	posFrames int // frames produced so far
}

func (m *midiStreamer) Stream(samples [][2]float64) (int, bool) {
	if m.closed || m.err != nil {
		return 0, false
	}

	frameCount := len(samples)
	// cap to our buffer size
	if frameCount > len(m.lbuf) {
		frameCount = len(m.lbuf)
	}

	// if frameCount == 0 return
	if frameCount == 0 {
		return 0, true
	}

	// Call FluidSynth to render `frameCount` frames into left/right int16 buffers.
	// fluid_synth_write_s16(synth, len, lout, loff, lstride, rout, roff, rstride)
	C.fluid_synth_write_s16(
		m.synth,
		C.int(frameCount),
		unsafe.Pointer(&m.lbuf[0]),
		C.int(0), /* loff */
		C.int(1), /* lstride */
		unsafe.Pointer(&m.rbuf[0]),
		C.int(0), /* roff */
		C.int(1), /* rstride */
	)

	// Convert int16 to float64 [-1.0, +1.0)
	const scale = 1.0 / 32768.0
	for i := 0; i < frameCount; i++ {
		samples[i][0] = float64(m.lbuf[i]) * scale
		samples[i][1] = float64(m.rbuf[i]) * scale
	}

	m.posFrames += frameCount
	return frameCount, true
}

func (m *midiStreamer) Err() error { return m.err }

func (m *midiStreamer) Close() error {
	if m.closed {
		return nil
	}
	m.closed = true

	// Stop player if present
	if m.player != nil {
		C.fluid_player_stop(m.player)
		C.fluid_player_join(m.player)
		C.delete_fluid_player(m.player)
		m.player = nil
	}

	if m.synth != nil {
		C.delete_fluid_synth(m.synth)
		m.synth = nil
	}

	if m.settings != nil {
		C.delete_fluid_settings(m.settings)
		m.settings = nil
	}

	return nil
}

// Position returns the number of sample-frames (stereo frames) produced so far.
func (m *midiStreamer) Position() int {
	return m.posFrames
}

// Seek is not supported for MIDI stream in this simple implementation.
func (m *midiStreamer) Seek(p int) error {
	return errors.New("seek not supported for MIDI streamer")
}

// Len returns 0 (unknown). You may implement estimation if you parse MIDI meta events.
func (m *midiStreamer) Len() int {
	return 0
}

// newMIDIStreamer creates a midiStreamer from a physfs.File.
// It reads the entire MIDI file into memory and uses fluid_player_add_mem.
func newMIDIStreamer(f *physfs.File) (*midiStreamer, error) {
	// read all bytes from PhysFS file
	data, err := physfs.ReadAll(f)
	if err != nil {
		return nil, err
	}

	// create fluid settings
	settings := C.new_fluid_settings()
	if settings == nil {
		return nil, errors.New("failed to create fluidsynth settings")
	}

	// set sample rate to our target
	C.fluid_settings_setnum(settings, C.CString("synth.sample-rate"), C.double(audioFrequency))

	// create synth
	synth := C.new_fluid_synth(settings)
	if synth == nil {
		C.delete_fluid_settings(settings)
		return nil, errors.New("failed to create fluidsynth synth")
	}

	// create player
	player := C.new_fluid_player(synth)
	if player == nil {
		C.delete_fluid_synth(synth)
		C.delete_fluid_settings(settings)
		return nil, errors.New("failed to create fluidsynth player")
	}

	// Optionally load SoundFont: env FLUIDSYNTH_SOUNDFONT or fallback
	sfPath := os.Getenv("FLUIDSYNTH_SOUNDFONT")
	if sfPath == "" {
		sfPath = "sound/soundfont.sf2"
	}
	cSf := C.CString(sfPath)
	defer C.free(unsafe.Pointer(cSf))
	if C.fluid_synth_sfload(synth, cSf, 1) == -1 {
		// Not fatal — user might choose to use default internal SF; but warn by returning error
		// Free previously allocated resources
		C.delete_fluid_player(player)
		C.delete_fluid_synth(synth)
		C.delete_fluid_settings(settings)
		return nil, errors.New("failed to load SoundFont: " + sfPath)
	}

	// Add MIDI file from memory into player
	memPtr := C.CBytes(data)
	defer C.free(memPtr)
	if C.fluid_player_add_mem(player, memPtr, C.size_t(len(data))) != 0 {
		C.delete_fluid_player(player)
		C.delete_fluid_synth(synth)
		C.delete_fluid_settings(settings)
		return nil, errors.New("failed to add MIDI data to fluid player")
	}

	// start player (it will play asynchronously - we'll pull audio via synth rendering)
	if C.fluid_player_play(player) != 0 {
		C.delete_fluid_player(player)
		C.delete_fluid_synth(synth)
		C.delete_fluid_settings(settings)
		return nil, errors.New("failed to start fluid player")
	}

	// Create Go int16 buffers for left/right channels
	lbuf := make([]int16, audioOutLen)
	rbuf := make([]int16, audioOutLen)

	ms := &midiStreamer{
		settings: settings,
		synth:    synth,
		player:   player,
		lbuf:     lbuf,
		rbuf:     rbuf,
	}

	// ensure cleanup if ms is garbage-collected
	runtime.SetFinalizer(ms, func(m *midiStreamer) { m.Close() })
	return ms, nil
}

// Decode is the beep decoder entrypoint for MIDI via fluidsynth
func Decode(f *physfs.File) (beep.StreamSeekCloser, beep.Format, error) {
	streamer, err := newMIDIStreamer(f)
	if err != nil {
		return nil, beep.Format{}, err
	}
	format := beep.Format{
		SampleRate:  audioFrequency,
		NumChannels: 2,
		Precision:   2, // bytes per sample (int16)
	}
	return streamer, format, nil
}
