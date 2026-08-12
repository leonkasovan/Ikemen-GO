//go:build armdevice

package main

/*
#cgo LDFLAGS: -lEGL
#include <EGL/egl.h>
#include <stdlib.h>
*/
import "C"
import (
	_ "embed"
	"fmt"
	"unsafe"

	findfont "github.com/flopp/go-findfont"
)

//go:embed resources/defaultMotif.ini
var defaultMotif []byte

// init marks this build as the ARM device variant so config.go can select the
// armdevice default config overrides at runtime (build tags are file-scoped).
func init() {
	armDevice = true
}

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

// platformDefaultConfig applies ARM device performance defaults after config
// loading. Only sets values that haven't been explicitly overridden by the user
// (i.e. still at their zero/unset value), except for the model flags which are
// always forced off because models are not viable at R36S performance levels.
func platformDefaultConfig(cfg *Config) {
	// Render at 75% resolution → ~56% fill rate reduction with minimal visual loss
	if cfg.Video.RenderScale <= 0 {
		cfg.Video.RenderScale = 0.75
	}
	// 3D models and shadows are too costly on Mali-G31; disable unconditionally
	cfg.Video.EnableModel = false
	cfg.Video.EnableModelShadow = false
	// MSAA is expensive on tile-based GPUs; force off
	cfg.Video.MSAA = 0
	// Enable deferred sprite queue — prerequisite for Phase 4 instanced batching
	cfg.Video.EnableSpriteBatching = true
	// Cap to display refresh — uncapped burns CPU/GPU for no gain
	if cfg.Video.VSync == 0 {
		cfg.Video.VSync = 1
	}
}

