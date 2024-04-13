//go:build glfw

package main

import (
	glfw "github.com/go-gl/glfw/v3.3/glfw"
)

type Input struct {
	joystick []glfw.Joystick
}

type Key = glfw.Key
type ModifierKey = glfw.ModifierKey

const (
	KeyUnknown = glfw.KeyUnknown
	KeyEscape  = glfw.KeyEscape
	KeyEnter   = glfw.KeyEnter
	KeyInsert  = glfw.KeyInsert
	KeyF12     = glfw.KeyF12
)

var KeyToStringLUT = map[glfw.Key]string{
	glfw.KeyEnter:        "RETURN",
	glfw.KeyEscape:       "ESCAPE",
	glfw.KeyBackspace:    "BACKSPACE",
	glfw.KeyTab:          "TAB",
	glfw.KeySpace:        "SPACE",
	glfw.KeyApostrophe:   "QUOTE",
	glfw.KeyComma:        "COMMA",
	glfw.KeyMinus:        "MINUS",
	glfw.KeyPeriod:       "PERIOD",
	glfw.KeySlash:        "SLASH",
	glfw.Key0:            "0",
	glfw.Key1:            "1",
	glfw.Key2:            "2",
	glfw.Key3:            "3",
	glfw.Key4:            "4",
	glfw.Key5:            "5",
	glfw.Key6:            "6",
	glfw.Key7:            "7",
	glfw.Key8:            "8",
	glfw.Key9:            "9",
	glfw.KeySemicolon:    "SEMICOLON",
	glfw.KeyEqual:        "EQUALS",
	glfw.KeyLeftBracket:  "LBRACKET",
	glfw.KeyBackslash:    "BACKSLASH",
	glfw.KeyRightBracket: "RBRACKET",
	glfw.KeyGraveAccent:  "BACKQUOTE",
	glfw.KeyA:            "a",
	glfw.KeyB:            "b",
	glfw.KeyC:            "c",
	glfw.KeyD:            "d",
	glfw.KeyE:            "e",
	glfw.KeyF:            "f",
	glfw.KeyG:            "g",
	glfw.KeyH:            "h",
	glfw.KeyI:            "i",
	glfw.KeyJ:            "j",
	glfw.KeyK:            "k",
	glfw.KeyL:            "l",
	glfw.KeyM:            "m",
	glfw.KeyN:            "n",
	glfw.KeyO:            "o",
	glfw.KeyP:            "p",
	glfw.KeyQ:            "q",
	glfw.KeyR:            "r",
	glfw.KeyS:            "s",
	glfw.KeyT:            "t",
	glfw.KeyU:            "u",
	glfw.KeyV:            "v",
	glfw.KeyW:            "w",
	glfw.KeyX:            "x",
	glfw.KeyY:            "y",
	glfw.KeyZ:            "z",
	glfw.KeyCapsLock:     "CAPSLOCK",
	glfw.KeyF1:           "F1",
	glfw.KeyF2:           "F2",
	glfw.KeyF3:           "F3",
	glfw.KeyF4:           "F4",
	glfw.KeyF5:           "F5",
	glfw.KeyF6:           "F6",
	glfw.KeyF7:           "F7",
	glfw.KeyF8:           "F8",
	glfw.KeyF9:           "F9",
	glfw.KeyF10:          "F10",
	glfw.KeyF11:          "F11",
	glfw.KeyF12:          "F12",
	glfw.KeyPrintScreen:  "PRINTSCREEN",
	glfw.KeyScrollLock:   "SCROLLLOCK",
	glfw.KeyPause:        "PAUSE",
	glfw.KeyInsert:       "INSERT",
	glfw.KeyHome:         "HOME",
	glfw.KeyPageUp:       "PAGEUP",
	glfw.KeyDelete:       "DELETE",
	glfw.KeyEnd:          "END",
	glfw.KeyPageDown:     "PAGEDOWN",
	glfw.KeyRight:        "RIGHT",
	glfw.KeyLeft:         "LEFT",
	glfw.KeyDown:         "DOWN",
	glfw.KeyUp:           "UP",
	glfw.KeyNumLock:      "NUMLOCKCLEAR",
	glfw.KeyKPDivide:     "KP_DIVIDE",
	glfw.KeyKPMultiply:   "KP_MULTIPLY",
	glfw.KeyKPSubtract:   "KP_MINUS",
	glfw.KeyKPAdd:        "KP_PLUS",
	glfw.KeyKPEnter:      "KP_ENTER",
	glfw.KeyKP1:          "KP_1",
	glfw.KeyKP2:          "KP_2",
	glfw.KeyKP3:          "KP_3",
	glfw.KeyKP4:          "KP_4",
	glfw.KeyKP5:          "KP_5",
	glfw.KeyKP6:          "KP_6",
	glfw.KeyKP7:          "KP_7",
	glfw.KeyKP8:          "KP_8",
	glfw.KeyKP9:          "KP_9",
	glfw.KeyKP0:          "KP_0",
	glfw.KeyKPDecimal:    "KP_PERIOD",
	glfw.KeyKPEqual:      "KP_EQUALS",
	glfw.KeyF13:          "F13",
	glfw.KeyF14:          "F14",
	glfw.KeyF15:          "F15",
	glfw.KeyF16:          "F16",
	glfw.KeyF17:          "F17",
	glfw.KeyF18:          "F18",
	glfw.KeyF19:          "F19",
	glfw.KeyF20:          "F20",
	glfw.KeyF21:          "F21",
	glfw.KeyF22:          "F22",
	glfw.KeyF23:          "F23",
	glfw.KeyF24:          "F24",
	glfw.KeyMenu:         "MENU",
	glfw.KeyLeftControl:  "LCTRL",
	glfw.KeyLeftShift:    "LSHIFT",
	glfw.KeyLeftAlt:      "LALT",
	glfw.KeyLeftSuper:    "LGUI",
	glfw.KeyRightControl: "RCTRL",
	glfw.KeyRightShift:   "RSHIFT",
	glfw.KeyRightAlt:     "RALT",
	glfw.KeyRightSuper:   "RGUI",
}

var StringToKeyLUT = map[string]glfw.Key{}

func init() {
	for k, v := range KeyToStringLUT {
		StringToKeyLUT[v] = k
	}
}

func StringToKey(s string) glfw.Key {
	if key, ok := StringToKeyLUT[s]; ok {
		return key
	}
	return glfw.KeyUnknown
}

func KeyToString(k glfw.Key) string {
	if s, ok := KeyToStringLUT[k]; ok {
		return s
	}
	return ""
}

func NewModifierKey(ctrl, alt, shift bool) (mod glfw.ModifierKey) {
	if ctrl {
		mod |= glfw.ModControl
	}
	if alt {
		mod |= glfw.ModAlt
	}
	if shift {
		mod |= glfw.ModShift
	}
	return
}

var input = Input{
	joystick: []glfw.Joystick{glfw.Joystick1, glfw.Joystick2, glfw.Joystick3,
		glfw.Joystick4, glfw.Joystick5, glfw.Joystick6, glfw.Joystick7,
		glfw.Joystick8, glfw.Joystick9, glfw.Joystick10, glfw.Joystick11,
		glfw.Joystick12, glfw.Joystick13, glfw.Joystick14, glfw.Joystick15,
		glfw.Joystick16},
}

func (input *Input) GetMaxJoystickCount() int {
	return len(input.joystick)
}

func (input *Input) IsJoystickPresent(joy int) bool {
	if joy < 0 || joy >= len(input.joystick) {
		return false
	}
	return input.joystick[joy].Present()
}

func (input *Input) GetJoystickName(joy int) string {
	if joy < 0 || joy >= len(input.joystick) {
		return ""
	}
	return input.joystick[joy].GetGamepadName()
}

func (input *Input) GetJoystickAxes(joy int) []float32 {
	if joy < 0 || joy >= len(input.joystick) {
		return []float32{}
	}
	return input.joystick[joy].GetAxes()
}

func (input *Input) GetJoystickButtons(joy int) []glfw.Action {
	if joy < 0 || joy >= len(input.joystick) {
		return []glfw.Action{}
	}
	return input.joystick[joy].GetButtons()
}

func JoystickState(joy, button int) bool {
	if joy < 0 {
		return sys.keyState[Key(button)]
	}
	if joy >= input.GetMaxJoystickCount() {
		return false
	}
	if button >= 0 {
		// Query button state
		btns := input.GetJoystickButtons(joy)
		if button >= len(btns) {
			return false
		}
		return btns[button] != 0
	} else {
		// Query axis state
		axis := -button - 1
		axes := input.GetJoystickAxes(joy)
		if axis >= len(axes)*2 {
			return false
		}

		// Read value and invert sign for odd indices
		val := axes[axis/2] * float32((axis&1)*2-1)

		var joyName = input.GetJoystickName(joy)

		// Xbox360コントローラーのLRトリガー判定
		// "Evaluate LR triggers on the Xbox 360 controller"
		if (axis == 9 || axis == 11) && (strings.Contains(joyName, "XInput") || strings.Contains(joyName, "X360")) {
			return val > sys.xinputTriggerSensitivity
		}

		// Ignore trigger axis on PS4 (We already have buttons)
		if (axis >= 6 && axis <= 9) && joyName == "PS4 Controller" {
			return false
		}

		return val > sys.controllerStickSensitivity
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