//go:build sdl

package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"image"
	"os"

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
	var mode sdl.DisplayMode

	// Initialize OpenGL
	chk(sdl.Init(sdl.INIT_EVERYTHING))
	if Renderer_API == 2 { // OpenGL ES
		sdl.GLSetAttribute(sdl.GL_CONTEXT_MAJOR_VERSION, 3)
		sdl.GLSetAttribute(sdl.GL_CONTEXT_MINOR_VERSION, 1)
		sdl.GLSetAttribute(sdl.GL_CONTEXT_PROFILE_MASK, sdl.GL_CONTEXT_PROFILE_ES)
	} else {
		sdl.GLSetAttribute(sdl.GL_CONTEXT_MAJOR_VERSION, 2)
		sdl.GLSetAttribute(sdl.GL_CONTEXT_MINOR_VERSION, 1)
	}
	// Create main window.
	if s.fullscreen {
		window, err = sdl.CreateWindow(s.windowTitle, sdl.WINDOWPOS_UNDEFINED, sdl.WINDOWPOS_UNDEFINED,
			int32(w), int32(h), sdl.WINDOW_OPENGL|sdl.WINDOW_FULLSCREEN)
	} else {
		window, err = sdl.CreateWindow(s.windowTitle, sdl.WINDOWPOS_UNDEFINED, sdl.WINDOWPOS_UNDEFINED,
			int32(w), int32(h), sdl.WINDOW_OPENGL|sdl.WINDOW_RESIZABLE|sdl.WINDOW_SHOWN)
	}
	if err != nil {
		return nil, fmt.Errorf("\nfailed to sdl.CreateWindow: %w\n", err)
	}
	_, err = window.GLCreateContext()
	if err != nil {
		return nil, fmt.Errorf("\nfailed to window.GLCreateContext: %w\n", err)
	}

	// Set Window in center
	mode, err = sdl.GetCurrentDisplayMode(0)
	sys.errLog.Printf("GetCurrentDisplayMode: %vx%v", mode.W, mode.H)
	var x, y = (int(mode.W) - w) / 2, (int(mode.H) - h) / 2
	window.SetPosition(int32(x), int32(y))

	// V-Sync
	if s.vRetrace >= 0 {
		sdl.GLSetSwapInterval(s.vRetrace)
	}
	// Store current timestamp
	s.prevTimestampUint = sdl.GetTicks64()
	ret := &Window{window, s.windowTitle, s.fullscreen, false, x, y, w, h}
	return ret, err
}

func (w *Window) SwapBuffers() {
	w.Window.GLSwap()
	// Retrieve GL timestamp now
	sdlNow := sdl.GetTicks64()
	delta := sdlNow - sys.prevTimestampUint
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
	ww, hh := w.Window.GetSize()
	return int(ww), int(hh)
}

func (w *Window) GetScaledViewportSize() (int32, int32, int32, int32) {
	// calculates a position and size for the viewport to fill the window while centered (see render_gl.go)
	// returns x, y, width, height respectively
	winWidth, winHeight := w.GetSize()
	ratioWidth := float32(winWidth) / float32(sys.gameWidth)
	ratioHeight := float32(winHeight) / float32(sys.gameHeight)
	var ratio float32
	var x, y int32 = 0, 0

	if ratioWidth < ratioHeight {
		ratio = ratioWidth
	} else {
		ratio = ratioHeight
	}

	resizedWidth := int32(float32(sys.gameWidth) * ratio)
	resizedHeight := int32(float32(sys.gameHeight) * ratio)

	// calculate an X offset for the resized width to center it to the window
	if resizedWidth < int32(winWidth) {
		x = (int32(winWidth) - resizedWidth) / 2
	}

	return x, y, resizedWidth, resizedHeight
}

func (w *Window) GetClipboardString() string {
	res, _ := sdl.GetClipboardText()
	return res
}

func (w *Window) toggleFullscreen() {
	w.fullscreen = !w.fullscreen
	if w.fullscreen {
		w.Window.SetFullscreen(sdl.WINDOW_FULLSCREEN)
	} else {
		w.Window.SetFullscreen(0)
	}
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
			if t.Keysym.Scancode == 261 { // 261 = Reset Button in Playstation Classic
				// Create a quit event
				quitEvent := sdl.QuitEvent{
					Type:      sdl.QUIT,
					Timestamp: sdl.GetTicks(),
				}
				// Push the quit event to the SDL event queue
				sdl.PushEvent(&quitEvent)
			} else {
				OnKeyPressed(t.Keysym.Sym, sdl.Keymod(t.Keysym.Mod))
			}
		} else if t.Type == sdl.KEYUP {
			OnKeyReleased(t.Keysym.Sym, sdl.Keymod(t.Keysym.Mod))
		}
		break
	case *sdl.JoyDeviceAddedEvent:
		var isExist bool
		var kc KeyConfig
		input.joysticks[int(t.Which)] = sdl.JoystickOpen(int(t.Which))
		if input.joysticks[int(t.Which)] != nil {
			id := int(t.Which)
			sys.errLog.Printf("Joystick (%v) id=%v connected\n", input.joysticks[id].Name(), t.Which)
			if os.Getenv("XDG_SESSION_DESKTOP") == "KDE" { // in steamdeck there is 2 env: desktop mode(KDE) and gaming mode(gamescope), which each has spesific controller setting
				if input.joysticks[id].Name() == "Logitech Dual Action" {
					kc, isExist = sys.joystickDefaultConfig[input.joysticks[id].Name()+".KDE"]
				} else {
					kc, isExist = sys.joystickDefaultConfig[input.joysticks[id].Name()]
				}
			} else {
				kc, isExist = sys.joystickDefaultConfig[input.joysticks[id].Name()]
			}
			if isExist {
				sys.joystickConfig[id] = KeyConfig{id, kc.dU, kc.dD, kc.dL, kc.dR, kc.kA, kc.kB, kc.kC, kc.kX, kc.kY, kc.kZ, kc.kS, kc.kD, kc.kW, kc.kM}
				sys.errLog.Printf("Joystick [%v] is overwritten with %v\n", input.joysticks[id].Name(), sys.joystickConfig[id])
			}
		}
		break
	case *sdl.JoyDeviceRemovedEvent:
		if joystick := input.joysticks[int(t.Which)]; joystick != nil {
			joystick.Close()
		}
		sys.errLog.Printf("Joystick %v disconnected\n", t.Which)
		break
		// case *sdl.JoyButtonEvent:
		// 	if t.Button == 8 && t.State == 0 {
		// 		OnKeyPressed(KeyEscape, 0)
		// 	}
		// 	if t.Button == 9 && t.State == 0 {
		// 		OnKeyPressed(KeyEnter, 0)
		// 	}
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
