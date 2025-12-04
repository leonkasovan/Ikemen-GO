// ./configure --with-network=none --with-audio=dummy --disable-feature_report --enable-libmpg123 --disable-components --enable-static
package mpg123

/*
#cgo CFLAGS: -Iinclude
#cgo LDFLAGS: -L.

// Windows Build Tags
#cgo windows CFLAGS: -D_WIN32

// Linux Build Tags
#cgo linux CFLAGS: -D__linux -D__linux__
*/
import "C"
