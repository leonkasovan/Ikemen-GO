//========================================================================
// KMSDRM - A platform for GLFW based on Kernel Mode Setting (KMS) and Direct Rendering Manager (DRM)
//------------------------------------------------------------------------
#include <xf86drm.h>
#include <xf86drmMode.h>
#include <gbm.h>
#include <libdrm/drm_fourcc.h>
#include <fcntl.h>
#include <errno.h>
#include <dirent.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <inttypes.h>
#include "posix_poll.h"

#define WEAK __attribute__((weak))
// #define GLFW_KMSDRM_WINDOW_STATE          _GLFWwindowKMSDRM kmsdrm;
// #define GLFW_KMSDRM_LIBRARY_WINDOW_STATE  _GLFWlibraryKMSDRM kmsdrm;
// #define GLFW_KMSDRM_MONITOR_STATE         _GLFWmonitorKMSDRM kmsdrm;
#define GLFW_KMSDRM_WINDOW_STATE
#define GLFW_KMSDRM_LIBRARY_WINDOW_STATE  _GLFWlibraryKMSDRM kmsdrm;
#define GLFW_KMSDRM_MONITOR_STATE

#define GLFW_KMSDRM_CONTEXT_STATE
#define GLFW_KMSDRM_CURSOR_STATE
#define GLFW_KMSDRM_LIBRARY_CONTEXT_STATE

#define MAX_DRM_DEVICES 8
#ifndef DRM_FORMAT_MOD_LINEAR
#define DRM_FORMAT_MOD_LINEAR 0
#endif

#ifndef DRM_FORMAT_MOD_INVALID
#define DRM_FORMAT_MOD_INVALID ((((__u64)0) << 56) | ((1ULL << 56) - 1))
#endif

#ifndef EGL_KHR_platform_gbm
#define EGL_KHR_platform_gbm 1
#define EGL_PLATFORM_GBM_KHR 0x31D7
#endif /* EGL_KHR_platform_gbm */

// #ifndef EGL_EXT_platform_base
// #define EGL_EXT_platform_base 1
// typedef EGLDisplay(EGLAPIENTRYP PFNEGLGETPLATFORMDISPLAYEXTPROC) (EGLenum platform, void* native_display, const EGLint* attrib_list);
// typedef EGLSurface(EGLAPIENTRYP PFNEGLCREATEPLATFORMWINDOWSURFACEEXTPROC) (EGLDisplay dpy, EGLConfig config, void* native_window, const EGLint* attrib_list);
// typedef EGLSurface(EGLAPIENTRYP PFNEGLCREATEPLATFORMPIXMAPSURFACEEXTPROC) (EGLDisplay dpy, EGLConfig config, void* native_pixmap, const EGLint* attrib_list);
// #ifdef EGL_EGLEXT_PROTOTYPES
// EGLAPI EGLDisplay EGLAPIENTRY eglGetPlatformDisplayEXT(EGLenum platform, void* native_display, const EGLint* attrib_list);
// EGLAPI EGLSurface EGLAPIENTRY eglCreatePlatformWindowSurfaceEXT(EGLDisplay dpy, EGLConfig config, void* native_window, const EGLint* attrib_list);
// EGLAPI EGLSurface EGLAPIENTRY eglCreatePlatformPixmapSurfaceEXT(EGLDisplay dpy, EGLConfig config, void* native_pixmap, const EGLint* attrib_list);
// #endif
// #endif /* EGL_EXT_platform_base */

#ifndef DRM_DISPLAY_MODE_LEN
#define DRM_DISPLAY_MODE_LEN 32
#endif

struct drm_props {
    uint32_t fb_id;
    uint32_t mode_id;
    uint32_t active;
    uint32_t crtc_id;
};

struct drm {
    int fd;
    int crtc_index;
    drmModeModeInfo* mode;
    uint32_t crtc_id;
    uint32_t connector_id;
    unsigned int count;
    bool nonblocking;
    bool atomic;
    bool async_flip;
    struct drm_props props;
};

struct drm_fb {
    struct gbm_bo* bo;
    uint32_t fb_id;
};

struct gbm {
    struct gbm_device* dev;
    struct gbm_surface* surface;
    uint32_t format;
    int width, height;
    struct gbm_bo* bo;
    struct drm_fb* fb;
};

struct egl {
    EGLDisplay display;
    EGLConfig config;
    EGLContext context;
    EGLSurface surface;
    // PFNEGLGETPLATFORMDISPLAYEXTPROC eglGetPlatformDisplayEXT;
    bool modifiers_supported;
};

// int init_egl(struct egl* egl, const struct gbm* gbm, int samples);
int create_program(const char* vs_src, const char* fs_src);
int link_program(unsigned program);
struct drm_fb* drm_fb_get_from_bo(struct gbm_bo* bo);

// KMS-specific global data
//
typedef struct _GLFWlibraryKMSDRM {
    struct gbm gbm;
    struct drm drm;

    const char* device;
    char mode_str[DRM_DISPLAY_MODE_LEN];
    uint32_t format;
    uint64_t modifier;
    int samples;
    int atomic;
    int gears;
    int offscreen;
    int connector_id;
    int opt;
    unsigned int len;
    unsigned int vrefresh;
    unsigned int count;
    bool surfaceless;
    bool nonblocking;
    int keyboard_fd;
    _GLFWwindow* window;
    short int keycodes[256];
} _GLFWlibraryKMSDRM;

// Functions for initializing, terminating, and managing the KMSDRM platform
GLFWbool _glfwConnectKMSDRM(int platformID, _GLFWplatform* platform);
int _glfwInitKMSDRM(void);
void _glfwTerminateKMSDRM(void);

// Monitor functions
void _glfwPollMonitorsKMSDRM(void);
GLFWbool _glfwDetectMonitorsKMSDRM(void);
void _glfwFreeMonitorKMSDRM(_GLFWmonitor* monitor);
void _glfwGetMonitorPosKMSDRM(_GLFWmonitor* monitor, int* xpos, int* ypos);
void _glfwGetMonitorContentScaleKMSDRM(_GLFWmonitor* monitor, float* xscale, float* yscale);
void _glfwGetMonitorWorkareaKMSDRM(_GLFWmonitor* monitor, int* xpos, int* ypos, int* width, int* height);
GLFWvidmode* _glfwGetVideoModesKMSDRM(_GLFWmonitor* monitor, int* found);
GLFWbool _glfwGetVideoModeKMSDRM(_GLFWmonitor* monitor, GLFWvidmode* mode);
GLFWbool _glfwGetGammaRampKMSDRM(_GLFWmonitor* monitor, GLFWgammaramp* ramp);
void _glfwSetGammaRampKMSDRM(_GLFWmonitor* monitor, const GLFWgammaramp* ramp);

// Window functions
GLFWbool _glfwCreateWindowKMSDRM(_GLFWwindow* window, const _GLFWwndconfig* wndconfig, const _GLFWctxconfig* ctxconfig, const _GLFWfbconfig* fbconfig);
void _glfwDestroyWindowKMSDRM(_GLFWwindow* window);
void _glfwSetWindowTitleKMSDRM(_GLFWwindow* window, const char* title);
void _glfwSetWindowIconKMSDRM(_GLFWwindow* window, int count, const GLFWimage* images);
void _glfwSetWindowMonitorKMSDRM(_GLFWwindow* window, _GLFWmonitor* monitor, int xpos, int ypos, int width, int height, int refreshRate);
void _glfwGetWindowPosKMSDRM(_GLFWwindow* window, int* xpos, int* ypos);
void _glfwSetWindowPosKMSDRM(_GLFWwindow* window, int xpos, int ypos);
void _glfwGetWindowSizeKMSDRM(_GLFWwindow* window, int* width, int* height);
void _glfwSetWindowSizeKMSDRM(_GLFWwindow* window, int width, int height);
void _glfwSetWindowSizeLimitsKMSDRM(_GLFWwindow* window, int minwidth, int minheight, int maxwidth, int maxheight);
void _glfwSetWindowAspectRatioKMSDRM(_GLFWwindow* window, int n, int d);
void _glfwGetFramebufferSizeKMSDRM(_GLFWwindow* window, int* width, int* height);
void _glfwGetWindowFrameSizeKMSDRM(_GLFWwindow* window, int* left, int* top, int* right, int* bottom);
void _glfwGetWindowContentScaleKMSDRM(_GLFWwindow* window, float* xscale, float* yscale);
void _glfwIconifyWindowKMSDRM(_GLFWwindow* window);
void _glfwRestoreWindowKMSDRM(_GLFWwindow* window);
void _glfwMaximizeWindowKMSDRM(_GLFWwindow* window);
GLFWbool _glfwWindowMaximizedKMSDRM(_GLFWwindow* window);
GLFWbool _glfwWindowHoveredKMSDRM(_GLFWwindow* window);
GLFWbool _glfwFramebufferTransparentKMSDRM(_GLFWwindow* window);
void _glfwSetWindowResizableKMSDRM(_GLFWwindow* window, GLFWbool enabled);
void _glfwSetWindowDecoratedKMSDRM(_GLFWwindow* window, GLFWbool enabled);
void _glfwSetWindowFloatingKMSDRM(_GLFWwindow* window, GLFWbool enabled);
void _glfwSetWindowMousePassthroughKMSDRM(_GLFWwindow* window, GLFWbool enabled);
float _glfwGetWindowOpacityKMSDRM(_GLFWwindow* window);
void _glfwSetWindowOpacityKMSDRM(_GLFWwindow* window, float opacity);
void _glfwSetRawMouseMotionKMSDRM(_GLFWwindow* window, GLFWbool enabled);
GLFWbool _glfwRawMouseMotionSupportedKMSDRM(void);
void _glfwShowWindowKMSDRM(_GLFWwindow* window);
void _glfwRequestWindowAttentionKMSDRM(_GLFWwindow* window);
void _glfwHideWindowKMSDRM(_GLFWwindow* window);
void _glfwFocusWindowKMSDRM(_GLFWwindow* window);
GLFWbool _glfwWindowFocusedKMSDRM(_GLFWwindow* window);
GLFWbool _glfwWindowIconifiedKMSDRM(_GLFWwindow* window);
GLFWbool _glfwWindowVisibleKMSDRM(_GLFWwindow* window);
void _glfwPollEventsKMSDRM(void);
void _glfwWaitEventsKMSDRM(void);
void _glfwWaitEventsTimeoutKMSDRM(double timeout);
void _glfwPostEmptyEventKMSDRM(void);
void _glfwGetCursorPosKMSDRM(_GLFWwindow* window, double* xpos, double* ypos);
void _glfwSetCursorPosKMSDRM(_GLFWwindow* window, double x, double y);
void _glfwSetCursorModeKMSDRM(_GLFWwindow* window, int mode);
GLFWbool _glfwCreateCursorKMSDRM(_GLFWcursor* cursor, const GLFWimage* image, int xhot, int yhot);
GLFWbool _glfwCreateStandardCursorKMSDRM(_GLFWcursor* cursor, int shape);
void _glfwDestroyCursorKMSDRM(_GLFWcursor* cursor);
void _glfwSetCursorKMSDRM(_GLFWwindow* window, _GLFWcursor* cursor);
void _glfwSetClipboardStringKMSDRM(const char* string);
const char* _glfwGetClipboardStringKMSDRM(void);
const char* _glfwGetScancodeNameKMSDRM(int scancode);
int _glfwGetKeyScancodeKMSDRM(int key);

EGLenum _glfwGetEGLPlatformKMSDRM(EGLint** attribs);
EGLNativeDisplayType _glfwGetEGLNativeDisplayKMSDRM(void);
EGLNativeWindowType _glfwGetEGLNativeWindowKMSDRM(_GLFWwindow* window);

void _glfwGetRequiredInstanceExtensionsKMSDRM(char** extensions);
GLFWbool _glfwGetPhysicalDevicePresentationSupportKMSDRM(VkInstance instance, VkPhysicalDevice device, uint32_t queuefamily);
VkResult _glfwCreateWindowSurfaceKMSDRM(VkInstance instance, _GLFWwindow* window, const VkAllocationCallbacks* allocator, VkSurfaceKHR* surface);

void _glfwPollMonitorsKMSDRM(void);

