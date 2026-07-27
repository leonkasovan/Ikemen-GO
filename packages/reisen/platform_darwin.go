package reisen

import "C"

func bufferSize(maxBufferSize C.int) C.ulong {
	var byteSize C.ulong = 8
	return C.ulong(maxBufferSize) * byteSize
}

func rewindPosition(dur int64) C.longlong {
	return C.longlong(dur)
}
