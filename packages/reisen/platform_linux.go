package reisen

// #include <stdint.h>
import "C"

func bufferSize(maxBufferSize C.int) C.size_t {
	var byteSize C.size_t = 8
	return C.size_t(maxBufferSize) * byteSize
}

func rewindPosition(dur int64) C.int64_t {
	return C.int64_t(dur)
}
