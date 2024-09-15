//go:build sdl

package main

import (
	"fmt"
	"runtime"
	"strconv"

	sdl "github.com/veandco/go-sdl2/sdl"
)

const MAX_JOYSTICK_COUNT = 4

type Input struct {
	joysticks [MAX_JOYSTICK_COUNT]*sdl.Joystick
}

type Key = sdl.Keycode
type ModifierKey = sdl.Keymod

const (
	KeyUnknown = sdl.K_UNKNOWN
	KeyEscape  = sdl.K_ESCAPE
	KeyEnter   = sdl.K_RETURN
	KeyInsert  = sdl.K_INSERT
	KeyF12     = sdl.K_F12
)

var KeyToStringLUT = map[sdl.Keycode]string{
	sdl.K_RETURN:       "RETURN",
	sdl.K_ESCAPE:       "ESCAPE",
	sdl.K_BACKSPACE:    "BACKSPACE",
	sdl.K_TAB:          "TAB",
	sdl.K_SPACE:        "SPACE",
	sdl.K_QUOTE:        "QUOTE",
	sdl.K_COMMA:        "COMMA",
	sdl.K_MINUS:        "MINUS",
	sdl.K_PERIOD:       "PERIOD",
	sdl.K_SLASH:        "SLASH",
	sdl.K_0:            "0",
	sdl.K_1:            "1",
	sdl.K_2:            "2",
	sdl.K_3:            "3",
	sdl.K_4:            "4",
	sdl.K_5:            "5",
	sdl.K_6:            "6",
	sdl.K_7:            "7",
	sdl.K_8:            "8",
	sdl.K_9:            "9",
	sdl.K_SEMICOLON:    "SEMICOLON",
	sdl.K_EQUALS:       "EQUALS",
	sdl.K_LEFTBRACKET:  "LBRACKET",
	sdl.K_BACKSLASH:    "BACKSLASH",
	sdl.K_RIGHTBRACKET: "RBRACKET",
	sdl.K_BACKQUOTE:    "BACKQUOTE",
	sdl.K_a:            "a",
	sdl.K_b:            "b",
	sdl.K_c:            "c",
	sdl.K_d:            "d",
	sdl.K_e:            "e",
	sdl.K_f:            "f",
	sdl.K_g:            "g",
	sdl.K_h:            "h",
	sdl.K_i:            "i",
	sdl.K_j:            "j",
	sdl.K_k:            "k",
	sdl.K_l:            "l",
	sdl.K_m:            "m",
	sdl.K_n:            "n",
	sdl.K_o:            "o",
	sdl.K_p:            "p",
	sdl.K_q:            "q",
	sdl.K_r:            "r",
	sdl.K_s:            "s",
	sdl.K_t:            "t",
	sdl.K_u:            "u",
	sdl.K_v:            "v",
	sdl.K_w:            "w",
	sdl.K_x:            "x",
	sdl.K_y:            "y",
	sdl.K_z:            "z",
	sdl.K_CAPSLOCK:     "CAPSLOCK",
	sdl.K_F1:           "F1",
	sdl.K_F2:           "F2",
	sdl.K_F3:           "F3",
	sdl.K_F4:           "F4",
	sdl.K_F5:           "F5",
	sdl.K_F6:           "F6",
	sdl.K_F7:           "F7",
	sdl.K_F8:           "F8",
	sdl.K_F9:           "F9",
	sdl.K_F10:          "F10",
	sdl.K_F11:          "F11",
	sdl.K_F12:          "F12",
	sdl.K_PRINTSCREEN:  "PRINTSCREEN",
	sdl.K_SCROLLLOCK:   "SCROLLLOCK",
	sdl.K_PAUSE:        "PAUSE",
	sdl.K_INSERT:       "INSERT",
	sdl.K_HOME:         "HOME",
	sdl.K_PAGEUP:       "PAGEUP",
	sdl.K_DELETE:       "DELETE",
	sdl.K_END:          "END",
	sdl.K_PAGEDOWN:     "PAGEDOWN",
	sdl.K_RIGHT:        "RIGHT",
	sdl.K_LEFT:         "LEFT",
	sdl.K_DOWN:         "DOWN",
	sdl.K_UP:           "UP",
	sdl.K_NUMLOCKCLEAR: "NUMLOCKCLEAR",
	sdl.K_KP_DIVIDE:    "KP_DIVIDE",
	sdl.K_KP_MULTIPLY:  "KP_MULTIPLY",
	sdl.K_KP_MINUS:     "KP_MINUS",
	sdl.K_KP_PLUS:      "KP_PLUS",
	sdl.K_KP_ENTER:     "KP_ENTER",
	sdl.K_KP_1:         "KP_1",
	sdl.K_KP_2:         "KP_2",
	sdl.K_KP_3:         "KP_3",
	sdl.K_KP_4:         "KP_4",
	sdl.K_KP_5:         "KP_5",
	sdl.K_KP_6:         "KP_6",
	sdl.K_KP_7:         "KP_7",
	sdl.K_KP_8:         "KP_8",
	sdl.K_KP_9:         "KP_9",
	sdl.K_KP_0:         "KP_0",
	sdl.K_KP_PERIOD:    "KP_PERIOD",
	sdl.K_KP_EQUALS:    "KP_EQUALS",
	sdl.K_F13:          "F13",
	sdl.K_F14:          "F14",
	sdl.K_F15:          "F15",
	sdl.K_F16:          "F16",
	sdl.K_F17:          "F17",
	sdl.K_F18:          "F18",
	sdl.K_F19:          "F19",
	sdl.K_F20:          "F20",
	sdl.K_F21:          "F21",
	sdl.K_F22:          "F22",
	sdl.K_F23:          "F23",
	sdl.K_F24:          "F24",
	sdl.K_MENU:         "MENU",
	sdl.K_LCTRL:        "LCTRL",
	sdl.K_LSHIFT:       "LSHIFT",
	sdl.K_LALT:         "LALT",
	sdl.K_LGUI:         "LGUI",
	sdl.K_RCTRL:        "RCTRL",
	sdl.K_RSHIFT:       "RSHIFT",
	sdl.K_RALT:         "RALT",
	sdl.K_RGUI:         "RGUI",
}

var StringToKeyLUT = map[string]sdl.Keycode{}

func init() {
	sdl.JoystickEventState(sdl.ENABLE)
	for k, v := range KeyToStringLUT {
		StringToKeyLUT[v] = k
	}
}

func StringToKey(s string) sdl.Keycode {
	if key, ok := StringToKeyLUT[s]; ok {
		return key
	}
	return sdl.K_UNKNOWN
}

func KeyToString(k sdl.Keycode) string {
	if s, ok := KeyToStringLUT[k]; ok {
		return s
	}
	return ""
}

// to be fix: doesn't work toggle Full Screen
func NewModifierKey(ctrl, alt, shift bool) (mod ModifierKey) {
	if ctrl {
		mod |= sdl.KMOD_CTRL
	}
	if alt {
		mod |= sdl.KMOD_ALT
	}
	if shift {
		mod |= sdl.KMOD_SHIFT
	}
	return mod
}

var input Input

func (input *Input) GetMaxJoystickCount() int {
	return len(input.joysticks)
}

func (input *Input) IsJoystickPresent(joy int) bool {
	if joy < 0 || joy >= len(input.joysticks) {
		return false
	}
	return input.joysticks[joy].Attached()
}

func (input *Input) GetJoystickName(joy int) string {
	if joy < 0 || joy >= len(input.joysticks) {
		return ""
	}
	return input.joysticks[joy].Name()
}

func (input *Input) GetJoystickAxis(joy int, axis int) int16 {
	if joy < 0 || joy >= len(input.joysticks) {
		return 0
	}
	return input.joysticks[joy].Axis(axis)
}

func (input *Input) GetJoystickAxes(joy int) []int16 {
	if joy < 0 || joy >= len(input.joysticks) {
		return []int16{}
	}
	axes := make([]int16, input.joysticks[joy].NumAxes())
	for i := range axes {
		axes[i] = input.joysticks[joy].Axis(i)
	}
	return axes
}

func (input *Input) GetJoystickButtons(joy int) []byte {
	if joy < 0 || joy >= len(input.joysticks) {
		return []byte{}
	}
	buttons := make([]byte, input.joysticks[joy].NumButtons())
	for i := range buttons {
		buttons[i] = input.joysticks[joy].Button(i)
	}
	return buttons
}

func (input *Input) GetJoystickHats(joy int) []byte {
	if joy < 0 || joy >= len(input.joysticks) {
		return []byte{}
	}
	hats := make([]byte, input.joysticks[joy].NumHats())
	for i := range hats {
		hats[i] = input.joysticks[joy].Hat(i)
	}
	return hats
}

func JoystickState(joy, button int) bool {
	if joy < 0 {
		return sys.keyState[Key(button)]
	}
	if joy >= input.GetMaxJoystickCount() {
		return false
	}
	js := input.joysticks[joy]
	if button >= js.NumButtons() { // only check for Hats or "axis for dpad" (max_button,max_button+1,max_button+2,max_button+3)
		if js.NumAxes() >= 2 { // check axes for dpad
			switch button { // check HAT0, AXIS1, AXIS2
			case sys.joystickConfig[joy].dU: // Up
				return (js.Axis(1) < -sys.controllerStickSensitivitySDL) || ((js.Hat(0) & 1) != 0)
			case sys.joystickConfig[joy].dR: // Right
				return (js.Axis(0) > sys.controllerStickSensitivitySDL) || ((js.Hat(0) & 2) != 0)
			case sys.joystickConfig[joy].dD: // Down
				return (js.Axis(1) > sys.controllerStickSensitivitySDL) || ((js.Hat(0) & 4) != 0)
			case sys.joystickConfig[joy].dL: // Left
				return (js.Axis(0) < -sys.controllerStickSensitivitySDL) || ((js.Hat(0) & 8) != 0)
			default: // invalid button code if > max_button+3
				return false
			}
		} else {
			switch button { // check HAT0 only
			case sys.joystickConfig[joy].dU: // Up
				return js.Hat(0)&1 != 0
			case sys.joystickConfig[joy].dR: // Right
				return js.Hat(0)&2 != 0
			case sys.joystickConfig[joy].dD: // Down
				return js.Hat(0)&4 != 0
			case sys.joystickConfig[joy].dL: // Left
				return js.Hat(0)&8 != 0
			default: // invalid button code if > max_button+3
				return false
			}
		}
	} else if button >= 0 { // Check for button code (0,1,2,...,10,11,max_button-1)
		if js.NumAxes() >= 2 {
			switch button { // check BUTTON, AXIS1, AXIS2
			case sys.joystickConfig[joy].dU: // Up: check axis, d.pad(hat), button
				return (js.Axis(1) < -sys.controllerStickSensitivitySDL) || (js.Button(button) != 0)
			case sys.joystickConfig[joy].dR: // Right: check axis and d.pad(hat), button
				return (js.Axis(0) > sys.controllerStickSensitivitySDL) || (js.Button(button) != 0)
			case sys.joystickConfig[joy].dD: // Down: check axis and d.pad(hat), button
				return (js.Axis(1) > sys.controllerStickSensitivitySDL) || (js.Button(button) != 0)
			case sys.joystickConfig[joy].dL: // Left: check axis and d.pad(hat), button
				return (js.Axis(0) < -sys.controllerStickSensitivitySDL) || (js.Button(button) != 0)
			default: // Other (normal) button
				// if js.Button(button) != 0 {
				// 	fmt.Printf("[default] input.joysticks[%v].Button(%v)=%v\n", joy, button, js.Button(button))
				// }
				return js.Button(button) != 0
			}
		} else { // check BUTTON only
			return js.Button(button) != 0
		}

	} else { // Check for Axis code, button is negatif (-1,-2,...,-10,-11,...)
		var axis int
		if button&1 == 0 {
			axis = (-button - 1) / 2
		} else {
			axis = -button / 2
		}
		if js.NumAxes() > axis {
			value := js.Axis(axis)
			if button&1 == 0 {
				return value > sys.controllerStickSensitivitySDL
			} else {
				return -value > sys.controllerStickSensitivitySDL
			}
		} else {
			return false
		}
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
		sdl.JoystickUpdate()
		joyS := sys.joystickConfig[in].Joy
		if joyS >= 0 {
			U = U || sys.joystickConfig[in].U() // Does not override keyboard
			D = D || sys.joystickConfig[in].D()
			L = L || sys.joystickConfig[in].L()
			R = R || sys.joystickConfig[in].R()
			a = a || sys.joystickConfig[in].a()
			b = b || sys.joystickConfig[in].b()
			c = c || sys.joystickConfig[in].c()
			x = x || sys.joystickConfig[in].x()
			y = y || sys.joystickConfig[in].y()
			z = z || sys.joystickConfig[in].z()
			s = s || sys.joystickConfig[in].s()
			d = d || sys.joystickConfig[in].d()
			w = w || sys.joystickConfig[in].w()
			m = m || sys.joystickConfig[in].m()
		}
	}
	// Button assist is checked locally so the sent inputs are already processed
	if sys.inputButtonAssist {
		a, b, c, x, y, z, s, d, w = ir.ButtonAssistCheck(a, b, c, x, y, z, s, d, w)
	}
	return U, D, L, R, a, b, c, x, y, z, s, d, w, m
}

func checkAxisForDpad(joy int, axes *[]int16, base int) string {
	var s string
	if (*axes)[0] > sys.controllerStickSensitivitySDL { // right
		s = strconv.Itoa(2 + base)
		fmt.Printf("[input_sdl.go][checkAxisForDpad] AXIS for DPAD RIGHT joy=%v s: %v\n", joy, s)
	} else if -(*axes)[0] > sys.controllerStickSensitivitySDL { // left
		s = strconv.Itoa(1 + base)
		fmt.Printf("[input_sdl.go][checkAxisForDpad] AXIS for DPAD LEFT joy=%v s: %v\n", joy, s)
	}
	if (*axes)[1] > sys.controllerStickSensitivitySDL { // down
		s = strconv.Itoa(3 + base)
		fmt.Printf("[input_sdl.go][checkAxisForDpad] AXIS for DPAD DOWN joy=%v s: %v\n", joy, s)
	} else if -(*axes)[1] > sys.controllerStickSensitivitySDL { // up
		s = strconv.Itoa(base)
		fmt.Printf("[input_sdl.go][checkAxisForDpad] AXIS  for DPAD UP joy=%v s: %v\n", joy, s)
	}
	return s
}

func checkAxisForTrigger(joy int, axes *[]int16) string {
	var s string = ""
	for i := range *axes {
		if (*axes)[i] < -sys.controllerStickSensitivitySDL {
			name := input.GetJoystickName(joy) + "." + runtime.GOOS + "." + runtime.GOARCH + ".sdl"
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
				fmt.Printf("[input_sdl.go][checkAxisForTrigger] 1.AXIS joy=%v i=%v s:%v axes[i]=%v\n", joy, i, s, (*axes)[i])
				break
			}
		} else if (*axes)[i] > sys.controllerStickSensitivitySDL {
			s = strconv.Itoa(-i*2 - 2)
			fmt.Printf("[input_sdl.go][checkAxisForTrigger] 2.AXIS joy=%v i=%v s:%v axes[i]=%v\n", joy, i, s, (*axes)[i])
			break
		}
	}
	return s
}
