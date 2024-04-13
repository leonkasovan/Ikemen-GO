# Ikemen GO

Ikemen GO is an open source fighting game engine that supports resources from the [M.U.G.E.N](https://en.wikipedia.org/wiki/Mugen_(game_engine)) engine, written in Google’s programming language, [Go](https://go.dev/). It is a complete rewrite of a prior engine known simply as Ikemen.

```
git clone -b SDL2 https://github.com/leonkasovan/Ikemen-GO.git
make
```

```
/home/ark/go/pkg/mod/github.com/veandco/go-sdl2@v0.4.38/_libs/include/SDL2/SDL_config.h
line 22: #	include "SDL_config_linux_arm.h"

/home/ark/go/pkg/mod/github.com/go-gl/gl@v0.0.0-20231021071112-07e5d0ea2e71/v2.1/gl/procaddr.go
/*
#cgo gl pkg-config: gl
#cgo gl LDFLAGS: -ldl
#cgo gl CFLAGS: -DNOX11 -DNOEGL
#include <dlfcn.h>
#include <stdlib.h>
static void* libHandle = NULL;
void* GlowGetProcAddress(const char* name) {
        if (libHandle == NULL)
                libHandle = dlopen("/usr/lib/libGL.so", RTLD_LAZY);
        if (libHandle == NULL)
                libHandle = dlopen("/lib/x86_64-linux-gnu/libGL.so", RTLD_LAZY);
        if (libHandle == NULL)
                libHandle = dlopen("/lib/aarch64-linux-gnu/libGL.so", RTLD_LAZY);
        if (libHandle)
                return dlsym(libHandle, name);
}
*/

/home/ark/go/pkg/mod/github.com/go-gl/gl@v0.0.0-20231021071112-07e5d0ea2e71/v2.1/gl/package.go
Line 32428: add comment for function that are not called
gpUniformMatrix2x3fv = (C.GPUNIFORMMATRIX2X3FV)(getProcAddr("glUniformMatrix2x3fv"))
dst
```