package reisen

/*
#include <errno.h>
#include <stdint.h>
#include <libavformat/avformat.h>

extern int reisenReadPacket(void *opaque, uint8_t *buf, int size);
extern int64_t reisenSeek(void *opaque, int64_t offset, int whence);

static AVIOContext *newReaderIO(uintptr_t handle) {
	const int size = 32768;
	uint8_t *buffer = av_malloc(size);
	uintptr_t *opaque = av_malloc(sizeof(*opaque));
	if (!buffer || !opaque) {
		av_free(buffer);
		av_free(opaque);
		return NULL;
	}
	*opaque = handle;
	AVIOContext *ctx = avio_alloc_context(buffer, size, 0, opaque,
		reisenReadPacket, NULL, reisenSeek);
	if (!ctx) {
		av_free(buffer);
		av_free(opaque);
	}
	return ctx;
}
*/
import "C"

import (
	"fmt"
	"io"
	"runtime/cgo"
	"unsafe"
)

// NewMediaFromReader opens a seekable reader as a media container.
// The caller must keep reader open until Media.Close returns, and is responsible
// for closing it. Reads and seeks must not run concurrently with decoding.
func NewMediaFromReader(reader io.ReadSeeker) (*Media, error) {
	media := &Media{ctx: C.avformat_alloc_context()}
	if media.ctx == nil {
		return nil, fmt.Errorf("couldn't create a new media context")
	}

	handle := cgo.NewHandle(reader)
	media.readerIO = C.newReaderIO(C.uintptr_t(handle))
	if media.readerIO == nil {
		handle.Delete()
		media.Close()
		return nil, fmt.Errorf("couldn't create a reader IO context")
	}
	media.ctx.pb = media.readerIO
	media.ctx.flags |= C.AVFMT_FLAG_CUSTOM_IO
	if status := C.avformat_open_input(&media.ctx, nil, nil, nil); status < 0 {
		media.Close()
		return nil, fmt.Errorf("%d: couldn't open media reader", status)
	}
	if err := media.findStreams(); err != nil {
		media.Close()
		return nil, err
	}
	return media, nil
}

func closeReaderIO(ctx *C.AVIOContext) {
	cgo.Handle(*(*C.uintptr_t)(ctx.opaque)).Delete()
	C.av_free(ctx.opaque)
	// FFmpeg may replace the buffer while probing, so free the current one.
	C.av_free(unsafe.Pointer(ctx.buffer))
	C.avio_context_free(&ctx)
}

//export reisenReadPacket
func reisenReadPacket(opaque unsafe.Pointer, buf *C.uint8_t, size C.int) C.int {
	reader := cgo.Handle(*(*C.uintptr_t)(opaque)).Value().(io.ReadSeeker)
	n, err := reader.Read(unsafe.Slice((*byte)(unsafe.Pointer(buf)), int(size)))
	if n > 0 {
		return C.int(n)
	}
	if err == io.EOF {
		return C.AVERROR_EOF
	}
	return -C.EIO
}

//export reisenSeek
func reisenSeek(opaque unsafe.Pointer, offset C.int64_t, whence C.int) C.int64_t {
	reader := cgo.Handle(*(*C.uintptr_t)(opaque)).Value().(io.ReadSeeker)
	whence &^= C.AVSEEK_FORCE
	if whence == C.AVSEEK_SIZE {
		pos, err := reader.Seek(0, io.SeekCurrent)
		if err != nil {
			return -C.EIO
		}
		size, err := reader.Seek(0, io.SeekEnd)
		_, restoreErr := reader.Seek(pos, io.SeekStart)
		if err != nil || restoreErr != nil {
			return -C.EIO
		}
		return C.int64_t(size)
	}
	pos, err := reader.Seek(int64(offset), int(whence))
	if err != nil {
		return -C.EIO
	}
	return C.int64_t(pos)
}
