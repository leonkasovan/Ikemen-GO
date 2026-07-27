package reisen

import "C"

func bufferSize(maxBufferSize C.int) C.ulonglong {
	var byteSize C.ulonglong = 8
	return C.ulonglong(maxBufferSize) * byteSize
}

func rewindPosition(dur int64) C.longlong {
	return C.longlong(dur)
}
