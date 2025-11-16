//========================================================================
// GLFW 3.5 Linux - www.glfw.org
//------------------------------------------------------------------------
// Copyright (c) 2002-2006 Marcus Geelnard
// Copyright (c) 2006-2017 Camilla Löwy <elmindreda@glfw.org>
//
// This software is provided 'as-is', without any express or implied
// warranty. In no event will the authors be held liable for any damages
// arising from the use of this software.
//
// Permission is granted to anyone to use this software for any purpose,
// including commercial applications, and to alter it and redistribute it
// freely, subject to the following restrictions:
//
// 1. The origin of this software must not be misrepresented; you must not
//    claim that you wrote the original software. If you use this software
//    in a product, an acknowledgment in the product documentation would
//    be appreciated but is not required.
//
// 2. Altered source versions must be plainly marked as such, and must not
//    be misrepresented as being the original software.
//
// 3. This notice may not be removed or altered from any source
//    distribution.
//
//========================================================================
/*
glfwGetJoystickAxes
glfwGetJoystickButtons
glfwGetJoystickHats
glfwGetJoystickName
glfwGetJoystickGUID
glfwJoystickIsGamepad
glfwGetGamepadName
glfwGetGamepadState
    _glfwPollJoystickLinux
        pollAbsState(js);
        handleKeyEvent(js, e.code, e.value);
        handleAbsEvent(js, e.code, e.value);
*/

#include "internal.h"
#if defined(GLFW_BUILD_LINUX_JOYSTICK)
#include <sys/types.h>
#include <sys/stat.h>
#include <sys/inotify.h>
#include <fcntl.h>
#include <errno.h>
#include <dirent.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <ctype.h>
#include <linux/input.h>
#include <linux/joystick.h>

#define BITS_PER_LONG        (sizeof(unsigned long) * 8)
#define NBITS(x)             ((((x)-1) / BITS_PER_LONG) + 1)
#define EVDEV_OFF(x)         ((x) % BITS_PER_LONG)
#define EVDEV_LONG(x)        ((x) / BITS_PER_LONG)
#define test_bit(bit, array) ((array[EVDEV_LONG(bit)] >> EVDEV_OFF(bit)) & 1)

#ifndef SYN_DROPPED // < v2.6.39 kernel headers
// Workaround for CentOS-6, which is supported till 2020-11-30, but still on v2.6.32
#define SYN_DROPPED 3
#endif

static uint16_t crc16_for_byte(uint8_t r) {
    uint16_t crc = 0;
    int i;
    for (i = 0; i < 8; ++i) {
        crc = ((crc ^ r) & 1 ? 0xA001 : 0) ^ crc >> 1;
        r >>= 1;
    }
    return crc;
}

static uint16_t crc16(uint16_t crc, const void* data, size_t len) {
    // As an optimization we can precalculate a 256 entry table for each byte
    size_t i;
    for (i = 0; i < len; ++i) {
        crc = crc16_for_byte((uint8_t) crc ^ ((const uint8_t*) data)[i]) ^ crc >> 8;
    }
    return crc;
}

static bool GuessIfAxesAreDigitalHat(struct input_absinfo* absinfo_x, struct input_absinfo* absinfo_y) {
    /* A "hat" is assumed to be a digital input with at most 9 possible states
     * (3 per axis: negative/zero/positive), as opposed to a true "axis" which
     * can report a continuous range of possible values. Unfortunately the Linux
     * joystick interface makes no distinction between digital hat axes and any
     * other continuous analog axis, so we have to guess. */

     // If both axes are missing, they're not anything.
    if (!absinfo_x && !absinfo_y) {
        return false;
    }

    // If both axes have ranges constrained between -1 and 1, they're definitely digital.
    if ((!absinfo_x || (absinfo_x->minimum == -1 && absinfo_x->maximum == 1)) && (!absinfo_y || (absinfo_y->minimum == -1 && absinfo_y->maximum == 1))) {
        return true;
    }

    // If both axes lack fuzz, flat, and resolution values, they're probably digital.
    if ((!absinfo_x || (!absinfo_x->fuzz && !absinfo_x->flat && !absinfo_x->resolution)) && (!absinfo_y || (!absinfo_y->fuzz && !absinfo_y->flat && !absinfo_y->resolution))) {
        return true;
    }

    // Otherwise, treat them as analog.
    return false;
}

// Apply an EV_KEY event to the specified joystick
//
static void handleKeyEvent(_GLFWjoystick* js, int code, int value) {
    // debug_printf("[GLFW][linux_joystick.c] Key event: code %d, value %d, mapped to %d\n", code, value, js->linjs.keyMap[code]);
    _glfwInputJoystickButton(js,
        js->linjs.keyMap[code],
        value ? GLFW_PRESS : GLFW_RELEASE);
}

// Apply an EV_ABS event to the specified joystick
//
static void handleAbsEvent(_GLFWjoystick* js, int code, int value) {
    const int index = js->linjs.absMap[code];
    if (index < 0)
        return;

    if (js->linjs.hasAbs[code]) { // Check for axis
        const struct input_absinfo* info = &js->linjs.absInfo[code];
        float normalized = value;
        // debug_printf("[GLFW] Axis event: code %d, value %d, mapped to %d\n", code, value, index);

        const int range = info->maximum - info->minimum;
        if (range) {
            // Normalize to 0.0 -> 1.0
            normalized = (normalized - info->minimum) / range;
            // Normalize to -1.0 -> 1.0
            normalized = normalized * 2.0f - 1.0f;
        }

        _glfwInputJoystickAxis(js, index, normalized);
    } else if (code >= ABS_HAT0X && code <= ABS_HAT3Y) { // Check for hat
        static const char stateMap[3][3] =
        {
            { GLFW_HAT_CENTERED, GLFW_HAT_UP,       GLFW_HAT_DOWN },
            { GLFW_HAT_LEFT,     GLFW_HAT_LEFT_UP,  GLFW_HAT_LEFT_DOWN },
            { GLFW_HAT_RIGHT,    GLFW_HAT_RIGHT_UP, GLFW_HAT_RIGHT_DOWN },
        };

        const int hat = (code - ABS_HAT0X) / 2;
        const int axis = (code - ABS_HAT0X) % 2;
        int* state = js->linjs.hats[hat];

        // NOTE: Looking at several input drivers, it seems all hat events use
        //       -1 for left / up, 0 for centered and 1 for right / down
        if (value == 0)
            state[axis] = 0;
        else if (value < 0)
            state[axis] = 1;
        else if (value > 0)
            state[axis] = 2;

        // debug_printf("[GLFW] Hat event: code %d, state:%d value:%d, mapped to %d\n", code, stateMap[state[0]][state[1]], value, index);
        _glfwInputJoystickHat(js, index, stateMap[state[0]][state[1]]);
    }
}

// Poll state of absolute axes
//
static void pollAbsState(_GLFWjoystick* js) {
    for (int code = 0; code < ABS_CNT; code++) {
        if (js->linjs.absMap[code] < 0)
            continue;

        struct input_absinfo* info = &js->linjs.absInfo[code];

        if (ioctl(js->linjs.fd, EVIOCGABS(code), info) < 0)
            continue;

        handleAbsEvent(js, code, info->value);
    }
}

// Attempt to open the specified joystick device
//
static GLFWbool openJoystickDevice(const char* path) {
    for (int jid = 0; jid <= GLFW_JOYSTICK_LAST; jid++) {
        if (!_glfw.joysticks[jid].connected)
            continue;
        if (strcmp(_glfw.joysticks[jid].linjs.path, path) == 0)
            return GLFW_FALSE;
    }

    _GLFWjoystickLinux linjs = { 0 };
    linjs.fd = open(path, O_RDONLY | O_NONBLOCK | O_CLOEXEC);
    if (linjs.fd == -1)
        return GLFW_FALSE;

    unsigned long keybit[NBITS(KEY_MAX)] = { 0 };
    unsigned long absbit[NBITS(ABS_MAX)] = { 0 };
    struct input_id id;

    if ((ioctl(linjs.fd, EVIOCGBIT(EV_KEY, sizeof(keybit)), keybit) < 0) ||
        (ioctl(linjs.fd, EVIOCGBIT(EV_ABS, sizeof(absbit)), absbit) < 0) ||
        (ioctl(linjs.fd, EVIOCGID, &id) < 0)) {
        _glfwInputError(GLFW_PLATFORM_ERROR,
            "Linux: Failed to query input device: %s",
            strerror(errno));
        close(linjs.fd);
        return GLFW_FALSE;
    }

    // Ensure this device supports the events expected of a joystick
    if (!(test_bit(BTN_TRIGGER, keybit) ||
        test_bit(BTN_A, keybit) ||
        test_bit(BTN_1, keybit) ||
        test_bit(ABS_RX, absbit) ||
        test_bit(ABS_RY, absbit) ||
        test_bit(ABS_RZ, absbit) ||
        test_bit(ABS_THROTTLE, absbit) ||
        test_bit(ABS_RUDDER, absbit) ||
        test_bit(ABS_WHEEL, absbit) ||
        test_bit(ABS_GAS, absbit) ||
        test_bit(ABS_BRAKE, absbit))) {
        close(linjs.fd);
        return GLFW_FALSE;
    }

    char name[256] = "";

    if (ioctl(linjs.fd, JSIOCGNAME(sizeof(name)), name) <= 0) {
        if (ioctl(linjs.fd, EVIOCGNAME(sizeof(name)), name) < 0) {
            close(linjs.fd);
            return GLFW_FALSE;
        }
    }

    char guid[64] = "";
    uint16_t crc = 0;
    crc = crc16(crc, name, strlen(name));
    int axisCount = 0, buttonCount = 0, hatCount = 0;

#ifdef _GLFW_SDL2
    // Generate a joystick GUID that matches the SDL2
    if (_glfw.platform.platformID == GLFW_PLATFORM_SDL2) {
        SDL_Joystick* joy = SDL_NumJoysticks() ? SDL_JoystickOpen(0) : NULL; // TODO: make sure path if referenced by joystick id 0
        if (joy) {
            debug_printf("[GLFW] Build joystick guid with SDL2 function\n");
            SDL_JoystickGetGUIDString(SDL_JoystickGetGUID(joy), guid, sizeof(guid));
            SDL_JoystickClose(joy);
        }
    } else {
#endif
        debug_printf("[GLFW] Build joystick guid with GLFW function\n");
        if (id.vendor && id.product && id.version) {
            sprintf(guid, "%02x%02x%02x%02x%02x%02x0000%02x%02x0000%02x%02x0000",
                id.bustype & 0xff, id.bustype >> 8,
                crc & 0xff, crc >> 8,
                id.vendor & 0xff, id.vendor >> 8,
                id.product & 0xff, id.product >> 8,
                id.version & 0xff, id.version >> 8);
        } else {
            sprintf(guid, "%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x00",
                id.bustype & 0xff, id.bustype >> 8,
                crc & 0xff, crc >> 8,
                name[0], name[1], name[2], name[3],
                name[4], name[5], name[6], name[7],
                name[8], name[9], name[10]);
        }
#ifdef _GLFW_SDL2
    }
#endif

#ifdef _GLFW_SDL2
    if (strcmp(_glfw.sdl2.cfw, "knulli") == 0 || strcmp(_glfw.sdl2.cfw, "muOS") == 0) {    // Knulli spesific code
        for (int code = 0; code < KEY_MAX; code++) {
            if (test_bit(code, keybit)) {
                debug_printf("[GLFW] Joystick has button: %d mapped to %d\n", code, buttonCount);
                linjs.keyMap[code] = buttonCount;
                buttonCount++;
            }
        }
    } else {
#endif
        for (int code = BTN_JOYSTICK; code < KEY_MAX; code++) {
            if (!test_bit(code, keybit))
                continue;
            debug_printf("[GLFW] Joystick has button: %d mapped to %d\n", code, buttonCount);
            linjs.keyMap[code] = buttonCount;
            buttonCount++;
        }

        for (int code = 0; code < BTN_JOYSTICK; code++) {
            if (test_bit(code, keybit)) {
                debug_printf("[GLFW] Joystick has button: %d mapped to %d\n", code, buttonCount);
                linjs.keyMap[code] = buttonCount;
                buttonCount++;
            }
        }
#ifdef _GLFW_SDL2        
    }
#endif
    for (int i = ABS_HAT0X; i <= ABS_HAT3Y; i += 2) {
        int hat_x = -1;
        int hat_y = -1;
        // const int hat_index = (i - ABS_HAT0X) / 2;
        linjs.absMap[i] = -1;
        linjs.absMap[i + 1] = -1;
        struct input_absinfo absinfo_x;
        struct input_absinfo absinfo_y;
        if (test_bit(i, absbit)) {
            hat_x = ioctl(linjs.fd, EVIOCGABS(i), &absinfo_x);
        }
        if (test_bit(i + 1, absbit)) {
            hat_y = ioctl(linjs.fd, EVIOCGABS(i + 1), &absinfo_y);
        }
        if (GuessIfAxesAreDigitalHat((hat_x < 0 ? (void*) 0 : &absinfo_x),
            (hat_y < 0 ? (void*) 0 : &absinfo_y))) {

            debug_printf("[GLFW] Joystick has digital hat: %d mapped to %d\n", i, hatCount);
            linjs.absMap[i] = hatCount;
            linjs.absMap[i + 1] = hatCount;
            ++hatCount;
        }
    }

    for (int i = 0; i < ABS_MAX; ++i) {
        linjs.hasAbs[i] = GLFW_FALSE;
        // Skip digital hats
        if (i >= ABS_HAT0X && i <= ABS_HAT3Y && linjs.absMap[i] >= 0) {
            continue;
        }
        if (test_bit(i, absbit)) {
            // struct input_absinfo absinfo;
            if (ioctl(linjs.fd, EVIOCGABS(i), &linjs.absInfo[i]) < 0) {
                continue;
            }
            debug_printf("[GLFW] Joystick has absolute axis: %d mapped to %d\n", i, axisCount);
            linjs.absMap[i] = axisCount;
            linjs.hasAbs[i] = GLFW_TRUE;
            ++axisCount;
        }
    }

    //debug count of buttons, hat and axis
    debug_printf("[GLFW] Joystick '%s' connected\n", name);
    debug_printf("[GLFW]  - GUID: %s\n", guid);
    debug_printf("[GLFW]  - Axes: %d\n", axisCount);
    debug_printf("[GLFW]  - Buttons: %d\n", buttonCount);
    debug_printf("[GLFW]  - Hats: %d\n", hatCount);

    _GLFWjoystick* js =
        _glfwAllocJoystick(name, guid, axisCount, buttonCount, hatCount);
    if (!js) {
        close(linjs.fd);
        return GLFW_FALSE;
    }

    strncpy(linjs.path, path, sizeof(linjs.path) - 1);
    memcpy(&js->linjs, &linjs, sizeof(linjs));

    pollAbsState(js);

    _glfwInputJoystick(js, GLFW_CONNECTED);
    return GLFW_TRUE;
}

#undef test_bit

// Frees all resources associated with the specified joystick
//
static void closeJoystick(_GLFWjoystick* js) {
    _glfwInputJoystick(js, GLFW_DISCONNECTED);
    close(js->linjs.fd);
    _glfwFreeJoystick(js);
}

// Lexically compare joysticks by name; used by qsort
//
static int compareJoysticks(const void* fp, const void* sp) {
    const _GLFWjoystick* fj = fp;
    const _GLFWjoystick* sj = sp;
    return strcmp(fj->linjs.path, sj->linjs.path);
}


//////////////////////////////////////////////////////////////////////////
//////                       GLFW internal API                      //////
//////////////////////////////////////////////////////////////////////////

void _glfwDetectJoystickConnectionLinux(void) {
    if (_glfw.linjs.inotify <= 0)
        return;

    ssize_t offset = 0;
    char buffer[16384];
    const ssize_t size = read(_glfw.linjs.inotify, buffer, sizeof(buffer));

    while (size > offset) {
        regmatch_t match;
        const struct inotify_event* e = (struct inotify_event*) (buffer + offset);

        offset += sizeof(struct inotify_event) + e->len;

        if (regexec(&_glfw.linjs.regex, e->name, 1, &match, 0) != 0)
            continue;

        char path[PATH_MAX];
        snprintf(path, sizeof(path), "/dev/input/%s", e->name);

        if (e->mask & (IN_CREATE | IN_ATTRIB))
            openJoystickDevice(path);
        else if (e->mask & IN_DELETE) {
            for (int jid = 0; jid <= GLFW_JOYSTICK_LAST; jid++) {
                if (strcmp(_glfw.joysticks[jid].linjs.path, path) == 0) {
                    closeJoystick(_glfw.joysticks + jid);
                    break;
                }
            }
        }
    }
}


//////////////////////////////////////////////////////////////////////////
//////                       GLFW platform API                      //////
//////////////////////////////////////////////////////////////////////////

GLFWbool _glfwInitJoysticksLinux(void) {
    const char* dirname = "/dev/input";

    _glfw.linjs.inotify = inotify_init1(IN_NONBLOCK | IN_CLOEXEC);
    if (_glfw.linjs.inotify > 0) {
        // HACK: Register for IN_ATTRIB to get notified when udev is done
        //       This works well in practice but the true way is libudev

        _glfw.linjs.watch = inotify_add_watch(_glfw.linjs.inotify,
            dirname,
            IN_CREATE | IN_ATTRIB | IN_DELETE);
    }

    // Continue without device connection notifications if inotify fails

    _glfw.linjs.regexCompiled = (regcomp(&_glfw.linjs.regex, "^event[0-9]\\+$", 0) == 0);
    if (!_glfw.linjs.regexCompiled) {
        _glfwInputError(GLFW_PLATFORM_ERROR, "Linux: Failed to compile regex");
        return GLFW_FALSE;
    }

    int count = 0;

    DIR* dir = opendir(dirname);
    if (dir) {
        struct dirent* entry;

        while ((entry = readdir(dir))) {
            regmatch_t match;

            if (regexec(&_glfw.linjs.regex, entry->d_name, 1, &match, 0) != 0)
                continue;

            char path[PATH_MAX];

            snprintf(path, sizeof(path), "%s/%s", dirname, entry->d_name);

            if (openJoystickDevice(path))
                count++;
        }

        closedir(dir);
    }

    // Continue with no joysticks if enumeration fails

    qsort(_glfw.joysticks, count, sizeof(_GLFWjoystick), compareJoysticks);
    return GLFW_TRUE;
}

void _glfwTerminateJoysticksLinux(void) {
    for (int jid = 0; jid <= GLFW_JOYSTICK_LAST; jid++) {
        _GLFWjoystick* js = _glfw.joysticks + jid;
        if (js->connected)
            closeJoystick(js);
    }

    if (_glfw.linjs.inotify > 0) {
        if (_glfw.linjs.watch > 0)
            inotify_rm_watch(_glfw.linjs.inotify, _glfw.linjs.watch);

        close(_glfw.linjs.inotify);
    }

    if (_glfw.linjs.regexCompiled)
        regfree(&_glfw.linjs.regex);
}

GLFWbool _glfwPollJoystickLinux(_GLFWjoystick* js, int mode) {
    // Read all queued events (non-blocking)
    for (;;) {
        struct input_event e;

        errno = 0;
        if (read(js->linjs.fd, &e, sizeof(e)) < 0) {
            // Reset the joystick slot if the device was disconnected
            if (errno == ENODEV)
                closeJoystick(js);

            break;
        }

        if (e.type == EV_SYN) {
            if (e.code == SYN_DROPPED)
                _glfw.linjs.dropped = GLFW_TRUE;
            else if (e.code == SYN_REPORT) {
                _glfw.linjs.dropped = GLFW_FALSE;
                pollAbsState(js);
            }
        }

        if (_glfw.linjs.dropped)
            continue;

        if (e.type == EV_KEY)
            handleKeyEvent(js, e.code, e.value);
        else if (e.type == EV_ABS)
            handleAbsEvent(js, e.code, e.value);
    }

    return js->connected;
}

const char* _glfwGetMappingNameLinux(void) {
    return "Linux";
}

void _glfwUpdateGamepadGUIDLinux(char* guid) {}

#endif // GLFW_BUILD_LINUX_JOYSTICK
