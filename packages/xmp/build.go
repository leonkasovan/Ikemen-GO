package xmp

/*
#cgo CFLAGS: -DLIBXMP_CORE_PLAYER -DLIBXMP_NO_DEPACKERS -DLIBXMP_NO_PROWIZARD

// Windows Build Tags
#cgo windows CFLAGS: -D_WIN32

// Linux Build Tags
#cgo linux CFLAGS: -D__linux -D__linux__

#include "loaders/it_load.c"
#include "loaders/itsex.c"
*/
import "C"