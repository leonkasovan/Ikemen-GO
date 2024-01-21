//go:build sdl
package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"image"

	// gl "github.com/leonkasovan/gl/v3.1/gles2"
	sdl "github.com/veandco/go-sdl2/sdl"
)

type Window struct {
	*sdl.Window
	title       string
	fullscreen  bool
	shouldclose bool
	x, y, w, h  int
}

func (s *System) newWindow(w, h int) (*Window, error) {
	var err error
	var window *sdl.Window
	sys.errLog.Printf("src/system_sdl.go:28\n")
	// Initialize OpenGL
	chk(sdl.Init(sdl.INIT_EVERYTHING))
	sys.errLog.Printf("src/system_sdl.go:31\n")
	// Create main window.
	window, err = sdl.CreateWindow(s.windowTitle, sdl.WINDOWPOS_UNDEFINED, sdl.WINDOWPOS_UNDEFINED,
		int32(w), int32(h), sdl.WINDOW_OPENGL)
	if err != nil {
		return nil, fmt.Errorf("failed to sdl.CreateWindow: %w", err)
	}
	sys.errLog.Printf("src/system_sdl.go:38\n")
	_, err = window.GLCreateContext()
	if err != nil {
		return nil, fmt.Errorf("failed to window.GLCreateContext: %w", err)
	}
	sys.errLog.Printf("src/system_sdl.go:43\n")
	//gl.Viewport(0, 0, int32(w), int32(h))
	sys.errLog.Printf("src/system_sdl.go:45\n")
	// V-Sync
	if s.vRetrace >= 0 {
		sdl.GLSetSwapInterval(s.vRetrace)
	}
	sys.errLog.Printf("src/system_sdl.go:50\n")
	ret := &Window{window, s.windowTitle, true, false, 0, 0, w, h}
	return ret, err
}

func (w *Window) SwapBuffers() {
	w.Window.GLSwap()
}

func (w *Window) SetIcon(icon []image.Image) {
	// w.Window.SetIcon(icon)
}

func (w *Window) SetSwapInterval(interval int) {
	sdl.GLSetSwapInterval(interval)
}

func (w *Window) GetSize() (int, int) {
	ww,hh := w.Window.GetSize()
	return int(ww),int(hh)
}

func (w *Window) GetClipboardString() string {
	res, _ := sdl.GetClipboardText()
	return res
}

func (w *Window) toggleFullscreen() {
	// not implemented in KMS DRM
}

func (w *Window) pollEvents() {
	event := sdl.PollEvent()
	switch t := event.(type) {
	case *sdl.QuitEvent:
		fmt.Println("Quit: QuitEvent")
		w.shouldclose = true
		break
	case *sdl.WindowEvent:
		if t.Event == sdl.WINDOWEVENT_CLOSE {
			w.shouldclose = true
			fmt.Println("Quit: WindowEvent")
		}
		break
	case *sdl.KeyboardEvent:
		if t.Type == sdl.KEYDOWN {
			OnKeyPressed(t.Keysym.Sym, sdl.Keymod(t.Keysym.Mod))
		}else if t.Type == sdl.KEYUP {
			OnKeyReleased(t.Keysym.Sym, sdl.Keymod(t.Keysym.Mod))
		}
		break
	// case *sdl.JoyAxisEvent:
		// fmt.Printf("[%v ms] JoyAxis\ttype:%v\twhich:%v\taxis:%v\tvalue:%v\n",
			// t.Timestamp, t.Type, t.Which, t.Axis, t.Value)
			// break
	case *sdl.JoyButtonEvent:
		input.sdlButtonState[t.Which][t.Button] = t.State
		break
	case *sdl.JoyHatEvent:
		if t.Value == 1 {	// Up
			input.sdlButtonState[t.Which][11] = 1
		}else if t.Value == 2 {	// Right
			input.sdlButtonState[t.Which][12] = 1
		}else if t.Value == 4 {	// Down
			input.sdlButtonState[t.Which][13] = 1
		}else if t.Value == 8 {	// Left
			input.sdlButtonState[t.Which][14] = 1
		}else{
			input.sdlButtonState[t.Which][11] = 0
			input.sdlButtonState[t.Which][12] = 0
			input.sdlButtonState[t.Which][13] = 0
			input.sdlButtonState[t.Which][14] = 0
		}
		break
	case *sdl.JoyDeviceAddedEvent:
		input.joysticks[int(t.Which)] = sdl.JoystickOpen(int(t.Which))
		if input.joysticks[int(t.Which)] != nil {
			fmt.Printf("Joystick (%v) %v connected\n", input.joysticks[int(t.Which)].Name(), t.Which)
		}
		break
	case *sdl.JoyDeviceRemovedEvent:
		if joystick := input.joysticks[int(t.Which)]; joystick != nil {
			joystick.Close()
		}
		fmt.Printf("Joystick %v disconnected\n", t.Which)
		break
	}
}

func (w *Window) shouldClose() bool {
	return w.shouldclose
}

func (w *Window) Close() {
	w.Window.Destroy()
	sdl.Quit()
}

/*
t.Button

0 A
1 B
2 X
3 Y
4 L1
5 R1
6 SELECT
7 START
8
9 L3
10 R3
11 HAT0 UP
12 HAT1 RIGHT
13 HAT2 DOWN
14 HAT3 LEFT
15
16
*/