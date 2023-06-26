package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"fmt"

	gl "github.com/ikemen-engine/Ikemen-GO/dhaninovan/gl-js"
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

	// Initialize OpenGL
	chk(sdl.Init(sdl.INIT_EVERYTHING))

	// Create main window.
	window, err = sdl.CreateWindow(s.windowTitle, sdl.WINDOWPOS_UNDEFINED, sdl.WINDOWPOS_UNDEFINED,
		int32(w), int32(h), sdl.WINDOW_OPENGL)
	if err != nil {
		return nil, fmt.Errorf("failed to sdl.CreateWindow: %w", err)
	}

	_, err = window.GLCreateContext()
	if err != nil {
		return nil, fmt.Errorf("failed to window.GLCreateContext: %w", err)
	}

	gl.Viewport(0, 0, w, h)

	// V-Sync
	if s.vRetrace >= 0 {
		sdl.GLSetSwapInterval(s.vRetrace)
	}

	ret := &Window{window, s.windowTitle, true, false, 0, 0, w, h}
	return ret, err
}

func (w *Window) SwapBuffers() {
	w.Window.GLSwap()
}

func (w *Window) SetIcon(icon *sdl.Surface) {
	w.Window.SetIcon(icon)
}

func (w *Window) SetSwapInterval(interval int) {
	sdl.GLSetSwapInterval(interval)
}

func (w *Window) GetSize() (int32, int32) {
	return w.Window.GetSize()
}

func (w *Window) GetClipboardString() (string, error) {
	return sdl.GetClipboardText()
}

func (w *Window) toggleFullscreen() {
	// not implemented in KMS DRM
}

func (w *Window) pollEvents() {
	event := sdl.PollEvent()
	switch t := event.(type) {
	// case *sdl.QuitEvent:
	// 	fmt.Println("Quit: QuitEvent")
	// 	w.shouldclose = true
	// 	break
	// case *sdl.WindowEvent:
	// 	if t.Event == sdl.WINDOWEVENT_CLOSE {
	// 		w.shouldclose = true
	// 		fmt.Println("Quit: WindowEvent")
	// 	}
	// 	break
	// case *sdl.JoyAxisEvent:
		// fmt.Printf("[%v ms] JoyAxis\ttype:%v\twhich:%v\taxis:%v\tvalue:%v\n",
		// 	t.Timestamp, t.Type, t.Which, t.Axis, t.Value)
			// break
	// case *sdl.JoyButtonEvent:
		// fmt.Printf("[%v ms] JoyButton\ttype:%v\twhich:%v\tbutton:%v\tstate:%v\n",
		// 	t.Timestamp, t.Type, t.Which, t.Button, t.State)
			// break
	case *sdl.JoyDeviceAddedEvent:
		input.joysticks[int(t.Which)] = sdl.JoystickOpen(int(t.Which))
		if input.joysticks[int(t.Which)] != nil {
			fmt.Printf("Joystick (%v) %v connected\n", input.joysticks[int(t.Which)].Name(), t.Which)
		}
		break
	// case *sdl.JoyDeviceRemovedEvent:
	// 	if joystick := input.joysticks[int(t.Which)]; joystick != nil {
	// 		joystick.Close()
	// 	}
	// 	fmt.Printf("Joystick %v disconnected\n", t.Which)
	// 	break
	}
}

func (w *Window) shouldClose() bool {
	return w.shouldclose
}

func (w *Window) Close() {
	w.Window.Destroy()
	sdl.Quit()
}
