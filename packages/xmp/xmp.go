package xmp

/*
#cgo CFLAGS: -Isrc -DLIBXMP_CORE_PLAYER -DLIBXMP_NO_DEPACKERS -DLIBXMP_NO_PROWIZARD -DLIBXMP_STATIC

// Windows Build Tags
#cgo windows CFLAGS: -D_WIN32 -DLIBXMP_STATIC

// Linux Build Tags
#cgo linux CFLAGS: -D__linux -D__linux__

#include <stdlib.h>
#include <stdio.h>
#include "xmp.h"
#include "../physfs/physfs.h"

static long unsigned int  xmp_physfs_read(void *dest, long unsigned int size, long unsigned int nmemb, void *priv) {
    PHYSFS_File *file = (PHYSFS_File *)priv;
    PHYSFS_sint64 want = (PHYSFS_sint64)(size * nmemb);
    PHYSFS_sint64 got = PHYSFS_readBytes(file, dest, want);
    if (got < 0)
        return 0;
    return (int)(got / size);
}

static int xmp_physfs_seek(void *priv, long offset, int whence) {
    PHYSFS_File *file = (PHYSFS_File *)priv;
    PHYSFS_sint64 newPos = 0;

    switch (whence) {
    case SEEK_SET:
        newPos = offset;
        break;
    case SEEK_CUR:
        newPos = PHYSFS_tell(file) + offset;
        break;
    case SEEK_END:
        newPos = PHYSFS_fileLength(file) + offset;
        break;
    default:
        return -1;
    }

    if (!PHYSFS_seek(file, newPos))
        return -1;
    return 0;
}

static long xmp_physfs_tell(void *priv) {
    PHYSFS_File *file = (PHYSFS_File *)priv;
    return (long)PHYSFS_tell(file);
}

static int xmp_physfs_close(void *priv) {
    PHYSFS_File *file = (PHYSFS_File *)priv;
    PHYSFS_close(file);
    return 0;
}

struct xmp_callbacks physfs_cb = {
    .read_func  = xmp_physfs_read,
    .seek_func  = xmp_physfs_seek,
    .tell_func  = xmp_physfs_tell,
    .close_func = xmp_physfs_close
};
*/
import "C"
import (
	"errors"
	"runtime"
	"unsafe"

	"github.com/gopxl/beep/v2"
	"github.com/ikemen-engine/Ikemen-GO/packages/physfs"
	_ "github.com/ikemen-engine/Ikemen-GO/packages/xmp/loaders"
	_ "github.com/ikemen-engine/Ikemen-GO/packages/xmp/src"
)

const (
	audioOutLen    = 2048
	audioFrequency = 44100
)

// ------------------------------------------------------------------
// xmStreamer wraps libxmp context for streaming
type xmStreamer struct {
	ctx        C.xmp_context
	channels   int
	sampleRate int
	buffer     []int16
	closed     bool
	err        error

	// runtime tracking
	posFrames   int // frames already produced (a frame == one sample per channel)
	totalFrames int // estimated total frames (from total_time)
}

// Stream fills the provided buffer with audio frames (Optimized version).
func (x *xmStreamer) Stream(samples [][2]float64) (int, bool) {
	if x.closed || x.err != nil {
		return 0, false
	}

	frameCount := len(samples)
	if frameCount*2 > len(x.buffer) {
		frameCount = len(x.buffer) / 2
	}

	res := C.xmp_play_buffer(x.ctx, unsafe.Pointer(&x.buffer[0]), C.int(frameCount*2*2), 0)
	if res < 0 {
		x.err = errors.New("xmp playback ended or failed")
		return 0, false
	}

	buf := x.buffer
	const scale = 1.0 / 32768.0
	for i := 0; i < frameCount; i++ {
		j := i * 2
		samples[i][0] = float64(buf[j]) * scale
		samples[i][1] = float64(buf[j+1]) * scale
	}
	return frameCount, true
}

// Err returns the last error that occurred.
func (x *xmStreamer) Err() error { return x.err }

// Close releases all libxmp resources.
func (x *xmStreamer) Close() error {
	if x.closed {
		return nil
	}
	x.closed = true
	C.xmp_end_player(x.ctx)
	C.xmp_release_module(x.ctx)
	C.xmp_free_context(x.ctx)
	return nil
}

func (x *xmStreamer) Position() int {
	return 0
}

// Seek attempts to position to absolute frame p.
// Beep's Seek uses sample-frame positions (frames == sample pairs).
func (x *xmStreamer) Seek(p int) error {
	return nil
}

func (x *xmStreamer) Len() int {
	return x.totalFrames
}

// newXMStreamer initializes a libxmp context for a given XM file.
func newXMStreamer(f *physfs.File) (*xmStreamer, error) {
	ctx := C.xmp_create_context()
	if ctx == nil {
		return nil, errors.New("failed to create xmp context")
	}

	// Load module using callbacks
	if C.xmp_load_module_from_callbacks(ctx, unsafe.Pointer(f), C.physfs_cb) != 0 {
		C.xmp_free_context(ctx)
		return nil, errors.New("Failed to load XM module from callbacks")
	}

	var info C.struct_xmp_frame_info
	C.xmp_get_frame_info(ctx, &info)

	if C.xmp_start_player(ctx, audioFrequency, 0) != 0 {
		C.xmp_release_module(ctx)
		C.xmp_free_context(ctx)
		return nil, errors.New("failed to start XM player")
	}

	s := &xmStreamer{
		ctx:         ctx,
		channels:    2,
		sampleRate:  audioFrequency,
		totalFrames: int(float64(info.total_time) * float64(audioFrequency) / 1000.0),
		buffer:      make([]int16, audioOutLen*2), // 2048 stereo frames → lower memory
	}
	runtime.SetFinalizer(s, func(s *xmStreamer) { s.Close() })
	return s, nil
}

func Decode(f *physfs.File) (beep.StreamSeekCloser, beep.Format, error) {
	streamer, err := newXMStreamer(f)
	if err != nil {
		return nil, beep.Format{}, err
	}
	format := beep.Format{
		SampleRate:  audioFrequency,
		NumChannels: 2,
		Precision:   2,
	}
	return streamer, format, nil
}
