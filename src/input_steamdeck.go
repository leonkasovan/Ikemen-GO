//go:build steamdeck
package main

import (
	"strings"
)

func JoystickState(joy, button int) bool {
	if joy < 0 {
		return sys.keyState[Key(button)]
	}
	if joy >= input.GetMaxJoystickCount() {
		return false
	}
	// Query axis state
	axis := -button - 1
	axes := input.GetJoystickAxes(joy)
	if button >= 0 {
		// Query button state
		btns := input.GetJoystickButtons(joy)
		if button >= len(btns) {
			return false
		}
		switch button {
		case 10:	// Up: check axis and d.pad(hat)
			return (axes[1] > 0.5) || (btns[13] != 0)
		case 11:	// Right: check axis and d.pad(hat)
			return (axes[0] > 0.5) || (btns[12] != 0)
		case 12:	// Down: check axis and d.pad(hat)
			return (axes[1] < -0.5) || (btns[11] != 0)
		case 13:	// Left: check axis and d.pad(hat)
			return (axes[0] < -0.5) || (btns[14] != 0)
		default:	// Other (normal) button
			return btns[button] != 0
		}
	} else {
		switch button {
		case -12:
			return (input.joysticks[joy].Axis(2) > 0.5)
		case -10:
			return (input.joysticks[joy].Axis(5) > 0.5)
		default:
			return false
		}
	}
}

// Reads controllers and converts inputs to letters for later processing
func (ir *InputReader) LocalInput(in int) (bool, bool, bool, bool, bool, bool, bool, bool, bool, bool, bool, bool, bool, bool) {
	var U, D, L, R, a, b, c, x, y, z, s, d, w, m bool
	// Keyboard
	if in < len(sys.keyConfig) {
		joy := sys.keyConfig[in].Joy
		if joy == -1 {
			U = sys.keyConfig[in].U()
			D = sys.keyConfig[in].D()
			L = sys.keyConfig[in].L()
			R = sys.keyConfig[in].R()
			a = sys.keyConfig[in].a()
			b = sys.keyConfig[in].b()
			c = sys.keyConfig[in].c()
			x = sys.keyConfig[in].x()
			y = sys.keyConfig[in].y()
			z = sys.keyConfig[in].z()
			s = sys.keyConfig[in].s()
			d = sys.keyConfig[in].d()
			w = sys.keyConfig[in].w()
			m = sys.keyConfig[in].m()
		}
	}
	// Joystick
	if in < len(sys.joystickConfig) {
		joyS := sys.joystickConfig[in].Joy
		if joyS >= 0 {
			U = sys.joystickConfig[in].U() || U // Does not override keyboard
			D = sys.joystickConfig[in].D() || D
			L = sys.joystickConfig[in].L() || L
			R = sys.joystickConfig[in].R() || R
			a = sys.joystickConfig[in].a() || a
			b = sys.joystickConfig[in].b() || b
			c = sys.joystickConfig[in].c() || c
			x = sys.joystickConfig[in].x() || x
			y = sys.joystickConfig[in].y() || y
			z = sys.joystickConfig[in].z() || z
			s = sys.joystickConfig[in].s() || s
			d = sys.joystickConfig[in].d() || d
			w = sys.joystickConfig[in].w() || w
			m = sys.joystickConfig[in].m() || m
		}
	}
	// Button assist is checked locally so the sent inputs are already processed
	if sys.inputButtonAssist {
		a, b, c, x, y, z, s, d, w = ir.ButtonAssistCheck(a, b, c, x, y, z, s, d, w)
	}
	return U, D, L, R, a, b, c, x, y, z, s, d, w, m
}
