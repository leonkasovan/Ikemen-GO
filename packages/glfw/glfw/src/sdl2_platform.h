
#include <SDL2/SDL.h>

typedef int (* PFN_SDL_Init)(Uint32 flags);
typedef void (* PFN_SDL_Quit)(void);
typedef void (* PFN_SDL_Log)(const char* fmt, ...);
typedef SDL_Window* (* PFN_SDL_CreateWindow)(const char* title, int x, int y, int w, int h, Uint32 flags);
typedef const char* (* PFN_SDL_GetError)(void);
typedef void (* PFN_SDL_DestroyWindow)(SDL_Window* window);
typedef SDL_GLContext (* PFN_SDL_GL_CreateContext)(SDL_Window* window);
typedef int (* PFN_SDL_GL_MakeCurrent)(SDL_Window* window, SDL_GLContext context);
typedef int (* PFN_SDL_GL_SetAttribute)(SDL_GLattr attr, int value);
typedef int (* PFN_SDL_GL_GetAttribute)(SDL_GLattr attr, int* value);
typedef void (* PFN_SDL_GL_DeleteContext)(SDL_GLContext context);
typedef void (* PFN_SDL_GL_SwapWindow)(SDL_Window* window);
typedef int (* PFN_SDL_PollEvent)(SDL_Event* event);
typedef int (* PFN_SDL_WaitEventTimeout)(SDL_Event* event, int timeout);
typedef Uint32 (* PFN_SDL_GetTicks)(void);
typedef void (* PFN_SDL_Delay)(Uint32 ms);
typedef void (* PFN_SDL_SetWindowTitle)(SDL_Window* window, const char* title);
typedef int (* PFN_SDL_GL_SetSwapInterval)(int interval);
typedef void* (* PFN_SDL_GL_GetProcAddress)(const char* proc);
typedef int (* PFN_SDL_InitSubSystem)(Uint32 flags);
typedef void (* PFN_SDL_QuitSubSystem)(Uint32 flags);
typedef void (* PFN_SDL_JoystickUpdate)(void);
typedef SDL_bool (* PFN_SDL_JoystickGetAttached)(SDL_Joystick* joystick);
typedef const char* (* PFN_SDL_JoystickName)(SDL_Joystick* joystick);
typedef int (* PFN_SDL_JoystickNumAxes)(SDL_Joystick* joystick);
typedef Sint16 (* PFN_SDL_JoystickGetAxis)(SDL_Joystick* joystick, int axis);
typedef int (* PFN_SDL_JoystickNumButtons)(SDL_Joystick* joystick);
typedef Uint8 (* PFN_SDL_JoystickGetButton)(SDL_Joystick* joystick, int button);
typedef int (* PFN_SDL_JoystickNumHats)(SDL_Joystick* joystick);
typedef Uint8 (* PFN_SDL_JoystickGetHat)(SDL_Joystick* joystick, int hat);
typedef int (* PFN_SDL_JoystickEventState)(int state);
typedef void (* PFN_SDL_GL_GetDrawableSize)(SDL_Window* window, int* w, int* h);
typedef void (* PFN_SDL_GetWindowSize)(SDL_Window* window, int* w, int* h);
typedef void (* PFN_SDL_GetVersion)(SDL_version* ver);
typedef void (* PFN_SDL_JoystickGetGUIDString)(SDL_JoystickGUID guid, char* pszGUID, int cbGUID);
typedef SDL_JoystickGUID (* PFN_SDL_JoystickGetGUID)(SDL_Joystick* joystick);
typedef SDL_Joystick* (* PFN_SDL_JoystickOpen)(int device_index);
typedef int (* PFN_SDL_JoystickClose)(SDL_Joystick* joystick);
typedef int (* PFN_SDL_JoystickInstanceID)(SDL_Joystick* joystick);
typedef int (* PFN_SDL_JoystickGetDeviceIndexFromInstanceID)(int instance_id);
typedef int (* PFN_SDL_JoystickAxisEventCodeById)(int device_instance_id, int axis); // optional
typedef int (* PFN_SDL_JoystickButtonEventCodeById)(int device_instance_id, int button); // optional
typedef int (* PFN_SDL_JoystickHatEventCodeById)(int device_instance_id, int hat); // optional
typedef int (* PFN_SDL_NumJoysticks)(void);

#define SDL_Init _glfw.sdl2.sdl.Init
#define SDL_Quit _glfw.sdl2.sdl.Quit
#define SDL_Log _glfw.sdl2.sdl.Log
#define SDL_CreateWindow _glfw.sdl2.sdl.CreateWindow
#define SDL_GetError _glfw.sdl2.sdl.GetError
#define SDL_DestroyWindow _glfw.sdl2.sdl.DestroyWindow
#define SDL_GL_CreateContext _glfw.sdl2.sdl.GL_CreateContext
#define SDL_GL_MakeCurrent _glfw.sdl2.sdl.GL_MakeCurrent
#define SDL_GL_SetAttribute _glfw.sdl2.sdl.GL_SetAttribute
#define SDL_GL_GetAttribute _glfw.sdl2.sdl.GL_GetAttribute
#define SDL_GL_DeleteContext _glfw.sdl2.sdl.GL_DeleteContext
#define SDL_GL_SwapWindow _glfw.sdl2.sdl.GL_SwapWindow
#define SDL_PollEvent _glfw.sdl2.sdl.PollEvent
#define SDL_WaitEventTimeout _glfw.sdl2.sdl.WaitEventTimeout
#define SDL_GetTicks _glfw.sdl2.sdl.GetTicks
#define SDL_Delay _glfw.sdl2.sdl.Delay
#define SDL_SetWindowTitle _glfw.sdl2.sdl.SetWindowTitle
#define SDL_GL_SetSwapInterval _glfw.sdl2.sdl.GL_SetSwapInterval
#define SDL_GL_GetProcAddress _glfw.sdl2.sdl.GL_GetProcAddress
#define SDL_InitSubSystem _glfw.sdl2.sdl.InitSubSystem
#define SDL_QuitSubSystem _glfw.sdl2.sdl.QuitSubSystem
#define SDL_JoystickUpdate _glfw.sdl2.sdl.JoystickUpdate
#define SDL_JoystickGetAttached _glfw.sdl2.sdl.JoystickGetAttached
#define SDL_JoystickName _glfw.sdl2.sdl.JoystickName
#define SDL_JoystickNumAxes _glfw.sdl2.sdl.JoystickNumAxes
#define SDL_JoystickGetAxis _glfw.sdl2.sdl.JoystickGetAxis
#define SDL_JoystickNumButtons _glfw.sdl2.sdl.JoystickNumButtons
#define SDL_JoystickGetButton _glfw.sdl2.sdl.JoystickGetButton
#define SDL_JoystickNumHats _glfw.sdl2.sdl.JoystickNumHats
#define SDL_JoystickGetHat _glfw.sdl2.sdl.JoystickGetHat
#define SDL_JoystickEventState _glfw.sdl2.sdl.JoystickEventState
#define SDL_GL_GetDrawableSize _glfw.sdl2.sdl.GL_GetDrawableSize
#define SDL_GetWindowSize _glfw.sdl2.sdl.GetWindowSize
#define SDL_GetVersion _glfw.sdl2.sdl.GetVersion
#define SDL_JoystickGetGUIDString _glfw.sdl2.sdl.JoystickGetGUIDString
#define SDL_JoystickGetGUID _glfw.sdl2.sdl.JoystickGetGUID
#define SDL_JoystickOpen _glfw.sdl2.sdl.JoystickOpen
#define SDL_JoystickClose _glfw.sdl2.sdl.JoystickClose
#define SDL_JoystickInstanceID _glfw.sdl2.sdl.JoystickInstanceID
#define SDL_JoystickGetDeviceIndexFromInstanceID _glfw.sdl2.sdl.JoystickGetDeviceIndexFromInstanceID
#define SDL_JoystickAxisEventCodeById _glfw.sdl2.sdl.JoystickAxisEventCodeById
#define SDL_JoystickButtonEventCodeById _glfw.sdl2.sdl.JoystickButtonEventCodeById
#define SDL_JoystickHatEventCodeById _glfw.sdl2.sdl.JoystickHatEventCodeById
#define SDL_NumJoysticks _glfw.sdl2.sdl.NumJoysticks

#define GLFW_SDL2_WINDOW_STATE         _GLFWwindowSDL2  sdl2;
#define GLFW_SDL2_LIBRARY_WINDOW_STATE _GLFWlibrarySDL2 sdl2;
// #define GLFW_SDL2_MONITOR_STATE        _GLFWmonitorSDL2 sdl2;
// #define GLFW_SDL2_CURSOR_STATE         _GLFWcursorSDL2  sdl2;

// SDL2-specific global data
//
typedef struct _GLFWlibrarySDL2 {
	// SDL_Window* window;
	// SDL_GLContext context;
	SDL_version compiled;
	SDL_version linked;
	char cfw[32];

	struct {
		void* handle;

		PFN_SDL_Init Init;
		PFN_SDL_Quit Quit;
		PFN_SDL_Log Log;
		PFN_SDL_CreateWindow CreateWindow;
		PFN_SDL_GetError GetError;
		PFN_SDL_DestroyWindow DestroyWindow;
		PFN_SDL_GL_CreateContext GL_CreateContext;
		PFN_SDL_GL_MakeCurrent GL_MakeCurrent;
		PFN_SDL_GL_SetAttribute GL_SetAttribute;
		PFN_SDL_GL_GetAttribute GL_GetAttribute;
		PFN_SDL_GL_DeleteContext GL_DeleteContext;
		PFN_SDL_GL_SwapWindow GL_SwapWindow;
		PFN_SDL_PollEvent PollEvent;
		PFN_SDL_WaitEventTimeout WaitEventTimeout;
		PFN_SDL_GetTicks GetTicks;
		PFN_SDL_Delay Delay;
		PFN_SDL_SetWindowTitle SetWindowTitle;
        PFN_SDL_GL_SetSwapInterval GL_SetSwapInterval;
        PFN_SDL_GL_GetProcAddress GL_GetProcAddress;
        PFN_SDL_InitSubSystem InitSubSystem;
        PFN_SDL_QuitSubSystem QuitSubSystem;
        PFN_SDL_JoystickUpdate JoystickUpdate;
        PFN_SDL_JoystickGetAttached JoystickGetAttached;
        PFN_SDL_JoystickName JoystickName;
        PFN_SDL_JoystickNumAxes JoystickNumAxes;
        PFN_SDL_JoystickGetAxis JoystickGetAxis;
        PFN_SDL_JoystickNumButtons JoystickNumButtons;
        PFN_SDL_JoystickGetButton JoystickGetButton;
        PFN_SDL_JoystickNumHats JoystickNumHats;
        PFN_SDL_JoystickGetHat JoystickGetHat;
        PFN_SDL_JoystickEventState JoystickEventState;
		PFN_SDL_GL_GetDrawableSize GL_GetDrawableSize;
		PFN_SDL_GetWindowSize GetWindowSize;
		PFN_SDL_GetVersion GetVersion;
		PFN_SDL_JoystickGetGUIDString JoystickGetGUIDString;
		PFN_SDL_JoystickGetGUID JoystickGetGUID;
		PFN_SDL_JoystickOpen JoystickOpen;
		PFN_SDL_JoystickClose JoystickClose;
		PFN_SDL_JoystickInstanceID JoystickInstanceID;
		PFN_SDL_JoystickGetDeviceIndexFromInstanceID JoystickGetDeviceIndexFromInstanceID;
		PFN_SDL_JoystickAxisEventCodeById JoystickAxisEventCodeById; // optional
		PFN_SDL_JoystickButtonEventCodeById JoystickButtonEventCodeById; // optional
		PFN_SDL_JoystickHatEventCodeById JoystickHatEventCodeById; // optional
		PFN_SDL_NumJoysticks NumJoysticks;
	} sdl;

} _GLFWlibrarySDL2;

// SDL2-specific per-window data
//
typedef struct _GLFWwindowSDL2 {
    SDL_Window* window;
} _GLFWwindowSDL2;

GLFWbool _glfwConnectSDL2(int platformID, _GLFWplatform* platform);
int _glfwInitSDL2(void);
void _glfwTerminateSDL2(void);
GLFWbool _glfwCreateWindowSDL2(_GLFWwindow* window,
                             const _GLFWwndconfig* wndconfig,
                             const _GLFWctxconfig* ctxconfig,
                             const _GLFWfbconfig* fbconfig);
void _glfwDestroyWindowSDL2(_GLFWwindow* window);