# Build toolchain — current Makefile

The Makefile is **cross-platform** (Windows / Linux / macOS). It detects the host
OS via `uname -s` and sets platform-specific variables (`GOOS`, `CC`, `BINNAME`,
link flags, etc.) automatically.

## Windows (MSYS2 / MINGW64)

The compiler is the MinGW-w64 toolchain bundled with MSYS2, **not** the
`usr/bin` shell tools. `make` lives in `usr/bin`; `gcc` lives in `mingw64/bin`.

Prepend these to PATH before building:

```
C:\msys64\usr\bin        # make, sh
C:\msys64\mingw64\bin    # gcc, the actual compiler
```

Run:

```bash
make                    # Win64 release → Ikemen_GO.exe
make CONFIG=debug       # Win64 debug
```

The Makefile sets `GOROOT=/mingw64/lib/go`, `GOPATH=$HOME/go`,
`GOCACHE=$HOME/.cache/go-build` automatically.

## Linux / macOS

```bash
make                    # Native release → Ikemen_GO
make CONFIG=debug       # Debug build
```

## Build overview

All external libraries (SDL2, FFmpeg, XMP) are **built from source** by the
Makefile and linked statically into the binary.

- **Windows**: `-tags static` activates vendored static SDL2 (`sdl_cgo_static.go`).
  `-static -extldflags '-static'` fully links the MinGW runtime (no DLLs).
  `windres` embeds icon + manifest.
- **Linux/macOS**: SDL2 linked via `pkg-config` (`sdl_cgo.go`). No `-static`.
  System libs (glibc, X11, frameworks) remain dynamic.

## Targets

| Command | Description |
|---------|-------------|
| `make` / `make release` | Release build |
| `make CONFIG=debug` | Debug build (console + memory instrumentation) |
| `make ffmpeg` | Build FFmpeg libraries |
| `make xmp` | Build libxmp |
| `make sdl2` | Build SDL2 |
| `make screenpack` | Clone/update screenpack |
| `make install` | Assemble runnable install/ |
| `make clean` | Remove build artifacts |
| `make distclean` | Remove artifacts + library sources |

## Options

- `CONFIG=debug` — debug build (default: release)
- `ARCH=386` — 32-bit Windows build (default: amd64)
- `APP_VERSION=X.Y` — version string (default: nightly)
- `APP_BUILDTIME=X` — build timestamp

## Verifying a build (Windows)

```bash
make
ldd Ikemen_GO.exe | grep -iE 'winpthread|SDL2|libgcc_s|libstdc'  # expect: nothing
```
