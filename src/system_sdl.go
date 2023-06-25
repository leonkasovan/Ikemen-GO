//go:build !kinc

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
	// var context sdl.GLContext

	// Initialize OpenGL
	chk(sdl.Init(sdl.INIT_EVERYTHING))

	// Create main window.
	window, err = sdl.CreateWindow(s.windowTitle, sdl.WINDOWPOS_UNDEFINED, sdl.WINDOWPOS_UNDEFINED,
		640, 480, sdl.WINDOW_OPENGL)
	if err != nil {
		return nil, fmt.Errorf("failed to sdl.CreateWindow: %w", err)
	}

	_, err = window.GLCreateContext()
	if err != nil {
		return nil, fmt.Errorf("failed to window.GLCreateContext: %w", err)
	}

	// if err = gl.Init(); err != nil {
	// 	panic(err)
	// }
	gl.Viewport(0, 0, 640, 480)

	// window.MakeContextCurrent()
	// window.SetKeyCallback(keyCallback)
	// window.SetCharModsCallback(charCallback)

	// V-Sync
	if s.vRetrace >= 0 {
		sdl.GLSetSwapInterval(s.vRetrace)
	}

	ret := &Window{window, s.windowTitle, true, false, 0, 0, w, h}
	return ret, err
}

func (w *Window) SwapBuffers() {
	// w.Window.SwapBuffers()
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
	case *sdl.QuitEvent:
		println("Quit: QuitEvent")
		w.shouldclose = true
		break
	case *sdl.WindowEvent:
		if t.Event == sdl.WINDOWEVENT_CLOSE {
			w.shouldclose = true
			println("Quit: WindowEvent")
		}
		break
	case *sdl.TextInputEvent:
		OnTextEntered(t.GetText())
		break
	case *sdl.KeyboardEvent:
		keyCode := t.Keysym.Sym
		Mod := t.Keysym.Mod
		fmt.Printf("[%v ms] Keyboard\ttype:%v\tsym:%v\tmodifiers:%v\tstate:%v\trepeat:%v\n",
			t.Timestamp, t.Type, t.Keysym.Sym, t.Keysym.Mod, t.State, t.Repeat)
		switch Mod {
		case sdl.KMOD_LALT:
			//keys += "Left Alt"
		case sdl.KMOD_LCTRL:
			//keys += "Left Control"
		case sdl.KMOD_LSHIFT:
			//keys += "Left Shift"
		case sdl.KMOD_LGUI:
			//keys += "Left Meta or Windows key"
		case sdl.KMOD_RALT:
			//keys += "Right Alt"
		case sdl.KMOD_RCTRL:
			//keys += "Right Control"
		case sdl.KMOD_RSHIFT:
			//keys += "Right Shift"
		case sdl.KMOD_RGUI:
			//keys += "Right Meta or Windows key"
		case sdl.KMOD_NUM:
			//keys += "Num Lock"
		case sdl.KMOD_CAPS:
			// keys += "Caps Lock"
		case sdl.KMOD_MODE:
			// keys += "AltGr Key"
		}
		if t.State == sdl.RELEASED {
			fmt.Println(string(keyCode) + " pressed")
			//OnKeyReleased(keyCode, Mod)
		} else if t.State == sdl.PRESSED {
			fmt.Println(string(keyCode) + " pressed")
			//OnKeyPressed(keyCode, Mod)
		}
		break
	case *sdl.JoyAxisEvent:
		// fmt.Printf("[%v ms] JoyAxis\ttype:%v\twhich:%v\taxis:%v\tvalue:%v\n",
		// 	t.Timestamp, t.Type, t.Which, t.Axis, t.Value)
			break
	case *sdl.JoyButtonEvent:
		// fmt.Printf("[%v ms] JoyButton\ttype:%v\twhich:%v\tbutton:%v\tstate:%v\n",
		// 	t.Timestamp, t.Type, t.Which, t.Button, t.State)
			break
	case *sdl.JoyDeviceAddedEvent:
		input.joysticks[int(t.Which)] = sdl.JoystickOpen(int(t.Which))
		if input.joysticks[int(t.Which)] != nil {
			fmt.Printf("Joystick %v connected\n", t.Which)
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
