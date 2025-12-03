package glfw

/*
// Windows Build Tags
// ----------------
// GLFW Options:
#cgo windows CFLAGS: -D_WIN32 -D_GLFW_WIN32 -Iglfw/deps/mingw

// Linker Options:
#cgo windows LDFLAGS: -lgdi32

#cgo !gles2,windows LDFLAGS: -lopengl32
#cgo gles2,windows LDFLAGS: -lGLESv2

// Darwin Build Tags
// ----------------
// GLFW Options:
#cgo darwin CFLAGS: -D_GLFW_COCOA -Wno-deprecated-declarations

// Linker Options:
#cgo darwin LDFLAGS: -framework Cocoa -framework IOKit -framework CoreVideo

#cgo !gles2,darwin LDFLAGS: -framework OpenGL
#cgo gles2,darwin LDFLAGS: -lGLESv2

// Linux Build Tags
// ----------------
// GLFW Options:
#cgo debug CFLAGS: -DDEBUG
#cgo linux,x11 CFLAGS: -D_GLFW_X11 -D_GNU_SOURCE
#cgo linux,wayland CFLAGS: -D_GLFW_WAYLAND -D_GNU_SOURCE -Iglfw/deps/wayland
#cgo linux,kmsdrm CFLAGS: -D_GLFW_KMSDRM -D_GNU_SOURCE -I/usr/include/libdrm
#cgo linux,sdl2 CFLAGS: -D_GLFW_SDL2 -D_GNU_SOURCE
#cgo gles2 CFLAGS: -DGLAD_GLES2_IMPLEMENTATION

// Linker Options:
#cgo linux,!gles1,!gles2,!gles3,!vulkan LDFLAGS: -lGL
#cgo linux,gles1 LDFLAGS: -lGLESv1
#cgo linux,gles2 LDFLAGS: -lGLESv2
#cgo linux,gles3 LDFLAGS: -lGLESv3
#cgo linux,vulkan LDFLAGS: -lvulkan
#cgo linux,x11 LDFLAGS: -lX11 -lXrandr -lXxf86vm -lXi -lXcursor -lm -lXinerama -ldl -lrt
#cgo linux,wayland LDFLAGS: -lxkbcommon -lm -ldl -lrt
#cgo linux,kmsdrm LDFLAGS: -lm -ldl -lrt -lgbm -lEGL -ldrm
#cgo linux,sdl2 LDFLAGS: -lm

// BSD Build Tags
// ----------------
// GLFW Options:
#cgo freebsd,!wayland netbsd,!wayland openbsd pkg-config: x11 xau xcb xdmcp
#cgo freebsd,wayland netbsd,wayland pkg-config: wayland-client wayland-cursor wayland-egl epoll-shim
#cgo freebsd netbsd openbsd CFLAGS: -D_GLFW_HAS_DLOPEN
#cgo freebsd,!wayland netbsd,!wayland openbsd CFLAGS: -D_GLFW_X11 -D_GLFW_HAS_GLXGETPROCADDRESSARB
#cgo freebsd,wayland netbsd,wayland CFLAGS: -D_GLFW_WAYLAND -Iglfw/deps/wayland

// Linker Options:
#cgo freebsd netbsd openbsd LDFLAGS: -lm
*/
import "C"
