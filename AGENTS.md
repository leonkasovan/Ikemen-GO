## Build toolchain (Windows / MSYS2)

The compiler is the MinGW-w64 toolchain bundled with MSYS2, **not** the
`usr/bin` shell tools. `make` lives in `usr/bin`; `gcc` lives in `mingw64/bin`.
Both must be on PATH or the build fails (`make: gcc: No such file or directory`).

Prepend these two directories to PATH before building:

```
C:\msys64\usr\bin        # make, sh
C:\msys64\mingw64\bin    # gcc, the actual compiler
```

Run make through `sh` so it uses the MSYS environment:

```
sh -c 'make'

C:\msys64\usr\bin\sh -lc 'cd /c/Projects/ikemen-develop-update && export PATH=/mingw64/bin:/usr/bin:$PATH && make clean >/tmp/mk2.txt 2>&1; make >>/tmp/mk2.txt 2>&1; echo "MAKE_EXIT=$?" >> /tmp/mk2.txt'

```

### Fully static build (no MinGW/SDL2 DLLs)

1. Add packages\go-sdl2 and packages\gl
2. Add build\ffmpeg-src and build\xmp-src
3. Replace in go source code: from "github.com/veandco/go-sdl2/sdl" to "github.com/ikemen-engine/Ikemen-GO/packages/go-sdl2/sdl"
4. Replace in go source code: from "github.com/go-gl/gl/v3.3-core/gl" to "github.com/ikemen-engine/Ikemen-GO/packages/gl/v3.3-core/gl"
5. Replace in go source code: from "github.com/leonkasovan/gl/v3.2/gles2" to "github.com/ikemen-engine/Ikemen-GO/packages/gl/v3.2/gles2"

The Windows binary (`Ikemen_GO.exe`) links **fully static** — no
`libwinpthread-1.dll`, `libgcc_s_seh-1.dll`, `libstdc++-6.dll`, or `SDL2.dll`
ships. At runtime `ldd` shows only genuine Windows system DLLs (KERNEL32,
USER32, OPENGL32, SETUPAPI, dxcore, …).

How it works:
- **`-tags static`** activates the repo's vendored static SDL2 in
  `packages/go-sdl2/sdl/sdl_cgo_static.go`, which links the prebuilt
  `packages/go-sdl2/_libs/libSDL2_windows_amd64.a` (and the Win32 dep chain:
  setupapi / imm32 / version / oleaut32 / dinput8 / dxguid / …) instead of the
  shared `SDL2.dll`. The SDL2 `delaylibs` step is therefore removed.
- **`-extldflags '-static -Wl,--defsym,__ms_vsscanf=__mingw_vsscanf'`**
  statically links the MinGW runtime (winpthread/gcc/stdc++) and works around
  an ABI gap: the prebuilt static SDL2 was compiled against an older MinGW that
  exported `__ms_vsscanf`, renamed to `__mingw_vsscanf` on gcc 16.1.0. The
  `--defsym` aliases it. NOTE: the `-extldflags` value MUST use **single
  quotes** — double quotes break the build by prematurely closing the outer
  `-ldflags "..."`.
- FFmpeg + XMP are resolved statically via a LOCAL-only `pkg-config --static
  --libs` against `build/output/lib/pkgconfig` (`$(BUILD_PREFIX)`). Never
  `sed` the pkg-config output (it mangles `-lrtmp`).

Toolchain notes:
- The user uses **plain MinGW64, NOT UCRT**. Do not add UCRT libs.
- gcc is 16.1.0 (MSYS2 Rev5).
- `make` must run from a **MINGW64** prompt (not an MSYS `sh`) — MSYS breaks
  FFmpeg's `./configure` ("Native MSYS builds are discouraged").

### Verifying a build

```sh
make                                  # Win64 release (fully static)
ldd Ikemen_GO.exe | grep -iE 'winpthread|SDL2|libgcc_s|libstdc'  # expect: nothing
# Launch from a real Windows console to confirm no clock_gettime64 crash:
cmd.exe /c Ikemen_GO.exe
```

The exe must launch from `cmd.exe` (Explorer), not only the MSYS2 terminal —
that was the original regression the static link fixes.
