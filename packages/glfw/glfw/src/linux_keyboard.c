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
glfwPollEvents
    _glfwPollEventsWayland
        void handleEvents(double* timeout)
            _glfwDetectJoystickConnectionLinux();
            _glfwDetectKeyboardConnectionLinux();
            _glfwPollPOSIX(struct pollfd* fds, nfds_t count, double* timeout)
    _glfwPollEventsKMSDRM
        void handleEvents(double* timeout)  // FIX src/kmsdrm_window.c
            _glfwDetectJoystickConnectionLinux();
            _glfwDetectKeyboardConnectionLinux();
            _glfwPollPOSIX(struct pollfd* fds, nfds_t count, double* timeout)
            _glfwPollKeyboardLinux();
                handleKeyEvent(js, e.code, e.value);
*/

#include "internal.h"

#if defined(GLFW_BUILD_LINUX_KEYBOARD)

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

#ifndef SYN_DROPPED // < v2.6.39 kernel headers
// Workaround for CentOS-6, which is supported till 2020-11-30, but still on v2.6.32
#define SYN_DROPPED 3
#endif

#define isBitSet(bit, arr) (arr[(bit) / 8] & (1 << ((bit) % 8)))

static int linux_translateKey(uint32_t scancode) {
    if (scancode < sizeof(_glfw.kmsdrm.keycodes) / sizeof(_glfw.kmsdrm.keycodes[0]))
        return _glfw.kmsdrm.keycodes[scancode];

    return GLFW_KEY_UNKNOWN;
}

// Return true if the specified device is a keyboard, store its name and open a file descriptor to it
// Return falsae if the device is not a keyboard
int isKeyboardDevice(/*in*/ const char* devicePath, /*out*/ char* deviceName, /*in */size_t nameSize, int *fd) {
    *fd = -1;
    *fd = open(devicePath, O_RDONLY | O_NONBLOCK | O_CLOEXEC);
    if (*fd < 0) {
        return 0;
    }

    struct input_id deviceInfo;
    if (ioctl(*fd, EVIOCGID, &deviceInfo) < 0) {
        close(*fd);
        *fd = -1;
        return 0;
    }

    if (ioctl(*fd, EVIOCGNAME(nameSize), deviceName) < 0) {
        strncpy(deviceName, "Unknown", nameSize - 1);
        deviceName[nameSize - 1] = '\0';
    }

    unsigned long evbit[EV_MAX / sizeof(unsigned long)];
    if (ioctl(*fd, EVIOCGBIT(0, sizeof(evbit)), evbit) < 0) {
        close(*fd);
        *fd = -1;
        return 0;
    }

    unsigned long keybit[KEY_MAX / sizeof(unsigned long)];
    if (ioctl(*fd, EVIOCGBIT(EV_KEY, sizeof(keybit)), keybit) < 0) {
        close(*fd);
        *fd = -1;
        return 0;
    }

    // debug_printf("Open input device (%s) fd=%d\n", deviceName, *fd);

    // Check if the device supports EV_KEY events and has keys typically found on keyboards
    return (evbit[EV_KEY / (8 * sizeof(unsigned long))] & (1 << (EV_KEY % (8 * sizeof(unsigned long))))) &&
           (keybit[KEY_A / (8 * sizeof(unsigned long))] & (1 << (KEY_A % (8 * sizeof(unsigned long)))));
}

// Attempt to open the specified keyboard device
//
static GLFWbool openKeyboardDevice(const char* path) {
    char name[256] = "";

    if (!isKeyboardDevice(path, name, sizeof(name), &_glfw.kmsdrm.keyboard_fd))
        return GLFW_FALSE;

    char evBits[(EV_CNT + 7) / 8] = { 0 };
    char keyBits[(KEY_CNT + 7) / 8] = { 0 };
    char absBits[(ABS_CNT + 7) / 8] = { 0 };
    struct input_id id;

    if (ioctl(_glfw.kmsdrm.keyboard_fd, EVIOCGBIT(0, sizeof(evBits)), evBits) < 0 ||
        ioctl(_glfw.kmsdrm.keyboard_fd, EVIOCGBIT(EV_KEY, sizeof(keyBits)), keyBits) < 0 ||
        ioctl(_glfw.kmsdrm.keyboard_fd, EVIOCGBIT(EV_ABS, sizeof(absBits)), absBits) < 0 ||
        ioctl(_glfw.kmsdrm.keyboard_fd, EVIOCGID, &id) < 0) {
        _glfwInputError(GLFW_PLATFORM_ERROR,
            "Linux: Failed to query input device: %s",
            strerror(errno));
        close(_glfw.kmsdrm.keyboard_fd);
        return GLFW_FALSE;
    }

    char guid[33] = "";
    // Generate a keyboard GUID that matches the SDL 2.0.5+ one
    if (id.vendor && id.product && id.version) {
        sprintf(guid, "%02x%02x0000%02x%02x0000%02x%02x0000%02x%02x0000",
            id.bustype & 0xff, id.bustype >> 8,
            id.vendor & 0xff, id.vendor >> 8,
            id.product & 0xff, id.product >> 8,
            id.version & 0xff, id.version >> 8);
    } else {
        sprintf(guid, "%02x%02x0000%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x00",
            id.bustype & 0xff, id.bustype >> 8,
            name[0], name[1], name[2], name[3],
            name[4], name[5], name[6], name[7],
            name[8], name[9], name[10]);
    }

    int axisCount = 0, buttonCount = 0, hatCount = 0;

    for (int code = BTN_MISC; code < KEY_CNT; code++) {
        if (!isBitSet(code, keyBits))
            continue;
        buttonCount++;
    }

    for (int code = 0; code < ABS_CNT; code++) {
        if (!isBitSet(code, absBits))
            continue;

        if (code >= ABS_HAT0X && code <= ABS_HAT3Y) {
            hatCount++;
            // Skip the Y axis
            code++;
        } else {
            if (ioctl(_glfw.kmsdrm.keyboard_fd, EVIOCGABS(code), NULL) < 0)
                continue;
            axisCount++;
        }
    }
    debug_printf("openKeyboardDevice: %s fd=%d \"%s\" [OK]\n\t%d keys\n\t%d hats\n\t%d axes\n", path, _glfw.kmsdrm.keyboard_fd, name, buttonCount, hatCount, axisCount);
    return GLFW_TRUE;
}

// Frees all resources associated with the specified keyboard
//
static void closeKeyboard() {
    if (_glfw.kmsdrm.keyboard_fd > 0)
        close(_glfw.kmsdrm.keyboard_fd);
}

//////////////////////////////////////////////////////////////////////////
//////                       GLFW platform API                      //////
//////////////////////////////////////////////////////////////////////////

GLFWbool _glfwInitKeyboardsLinux(void) {
    const char* dirname = "/dev/input";
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
            if (openKeyboardDevice(path)) {
                count++;
                break;
            }
        }
        closedir(dir);
    }
    if (count == 0) {
        debug_puts("There is NO input devices");
    } else {
        debug_printf("Found %d input devices\n", count);
    }

    return GLFW_TRUE;
}

void _glfwTerminateKeyboardsLinux(void) {
    closeKeyboard();

    if (_glfw.linjs.inotify > 0) {
        if (_glfw.linjs.watch > 0)
            inotify_rm_watch(_glfw.linjs.inotify, _glfw.linjs.watch);

        close(_glfw.linjs.inotify);
    }

    if (_glfw.linjs.regexCompiled)
        regfree(&_glfw.linjs.regex);
}

GLFWbool _glfwPollKeyboardLinux() {
    int ret = GLFW_TRUE;
    // Read all queued events (non-blocking)
    for (;;) {
        struct input_event e;
        errno = 0;
        if (read(_glfw.kmsdrm.keyboard_fd, &e, sizeof(e)) < 0) {
            // Reset the keyboard slot if the device was disconnected
            if (errno == ENODEV) {
                ret = GLFW_FALSE;
                closeKeyboard();
            }
            break;
        }
        if (e.type == EV_KEY) {
            // Handle keyboard event
            _glfwInputKey(_glfw.kmsdrm.window, linux_translateKey(e.code), e.code, e.value ? GLFW_PRESS : GLFW_RELEASE, 0);
        }
    }
    return ret;
}

#endif // GLFW_BUILD_LINUX_KEYBOARD

