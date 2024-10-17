//go:build !js && !raw

package main

import (
	"io"
	"os"

	findfont "github.com/flopp/go-findfont"
	"github.com/ikemen-engine/Ikemen-GO/glfont"
)

// Log writer implementation
func NewLogWriter() io.Writer {
	return os.Stderr
}

// Message box implementation
func ShowInfoDialog(message, title string) {
	print(title + "\n\n" + message)
}

func ShowErrorDialog(message string) {
	print("I.K.E.M.E.N Error\n\n" + message)
}

// TTF font loading
func LoadFntTtf(f *Fnt, fontfile string, filename string, height int32) {
	//Search in local directory
	fileDir := SearchFile(filename, []string{fontfile, sys.motifDir, "", "data/", "font/"})
	//Search in system directory
	fp := fileDir
	if fp = FileExist(fp); len(fp) == 0 {
		var err error
		fileDir, err = findfont.Find(fileDir)
		if err != nil {
			panic(err)
		}
	}
	//Load ttf
	if height == -1 {
		height = int32(f.Size[1])
	} else {
		f.Size[1] = uint16(height)
	}
	if Renderer_API == 2 {	// 2=>OpenGLES
		sys.fontShaderVer = 300
	}
	ttf, err := glfont.LoadFont(fileDir, height, int(sys.gameWidth), int(sys.gameHeight), sys.fontShaderVer)
	if err != nil {
		panic(err)
	}
	f.ttf = ttf
	f.ttf.SetBatchMode(true)

	//Create Ttf dummy palettes
	f.palettes = make([][256]uint32, 1)
	for i := 0; i < 256; i++ {
		f.palettes[0][i] = 0
	}
}
