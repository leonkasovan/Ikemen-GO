//go:build glfw

package main

import (
	"fmt"
	"image"
	"os"
	"runtime"
	"strings"

	glfw "github.com/go-gl/glfw/v3.3/glfw"
)

type Window struct {
	*glfw.Window
	title      string
	fullscreen bool
	x, y, w, h int
}

func updateTimeStamp() {
	sys.prevTimestamp = glfw.GetTime()
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

	var mode = monitor.GetVideoMode()
	var x, y = (mode.Width - w) / 2, (mode.Height - h) / 2
	fmt.Printf("[system_glfw.go][newWindow] monitor.GetVideoMode() = %v\n", mode)

	// "-windowed" overrides the configuration setting but does not change it
	_, forceWindowed := sys.cmdFlags["-windowed"]
	fullscreen := s.fullscreen && !forceWindowed

	glfw.WindowHint(glfw.Resizable, glfw.True)

	// only macOS needs this
	if runtime.GOOS == "darwin" {
		glfw.WindowHint(glfw.ContextVersionMajor, 3)
		glfw.WindowHint(glfw.ContextVersionMinor, 2)
		glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)
		glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	} else {
		glfw.WindowHint(glfw.ContextVersionMajor, 2)
		glfw.WindowHint(glfw.ContextVersionMinor, 1)
	}

	// Create main window.
	// NOTE: Borderless fullscreen is in reality just a window without borders.
	if fullscreen && !s.borderless {
		window, err = glfw.CreateWindow(w, h, s.windowTitle, monitor, nil)
	} else {
		window, err = glfw.CreateWindow(w, h, s.windowTitle, nil, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create window: %w", err)
	}

	// Set windows attributes
	if fullscreen {
		window.SetPos(0, 0)
		if s.borderless {
			window.SetAttrib(glfw.Decorated, 0)
			window.SetSize(mode.Width, mode.Height)
		}
		window.SetInputMode(glfw.CursorMode, glfw.CursorHidden)
	} else {
		window.SetSize(w, h)
		window.SetInputMode(glfw.CursorMode, glfw.CursorNormal)
		if s.windowCentered {
			window.SetPos(x, y)
		}
	}

	window.MakeContextCurrent()
	window.SetKeyCallback(keyCallback)
	window.SetCharModsCallback(charCallback)

	// V-Sync
	if s.vRetrace >= 0 {
		glfw.SwapInterval(s.vRetrace)
	}

	for i := glfw.Joystick1; i <= glfw.JoystickLast; i++ {
		if i.Present() {
			var isExist bool
			var kc KeyConfig
			name := i.GetGamepadName() + "." + runtime.GOOS + "." + runtime.GOARCH + ".glfw"
			if os.Getenv("XDG_CURRENT_DESKTOP") == "KDE" { // in steamdeck there is 2 env: desktop mode(KDE) and gaming mode(gamescope), which each has spesific controller setting
				if strings.Contains(name, "Logitech Dual Action") || strings.Contains(name, "Steam Virtual Gamepad") {
					name = name + ".KDE"
				}
			}
			fmt.Printf("[system_glfw.go][newWindow] Using Joystick id=%v [%v]\n\tTotal Buttons=%v\n\tTotal Axes=%v\n\tTotal Hats=%v\n", i, name, len(i.GetButtons()), len(i.GetAxes()), len(i.GetHats()))
			kc, isExist = sys.joystickDefaultConfig[name]
			if isExist {
				sys.joystickConfig[i] = KeyConfig{int(i), kc.dU, kc.dD, kc.dL, kc.dR, kc.kA, kc.kB, kc.kC, kc.kX, kc.kY, kc.kZ, kc.kS, kc.kD, kc.kW, kc.kM}
				fmt.Printf("\tConfig is overwritten with %v\n", sys.joystickConfig[i])
			} else {
				fmt.Printf("\tConfig is NOT overwritten, using %v\n", sys.joystickConfig[i])
			}
		}
	}

	ret := &Window{window, s.windowTitle, fullscreen, x, y, w, h}
	return ret, err
}

func (w *Window) SwapBuffers() {
	w.Window.SwapBuffers()
	// Retrieve GL timestamp now
	glNow := glfw.GetTime()
	if glNow-sys.prevTimestamp >= 1 {
		sys.gameFPS = sys.absTickCountGLFW / float32(glNow-sys.prevTimestamp)
		sys.absTickCountGLFW = 0
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

	if sys.fullscreen || int32(winWidth) == sys.scrrect[2] && int32(winHeight) == sys.scrrect[3] {
		return 0, 0, int32(winWidth), int32(winHeight)
	}

	if ratioWidth < ratioHeight {
		ratio = ratioWidth
	} else {
		ratio = ratioHeight
	}

	if sys.keepAspect {
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
		if sys.borderless {
			w.SetSize(mode.Width, mode.Height)
			w.SetMonitor(&glfw.Monitor{}, 0, 0, mode.Width, mode.Height, mode.RefreshRate)
		} else {
			w.x, w.y = w.GetPos()
			w.SetMonitor(glfw.GetPrimaryMonitor(), w.x, w.y, w.w, w.h, mode.RefreshRate)
		}
		w.SetInputMode(glfw.CursorMode, glfw.CursorHidden)
	}
	if sys.vRetrace != -1 {
		glfw.SwapInterval(sys.vRetrace)
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

func nextTickCount() {
	sys.absTickCountGLFW++
}