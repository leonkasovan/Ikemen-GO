//go:build sdl
package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"image"

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
	if Renderer_API == 2 {	// OpenGL ES
		sdl.GLSetAttribute(sdl.GL_CONTEXT_MAJOR_VERSION, 3)
		sdl.GLSetAttribute(sdl.GL_CONTEXT_MINOR_VERSION, 1)
		sdl.GLSetAttribute(sdl.GL_CONTEXT_PROFILE_MASK, sdl.GL_CONTEXT_PROFILE_ES)
	} else {
		sdl.GLSetAttribute(sdl.GL_CONTEXT_MAJOR_VERSION, 2)
		sdl.GLSetAttribute(sdl.GL_CONTEXT_MINOR_VERSION, 1)
	}
	// Create main window.
	window, err = sdl.CreateWindow(s.windowTitle, sdl.WINDOWPOS_UNDEFINED, sdl.WINDOWPOS_UNDEFINED,
		int32(w), int32(h), sdl.WINDOW_OPENGL|sdl.WINDOW_FULLSCREEN)
	if err != nil {
		return nil, fmt.Errorf("\nfailed to sdl.CreateWindow: %w\n", err)
	}
	_, err = window.GLCreateContext()
	if err != nil {
		return nil, fmt.Errorf("\nfailed to window.GLCreateContext: %w\n", err)
	}
	// V-Sync
	if s.vRetrace >= 0 {
		sdl.GLSetSwapInterval(s.vRetrace)
	}
	// Store current timestamp
	s.prevTimestampUint = sdl.GetTicks64()
	ret := &Window{window, s.windowTitle, true, false, 0, 0, w, h}
	return ret, err
}

func (w *Window) SwapBuffers() {
	w.Window.GLSwap()
	// Retrieve GL timestamp now
	sdlNow := sdl.GetTicks64()
	delta := sdlNow-sys.prevTimestampUint
	if delta >= 1000 {
		// sys.gameFPS = float32(sys.absTickCount) / float32(delta/1000)
		sys.gameFPS = float32(sys.absTickCount)
		sys.absTickCount = 0
		sys.prevTimestampUint = sdlNow
	}
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
	if w.fullscreen {
		sdl.SetWindowFullscreen(w.Window, sdl.WINDOW_FULLSCREEN)
	} else {
		sdl.SetWindowFullscreen(w.Window, 0)
	}
	w.fullscreen = !w.fullscreen
}

func (w *Window) pollEvents() {
	event := sdl.PollEvent()
	switch t := event.(type) {
	case *sdl.QuitEvent:
		sys.errLog.Println("Quit: QuitEvent")
		w.shouldclose = true
		break
	case *sdl.WindowEvent:
		if t.Event == sdl.WINDOWEVENT_CLOSE {
			w.shouldclose = true
			sys.errLog.Println("Quit: WindowEvent")
		}
		break
	case *sdl.KeyboardEvent:
		if t.Type == sdl.KEYDOWN {
			OnKeyPressed(t.Keysym.Sym, sdl.Keymod(t.Keysym.Mod))
		}else if t.Type == sdl.KEYUP {
			OnKeyReleased(t.Keysym.Sym, sdl.Keymod(t.Keysym.Mod))
		}
		break
	case *sdl.JoyDeviceAddedEvent:
		input.joysticks[int(t.Which)] = sdl.JoystickOpen(int(t.Which))
		if input.joysticks[int(t.Which)] != nil {
			sys.errLog.Printf("Joystick (%v) id=%v connected\n", input.joysticks[int(t.Which)].Name(), t.Which)
		}
		break
	case *sdl.JoyDeviceRemovedEvent:
		if joystick := input.joysticks[int(t.Which)]; joystick != nil {
			joystick.Close()
		}
		sys.errLog.Printf("Joystick %v disconnected\n", t.Which)
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
