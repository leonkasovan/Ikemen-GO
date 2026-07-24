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
```

> The first build downloads and compiles SDL2, FFmpeg, and libxmp from source.
> Subsequent builds skip download if sources exist and skip compilation if the
> static libraries are already cached.

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
| `make appbundle`     | Create macOS `.app` bundle (I.K.E.M.E.N-Go.app) |
| `make clean`         | Remove entire `build/` directory (binary, libs, downloaded sources, everything) |
| `make distclean`     | Remove `build/` + `deploy/` — full reset to pristine checkout |
| `make deps-check`    | Verify required tools are installed |

### Options

| Variable        | Values              | Description |
|-----------------|---------------------|-------------|
| `CONFIG=debug`  | release / debug     | Debug build with memory instrumentation |
| `ARCH=386`      | amd64 / 386         | Build 32-bit instead of 64-bit |
| `APP_VERSION=X` | string              | Version string embedded in the binary (default: nightly) |
| `APP_BUILDTIME` | date string         | Build timestamp (default: current date) |

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
./Ikemen_GO.exe               # 64-bit
```

> 32-bit builds use `ARCH=386` and produce `Ikemen_GO_x86.exe`.

---

## Linux

### Prerequisites (Debian/Ubuntu)

```bash
sudo apt update && sudo apt install -y \
  git make cmake pkg-config golang-go gcc g++ nasm \
  wget unzip libx11-dev libxext-dev libxrandr-dev \
  libxcursor-dev libxi-dev libxinerama-dev libxss-dev \
  libxxf86vm-dev libasound2-dev libgl1-mesa-dev
```

> `mingw-w64-x86_64-yasm` is optional (nasm covers the assembler needs).
> No system SDL2, FFmpeg, or libxmp packages are needed — all are built from source.

### Build

```bashmake                          # Native release → Ikemen_GO
make CONFIG=debug             # Debug build
make install                  # Release → deploy/
make install CONFIG=debug     # Debug → deploy/
```

The Makefile detects your architecture and builds natively (x86-64 or ARM64).

### Run

```bash
./Ikemen_GO
# If you need a GL fallback on some drivers:
MESA_GL_VERSION_OVERRIDE=2.1 ./Ikemen_GO
```

---

## macOS (Apple Silicon / Intel)

### Prerequisites (Homebrew)

```bash
brew install git make cmake pkg-config go nasm wget molten-vk
```

> MoltenVK is required for the Vulkan renderer on macOS.
> No system SDL2, FFmpeg, or libxmp are needed — all are built from source.

### Build

```bashmake                          # Native release → Ikemen_GO
make CONFIG=debug             # Debug build
make install                  # Release → deploy/
make install CONFIG=debug     # Debug → deploy/
make appbundle                # Create I.K.E.M.E.N-Go.app
```

The Makefile detects your architecture — Apple Silicon → arm64, Intel → amd64.

### Run

```bash
./Ikemen_GO
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
2. Installs JDK 11 (Eclipse Temurin) for Gradle
3. Installs JDK 17 for sdkmanager
4. Installs Android NDK r27d (cross-compiler)
5. Cross-compiles SDL2 for Android arm64-v8a
6. Cross-compiles libxmp for Android arm64-v8a
7. Cross-compiles FFmpeg for Android arm64-v8a
8. Installs Android SDK (platform + build-tools)
9. Sets up environment variables in `~/.bashrc`
10. Builds `libmain.so` (Go c-shared library)
11. Downloads ikemen-droid source
12. Downloads screenpack assets
13. Builds and signs the APK

### Outputs

| File | Description |
|------|-------------|
| `build/ikemen-go.apk` | Installable (signed) APK |
| `build/android-deps/` | Cross-compiled library dependencies |
| `android/release.jks` | Auto-generated signing keystore |

### Customization

Override any setting via environment variables:

```bash
SDK_PLATFORM=android-33 SDK_BUILD_TOOLS=33.0.1 \
  ./tools/generate_android_via_native.sh --yes
```

See the script header for all overridable variables.

---

## Assets required to run (desktop builds)

Place these folders **next to the executable or app bundle**:
`data`, `external`, `font`, and a screenpack (chars, stages, sound, video).

Use `make install` to automatically assemble a runnable distribution with
the Elecbyte screenpack. The release CI bundles these automatically.

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
