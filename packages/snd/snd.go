package snd

/*
#cgo CFLAGS: -I../mpg123/include -I../opus/include

// Windows Build Tags
#cgo windows CFLAGS: -D_WIN32

// Linux Build Tags
#cgo linux CFLAGS: -D__linux -D__linux__

#include <stdlib.h>
#include <string.h>
#include "sndfile.h"
#include "../physfs/physfs.h"

// MemFile holds pointer to the data and cursor position for virtual IO
typedef struct {
    const unsigned char* data;
    sf_count_t size;
    sf_count_t pos;
} MemFile;

static sf_count_t mem_get_filelen(void* user_data) {
    MemFile* m = (MemFile*)user_data;
    return m->size;
}

static sf_count_t mem_seek(sf_count_t offset, int whence, void* user_data) {
    MemFile* m = (MemFile*)user_data;
    sf_count_t np = m->pos;
    if (whence == SEEK_SET) np = offset;
    else if (whence == SEEK_CUR) np += offset;
    else if (whence == SEEK_END) np = m->size + offset;
    if (np < 0 || np > m->size) return -1;
    m->pos = np;
    return m->pos;
}

static sf_count_t mem_read(void* ptr, sf_count_t count, void* user_data) {
    MemFile* m = (MemFile*)user_data;
    if (m->pos + count > m->size) count = m->size - m->pos;
    if (count <= 0) return 0;
    memcpy(ptr, m->data + m->pos, (size_t)count);
    m->pos += count;
    return count;
}

static sf_count_t mem_write(const void* ptr, sf_count_t count, void* user_data) {
    (void)ptr; (void)user_data;
    // not used: read-only virtual file
    return 0;
}

static sf_count_t mem_tell(void* user_data) {
    MemFile* m = (MemFile*)user_data;
    return m->pos;
}

// allocate and initialize MemFile and return pointer
static MemFile* create_memfile(const unsigned char* data, sf_count_t size) {
    MemFile* m = (MemFile*)malloc(sizeof(MemFile));
    if (!m) return NULL;
    m->data = data;
    m->size = size;
    m->pos = 0;
    return m;
}

static void free_memfile(MemFile* m) {
    if (m) free(m);
}

// ---- Global virtual IO struct ----
static SF_VIRTUAL_IO vio_mem_instance = {
    mem_get_filelen,
    mem_seek,
    mem_read,
    mem_write,
    mem_tell
};

SF_VIRTUAL_IO* get_mem_vio() {
    return &vio_mem_instance;
}

static sf_count_t physfs_get_filelen(void *user_data) {
    PHYSFS_File *file = (PHYSFS_File *)user_data;
    PHYSFS_sint64 len = PHYSFS_fileLength(file);
    return (len > 0) ? (sf_count_t)len : 0;
}

static sf_count_t physfs_seek(sf_count_t offset, int whence, void* user_data) {
    PHYSFS_File *file = (PHYSFS_File *)user_data;
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
    return PHYSFS_tell(file);
}

static sf_count_t physfs_read(void* ptr, sf_count_t count, void* user_data) {
    PHYSFS_File *file = (PHYSFS_File *)user_data;
    sf_count_t got = PHYSFS_readBytes(file, ptr, count);
    if (got < 0)
        return 0;
    return got;
}

static sf_count_t physfs_tell(void* user_data) {
    PHYSFS_File *file = (PHYSFS_File *)user_data;
    return PHYSFS_tell(file);
}

static SF_VIRTUAL_IO vio_physfs_instance = {
    .get_filelen = physfs_get_filelen,
    .seek        = physfs_seek,
    .read        = physfs_read,
    .write       = NULL,
    .tell        = physfs_tell
};

SF_VIRTUAL_IO* get_physfs_vio() {
    return &vio_physfs_instance;
}
*/
import "C"
import (
	"errors"
	"unsafe"
	"fmt"
	"github.com/gopxl/beep/v2"
	"github.com/ikemen-engine/Ikemen-GO/packages/physfs"
	_ "github.com/ikemen-engine/Ikemen-GO/packages/ogg"
	_ "github.com/ikemen-engine/Ikemen-GO/packages/vorbis"
	_ "github.com/ikemen-engine/Ikemen-GO/packages/opus"
	_ "github.com/ikemen-engine/Ikemen-GO/packages/mpg123"
	_ "github.com/ikemen-engine/Ikemen-GO/packages/flac"
)

const (
	audioOutLen          = 2048
	audioFrequency       = 44100
)

// sndfileStreamer wraps SNDFILE* and MemFile for streaming + seeking
type sndfileStreamer struct {
	sf       *C.SNDFILE
	info     C.SF_INFO
	mem      *C.MemFile
	channels int
	sampleRate int
	goBuffer  []byte // Pin Go buffer to ensure it's not GC'd
}

// Stream reads PCM into beep's stereo float64 buffer
func (s *sndfileStreamer) Stream(samples [][2]float64) (int, bool) {
	buf := make([]float32, len(samples)*s.channels)
	// prevPos := C.sf_seek(s.sf, 0, C.SEEK_CUR)
	read := C.sf_readf_float(s.sf,
		(*C.float)(unsafe.Pointer(&buf[0])),
		C.sf_count_t(len(samples)))
	// newPos := C.sf_seek(s.sf, 0, C.SEEK_CUR)
	// fmt.Printf("Stream: Requested %d samples, read %d, before pos %d, after pos %d\n", len(samples), int(read), int(prevPos), int(newPos))
	if read == 0 {
		errnum := C.sf_error(s.sf)
		if errnum != 0 {
			fmt.Printf("Libsndfile error: %d\n", int(errnum))
		}
		return 0, false
	}
	for i := 0; i < int(read); i++ {
		base := i * s.channels
		var l, r float32
		if s.channels == 1 {
			l, r = buf[base], buf[base]
		} else {
			l, r = buf[base], buf[base+1]
		}
		samples[i][0] = float64(l)
		samples[i][1] = float64(r)
	}
	return int(read), true
}

func (s *sndfileStreamer) Err() error { return nil }
func (s *sndfileStreamer) Close() error {
	if s.sf != nil {
		C.sf_close(s.sf)
		s.sf = nil
	}
	if s.mem != nil {
		C.free_memfile(s.mem)
		s.mem = nil
	}
	s.goBuffer = nil // Allow Go to GC
	return nil
}

// Position returns current frame index
func (s *sndfileStreamer) Position() int {
	pos := C.sf_seek(s.sf, 0, C.SEEK_CUR)
	return int(pos)
}

// Seek moves to absolute frame index
func (s *sndfileStreamer) Seek(frame int) error {
	newpos := C.sf_seek(s.sf, C.sf_count_t(frame), C.SEEK_SET)
	if newpos < 0 {
		return errors.New("seek failed")
	}
	return nil
}

// Len returns total frames
func (s *sndfileStreamer) Len() int {
	return int(s.info.frames)
}

// sndDecodeFromMemory returns a streaming beep.StreamSeekCloser using libsndfile
func DecodeFromMemory(data []byte) (beep.StreamSeekCloser, beep.Format, error) {
	if len(data) == 0 {
		return nil, beep.Format{}, errors.New("empty buffer")
	}
	ptr := (*C.uchar)(unsafe.Pointer(&data[0]))
	mem := C.create_memfile(ptr, C.sf_count_t(len(data)))
	if mem == nil {
		return nil, beep.Format{}, errors.New("create_memfile failed")
	}

	var info C.SF_INFO
	sf := C.sf_open_virtual(C.get_mem_vio(), C.SFM_READ, &info, unsafe.Pointer(mem))
	if sf == nil {
		C.free_memfile(mem)
		errnum := C.sf_error(sf)
		if errnum != 0 {
			fmt.Printf("Libsndfile error: %s(%d)\n", C.GoString(C.sf_strerror(sf)), int(errnum))
		}
		return nil, beep.Format{}, errors.New("sf_open_virtual failed")
	}

	streamer := &sndfileStreamer{
		sf:         sf,
		info:       info,
		mem:        mem,
		channels:   int(info.channels),
		sampleRate: int(info.samplerate),
		goBuffer:   data, // Pin buffer
	}

	format := beep.Format{
		SampleRate:  beep.SampleRate(streamer.sampleRate),
		NumChannels: 2, // always stereo for beep
		Precision:   4,
	}
	// runtime.KeepAlive is not strictly needed here, but if you refactor, be sure buffer stays alive
	return streamer, format, nil
}

func Decode(f *physfs.File) (beep.StreamSeekCloser, beep.Format, error) {
	if f == nil {
		return nil, beep.Format{}, errors.New("nil file handle")
	}

	var info C.SF_INFO

	// Use PhysFS-backed virtual I/O
	sf := C.sf_open_virtual(C.get_physfs_vio(), C.SFM_READ, &info, unsafe.Pointer(f))
	if sf == nil {
		return nil, beep.Format{}, errors.New("sf_open_virtual failed (libsndfile couldn't read file)")
	}

	streamer := &sndfileStreamer{
		sf:         sf,
		info:       info,
		mem:        nil, // Not using in-memory mode
		channels:   int(info.channels),
		sampleRate: int(info.samplerate),
	}

	format := beep.Format{
		SampleRate:  beep.SampleRate(streamer.sampleRate),
		NumChannels: 2,
		Precision:   4, // 32-bit float
	}

	return streamer, format, nil
}
