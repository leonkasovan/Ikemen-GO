// Package dummy prevents go tooling from stripping the c dependencies.
package xmp

/*
#cgo CFLAGS: -I.. -I../src -DLIBXMP_CORE_PLAYER -DLIBXMP_NO_DEPACKERS -DLIBXMP_NO_PROWIZARD

// Windows Build Tags
#cgo windows CFLAGS: -D_WIN32

// Linux Build Tags
#cgo linux CFLAGS: -D__linux -D__linux__
*/
import "C"