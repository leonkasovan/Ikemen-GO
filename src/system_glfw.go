package main

import (
	"fmt"
	"image"

	glfw "github.com/leonkasovan/glfw/v3.5/glfw"
)

type Window struct {
	*glfw.Window
	title      string
	fullscreen bool
	x, y, w, h int
}

func (s *System) GetTime() float64 {
	return glfw.GetTime()
}

func (s *System) newWindow(w, h int) (*Window, error) {
	var err error
	var window *glfw.Window
	var monitor *glfw.Monitor
	var fullscreen bool
	var x, y int
	var mode *glfw.VidMode
	var w2, h2 int
	var drm_mode bool

	// Initialize OpenGL
	glfw.InitHint(0x00053001, 0x00038002) // disable libdecor for wayland
	chk(glfw.Init())

	fmt.Printf("Platform: %v\n", glfw.GetVersionString())

	if monitor = glfw.GetPrimaryMonitor(); monitor == nil { // Get primary monitor, if it nil then we are using KMS DRM mode in fullscreen
		fullscreen = true
		x, y = 0, 0
		w2, h2 = w, h
		drm_mode = true
	} else {
		// "-windowed" overrides the configuration setting but does not change it
		_, forceWindowed := sys.cmdFlags["-windowed"]
		fullscreen = s.cfg.Video.Fullscreen && !forceWindowed
		// Calculate window size & offset it
		mode = monitor.GetVideoMode()
		w2, h2 = w, h
		if !fullscreen && (sys.cfg.Video.WindowWidth > 0 || sys.cfg.Video.WindowHeight > 0) {
			w2, h2 = sys.cfg.Video.WindowWidth, sys.cfg.Video.WindowHeight
		}
		x, y = (mode.Width-w2)/2, (mode.Height-h2)/2
		glfw.WindowHint(glfw.Resizable, glfw.True)
		drm_mode = false
	}

	// Initialize Gfx with OpenGL (ES)
	s.initGfx()

	// Create main window.
	// NOTE: Borderless fullscreen is in reality just a window without borders.
	if fullscreen && !s.cfg.Video.Borderless {
		window, err = glfw.CreateWindow(w, h, s.cfg.Config.WindowTitle, monitor, nil)
	} else {
		window, err = glfw.CreateWindow(w, h, s.cfg.Config.WindowTitle, nil, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create window: %w", err)
	}

	// Set windows attributes
	// if !drm_mode {
	// 	if fullscreen {
	// 		window.SetPos(0, 0)
	// 		if s.cfg.Video.Borderless {
	// 			window.SetAttrib(glfw.Decorated, 0)
	// 			window.SetSize(mode.Width, mode.Height)
	// 		}
	// 		window.SetInputMode(glfw.CursorMode, glfw.CursorHidden)
	// 	} else {
	// 		window.SetSize(w2, h2)
	// 		window.SetInputMode(glfw.CursorMode, glfw.CursorNormal)
	// 		if s.cfg.Video.WindowCentered {
	// 			window.SetPos(x, y)
	// 		}
	// 	}
	// }

	window.MakeContextCurrent()
	window.SetKeyCallback(keyCallback)
	window.SetCharModsCallback(charCallback)

	// V-Sync
	if s.cfg.Video.VSync >= 0 {
		glfw.SwapInterval(s.cfg.Video.VSync)
	}

	if drm_mode { // KMS DRM mode, override window size
		w, h = window.GetSize()
		if s.cfg.Video.WindowWidth != w {
			fmt.Printf("Overriding configuration Video.WindowWidth(%d) with Monitor's width(%d)\n", s.cfg.Video.WindowWidth, w)
			s.cfg.Video.WindowWidth = w
		}
		if s.cfg.Video.WindowHeight != h {
			fmt.Printf("Overriding configuration Video.WindowHeight(%d) with Monitor's height(%d)\n", s.cfg.Video.WindowHeight, h)
			s.cfg.Video.WindowHeight = h
		}
	}

	ret := &Window{window, s.cfg.Config.WindowTitle, fullscreen, x, y, w, h}
	return ret, err
}

func (w *Window) SwapBuffers() {
	w.Window.SwapBuffers()
	// Retrieve GL timestamp now
	glNow := glfw.GetTime()
	if glNow-sys.prevTimestamp >= 1 {
		sys.gameFPS = sys.absTickCountF / float32(glNow-sys.prevTimestamp)
		sys.absTickCountF = 0
		sys.prevTimestamp = glNow
	}
}

func (w *Window) SetIcon(icon []image.Image) {
	w.Window.SetIcon(icon)
}

func (w *Window) SetSwapInterval(interval int) {
	glfw.SwapInterval(interval)
}

func (w *Window) GetSize() (int, int) {
	return w.Window.GetSize()
}

func (w *Window) GetScaledViewportSize() (int32, int32, int32, int32) {
	// calculates a position and size for the viewport to fill the window while centered (see render_gl.go)
	// returns x, y, width, height respectively
	winWidth, winHeight := w.GetSize()
	ratioWidth := float32(winWidth) / float32(sys.gameWidth)
	ratioHeight := float32(winHeight) / float32(sys.gameHeight)
	var ratio float32
	var x, y, resizedWidth, resizedHeight int32 = 0, 0, int32(winWidth), int32(winHeight)

	if sys.cfg.Video.Fullscreen || int32(winWidth) == sys.scrrect[2] && int32(winHeight) == sys.scrrect[3] {
		return 0, 0, int32(winWidth), int32(winHeight)
	}

	if ratioWidth < ratioHeight {
		ratio = ratioWidth
	} else {
		ratio = ratioHeight
	}

	if sys.cfg.Video.KeepAspect {
		resizedWidth = int32(float32(sys.gameWidth) * ratio)
		resizedHeight = int32(float32(sys.gameHeight) * ratio)

		// calculate offsets for the resized width to center it to the window
		if resizedWidth < int32(winWidth) {
			x = (int32(winWidth) - resizedWidth) / 2
		}
		if resizedHeight < int32(winHeight) {
			y = (int32(winHeight) - resizedHeight) / 2
		}
	}

	return x, y, resizedWidth, resizedHeight
}

func (w *Window) GetClipboardString() string {
	return w.Window.GetClipboardString()
}

func (w *Window) toggleFullscreen() {
	var mode = glfw.GetPrimaryMonitor().GetVideoMode()

	if w.fullscreen {
		w.SetAttrib(glfw.Decorated, 1)
		w.SetMonitor(&glfw.Monitor{}, w.x, w.y, w.w, w.h, mode.RefreshRate)
		w.SetInputMode(glfw.CursorMode, glfw.CursorNormal)
	} else {
		w.SetAttrib(glfw.Decorated, 0)
		if sys.cfg.Video.Borderless {
			w.SetSize(mode.Width, mode.Height)
			w.SetMonitor(&glfw.Monitor{}, 0, 0, mode.Width, mode.Height, mode.RefreshRate)
		} else {
			w.x, w.y = w.GetPos()
			w.SetMonitor(glfw.GetPrimaryMonitor(), w.x, w.y, w.w, w.h, mode.RefreshRate)
		}
		w.SetInputMode(glfw.CursorMode, glfw.CursorHidden)
	}
	if sys.cfg.Video.VSync != -1 {
		glfw.SwapInterval(sys.cfg.Video.VSync)
	}
	w.fullscreen = !w.fullscreen
}

func (w *Window) pollEvents() {
	glfw.PollEvents()
}

func (w *Window) shouldClose() bool {
	return w.Window.ShouldClose()
}

func (w *Window) Close() {
	glfw.Terminate()
}

func keyCallback(_ *glfw.Window, key Key, _ int, action glfw.Action, mk ModifierKey) {
	switch action {
	case glfw.Release:
		OnKeyReleased(key, mk)
	case glfw.Press:
		OnKeyPressed(key, mk)
	}
}

func charCallback(_ *glfw.Window, char rune, mk ModifierKey) {
	OnTextEntered(string(char))
}
