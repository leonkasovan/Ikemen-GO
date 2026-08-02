# Building Ikemen GO

Ikemen GO links against **SDL2** (windowing, input, and game controller support via go-sdl2),
**FFmpeg** (background video: VP9/Opus/Vorbis in WebM/Matroska), and **libxmp** (module music:
MOD/XM/S3M/IT, etc.). All three are **built from source** by the Makefile and linked
statically into the binary.

On **Windows**, the MinGW runtime is also linked statically — the `.exe` needs only
Windows system DLLs at runtime.
On **Linux/macOS**, system libraries (glibc, X11, frameworks) stay dynamically linked.

---

## Quick start

```bash
git clone https://github.com/ikemen-engine/Ikemen-GO.git
cd Ikemen-GO
make                    # native build (release)
make install            # assemble runnable distribution in deploy/
make install-remote     # scp the binary to a device (opt-in, see below)
```

> The first build downloads and compiles SDL2, FFmpeg, and libxmp from source.
> Subsequent builds skip download if sources exist and skip compilation if the
> static libraries are already cached.

### Per-platform build directories

Build artifacts are separated by target platform under `build/`:

```
build/windows_amd64/    Windows x86-64 (also windows_386 for 32-bit)
build/linux_amd64/      Linux x86-64
build/linux_arm64/      Linux aarch64/arm64
build/darwin_arm64/     macOS Apple Silicon (darwin_amd64 for Intel)
```

Each platform keeps its own SDL2/FFmpeg/XMP libraries, Windows resources, and
binary, so you can build for several targets on one machine without one
platform's artifacts clobbering another's.

- `make clean` / `make distclean` only remove the **current** platform's tree.
- `rm -rf build/` wipes **all** platform build trees.

---

## Windows (MSYS2 / MINGW64)

### Prerequisites

Install MSYS2 from https://www.msys2.org and open **MSYS2 MINGW64**, then:

```bash
pacman -Syu --noconfirm
pacman -S --noconfirm git make mingw-w64-x86_64-pkg-config \
  mingw-w64-x86_64-go mingw-w64-x86_64-toolchain \
  mingw-w64-x86_64-nasm mingw-w64-x86_64-cmake
pacman -S --noconfirm wget unzip
```

> System libraries (SDL2, libxmp) are **not** needed — all three are built from source.
> `mingw-w64-x86_64-yasm` is optional (nasm covers the assembler needs).
>
> On MSYS2 the Makefile auto-fixes "trimmed" Go by setting `GOROOT=/mingw64/lib/go`.
> If you get a `GOROOT` error, run `export GOROOT=/mingw64/lib/go` before `make`.

### What gets built

| Library  | Build system | Source URL |
|----------|-------------|------------|
| SDL2     | CMake       | https://github.com/libsdl-org/SDL (release-2.32.10) |
| FFmpeg   | autotools   | https://github.com/FFmpeg/FFmpeg (n7.1, minimal config) |
| libxmp   | CMake       | https://github.com/libxmp/libxmp (libxmp-4.7.1) |

### Build targets

| Command              | Description |
|----------------------|-------------|
| `make` / `make release` | Release build → binary (GUI subsystem on Windows) |
| `make CONFIG=debug`  | Debug build (console + memory instrumentation) |
| `make ffmpeg`        | Build FFmpeg libraries only |
| `make xmp`           | Build libxmp only |
| `make sdl2`          | Build SDL2 only |
| `make screenpack`    | Clone/update Elecbyte screenpack into `deploy/` |
| `make install`       | Release build + screenpack → `deploy/` |
| `make install CONFIG=debug` | Debug build + screenpack → `deploy/` |
| `make install-remote` | Build binary, then scp it to a remote device (see [Deploying to a remote device](#deploying-to-a-remote-device)) |
| `make appbundle`     | Create macOS `.app` bundle (I.K.E.M.E.N-Go.app) |
| `make clean`         | Remove the current platform's build dir (e.g. `build/windows_amd64/`) — binary, libs, downloaded sources, everything for that platform |
| `make distclean`     | Remove current platform build dir + `deploy/` — full reset |
| `make deps-check`    | Verify required tools are installed |

### Options

| Variable        | Values              | Description |
|-----------------|---------------------|-------------|
| `CONFIG=debug`  | release / debug     | Debug build with memory instrumentation |
| `ARCH=386`      | amd64 / 386         | Build 32-bit instead of 64-bit |
| `APP_VERSION=X` | string              | Version string embedded in the binary (default: nightly) |
| `APP_BUILDTIME` | date string         | Build timestamp (default: current date) |
| `REMOTE_HOST`   | string              | scp destination for `make install-remote` (default: `ark@192.168.7.2`) |
| `REMOTE_DIR`    | string              | Remote directory for `make install-remote` (default: `/home/ark/ikemen`) |

### Examples

```bash
make                          # Release
make CONFIG=debug             # Debug build (console + memory instrumentation)
make install                  # Release → deploy/
make install CONFIG=debug     # Debug → deploy/
make ARCH=386                 # 32-bit build
make APP_VERSION=v1.0.0       # Tagged build
make APP_VERSION=v1.0.0 CONFIG=debug
```

### Run

```bash
./build/windows_amd64/Ikemen_GO.exe    # 64-bit
```

> 32-bit builds use `ARCH=386` and produce `Ikemen_GO_x86.exe` in `build/windows_386/`.

---

## Linux

### Prerequisites (Debian/Ubuntu amd64)

```bash
sudo apt update && sudo apt install -y \
  git make cmake pkg-config gcc g++ nasm \
  wget unzip libx11-dev libxext-dev libxrandr-dev \
  libxcursor-dev libxi-dev libxinerama-dev libxss-dev \
  libxxf86vm-dev libasound2-dev libgl1-mesa-dev libglvnd-dev \
  libgtk-3-dev
```

> `libgtk-3-dev` (GTK3 `.pc`) and `libglvnd-dev` (`gl.pc`) are required by the
> engine's Linux cgo dependencies (`sqweek/dialog`, vendored `gl` bindings).

**Go toolchain — auto-installed.** The Makefile requires **Go 1.22+**. On Linux,
`make` runs `check-go-env` before building: if `go` is missing or older than
1.22, it downloads the **latest Go** from https://go.dev/dl/ and installs it to
`/usr/local/go` (using `sudo` automatically when needed). You do **not** need to
`apt install golang-go` — Ubuntu's package is usually too old. If you can't use
`sudo`, point `GO_INSTALL_DIR` at a writable path:
`make GO_INSTALL_DIR=$HOME/go-toolchain`.

> Go 1.20+ is required because the engine uses the experimental `arena`
> standard-library package (state rollback system). The Makefile automatically
> enables `GOEXPERIMENT=arenas` for the build — don't build outside the
> Makefile without it, or `imports arena: build constraints exclude all Go
> files` will fail.

**SDL2 — auto-selected.** The Makefile prefers the **system SDL2** via
pkg-config (`sudo apt install libsdl2-dev`). If no system SDL2 is found, `make
sdl2` automatically builds a **dynamic** `libSDL2.so` from source into the
platform build dir — no manual step needed. (`make install` copies it next to
the binary; `make install-remote` scps it to the device.) On the build machine
an rpath makes it load automatically; on a remote device run with
`LD_LIBRARY_PATH=. ./Ikemen_GO` (or run `ldconfig`) so the loader finds the
dynamic lib.

### Prerequisites (Ubuntu arm64)
```bash
(root)
sudo apt install -y make cmake pkg-config gcc g++ wget unzip libasound2-dev libgl1-mesa-dev libxext-dev

> Go 1.22+ is auto-installed by the Makefile (see the amd64 section above): if
> `go` is missing or too old, `make check-go-env` downloads the latest Go to
> `/usr/local/go` (needs root, or set `GO_INSTALL_DIR`). Manual alternative:
>   wget https://go.dev/dl/go1.26.5.linux-arm64.tar.gz
>   sudo tar -C /usr/local -xzf go1.26.5.linux-arm64.tar.gz
>   export PATH=$PATH:/usr/local/go/bin

# (optional) install system SDL2 — if you skip this, the Makefile builds a
# dynamic libSDL2.so from source automatically (make sdl2).
(root)
wget https://github.com/libsdl-org/SDL/archive/refs/heads/release-2.32.x.zip
unzip release-2.32.x.zip
cd SDL-release-2.32.x/
./configure --prefix=/usr
make -j8
make install
```

> FFmpeg and libxmp are always built from source. SDL2: system lib via
> pkg-config is preferred; the Makefile falls back to building a dynamic
> `libSDL2.so` from source when no system SDL2 is installed.

### Build

```bashmake                          # Native release → Ikemen_GO
make CONFIG=debug             # Debug build
make install                  # Release → deploy/
make install CONFIG=debug     # Debug → deploy/
```

The Makefile detects your architecture and builds natively (x86-64 or ARM64)
into `build/linux_amd64/` or `build/linux_arm64/`. On the first run,
`check-go-env` verifies the Go toolchain and auto-installs Go 1.22+ (latest)
from go.dev to `/usr/local/go` when needed, and `sdl2` picks SDL2 (system lib,
or dynamic-from-source fallback) — the rest of the build then continues
automatically.

### Run

```bash
./build/linux_amd64/Ikemen_GO          # x86-64 build
./build/linux_arm64/Ikemen_GO          # arm64 build
# If you need a GL fallback on some drivers:
MESA_GL_VERSION_OVERRIDE=2.1 ./build/linux_amd64/Ikemen_GO
```

> The Makefile builds natively for the **host** platform. To produce a
> different target (e.g. `build/linux_arm64` from an x86-64 machine) you need
> cross toolchains (cross gcc, arm64 SDL2/FFmpeg/XMP builds, etc.) — the
> Makefile alone does not cross-compile.

---

## macOS (Apple Silicon / Intel)

### Prerequisites (Homebrew)

```bash
brew install git make cmake pkg-config go nasm wget sdl2 molten-vk
```

> MoltenVK is required for the Vulkan renderer on macOS.
> SDL2 is used via pkg-config (`brew install sdl2`); FFmpeg and libxmp are
> always built from source.

### Build

```bashmake                          # Native release → Ikemen_GO
make CONFIG=debug             # Debug build
make install                  # Release → deploy/
make install CONFIG=debug     # Debug → deploy/
make appbundle                # Create I.K.E.M.E.N-Go.app
```

The Makefile detects your architecture — Apple Silicon → arm64, Intel → amd64,
building into `build/darwin_arm64/` or `build/darwin_amd64/`.

### Run

```bash
./build/darwin_arm64/Ikemen_GO        # Apple Silicon build
./build/darwin_amd64/Ikemen_GO        # Intel build
```

You can also double-click **`tools/Ikemen_GO.command`**.

---

## Android (APK via Docker)

This builds the engine **and** produces a ready-to-install **APK** inside Docker.
No Android Studio required.

### Requirements

- Docker (Docker Desktop on Windows/macOS, or Docker Engine on Linux)

### Build (from repo root)

```bash
./tools/generate_android_via_docker.sh
```

Or run Docker Compose directly:

```bash
docker compose -f tools/docker/android/docker-compose.yml build
docker compose -f tools/docker/android/docker-compose.yml run --rm android-build
```

### Outputs

| File | Description |
|------|-------------|
| `build/ikemen-go.apk` | Installable APK |
| `build/libmain.so` + `build/libmain.h` | Engine shared library + header |
| `build/android-apk/ikemen-droid` | Cloned Android wrapper project |

### Configuration

```bash
APP_VERSION=my-build APP_BUILDTIME=2026.01.13 \
ANDROID_APK_REPO=https://github.com/Jesuszilla/ikemen-droid.git \
ANDROID_APK_REF=main \
docker compose -f tools/docker/android/docker-compose.yml run --rm android-build
```

Skip APK packaging (only build `.so` + deps):

```bash
BUILD_ANDROID_APK=0 docker compose -f tools/docker/android/docker-compose.yml run --rm android-build
```

### Customizing the Android wrapper

The APK is built from the `ikemen-droid` wrapper project, cloned into:
`build/android-apk/ikemen-droid`

To use a custom fork, set `ANDROID_APK_REPO` and `ANDROID_APK_REF`.

---

## Android (APK via native setup on Windows)

This builds the engine **and** produces a ready-to-install **APK** by installing
and configuring everything natively on your machine (JDK, NDK, SDK, cross-compilers).
Requires MSYS2 MINGW64 on Windows.

### Requirements

- MSYS2 MINGW64 shell on Windows
- ~10 GB free disk space
- Good internet connection (downloads NDK ~1.5 GB plus SDK, JDK, library sources)

### Build (from repo root)

```bash
./tools/generate_android_via_native.sh --yes
```

Run without `--yes` for interactive mode (asks before each step).

### What this does

The script runs 14 steps automatically:

1. Installs MSYS2 build tools (make, cmake, gcc, nasm, etc.)
2. Installs JDK 17 (Eclipse Temurin) for Gradle / sdkmanager
3. Installs Android NDK r27d (cross-compiler)
4. Cross-compiles SDL2 for Android (selected ABI)
5. Cross-compiles libxmp for Android (selected ABI)
6. Cross-compiles FFmpeg for Android (selected ABI)
7. Installs Android SDK (platform + build-tools)
8. Sets up environment variables in `~/.bashrc`
9. Builds `libmain.so` (Go c-shared library)
10. Downloads ikemen-droid source
11. Downloads screenpack assets
12. Builds and signs the APK

### Dependency overrides

The project maintains a local fork of the [`reisen`](https://github.com/ikemen-engine/reisen)
FFmpeg binding library at **`packages/reisen/`**. This fork applies a fix for
32-bit ARM (armeabi-v7a) builds where the upstream library uses C types (`C.ulong`,
`C.long`) that are incompatible on 32-bit architectures. The fix uses `C.size_t`
and `C.int64_t` instead, which are correct on both 32-bit and 64-bit ARM.

The local fork is enabled via a `replace` directive in `go.mod`:

```
replace github.com/ikemen-engine/reisen => ./packages/reisen
```

If you update the `reisen` dependency version, copy the updated source into
`packages/reisen/` and re-apply the platform fix if needed.

### Outputs

| File | Description |
|------|-------------|
| `build/ikemen-go-<ABI>.apk` | Installable signed APK (release), e.g. `ikemen-go-arm64-v8a.apk` |
| `build/ikemen-go-<ABI>-debug.apk` | Installable debug APK (see below), e.g. `ikemen-go-armeabi-v7a-debug.apk` |
| `build/android-deps-<ABI>/` | Cross-compiled library dependencies per ABI |
| `android/release.jks` | Auto-generated signing keystore |

### Build variants

| Build | Command | APK | Go `debug` tag | PProf |
|-------|---------|-----|----------------|-------|
| **Release** | `./tools/generate_android_via_native.sh --yes` | `build/ikemen-go.apk` | off | no-op |
| **Debug** | `CONFIG=debug ./tools/generate_android_via_native.sh --yes` | `build/ikemen-go-debug.apk` | on | `:6060` |

### Debugging with ADB

```bash
# Install debug APK
adb install -r build/ikemen-go-debug.apk

# Stream engine logcat output
adb logcat -s ikemen

# Stream asset extraction + engine logs
adb logcat -s AssetExtractor SDLActivity ikemen AndroidRuntime
```

### Profiling with pprof (debug build only)

The `debug` build tag activates an HTTP pprof server on `localhost:6060` inside the
app process. Use it from your PC:

```bash
# 1. Forward device port to your machine
#    (adjust adb path for your SDK install location)
/c/Android/SDK/platform-tools/adb.exe forward tcp:6060 tcp:6060

# 2. On MSYS2, trimmed Go needs explicit GOROOT:
export GOROOT=/mingw64/lib/go

# 3. Capture profiles
GOROOT=/mingw64/lib/go go tool pprof http://localhost:6060/debug/pprof/heap              # heap snapshot
GOROOT=/mingw64/lib/go go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30  # 30s CPU
GOROOT=/mingw64/lib/go go tool pprof http://localhost:6060/debug/pprof/goroutine          # goroutine stack
GOROOT=/mingw64/lib/go go tool pprof http://localhost:6060/debug/pprof/block              # blocking profiles

# Interactive commands once inside pprof:
#   top      — show top memory/cpu consumers
#   web      — open flame graph in browser
#   list fn  — show source lines for a function
```

The pprof server is compiled out entirely in release builds (`!debug` tag) —
zero overhead.

### Target ABI selection

By default the script builds for **arm64-v8a** (64-bit ARM). To target older
32-bit ARM devices, set `ANDROID_ABI=armeabi-v7a`:

```bash
ANDROID_ABI=armeabi-v7a ./tools/generate_android_via_native.sh --yes
```

When you change the ABI, all native dependencies (SDL2, libxmp, FFmpeg) are
rebuilt from scratch into a separate directory (`build/android-deps-<ABI>/`).

> **Important**: If you previously built a different ABI, make sure the
> `ANDROID_DEPS_PATH` environment variable is **not stale** from the previous
> build. A leftover `ANDROID_DEPS_PATH` in your shell or `~/.bashrc` will cause
> the linker to pick up libraries from the wrong ABI directory, resulting in:
> ```
> ld.lld: error: .../libSDL2.so is incompatible with aarch64linux
> ```
> To fix this, either unset the variable before building:
> ```bash
> unset ANDROID_DEPS_PATH
> ANDROID_ABI=armeabi-v7a ./tools/generate_android_via_native.sh --yes
> ```
> Or explicitly clear it:
> ```bash
> ANDROID_DEPS_PATH= ANDROID_ABI=armeabi-v7a ./tools/generate_android_via_native.sh --yes
> ```

### Customization

Override any setting via environment variables:

```bash
# Target a different Android API level
SDK_PLATFORM=android-33 SDK_BUILD_TOOLS=33.0.1 \
  ./tools/generate_android_via_native.sh --yes

# Target 32-bit ARM devices
ANDROID_ABI=armeabi-v7a ./tools/generate_android_via_native.sh --yes

# Build a debug APK
CONFIG=debug ./tools/generate_android_via_native.sh --yes
```

See the script header for all overridable variables (including `NDK_VERSION`,
`SDL2_VERSION`, `FFMPEG_VERSION`, `XMP_VERSION`, and more).

---

## Deploying to a remote device

`make install` assembles the local `deploy/` distribution; it never touches the
network. To copy the built binary to a device (e.g. an ARM handheld) over SSH
instead, use the opt-in `install-remote` target:

```bash
make install-remote                                    # uses the defaults below
make install-remote REMOTE_HOST=ark@192.168.7.2 \
                   REMOTE_DIR=/home/ark/ikemen         # explicit override
```

The target builds the binary (if needed) and `scp`s it to
`$(REMOTE_HOST):$(REMOTE_DIR)/`, prompting for the SSH password interactively.
Only the binary is transferred — engine data (`data`, `font`, `external`) and the
screenpack must already be present on the device.

---

## Assets required to run (desktop builds)

Place these folders **next to the executable or app bundle**:
`data`, `external`, `font`, and a screenpack (chars, stages, sound, video).

Use `make install` to automatically assemble a runnable distribution with
the Elecbyte screenpack. The release CI bundles these automatically.

---

## Profiling with pprof (debug build only)

The `debug` build tag activates an HTTP pprof server on `localhost:6060`.
Start the game with a `CONFIG=debug` build and navigate to your desired screen
before profiling.

### Capturing profiles from a live process

```bash
# Heap (in-use memory snapshot)
GOROOT=/mingw64/lib/go go tool pprof http://localhost:6060/debug/pprof/heap

# Heap (total allocations since process start)
GOROOT=/mingw64/lib/go go tool pprof -alloc_space http://localhost:6060/debug/pprof/heap

# CPU profile (30 seconds)
GOROOT=/mingw64/lib/go go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# Goroutine stacks
GOROOT=/mingw64/lib/go go tool pprof http://localhost:6060/debug/pprof/goroutine

# Blocking profile
GOROOT=/mingw64/lib/go go tool pprof http://localhost:6060/debug/pprof/block
```

### Examining a saved profile

Profiles are saved to `$HOME/pprof/` by default. To re-open a saved file with
source-level annotations:

```bash
# On MSYS2, Go needs explicit GOROOT and the binary's module path
# must be trimmed to find source files in the local checkout:
GOROOT=/mingw64/lib/go go tool pprof -trim_path 'github.com/ikemen-engine/Ikemen-GO/' \
  -source_path /path/to/your/Ikemen-GO \
  -list 'main.toLValue' \
  $HOME/pprof/pprof.Ikemen_GO_debug.exe.*.pb.gz

# Or, if running from the project root:
GOROOT=/mingw64/lib/go go tool pprof -trim_path 'github.com/ikemen-engine/Ikemen-GO/' \
  -source_path . \
  -list 'main.toLValue' \
  $HOME/pprof/pprof.Ikemen_GO_debug.exe.*.pb.gz
```

Common pprof commands:

| Command | Description |
|---------|-------------|
| `top` | Show top memory/cpu consumers |
| `top20 --cum` | Show top 20 by cumulative total |
| `list fn` | Show source lines for a function with per-line allocations |
| `peek fn` | Show callers/callees of a function |
| `tree` | Show hierarchical call tree |
| `web` | Open interactive flame graph in browser |
| `pdf` | Generate PDF call graph |

### Comparing two profiles

```bash
GOROOT=/mingw64/lib/go go tool pprof \
  -base /path/to/baseline.pb.gz \
  /path/to/current.pb.gz
```

Then use `top` to see what grew (positive = more allocations in current).

> The pprof server is compiled out entirely in release builds (`!debug` tag) —
> zero overhead.

---

## Notes & licensing

- The minimal FFmpeg we build matches CI: static libs only; `file` protocol;
  Matroska/WebM demuxers; VP9/Opus/Vorbis decoders and parsers; no FFmpeg CLI tools.
- FFmpeg is used under **LGPL v2.1**; releases attach the corresponding source snapshot.
- On Windows, the Makefile builds SDL2, FFmpeg, and libxmp from source and links them
  fully statically (including the MinGW runtime). The resulting `.exe` needs only
  Windows system DLLs at runtime.
- On Linux/macOS, SDL2, FFmpeg, and libxmp are compiled into the binary while system
  libraries (glibc, X11, frameworks) remain dynamically linked.
- Ikemen GO sources are MIT; bundled screenpack assets have their own licenses.

---

## Troubleshooting

- **Missing tools**: run `make deps-check` to see what's missing.
- **CMake errors**: ensure `mingw-w64-x86_64-cmake` is installed via pacman (Windows)
  or `cmake` via apt/brew (Linux/macOS).
- **SDL2 link errors**: run `make sdl2` separately to verify the SDL2 build completes.
- **FFmpeg link errors**: run `make ffmpeg` separately to verify the FFmpeg build.
- **libxmp not found**: run `make xmp` separately to verify the XMP build.
- **Linux GL compatibility**: try `MESA_GL_VERSION_OVERRIDE=2.1` for a fallback.
- **Android armeabi-v7a CGo type errors** (`cannot use _Ctype_ulong as _Ctype_size_t`):
  The upstream `reisen` library uses C types that are incompatible on 32-bit ARM.
  The project ships a patched local copy at `packages/reisen/` with the fix.
  If updating the dependency, re-apply the fix to `packages/reisen/platform_linux.go`.
- **Android linker error after switching ABIs** (`libSDL2.so is incompatible with
  aarch64linux`): The `ANDROID_DEPS_PATH` variable may still point to the previous
  ABI's library directory. Run `unset ANDROID_DEPS_PATH` before building.
