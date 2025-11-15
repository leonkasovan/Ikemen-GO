package main

import (
	"fmt"
	"image"
	"unsafe"

	glfw "github.com/ikemen-engine/Ikemen-GO/packages/glfw"
)

type Window struct {
	*glfw.Window
	title      string
	fullscreen bool
	x, y, w, h int
}

func (s *System) newWindow(w, h int) (*Window, error) {
	var err error
	var window *glfw.Window
	var monitor *glfw.Monitor

	// Initialize OpenGL
	chk(glfw.Init())

	if monitor = glfw.GetPrimaryMonitor(); monitor == nil {
		return nil, fmt.Errorf("failed to obtain primary monitor")
	}

	// "-windowed" overrides the configuration setting but does not change it
	_, forceWindowed := sys.cmdFlags["-windowed"]
	fullscreen := s.cfg.Video.Fullscreen && !forceWindowed

	// Calculate window size & offset it
	var mode = monitor.GetVideoMode()
	var w2, h2 = w, h
	if sys.cfg.Video.WindowWidth > 0 || sys.cfg.Video.WindowHeight > 0 {
		w2, h2 = sys.cfg.Video.WindowWidth, sys.cfg.Video.WindowHeight
	}
	var x, y = (mode.Width - w2) / 2, (mode.Height - h2) / 2

	glfw.WindowHint(glfw.Resizable, glfw.True)

	// only GL 3.2 needs this
	if sys.cfg.Video.RenderMode == "OpenGL 3.2" {
		glfw.WindowHint(glfw.ContextVersionMajor, 3)
		glfw.WindowHint(glfw.ContextVersionMinor, 2)
		glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)
		glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	} else if s.cfg.Video.RenderMode == "OpenGL ES 3.1" {
		glfw.WindowHint(glfw.ClientAPI, glfw.OpenGLESAPI)
		glfw.WindowHint(glfw.ContextVersionMajor, 3)
		glfw.WindowHint(glfw.ContextVersionMinor, 1)
		glfw.WindowHint(glfw.ContextCreationAPI, glfw.EGLContextAPI)
	} else if sys.cfg.Video.RenderMode == "OpenGL 2.1" {
		glfw.WindowHint(glfw.ContextVersionMajor, 2)
		glfw.WindowHint(glfw.ContextVersionMinor, 1)
	} else {
		glfw.WindowHint(glfw.ClientAPI, glfw.NoAPI)
		glfw.WindowHint(glfw.ContextVersionMinor, 1)
	}

	// Create main window.
	// NOTE: Borderless fullscreen is in reality just a window without borders.
	if fullscreen && !s.cfg.Video.Borderless {
		window, err = glfw.CreateWindow(w2, h2, s.cfg.Config.WindowTitle, monitor, nil)
	} else {
		window, err = glfw.CreateWindow(w2, h2, s.cfg.Config.WindowTitle, nil, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create window: %w", err)
	}

	// Set windows attributes
	if fullscreen {
		window.SetPos(0, 0)
		if s.cfg.Video.Borderless {
			window.SetAttrib(glfw.Decorated, 0)
			window.SetSize(mode.Width, mode.Height)
		}
		window.SetInputMode(glfw.CursorMode, glfw.CursorHidden)
	} else {
		window.SetSize(w2, h2)
		window.SetInputMode(glfw.CursorMode, glfw.CursorNormal)
		if s.cfg.Video.WindowCentered {
			window.SetPos(x, y)
		}
	}
	if sys.cfg.Video.RenderMode == "OpenGL 3.2" || sys.cfg.Video.RenderMode == "OpenGL 2.1" || sys.cfg.Video.RenderMode == "OpenGL ES 3.1" {
		window.MakeContextCurrent()
		// V-Sync
		if s.cfg.Video.VSync >= 0 {
			glfw.SwapInterval(s.cfg.Video.VSync)
		}
	}

	window.SetKeyCallback(keyCallback)
	window.SetCharModsCallback(charCallback)
	window.SetRefreshCallback(refreshCallback)

	if glfw.GetPlatform() == "kmsdrm" { // KMS DRM mode, override window size
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

func (s *System) GetTime() float64 {
	return glfw.GetTime()
}

func (s *System) GetVulkanGetInstanceProcAddress() unsafe.Pointer {
	return glfw.GetVulkanGetInstanceProcAddress()
}

func (s *System) GetProcAddress() func(string) unsafe.Pointer {
	return glfw.GetProcAddress
}

func (w *Window) SwapBuffers() {
	w.Window.SwapBuffers()
	// Retrieve GL timestamp now
	glNow := glfw.GetTime()
	if glNow-sys.prevTimestamp >= 1 {
		sys.gameFPS = sys.absTickCountF / float32(glNow-sys.prevTimestamp)
		sys.absTickCountF = 0
		sys.prevTimestamp = glNow
		fmt.Printf("%v FPS, %v Drawcall\n", sys.gameFPS, sys.Drawcall)
		sys.nDrawcall = 0
	}
}

func (w *Window) SetIcon(icon []image.Image) {
	w.Window.SetIcon(icon)
}

func (w *Window) SetSwapInterval(interval int) {
	if sys.cfg.Video.RenderMode == "OpenGL 3.2" || sys.cfg.Video.RenderMode == "OpenGL 2.1" || sys.cfg.Video.RenderMode == "OpenGL ES 3.1" {
		glfw.SwapInterval(interval)
	} else {
		gfx.SetVSync()
	}
}

func (w *Window) GetSize() (int, int) {
	return w.Window.GetSize()
}

// Calculates a position and size for the viewport to fill the window while centered (see render_gl.go)
// Returns x, y, width, height respectively
func (w *Window) GetScaledViewportSize() (int32, int32, int32, int32) {
	winWidth, winHeight := w.GetSize()

	// If aspect ratio should not be kept, just return full window
	if !sys.cfg.Video.KeepAspect {
		return 0, 0, int32(winWidth), int32(winHeight)
	}

	var x, y, resizedWidth, resizedHeight int32 = 0, 0, int32(winWidth), int32(winHeight)

	// Select stage or default aspect ratio
	aspectGame := sys.getCurrentAspect()
	aspectWindow := float32(winWidth) / float32(winHeight)

	// Keep aspect ratio
	if aspectWindow > aspectGame {
		// Window is wider: black bars on sides
		resizedHeight = int32(winHeight)
		resizedWidth = int32(float32(resizedHeight) * aspectGame)
		x = (int32(winWidth) - resizedWidth) / 2
		y = 0
	} else {
		// Window is taller: black bars on top and bottom
		resizedWidth = int32(winWidth)
		resizedHeight = int32(float32(resizedWidth) / aspectGame)
		x = 0
		y = (int32(winHeight) - resizedHeight) / 2
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
	if sys.cfg.Video.VSync != -1 && (sys.cfg.Video.RenderMode == "OpenGL 3.2" || sys.cfg.Video.RenderMode == "OpenGL 2.1" || sys.cfg.Video.RenderMode == "OpenGL ES 3.1") {
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

func refreshCallback(w *glfw.Window) {
	if sys.cfg.Video.RenderMode == "OpenGL 3.2" || sys.cfg.Video.RenderMode == "OpenGL 2.1" || sys.cfg.Video.RenderMode == "OpenGL ES 3.1" {
		gfx.EndFrame()
		w.SwapBuffers()
	}
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