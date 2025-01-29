package physfs

/*
// Windows Build Tags
#cgo windows CFLAGS: -D_WIN32 -DPHYSFS_SUPPORTS_ZIP=1 -DPHYSFS_SUPPORTS_DIR=1
#cgo windows LDFLAGS: -lgdi32

// Linux Build Tags
#cgo linux CFLAGS: -D__linux -D__linux__ -DPHYSFS_SUPPORTS_ZIP=1 -DPHYSFS_SUPPORTS_DIR=1
#cgo linux LDFLAGS: -lm
*/
import "C"