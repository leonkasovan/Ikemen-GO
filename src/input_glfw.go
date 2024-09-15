//go:build glfw

package main

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"

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

func (input *Input) GetJoystickHats(joy int) []glfw.JoystickHatState {
	if joy < 0 || joy >= len(input.joystick) {
		return []glfw.JoystickHatState{}
	}
	return input.joystick[joy].GetHats()
}

func JoystickState(joy, button int) bool {
	if joy < 0 {
		return sys.keyState[Key(button)]
	}
	if joy >= input.GetMaxJoystickCount() {
		return false
	}
	axes := input.GetJoystickAxes(joy)
	if button >= 0 {
		// Query button state
		btns := input.GetJoystickButtons(joy)
		// fmt.Printf("[input_glfw.go] joy_id=%v len(btns)=%v\n", joy, len(btns))
		if button >= len(btns) {
			if len(btns) == 0 {
				return false
			} else {
				if button == sys.joystickConfig[joy].dR {
					return axes[0] > sys.controllerStickSensitivityGLFW
				}
				if button == sys.joystickConfig[joy].dL {
					return -axes[0] > sys.controllerStickSensitivityGLFW
				}
				if button == sys.joystickConfig[joy].dU {
					return -axes[1] > sys.controllerStickSensitivityGLFW
				}
				if button == sys.joystickConfig[joy].dD {
					return axes[1] > sys.controllerStickSensitivityGLFW
				}
			}
			return false
		}

		// override with axes
		if button == sys.joystickConfig[joy].dR {
			if axes[0] > sys.controllerStickSensitivityGLFW {
				btns[button] = 1
			}
		}
		if button == sys.joystickConfig[joy].dL {
			if -axes[0] > sys.controllerStickSensitivityGLFW {
				btns[button] = 1
			}
		}
		if button == sys.joystickConfig[joy].dU {
			if -axes[1] > sys.controllerStickSensitivityGLFW {
				btns[button] = 1
			}
		}
		if button == sys.joystickConfig[joy].dD {
			if axes[1] > sys.controllerStickSensitivityGLFW {
				btns[button] = 1
			}
		}
		return btns[button] != 0
	} else {
		// Query axis state
		axis := -button - 1

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

		return val > sys.controllerStickSensitivityGLFW
	}
}

func checkAxisState(code int, axes *[]float32) bool {
	var axis int
	if code&1 == 0 {
		axis = (-code - 1) / 2
	} else {
		axis = -code / 2
	}
	if len(*axes) > axis {
		value := (*axes)[axis]
		if code&1 == 0 {
			return value > sys.controllerStickSensitivityGLFW
		} else {
			return -value > sys.controllerStickSensitivityGLFW
		}
	} else {
		return false
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
		jc := sys.joystickConfig[in]
		axes := input.GetJoystickAxes(in)
		use_axes := len(axes) > 0
		btns := input.GetJoystickButtons(in)
		use_btns := len(btns) > 0
		joyS := jc.Joy
		if joyS >= 0 {
			U = U || (use_axes && (-axes[1] > sys.controllerStickSensitivityGLFW)) || (use_btns && (btns[jc.dU] > 0))
			D = D || (use_axes && (axes[1] > sys.controllerStickSensitivityGLFW)) || (use_btns && (btns[jc.dD] > 0))
			L = L || (use_axes && (-axes[0] > sys.controllerStickSensitivityGLFW)) || (use_btns && (btns[jc.dL] > 0))
			R = R || (use_axes && (axes[0] > sys.controllerStickSensitivityGLFW)) || (use_btns && (btns[jc.dR] > 0))
			a = a || (use_btns && (btns[jc.kA] > 0))
			b = b || (use_btns && (btns[jc.kB] > 0))
			if jc.kC < 0 {
				if use_axes {
					c = c || checkAxisState(jc.kC, &axes)
				} else {
					c = c || false
				}
			} else {
				c = c || (use_btns && (btns[jc.kC] > 0))
			}
			x = x || (use_btns && (btns[jc.kX] > 0))
			y = y || (use_btns && (btns[jc.kY] > 0))
			if jc.kZ < 0 {
				if use_axes {
					z = z || checkAxisState(jc.kZ, &axes)
				} else {
					z = z || false
				}
			} else {
				z = z || (use_btns && (btns[jc.kZ] > 0))
			}
			if jc.kS < 0 {
				if use_axes {
					s = s || checkAxisState(jc.kS, &axes)
				} else {
					s = s || false
				}
			} else {
				s = s || (use_btns && (btns[jc.kS] > 0))
			}
			if jc.kD < 0 {
				if use_axes {
					d = d || checkAxisState(jc.kD, &axes)
				} else {
					d = d || false
				}
			} else {
				d = d || (use_btns && (btns[jc.kD] > 0))
			}
			if jc.kW < 0 {
				if use_axes {
					w = w || checkAxisState(jc.kW, &axes)
				} else {
					w = w || false
				}
			} else {
				w = w || (use_btns && (btns[jc.kW] > 0))
			}
			if jc.kM < 0 {
				if use_axes {
					m = m || checkAxisState(jc.kM, &axes)
				} else {
					m = m || false
				}
			} else {
				m = m || (use_btns && (btns[jc.kM] > 0))
			}
		}
	}
	// Button assist is checked locally so the sent inputs are already processed
	if sys.inputButtonAssist {
		a, b, c, x, y, z, s, d, w = ir.ButtonAssistCheck(a, b, c, x, y, z, s, d, w)
	}
	return U, D, L, R, a, b, c, x, y, z, s, d, w, m
}

func checkAxisForDpad(joy int, axes *[]float32, base int) string {
	var s string
	if (*axes)[0] > sys.controllerStickSensitivityGLFW { // right
		s = strconv.Itoa(2 + base)
		fmt.Printf("[input_glfw.go][checkAxisForDpad] AXIS for DPAD RIGHT joy=%v s: %v\n", joy, s)
	} else if -(*axes)[0] > sys.controllerStickSensitivityGLFW { // left
		s = strconv.Itoa(1 + base)
		fmt.Printf("[input_glfw.go][checkAxisForDpad] AXIS for DPAD LEFT joy=%v s: %v\n", joy, s)
	}
	if (*axes)[1] > sys.controllerStickSensitivityGLFW { // down
		s = strconv.Itoa(3 + base)
		fmt.Printf("[input_glfw.go][checkAxisForDpad] AXIS for DPAD DOWN joy=%v s: %v\n", joy, s)
	} else if -(*axes)[1] > sys.controllerStickSensitivityGLFW { // up
		s = strconv.Itoa(base)
		fmt.Printf("[input_glfw.go][checkAxisForDpad] AXIS  for DPAD UP joy=%v s: %v\n", joy, s)
	}
	return s
}

func checkAxisForTrigger(joy int, axes *[]float32) string {
	var s string = ""
	for i := range *axes {
		if (*axes)[i] < -sys.controllerStickSensitivityGLFW {
			name := input.GetJoystickName(joy) + "." + runtime.GOOS + "." + runtime.GOARCH + ".glfw"
			if (i == 4 || i == 5) && name == "XInput Gamepad (GLFW).windows.amd64.sdl" {
				// do nothing
			} else if (i == 4 || i == 5) && name == "PS4 Controller.windows.amd64.sdl" {
				// do nothing
			} else if (i == 2 || i == 5) && name == "Steam Virtual Gamepad.linux.amd64.glfw" {
				// do nothing
			} else if (i == 2 || i == 5) && name == "Steam Deck Controller.linux.amd64.sdl" {
				// do nothing
			} else if (i == 2 || i == 5) && name == "Logitech Dual Action.linux.amd64.glfw" {
				// do nothing
			} else if (i == 2 || i == 5) && name == "Logitech Dual Action.linux.amd64.sdl" {
				// do nothing
			} else {
				s = strconv.Itoa(-i*2 - 1)
				fmt.Printf("[input_glfw.go][checkAxisForTrigger] 1.AXIS joy=%v i=%v s:%v axes[i]=%v\n", joy, i, s, (*axes)[i])
				break
			}
		} else if (*axes)[i] > sys.controllerStickSensitivityGLFW {
			s = strconv.Itoa(-i*2 - 2)
			fmt.Printf("[input_glfw.go][checkAxisForTrigger] 2.AXIS joy=%v i=%v s:%v axes[i]=%v\n", joy, i, s, (*axes)[i])
			break
		}
	}
	return s
}
