//go:build armdevice

package main

/*
#cgo LDFLAGS: -lEGL
#include <EGL/egl.h>
#include <stdlib.h>
*/
import "C"
import (
	"fmt"
	"unsafe"

	findfont "github.com/flopp/go-findfont"
)

// Message box implementation using stderr
func ShowInfoDialog(message, title string) {
	print(title + "\n\n" + message)
}

func ShowErrorDialog(message string) {
	print("I.K.E.M.E.N Error\n\n" + message)
}

func LoadFntTtf(f *Fnt, fontfile string, filename string, height int32) {
	// Search in local directory
	fileDir := SearchFile(filename, []string{fontfile, sys.motif.Def, "", "data/"}, "font/")
	// Search in system directory
	fp := fileDir
	if fp = FileExist(fp); len(fp) == 0 {
		var err error
		fileDir, err = findfont.Find(fileDir)
		if err != nil {
			panic(fmt.Errorf("failed to find ttf font %v: %w", fileDir, err))
		}
	}
	// Load ttf
	if height == -1 {
		height = int32(f.Size[1])
	} else {
		f.Size[1] = uint16(height)
	}
	ttf, err := gfxFont.LoadFont(fileDir, height, int(sys.gameWidth), int(sys.gameHeight))
	if err != nil {
		panic(fmt.Errorf("failed to load ttf font %v: %w", fileDir, err))
	}
	f.ttf = ttf.(Font)

	// Create Ttf dummy palettes
	f.palettes = make([][256]uint32, 1)
	for i := 0; i < 256; i++ {
		f.palettes[0][i] = 0
	}
}

func eglGetProcAddress(name string) unsafe.Pointer {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	return unsafe.Pointer(C.eglGetProcAddress(cname))
}

func selectRenderer(cfgVal string) (Renderer, FontRenderer) {
	return &Renderer_GLES32{}, &FontRenderer_GLES32{}
}

