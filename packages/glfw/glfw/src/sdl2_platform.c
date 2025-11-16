//========================================================================
// GLFW 3.5 SDL2 Backend
//------------------------------------------------------------------------
// Copyright (c) 2025 Dhani Novan <dhani.novan@gmail.com>
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

#include "internal.h"
#include <linux/input-event-codes.h>
#if defined(_GLFW_SDL2)

const char *osGetValue(const char *name) {
    static char value[256] = {0};
    FILE *file;
    char line[512];

    // Clear previous value
    value[0] = '\0';

    file = fopen("/etc/os-release", "r");
    if (file == NULL) {
        return NULL;
    }

    while (fgets(line, sizeof(line), file)) {
        // Skip comments and empty lines
        if (line[0] == '#' || line[0] == '\n' || line[0] == '\0') {
            continue;
        }

        // Remove trailing newline
        line[strcspn(line, "\n")] = '\0';

        // Find the equals sign
        char *equals = strchr(line, '=');
        if (equals == NULL) {
            continue;
        }

        // Split key and value
        *equals = '\0';
        char *key = line;
        char *val = equals + 1;

        // Remove quotes from value if present
        if (val[0] == '"' && val[strlen(val)-1] == '"') {
            val[strlen(val)-1] = '\0';
            val++;
        }

        // Check if this is the key we're looking for
        if (strcmp(key, name) == 0) {
            strncpy(value, val, sizeof(value) - 1);
            value[sizeof(value) - 1] = '\0'; // Ensure null termination
            break;
        }
    }

    fclose(file);

    // Return NULL if value is empty, otherwise return the value
    return (value[0] == '\0') ? NULL : value;
}

int _glfwInitSDL2(void) {
    // These must be set before any failure checks
    // _glfw.sdl2.window = NULL;
    // _glfw.sdl2.context = NULL;

    const char* value;
    if (getenv("CFW_NAME")) {
        snprintf(_glfw.sdl2.cfw, sizeof(_glfw.sdl2.cfw), "%s", getenv("CFW_NAME"));
    } else if ((value = osGetValue("OS_NAME"))) {
        snprintf(_glfw.sdl2.cfw, sizeof(_glfw.sdl2.cfw), "%s", value);
    } else if ((value = osGetValue("NAME"))){
        snprintf(_glfw.sdl2.cfw, sizeof(_glfw.sdl2.cfw), "%s", value);
    } else {
        snprintf(_glfw.sdl2.cfw, sizeof(_glfw.sdl2.cfw), "Unknown OS");
    }
    debug_printf("Running on \"%s\" OS\n", _glfw.sdl2.cfw);

    _glfw.sdl2.sdl.Init = (PFN_SDL_Init) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_Init");
    _glfw.sdl2.sdl.Quit = (PFN_SDL_Quit) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_Quit");
    _glfw.sdl2.sdl.Log = (PFN_SDL_Log) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_Log");
    _glfw.sdl2.sdl.CreateWindow = (PFN_SDL_CreateWindow) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_CreateWindow");
    _glfw.sdl2.sdl.GetError = (PFN_SDL_GetError) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_GetError");
    _glfw.sdl2.sdl.DestroyWindow = (PFN_SDL_DestroyWindow) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_DestroyWindow");
    _glfw.sdl2.sdl.GL_CreateContext = (PFN_SDL_GL_CreateContext) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_GL_CreateContext");
    _glfw.sdl2.sdl.GL_MakeCurrent = (PFN_SDL_GL_MakeCurrent) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_GL_MakeCurrent");
    _glfw.sdl2.sdl.GL_SetAttribute = (PFN_SDL_GL_SetAttribute) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_GL_SetAttribute");
    _glfw.sdl2.sdl.GL_GetAttribute = (PFN_SDL_GL_GetAttribute) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_GL_GetAttribute");
    _glfw.sdl2.sdl.GL_DeleteContext = (PFN_SDL_GL_DeleteContext) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_GL_DeleteContext");
    _glfw.sdl2.sdl.GL_SwapWindow = (PFN_SDL_GL_SwapWindow) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_GL_SwapWindow");
    _glfw.sdl2.sdl.PollEvent = (PFN_SDL_PollEvent) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_PollEvent");
    _glfw.sdl2.sdl.WaitEventTimeout = (PFN_SDL_WaitEventTimeout) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_WaitEventTimeout");
    _glfw.sdl2.sdl.GetTicks = (PFN_SDL_GetTicks) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_GetTicks");
    _glfw.sdl2.sdl.Delay = (PFN_SDL_Delay) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_Delay");
    _glfw.sdl2.sdl.SetWindowTitle = (PFN_SDL_SetWindowTitle) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_SetWindowTitle");
    _glfw.sdl2.sdl.GL_SetSwapInterval = (PFN_SDL_GL_SetSwapInterval) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_GL_SetSwapInterval");
    _glfw.sdl2.sdl.GL_GetProcAddress = (PFN_SDL_GL_GetProcAddress) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_GL_GetProcAddress");
    _glfw.sdl2.sdl.InitSubSystem = (PFN_SDL_InitSubSystem) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_InitSubSystem");
    _glfw.sdl2.sdl.QuitSubSystem = (PFN_SDL_QuitSubSystem) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_QuitSubSystem");
    _glfw.sdl2.sdl.JoystickUpdate = (PFN_SDL_JoystickUpdate) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_JoystickUpdate");
    _glfw.sdl2.sdl.JoystickGetAttached = (PFN_SDL_JoystickGetAttached) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_JoystickGetAttached");
    _glfw.sdl2.sdl.JoystickName = (PFN_SDL_JoystickName) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_JoystickName");
    _glfw.sdl2.sdl.JoystickNumAxes = (PFN_SDL_JoystickNumAxes) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_JoystickNumAxes");
    _glfw.sdl2.sdl.JoystickGetAxis = (PFN_SDL_JoystickGetAxis) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_JoystickGetAxis");
    _glfw.sdl2.sdl.JoystickNumButtons = (PFN_SDL_JoystickNumButtons) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_JoystickNumButtons");
    _glfw.sdl2.sdl.JoystickGetButton = (PFN_SDL_JoystickGetButton) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_JoystickGetButton");
    _glfw.sdl2.sdl.JoystickNumHats = (PFN_SDL_JoystickNumHats) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_JoystickNumHats");
    _glfw.sdl2.sdl.JoystickGetHat = (PFN_SDL_JoystickGetHat) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_JoystickGetHat");
    _glfw.sdl2.sdl.JoystickEventState = (PFN_SDL_JoystickEventState) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_JoystickEventState");
    _glfw.sdl2.sdl.GL_GetDrawableSize = (PFN_SDL_GL_GetDrawableSize) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_GL_GetDrawableSize");
    _glfw.sdl2.sdl.GetWindowSize = (PFN_SDL_GetWindowSize) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_GetWindowSize");
    _glfw.sdl2.sdl.GetVersion = (PFN_SDL_GetVersion) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_GetVersion");
    _glfw.sdl2.sdl.JoystickGetGUIDString = (PFN_SDL_JoystickGetGUIDString) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_JoystickGetGUIDString");
    _glfw.sdl2.sdl.JoystickGetGUID = (PFN_SDL_JoystickGetGUID) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_JoystickGetGUID");
    _glfw.sdl2.sdl.JoystickOpen = (PFN_SDL_JoystickOpen) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_JoystickOpen");
    _glfw.sdl2.sdl.JoystickClose = (PFN_SDL_JoystickClose) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_JoystickClose");
    _glfw.sdl2.sdl.JoystickInstanceID = (PFN_SDL_JoystickInstanceID) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_JoystickInstanceID");
    _glfw.sdl2.sdl.JoystickGetDeviceIndexFromInstanceID = (PFN_SDL_JoystickGetDeviceIndexFromInstanceID) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_JoystickGetDeviceIndexFromInstanceID");
    _glfw.sdl2.sdl.JoystickAxisEventCodeById = (PFN_SDL_JoystickAxisEventCodeById) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_JoystickAxisEventCodeById");
    _glfw.sdl2.sdl.JoystickButtonEventCodeById = (PFN_SDL_JoystickButtonEventCodeById) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_JoystickButtonEventCodeById");
    _glfw.sdl2.sdl.JoystickHatEventCodeById = (PFN_SDL_JoystickHatEventCodeById) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_JoystickHatEventCodeById");
    _glfw.sdl2.sdl.NumJoysticks = (PFN_SDL_NumJoysticks) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_NumJoysticks");
    _glfw.sdl2.sdl.SetWindowPosition = (PFN_SDL_SetWindowPosition) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_SetWindowPosition");
    _glfw.sdl2.sdl.GetWindowPosition = (PFN_SDL_GetWindowPosition) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_GetWindowPosition");
    _glfw.sdl2.sdl.SetWindowSize = (PFN_SDL_SetWindowSize) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_SetWindowSize");
    _glfw.sdl2.sdl.GetMouseState = (PFN_SDL_GetMouseState) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_GetMouseState");
    _glfw.sdl2.sdl.WarpMouseInWindow = (PFN_SDL_WarpMouseInWindow) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_WarpMouseInWindow");
    _glfw.sdl2.sdl.SetWindowResizable = (PFN_SDL_SetWindowResizable) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_SetWindowResizable");
    _glfw.sdl2.sdl.SetWindowBordered = (PFN_SDL_SetWindowBordered) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_SetWindowBordered");
    _glfw.sdl2.sdl.SetWindowAlwaysOnTop = (PFN_SDL_SetWindowAlwaysOnTop) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_SetWindowAlwaysOnTop");
    _glfw.sdl2.sdl.SetWindowMouseGrab = (PFN_SDL_SetWindowMouseGrab) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_SetWindowMouseGrab");
    _glfw.sdl2.sdl.ShowCursor = (PFN_SDL_ShowCursor) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_ShowCursor");
    _glfw.sdl2.sdl.CreateSystemCursor = (PFN_SDL_CreateSystemCursor) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_CreateSystemCursor");
    _glfw.sdl2.sdl.SetCursor = (PFN_SDL_SetCursor) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_SetCursor");
    _glfw.sdl2.sdl.SetRelativeMouseMode = (PFN_SDL_SetRelativeMouseMode) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_SetRelativeMouseMode");
    _glfw.sdl2.sdl.GetNumVideoDisplays = (PFN_SDL_GetNumVideoDisplays) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_GetNumVideoDisplays");
    _glfw.sdl2.sdl.GetDisplayBounds = (PFN_SDL_GetDisplayBounds) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_GetDisplayBounds");
    _glfw.sdl2.sdl.GetDisplayName = (PFN_SDL_GetDisplayName) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_GetDisplayName");
    _glfw.sdl2.sdl.GetNumDisplayModes = (PFN_SDL_GetNumDisplayModes) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_GetNumDisplayModes");
    _glfw.sdl2.sdl.GetDisplayMode = (PFN_SDL_GetDisplayMode) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_GetDisplayMode");
    _glfw.sdl2.sdl.GetPixelFormatName = (PFN_SDL_GetPixelFormatName) _glfwPlatformGetModuleSymbol(_glfw.sdl2.sdl.handle, "SDL_GetPixelFormatName");

    if (!_glfw.sdl2.sdl.Init ||
        !_glfw.sdl2.sdl.Quit ||
        !_glfw.sdl2.sdl.Log ||
        !_glfw.sdl2.sdl.CreateWindow ||
        !_glfw.sdl2.sdl.GetError ||
        !_glfw.sdl2.sdl.DestroyWindow ||
        !_glfw.sdl2.sdl.GL_CreateContext ||
        !_glfw.sdl2.sdl.GL_MakeCurrent ||
        !_glfw.sdl2.sdl.GL_SetAttribute ||
        !_glfw.sdl2.sdl.GL_GetAttribute ||
        !_glfw.sdl2.sdl.GL_DeleteContext ||
        !_glfw.sdl2.sdl.GL_SwapWindow ||
        !_glfw.sdl2.sdl.PollEvent ||
        !_glfw.sdl2.sdl.WaitEventTimeout ||
        !_glfw.sdl2.sdl.GetTicks ||
        !_glfw.sdl2.sdl.Delay ||
        !_glfw.sdl2.sdl.SetWindowTitle ||
        !_glfw.sdl2.sdl.GL_SetSwapInterval ||
        !_glfw.sdl2.sdl.GL_GetProcAddress ||
        !_glfw.sdl2.sdl.InitSubSystem ||
        !_glfw.sdl2.sdl.QuitSubSystem ||
        !_glfw.sdl2.sdl.JoystickUpdate ||
        !_glfw.sdl2.sdl.JoystickGetAttached ||
        !_glfw.sdl2.sdl.JoystickName ||
        !_glfw.sdl2.sdl.JoystickNumAxes ||
        !_glfw.sdl2.sdl.JoystickGetAxis ||
        !_glfw.sdl2.sdl.JoystickNumButtons ||
        !_glfw.sdl2.sdl.JoystickGetButton ||
        !_glfw.sdl2.sdl.JoystickNumHats ||
        !_glfw.sdl2.sdl.JoystickGetHat ||
        !_glfw.sdl2.sdl.JoystickEventState ||
        !_glfw.sdl2.sdl.GL_GetDrawableSize ||
        !_glfw.sdl2.sdl.GetWindowSize ||
        !_glfw.sdl2.sdl.GetVersion ||
        !_glfw.sdl2.sdl.JoystickGetGUIDString ||
        !_glfw.sdl2.sdl.JoystickGetGUID ||
        !_glfw.sdl2.sdl.JoystickOpen ||
        !_glfw.sdl2.sdl.JoystickClose ||
        !_glfw.sdl2.sdl.JoystickInstanceID ||
        !_glfw.sdl2.sdl.NumJoysticks ||
        !_glfw.sdl2.sdl.SetWindowPosition ||
        !_glfw.sdl2.sdl.GetWindowPosition ||
        !_glfw.sdl2.sdl.SetWindowSize ||
        !_glfw.sdl2.sdl.GetMouseState ||
        !_glfw.sdl2.sdl.WarpMouseInWindow ||
        !_glfw.sdl2.sdl.SetWindowResizable ||
        !_glfw.sdl2.sdl.SetWindowBordered ||
        !_glfw.sdl2.sdl.SetWindowAlwaysOnTop ||
    	!_glfw.sdl2.sdl.SetWindowMouseGrab ||
     	!_glfw.sdl2.sdl.ShowCursor ||
        !_glfw.sdl2.sdl.CreateSystemCursor ||
        !_glfw.sdl2.sdl.SetCursor ||
        !_glfw.sdl2.sdl.SetRelativeMouseMode ||
        !_glfw.sdl2.sdl.GetNumVideoDisplays ||
        !_glfw.sdl2.sdl.GetDisplayBounds ||
        !_glfw.sdl2.sdl.GetDisplayName
    )
    {
        _glfwInputError(GLFW_PLATFORM_ERROR,
            "SDL2: Failed to load SDL functions entry point");
        return GLFW_FALSE;
    }

    if (SDL_Init(SDL_INIT_VIDEO | SDL_INIT_AUDIO | SDL_INIT_JOYSTICK) != 0) {
        _glfwInputError(GLFW_PLATFORM_ERROR,
            "SDL2: Failed to initialize SDL: %s", SDL_GetError());
        return GLFW_FALSE;
    }
    SDL_VERSION(&_glfw.sdl2.compiled);
    SDL_GetVersion(&_glfw.sdl2.linked);
    debug_printf("GLFW: Compiled with SDL version %d.%d.%d\n",
                 _glfw.sdl2.compiled.major,
                 _glfw.sdl2.compiled.minor,
                 _glfw.sdl2.compiled.patch);
    debug_printf("GLFW: Linked   with SDL version %d.%d.%d\n",
                 _glfw.sdl2.linked.major,
                 _glfw.sdl2.linked.minor,
                 _glfw.sdl2.linked.patch);

    int display_count = SDL_GetNumVideoDisplays();
    for (int i = 0; i < display_count; i++) {
        const char* display_name = SDL_GetDisplayName(i);
        SDL_Rect bounds;
        if (SDL_GetDisplayBounds(i, &bounds) == 0) {
            _GLFWmonitor* monitor = _glfwAllocMonitor(SDL_GetDisplayName(i), bounds.w, bounds.h);

            int modes_count = SDL_GetNumDisplayModes(i);
            if (modes_count < 1) {
                printf("  No display modes available: %s\n", SDL_GetError());
            } else {
            	monitor->modeCount = modes_count;
             	monitor->modes = _glfw_calloc(modes_count, sizeof(GLFWvidmode));
                for (int j = 0; j < modes_count; j++) {
                    SDL_DisplayMode mode;
                    if (SDL_GetDisplayMode(i, j, &mode) == 0) {
                        monitor->modes[j].width = mode.w;
                        monitor->modes[j].height = mode.h;
                        monitor->modes[j].refreshRate = mode.refresh_rate;
                        monitor->modes[j].redBits = SDL_BITSPERPIXEL(mode.format);
                        monitor->modes[j].greenBits = SDL_BITSPERPIXEL(mode.format);
                        monitor->modes[j].blueBits = SDL_BITSPERPIXEL(mode.format);
                    }
                }
            }

            _glfwInputMonitor(monitor, GLFW_CONNECTED, _GLFW_INSERT_LAST);
        } else {
            printf("  Could not get bounds: %s\n", SDL_GetError());
        }
    }

    return GLFW_TRUE;
}

void _glfwTerminateSDL2(void) {
    // Clean up any remaining monitors, no need because glfw.Terminate handles it
    // for (int i = 0; i < _glfw.monitorCount; i++) {
    //     _glfwFreeMonitor(_glfw.monitors[i]);
    // }
    SDL_Quit();
    // Free the library
    if (_glfw.sdl2.sdl.handle) {
        _glfwPlatformFreeModule(_glfw.sdl2.sdl.handle);
        _glfw.sdl2.sdl.handle = NULL;
    }
}

static void makeContextCurrentSDL2(_GLFWwindow* window) {
    if (window) {
        if (SDL_GL_MakeCurrent(window->sdl2.window, window->context.sdl2.handle) != 0) {
            _glfwInputError(GLFW_PLATFORM_ERROR,
                "SDL2: Failed to make context current: %s", SDL_GetError());
            return;
        }
    } else {
        if (SDL_GL_MakeCurrent(NULL, NULL) != 0) {
            _glfwInputError(GLFW_PLATFORM_ERROR,
                "SDL2: Failed to release current context: %s", SDL_GetError());
            return;
        }
    }
    _glfwPlatformSetTls(&_glfw.contextSlot, window);
}

// swapBuffersSDL2 implementation
static void swapBuffersSDL2(_GLFWwindow* window) {
#ifdef DEBUG
#ifdef __linux__
    static unsigned int frame = 0;
#endif
#endif
    SDL_GL_SwapWindow(window->sdl2.window);
#ifdef DEBUG
#ifdef __linux__
    int64_t cur_time = get_time_ns();
    if (cur_time > (_glfw.report_time + NSEC_PER_SEC)) {
        debug_printf("Render %u fps\n", frame);
        _glfw.report_time = cur_time;
        frame = 0;
    }
    frame++;
#endif
#endif
}

// swapIntervalSDL2 implementation
static void swapIntervalSDL2(int interval) {
    if (SDL_GL_SetSwapInterval(interval) != 0) {
        _glfwInputError(GLFW_PLATFORM_ERROR,
            "SDL2: Failed to set swap interval: %s", SDL_GetError());
    }
}

// extensionSupportedSDL2 implementation
static int extensionSupportedSDL2(const char* extension) {
    // SDL does not provide a way to query OpenGL extensions directly
    // You would typically use an OpenGL function like glGetString(GL_EXTENSIONS)
    // or glGetStringi for OpenGL 3.0 and above.
    // Here, we return 0 (not supported) as a placeholder.
    return 0; // Placeholder
}

// getProcAddressSDL2 implementation
static GLFWglproc getProcAddressSDL2(const char* procname) {
    return (GLFWglproc) SDL_GL_GetProcAddress(procname);
}

// destroyContextSDL2 implementation
static void destroyContextSDL2(_GLFWwindow* window) {
    if (window->context.sdl2.handle) {
        SDL_GL_DeleteContext(window->context.sdl2.handle);
        window->context.sdl2.handle = NULL;
    }
}

// _glfwCreateWindowSDL2 implementation
GLFWbool _glfwCreateWindowSDL2(_GLFWwindow* window,
    const _GLFWwndconfig* wndconfig,
    const _GLFWctxconfig* ctxconfig,
    const _GLFWfbconfig* fbconfig) {

    // Set OpenGL attributes
    if (ctxconfig->client == GLFW_OPENGL_API) {
        SDL_GL_SetAttribute(SDL_GL_CONTEXT_MAJOR_VERSION, ctxconfig->major);
        SDL_GL_SetAttribute(SDL_GL_CONTEXT_MINOR_VERSION, ctxconfig->minor);
        if (ctxconfig->profile == GLFW_OPENGL_CORE_PROFILE)
            SDL_GL_SetAttribute(SDL_GL_CONTEXT_PROFILE_MASK, SDL_GL_CONTEXT_PROFILE_CORE);
        else
            SDL_GL_SetAttribute(SDL_GL_CONTEXT_PROFILE_MASK, SDL_GL_CONTEXT_PROFILE_COMPATIBILITY);
        SDL_GL_SetAttribute(SDL_GL_CONTEXT_FLAGS, ctxconfig->forward ? SDL_GL_CONTEXT_FORWARD_COMPATIBLE_FLAG : 0);
        window->context.client = GLFW_OPENGL_API;
        window->context.major = ctxconfig->major;
        window->context.minor = ctxconfig->minor;
    } else if (ctxconfig->client == GLFW_OPENGL_ES_API) {
        SDL_GL_SetAttribute(SDL_GL_CONTEXT_MAJOR_VERSION, ctxconfig->major);
        SDL_GL_SetAttribute(SDL_GL_CONTEXT_MINOR_VERSION, ctxconfig->minor);
        SDL_GL_SetAttribute(SDL_GL_CONTEXT_PROFILE_MASK, SDL_GL_CONTEXT_PROFILE_ES);
        window->context.client = GLFW_OPENGL_ES_API;
        window->context.major = ctxconfig->major;
        window->context.minor = ctxconfig->minor;
    } else {
        _glfwInputError(GLFW_API_UNAVAILABLE, "SDL2: Unsupported context client API");
        return GLFW_FALSE;
    }

    const char* title = window->title ? window->title : "GLFW Window";
    Uint32 flags = SDL_WINDOW_OPENGL;
    if (wndconfig->resizable)
        flags |= SDL_WINDOW_RESIZABLE;
    if (wndconfig->decorated == GLFW_FALSE)
        flags |= SDL_WINDOW_BORDERLESS;
    if (wndconfig->visible == GLFW_FALSE)
        flags |= SDL_WINDOW_HIDDEN;

    window->sdl2.window = SDL_CreateWindow(title, SDL_WINDOWPOS_UNDEFINED, SDL_WINDOWPOS_UNDEFINED,
        wndconfig->width, wndconfig->height, flags);
    if (!window->sdl2.window) {
        _glfwInputError(GLFW_PLATFORM_ERROR, "SDL2: Failed to create window: %s", SDL_GetError());
        return GLFW_FALSE;
    }

    window->context.sdl2.handle = SDL_GL_CreateContext(window->sdl2.window);
    if (!window->context.sdl2.handle) {
        _glfwInputError(GLFW_PLATFORM_ERROR, "SDL2: Failed to create OpenGL context: %s", SDL_GetError());
        SDL_DestroyWindow(window->sdl2.window);
        window->sdl2.window = NULL;
        return GLFW_FALSE;
    }

    window->context.makeCurrent = makeContextCurrentSDL2;
    window->context.swapBuffers = swapBuffersSDL2;
    window->context.swapInterval = swapIntervalSDL2;
    window->context.extensionSupported = extensionSupportedSDL2;
    window->context.getProcAddress = getProcAddressSDL2;
    window->context.destroy = destroyContextSDL2;

    return GLFW_TRUE; // Placeholder
}

// _glfwDestroyWindowSDL2 implementation
void _glfwDestroyWindowSDL2(_GLFWwindow* window) {
    if (window->context.sdl2.handle) {
        SDL_GL_DeleteContext(window->context.sdl2.handle);
        window->context.sdl2.handle = NULL;
    }

    if (window->sdl2.window) {
        SDL_DestroyWindow(window->sdl2.window);
        window->sdl2.window = NULL;
    }
}

// _glfwPollEventsSDL2 implementation
void _glfwPollEventsSDL2(void) {
#if defined(GLFW_BUILD_LINUX_JOYSTICK)
    if (_glfw.joysticksInitialized)
        _glfwDetectJoystickConnectionLinux();
#endif
}

// _glfwGetFramebufferSizeSDL2 implementation
void _glfwGetFramebufferSizeSDL2(_GLFWwindow* window, int* width, int* height) {
    int w, h;
    SDL_GL_GetDrawableSize(window->sdl2.window, &w, &h);
    if (width)
        *width = w;
    if (height)
        *height = h;
}

// _glfwGetWindowSizeSDL2 implementation
void _glfwGetWindowSizeSDL2(_GLFWwindow* window, int* width, int* height) {
    SDL_GetWindowSize(window->sdl2.window, width, height);
}

// _glfwSetWindowIconSDL2 implementation
void _glfwSetWindowIconSDL2(_GLFWwindow* window, int count, const GLFWimage* images) {
    // SDL2 does not support setting window icon after creation
    // This function is a no-op
    (void) window;
    (void) count;
    (void) images;
}

// _glfwSetWindowPosSDL2 implementation
void _glfwSetWindowPosSDL2(_GLFWwindow* window, int xpos, int ypos) {
    SDL_SetWindowPosition(window->sdl2.window, xpos, ypos);
}

// _glfwGetWindowPosSDL2 implementation
void _glfwGetWindowPosSDL2(_GLFWwindow* window, int* xpos, int* ypos) {
    SDL_GetWindowPosition(window->sdl2.window, xpos, ypos);
}

// _glfwSetWindowSizeSDL2 implementation
void _glfwSetWindowSizeSDL2(_GLFWwindow* window, int width, int height) {
    SDL_SetWindowSize(window->sdl2.window, width, height);
}

// _glfwGetCursorPosSDL2 implementation
void _glfwGetCursorPosSDL2(_GLFWwindow* window, double* xpos, double* ypos) {
    int x, y;
    SDL_GetMouseState(&x, &y);
    if (xpos)
        *xpos = (double)x;
    if (ypos)
        *ypos = (double)y;
}

// _glfwSetCursorPosSDL2 implementation
void _glfwSetCursorPosSDL2(_GLFWwindow* window, double xpos, double ypos) {
    SDL_WarpMouseInWindow(window->sdl2.window, (int)xpos, (int)ypos);
}

// _glfwSetCursorModeSDL2 implementation
void _glfwSetCursorModeSDL2(_GLFWwindow* window, int mode) {
    // Uint32 sdlMode;
    // switch (mode) {
    //     case GLFW_CURSOR_NORMAL:
    //         sdlMode = SDL_SYSTEM_CURSOR_ARROW;
    //         SDL_ShowCursor(SDL_ENABLE);
    //         break;
    //     case GLFW_CURSOR_HIDDEN:
    //         SDL_ShowCursor(SDL_DISABLE);
    //         return;
    //     case GLFW_CURSOR_DISABLED:
    //         SDL_ShowCursor(SDL_DISABLE);
    //         SDL_SetRelativeMouseMode(SDL_TRUE);
    //         return;
    //     default:
    //         return;
    // }
    // SDL_ShowCursor(SDL_ENABLE);
    // static SDL_Cursor* cursor = NULL;
    // if (cursor) {
    //     SDL_FreeCursor(cursor);
    //     cursor = NULL;
    // }
    /* Use the dynamically loaded SDL function pointer instead of calling
       SDL_CreateSystemCursor directly to avoid mismatches with headers that
       may declare a different return type. */
    // if (_glfw.sdl2.sdl.CreateSystemCursor) {
    //     cursor = _glfw.sdl2.sdl.CreateSystemCursor(sdlMode);
    // } else {
    //     cursor = NULL;
    // }
    // if (cursor) {
    //     if (_glfw.sdl2.sdl.SetCursor)
    //         _glfw.sdl2.sdl.SetCursor(cursor);
    // }
}

// _glfwSetRawMouseMotionSDL2 implementation
void _glfwSetRawMouseMotionSDL2(_GLFWwindow* window, GLFWbool enabled) {
    SDL_SetRelativeMouseMode(enabled ? SDL_TRUE : SDL_FALSE);
}

// _glfwRawMouseMotionSupportedSDL2 implementation
GLFWbool _glfwRawMouseMotionSupportedSDL2(void) {
    return GLFW_TRUE;
}

// _glfwSetWindowResizableSDL2 implementation
void _glfwSetWindowResizableSDL2(_GLFWwindow* window, GLFWbool enabled) {
    SDL_SetWindowResizable(window->sdl2.window, enabled ? SDL_TRUE : SDL_FALSE);
}

// _glfwSetWindowDecoratedSDL2 implementation
void _glfwSetWindowDecoratedSDL2(_GLFWwindow* window, GLFWbool enabled) {
    SDL_SetWindowBordered(window->sdl2.window, enabled ? SDL_TRUE : SDL_FALSE);
}

// _glfwSetWindowFloatingSDL2 implementation
void _glfwSetWindowFloatingSDL2(_GLFWwindow* window, GLFWbool enabled) {
    SDL_SetWindowAlwaysOnTop(window->sdl2.window, enabled ? SDL_TRUE : SDL_FALSE);
}

void _glfwSetWindowMousePassthroughSDL2(_GLFWwindow* window, GLFWbool enabled) {
    SDL_SetWindowMouseGrab(window->sdl2.window, enabled ? SDL_FALSE : SDL_TRUE);
}

GLFWbool _glfwGetVideoModeSDL2(_GLFWmonitor* monitor, GLFWvidmode* mode) {
    mode->width = monitor->modes[0].width;
    mode->height = monitor->modes[0].height;
    mode->redBits = monitor->modes[0].redBits;
    mode->greenBits = monitor->modes[0].greenBits;
    mode->blueBits = monitor->modes[0].blueBits;
    mode->refreshRate = monitor->modes[0].refreshRate;
    return GLFW_TRUE;
}

GLFWvidmode* _glfwGetVideoModesSDL2(_GLFWmonitor* monitor, int* found) {
    *found = 1;
    return _glfw.monitors[0]->modes;
}

GLFWbool _glfwConnectSDL2(int platformID, _GLFWplatform* platform) {
    const _GLFWplatform sdl2 =
    {
        .platformID = GLFW_PLATFORM_SDL2,
        .init = _glfwInitSDL2,
        .terminate = _glfwTerminateSDL2,
        .getCursorPos = _glfwGetCursorPosSDL2,
        .setCursorPos = _glfwSetCursorPosSDL2,
        .setCursorMode = _glfwSetCursorModeSDL2,
        .setRawMouseMotion = _glfwSetRawMouseMotionSDL2,
        .rawMouseMotionSupported = _glfwRawMouseMotionSupportedSDL2,
        // .createCursor = _glfwCreateCursorSDL2,
        // .createStandardCursor = _glfwCreateStandardCursorSDL2,
        // .destroyCursor = _glfwDestroyCursorSDL2,
        // .setCursor = _glfwSetCursorSDL2,
        // .getScancodeName = _glfwGetScancodeNameSDL2,
        // .getKeyScancode = _glfwGetKeyScancodeSDL2,
        // .setClipboardString = _glfwSetClipboardStringSDL2,
        // .getClipboardString = _glfwGetClipboardStringSDL2,
#if defined(GLFW_BUILD_LINUX_JOYSTICK)
        .initJoysticks = _glfwInitJoysticksLinux,
        .terminateJoysticks = _glfwTerminateJoysticksLinux,
        .pollJoystick = _glfwPollJoystickLinux,
        .getMappingName = _glfwGetMappingNameLinux,
        .updateGamepadGUID = _glfwUpdateGamepadGUIDLinux,
#else
        .initJoysticks = _glfwInitJoysticksNull,
        .terminateJoysticks = _glfwTerminateJoysticksNull,
        .pollJoystick = _glfwPollJoystickNull,
        .getMappingName = _glfwGetMappingNameNull,
        .updateGamepadGUID = _glfwUpdateGamepadGUIDNull,
#endif
        // .freeMonitor = _glfwFreeMonitorSDL2,
        // .getMonitorPos = _glfwGetMonitorPosSDL2,
        // .getMonitorContentScale = _glfwGetMonitorContentScaleSDL2,
        // .getMonitorWorkarea = _glfwGetMonitorWorkareaSDL2,
        .getVideoModes = _glfwGetVideoModesSDL2,
        .getVideoMode = _glfwGetVideoModeSDL2,
        // .getGammaRamp = _glfwGetGammaRampSDL2,
        // .setGammaRamp = _glfwSetGammaRampSDL2,
        .createWindow = _glfwCreateWindowSDL2,
        .destroyWindow = _glfwDestroyWindowSDL2,
        // .setWindowTitle = _glfwSetWindowTitleSDL2,
        .setWindowIcon = _glfwSetWindowIconSDL2,
        .getWindowPos = _glfwGetWindowPosSDL2,
        .setWindowPos = _glfwSetWindowPosSDL2,
        .getWindowSize = _glfwGetWindowSizeSDL2,
        .setWindowSize = _glfwSetWindowSizeSDL2,
        // .setWindowSizeLimits = _glfwSetWindowSizeLimitsSDL2,
        // .setWindowAspectRatio = _glfwSetWindowAspectRatioSDL2,
        .getFramebufferSize = _glfwGetFramebufferSizeSDL2,
        // .getWindowFrameSize = _glfwGetWindowFrameSizeSDL2,
        // .getWindowContentScale = _glfwGetWindowContentScaleSDL2,
        // .iconifyWindow = _glfwIconifyWindowSDL2,
        // .restoreWindow = _glfwRestoreWindowSDL2,
        // .maximizeWindow = _glfwMaximizeWindowSDL2,
        // .showWindow = _glfwShowWindowSDL2,
        // .hideWindow = _glfwHideWindowSDL2,
        // .requestWindowAttention = _glfwRequestWindowAttentionSDL2,
        // .focusWindow = _glfwFocusWindowSDL2,
        // .setWindowMonitor = _glfwSetWindowMonitorSDL2,
        // .windowFocused = _glfwWindowFocusedSDL2,
        // .windowIconified = _glfwWindowIconifiedSDL2,
        // .windowVisible = _glfwWindowVisibleSDL2,
        // .windowMaximized = _glfwWindowMaximizedSDL2,
        // .windowHovered = _glfwWindowHoveredSDL2,
        // .framebufferTransparent = _glfwFramebufferTransparentSDL2,
        // .getWindowOpacity = _glfwGetWindowOpacitySDL2,
        .setWindowResizable = _glfwSetWindowResizableSDL2,
        .setWindowDecorated = _glfwSetWindowDecoratedSDL2,
        .setWindowFloating = _glfwSetWindowFloatingSDL2,
        // .setWindowOpacity = _glfwSetWindowOpacitySDL2,
        .setWindowMousePassthrough = _glfwSetWindowMousePassthroughSDL2,
        .pollEvents = _glfwPollEventsSDL2,
        // .waitEvents = _glfwWaitEventsSDL2,
        // .waitEventsTimeout = _glfwWaitEventsTimeoutSDL2,
        // .postEmptyEvent = _glfwPostEmptyEventSDL2,
        // .getEGLPlatform = _glfwGetEGLPlatformSDL2,
        // .getEGLNativeDisplay = _glfwGetEGLNativeDisplaySDL2,
        // .getEGLNativeWindow = _glfwGetEGLNativeWindowSDL2,
        // .getRequiredInstanceExtensions = _glfwGetRequiredInstanceExtensionsSDL2,
        // .getPhysicalDevicePresentationSupport = _glfwGetPhysicalDevicePresentationSupportSDL2,
        // .createWindowSurface = _glfwCreateWindowSurfaceSDL2
    };

    void* module = _glfwPlatformLoadModule("libSDL2-2.0.so.0");
    if (!module) {
        if (platformID == GLFW_PLATFORM_SDL2) {
            _glfwInputError(GLFW_PLATFORM_ERROR,
                "SDL2: Failed to load libSDL2-2.0.so.0");
        }

        return GLFW_FALSE;
    }

    _glfw.sdl2.sdl.handle = module;
    *platform = sdl2;
    return GLFW_TRUE;
}

#endif