# ============================================================================
# Ikemen-GO — Cross-Platform Makefile (Windows / Linux / macOS)
#
# All external libraries (SDL2, FFmpeg, XMP) are built from source and linked
# statically into the binary. On Windows the MinGW runtime is also linked
# statically; on Linux/macOS system libraries (glibc, X11, etc.) are linked
# dynamically.
#
# Usage:
#   make                    # Native build (release)
#   make debug              # Debug build (console + memory instrumentation)
#   make clean              # Remove build artifacts
#   make distclean          # Remove build artifacts + external lib sources
#   make screenpack         # Clone/update Elecbyte screenpack
#   make install            # Assemble runnable build in deploy/
#   make help               # Show all targets and options
#
# Prerequisites:
#   Windows (MSYS2 MINGW64 shell):
#     pacman -Syu --noconfirm
#     pacman -S --noconfirm make mingw-w64-x86_64-pkg-config \
#       mingw-w64-x86_64-go mingw-w64-x86_64-toolchain \
#       mingw-w64-x86_64-nasm mingw-w64-x86_64-cmake
#     pacman -S --noconfirm wget unzip
#   Linux (Debian/Ubuntu):
#     sudo apt update && sudo apt install -y \
#       make cmake pkg-config golang-go gcc g++ nasm \
#       wget unzip libsdl2-dev \
#       libx11-dev libxext-dev libxrandr-dev \
#       libxcursor-dev libxi-dev libxinerama-dev libxss-dev \
#       libxxf86vm-dev libasound2-dev libgl1-mesa-dev
#   macOS (Homebrew):
#     brew install make cmake pkg-config go nasm wget \
#       molten-vk
# ============================================================================

CONFIG ?= release
SHELL       := /bin/bash
.SHELLFLAGS := -euo pipefail -c
.ONESHELL:

# Library source URLs (downloaded as zip archives from GitHub)
SDL2_URL    := https://github.com/libsdl-org/SDL/archive/refs/tags/release-2.32.10.zip
FFMPEG_URL  := https://github.com/FFmpeg/FFmpeg/archive/refs/tags/n7.1.zip
XMP_URL     := https://github.com/libxmp/libxmp/archive/refs/tags/libxmp-4.7.1.zip
SCREENPACK_URL := https://github.com/ikemen-engine/Ikemen-GO-Screenpack/archive/refs/heads/master.zip

# ============================================================================
# Host OS / Architecture Detection
# ============================================================================

UNAME_S := $(shell uname -s 2>/dev/null || echo Unknown)
UNAME_M := $(shell uname -m 2>/dev/null || echo unknown)

ifeq ($(UNAME_S),Linux)
  HOST_OS := linux
else ifeq ($(UNAME_S),Darwin)
  HOST_OS := darwin
else
  # MSYS2 / MINGW64 / CYGWIN
  HOST_OS := windows
endif

# Map uname -m to Go GOARCH
ifeq ($(UNAME_M),x86_64)
  HOST_ARCH := amd64
else ifneq (,$(filter $(UNAME_M),arm64 aarch64))
  HOST_ARCH := arm64
else ifneq (,$(filter $(UNAME_M),i386 i686))
  HOST_ARCH := 386
else
  HOST_ARCH := amd64
endif

# ============================================================================
# Environment — Windows-specific PATH and Go env
# ============================================================================

ifeq ($(HOST_OS),windows)
  # /usr/bin first: MSYS2's make.exe handles .ONESHELL + env propagation
  # correctly; the MinGW native make.exe in /mingw64/bin does not.
  # /mingw64/bin still on PATH for gcc, g++, pkgconf, etc.
  export PATH    := /usr/bin:/mingw64/bin:$(PATH)
  export GOROOT ?= /mingw64/lib/go
  # Default GOPATH for environments where it isn't set (MSYS2, CI, etc.)
  export GOPATH  ?= $(HOME)/go
  # Go build cache location — needed because %LocalAppData% may be unset
  # in non-interactive MSYS2 shells.
  export GOCACHE ?= $(HOME)/.cache/go-build
else
  # Linux / macOS — common Go installation paths (manual install or non-default)
  export PATH := $(PATH):/usr/local/go/bin
endif

# ============================================================================
# Project Metadata (overridable by CI)
# ============================================================================

APP_VERSION     ?= nightly
APP_BUILDTIME   ?= $(shell date '+%Y.%m.%d')
COPY_START_YEAR ?= 2016

# ============================================================================
# Architecture & Platform Selection
# ============================================================================
# Override ARCH=386 for a 32-bit Windows build.
# On Linux/macOS, native arch is auto-detected.

ARCH ?= $(HOST_ARCH)

# --- Target OS ---
ifeq ($(HOST_OS),windows)
  GOOS := windows
else
  GOOS := $(HOST_OS)
endif

# --- Target Architecture ---
ifeq ($(HOST_OS),windows)
  # Windows: only x86/x64
  ifeq ($(ARCH),386)
    GOARCH      := 386
    CC          ?= i686-w64-mingw32-gcc
    CXX         ?= i686-w64-mingw32-g++
    WRTARGET    := pe-i386
    ASM_ARCH    := x86
    WIN_BINNAME := Ikemen_GO_x86.exe
  else
    GOARCH      := amd64
    CC          ?= x86_64-w64-mingw32-gcc
    CXX         ?= x86_64-w64-mingw32-g++
    WRTARGET    := pe-x86-64
    ASM_ARCH    := amd64
    WIN_BINNAME := Ikemen_GO.exe
  endif
  BINNAME := $(WIN_BINNAME)
  BINEXT  := .exe
else ifeq ($(HOST_OS),linux)
  GOOS    := linux
  GOARCH  := $(ARCH)
  CC      ?= gcc
  CXX     ?= g++
  BINNAME := Ikemen_GO
  BINEXT  :=
else ifeq ($(HOST_OS),darwin)
  GOOS    := darwin
  GOARCH  := $(ARCH)
  CC      ?= clang
  CXX     ?= clang++
  BINNAME := Ikemen_GO
  BINEXT  :=
endif

export GOOS GOARCH CC CXX

# ============================================================================
# Directories
# ============================================================================

BUILDDIR      := build
BUILD_PREFIX  := $(abspath $(BUILDDIR)/output)
OUTDIR        := $(BUILDDIR)
WINRES_DIR    := $(BUILDDIR)/winres

# External library source directories
SDL2_SRCDIR   := $(BUILDDIR)/SDL-release-2.32.10
FFMPEG_SRCDIR := $(BUILDDIR)/FFmpeg-n7.1
XMP_SRCDIR    := $(BUILDDIR)/libxmp-libxmp-4.7.1

# External library build directories (separate from source, used by CMake builds)
SDL2_BUILDDIR    := $(BUILDDIR)/build-sdl2
XMP_BUILDDIR     := $(BUILDDIR)/build-xmp

# FFmpeg static library targets
FFMPEG_LIBS := $(addprefix $(BUILD_PREFIX)/lib/, \
  libavformat.a libavcodec.a libavutil.a \
  libswscale.a libswresample.a libavfilter.a)

# Install directory
INSTALLDIR ?= deploy
# SCREENPACK_DIR points to where screenpack is extracted — now directly into
# $(INSTALLDIR) for a streamlined workflow.
SCREENPACK_DIR := $(INSTALLDIR)

# ============================================================================
# Toolchain
# ============================================================================

export GOEXPERIMENT := arenas
export CGO_ENABLED  := 1

PKG_CONFIG ?= pkg-config

# Tools required for building — nasm is an x86 assembler, not available/needed on ARM64
ifeq ($(HOST_ARCH),arm64)
  BUILD_TOOLS := make cmake pkg-config gcc g++ unzip wget
else
  BUILD_TOOLS := make cmake pkg-config gcc g++ nasm go unzip wget
endif

# Windows resource compiler (only used on Windows)
ifeq ($(HOST_OS),windows)
  WINDRES := $(shell command -v x86_64-w64-mingw32-windres 2>/dev/null \
               || command -v windres 2>/dev/null || echo windres)
endif

# ============================================================================
# pkg-config Packages
# ============================================================================
# On all platforms, we use pkg-config for FFmpeg and XMP (locally built).
# On Windows, SDL2 is linked via sdl_cgo_static.go (-tags static).
# On Linux/macOS, SDL2 is linked via pkg-config (sdl_cgo.go, !static tag),
# so its .pc file is patched post-install to include private dependencies.

# Local library install into $(BUILD_PREFIX)/lib/pkgconfig. Set at parse time
# so sub-make invocations inherit it; recipes override it at run time too
# (handles the first build when the directory doesn't yet exist).
#
# IMPORTANT — path separator: the Windows-native pkgconf.exe (mingw64) splits
# search paths on ';', NOT ':'. A colon-joined value like
#   C:/proj/build/output/lib/pkgconfig:C:/msys64/mingw64/lib/pkgconfig
# gets shredded at the drive-letter colons, so pkgconf finds none of them and
# silently falls back to its built-in system path — pulling in the full MSYS2
# FFmpeg (ggml, whisper, shaderc, libplacebo, rsvg, ...) and breaking the link.
#
# On Windows the build is fully static and SDL2 is linked via -tags static
# (not pkg-config), and our local FFmpeg/XMP .pc files are self-contained, so
# we isolate completely with PKG_CONFIG_LIBDIR = local dir only. On Linux/macOS
# we keep the colon-separated PKG_CONFIG_PATH prepend (Unix separator; SDL2's
# .pc still needs the system paths for X11/etc.).
ifneq ($(wildcard $(BUILD_PREFIX)/lib/pkgconfig),)
  ifeq ($(HOST_OS),windows)
    export PKG_CONFIG_LIBDIR := $(BUILD_PREFIX)/lib/pkgconfig
  else
    export PKG_CONFIG_PATH := $(BUILD_PREFIX)/lib/pkgconfig:$(PKG_CONFIG_PATH)
  endif
endif

# ============================================================================
# Go Build Flags
# ============================================================================

# Common version-stamping linker flags — defined early so LDFLAGS_GO can use them.
LDFLAGS_BASE := \
  -X 'main.Version=$(APP_VERSION)' \
  -X 'main.BuildTime=$(APP_BUILDTIME)'

# The `static` Go build tag activates sdl_cgo_static.go (Windows only) which
# links the vendored SDL2 headers and our locally-built libSDL2_windows_*.a.
# On Linux/macOS, SDL2 is linked via pkg-config (sdl_cgo.go with !static tag).
#
# Link flags:
# - Windows: -static for fully static (no MinGW DLLs). Release uses
#   -H windowsgui (no console window). Debug: console subsystem.
# - Linux: no -static; system libs (glibc, X11, pthreads, dl, etc.) stay
#   dynamically linked. SDL2/FFmpeg/XMP .a files are embedded via CGO.
# - macOS: no -static (not supported by Apple). Frameworks stay dynamic.
ifeq ($(CONFIG),debug)
  IS_DEBUG := 1
else
  IS_DEBUG :=
endif

# Debug builds get a distinct binary name (Ikemen_GO_debug.exe / Ikemen_GO_debug)
# so they never overwrite the release binary. $(basename) strips the extension
# ('.exe' on Windows, none elsewhere) before appending the suffix + $(BINEXT).
ifeq ($(IS_DEBUG),1)
  BINNAME := $(basename $(BINNAME))_debug$(BINEXT)
endif

ifeq ($(HOST_OS),windows)
  ifeq ($(IS_DEBUG),1)
    GO_TAGS := static debug
  else
    GO_TAGS := static
  endif
  EXTLDFLAGS := -static
  ifeq ($(IS_DEBUG),1)
    LDFLAGS_GO := $(LDFLAGS_BASE) -extldflags '$(EXTLDFLAGS)'
  else
    LDFLAGS_GO := -H windowsgui -s -w $(LDFLAGS_BASE) -extldflags '$(EXTLDFLAGS)'
  endif
else
  # Linux / macOS — no -tags static; SDL2 via pkg-config. No -static, no -H windowsgui.
  ifeq ($(IS_DEBUG),1)
    GO_TAGS := debug
  else
    GO_TAGS :=
  endif
  EXTLDFLAGS :=
  ifeq ($(IS_DEBUG),1)
    LDFLAGS_GO := -extldflags '$(EXTLDFLAGS)'
  else
    LDFLAGS_GO := -s -w $(LDFLAGS_BASE) -extldflags '$(EXTLDFLAGS)'
  endif
  ifeq ($(HOST_ARCH),arm64)
    ifeq ($(HOST_OS),linux)
      GO_TAGS += armdevice
    endif
  endif
endif

# ============================================================================
# Derived File Targets
# ============================================================================

BINARY   := $(OUTDIR)/$(BINNAME)

SRC_SYSO := src/rsrc_windows.syso

# Go source files — used as prerequisites for the binary target so make can
# detect when to recompile.
GO_SOURCES := $(shell find src -name '*.go' -type f)

# ============================================================================
# SxS Version Sanitization (Windows only)
# ============================================================================

ifeq ($(HOST_OS),windows)

_sxs_clean = $(shell v='$(strip $(subst v,,$(subst V,,$(1))))'; \
  [[ "$$v" =~ ^[0-9.]+$$ ]] || { echo "0.0.0.0"; exit; }; \
  IFS='.' read -ra p <<<"$$v"; \
  for ((i=$${#p[@]}; i<4; i++)); do p+=("0"); done; \
  out=(); for x in "$${p[@]:0:4}"; do \
    [[ "$$x" =~ ^[0-9]+$$ ]] || x=0; ((x<0)) && x=0; ((x>65535)) && x=65535; \
    out+=("$$x"); \
  done; IFS='.'; echo "$${out[*]}" )

SXS_VERSION := $(call _sxs_clean,$(APP_VERSION))

_sxs_major := $(word 1,$(subst ., ,$(SXS_VERSION)))
_sxs_minor := $(word 2,$(subst ., ,$(SXS_VERSION)))
_sxs_patch := $(word 3,$(subst ., ,$(SXS_VERSION)))
_sxs_build := $(word 4,$(subst ., ,$(SXS_VERSION)))

BUILD_YEAR    := $(subst -,,$(firstword $(subst ., ,$(APP_BUILDTIME))))
APP_COPYRIGHT ?= (c) $(COPY_START_YEAR)-$(BUILD_YEAR) Ikemen GO team (MIT)

endif


# ============================================================================
# Phony Targets
# ============================================================================

.PHONY: all release debug help \
        deps-check check-go-env \
        ffmpeg xmp sdl2 winres install appbundle \
        screenpack \
        clean distclean FORCE

# ============================================================================
# Default Target
# ============================================================================

all: release

# ============================================================================
# Release Build
# ============================================================================

release: deps-check xmp ffmpeg sdl2 $(BINARY)
	@echo "==> Build successful"
	@echo "    Binary: $(BINARY)"

# ============================================================================
# Convenience Targets
# ============================================================================

debug:
	$(MAKE) release CONFIG=debug

# ============================================================================
# Dependency Checks
# ============================================================================

deps-check:
	@echo "==> Checking build dependencies..."
	@missing=""; \
	for tool in $(BUILD_TOOLS); do \
		command -v $$tool >/dev/null 2>&1 || missing="$$missing $$tool"; \
	done; \
	if [ -n "$$missing" ]; then \
		echo "ERROR: Missing tools:$$missing" >&2; \
		case "$(HOST_OS)" in \
			windows) \
				echo "Install from the MINGW64 shell:" >&2; \
				echo "  pacman -Syu --noconfirm" >&2; \
				echo "  pacman -S --noconfirm make mingw-w64-x86_64-pkg-config \\" >&2; \
				echo "    mingw-w64-x86_64-go mingw-w64-x86_64-toolchain \\" >&2; \
				echo "    mingw-w64-x86_64-nasm mingw-w64-x86_64-cmake" >&2; \
				echo "  pacman -S --noconfirm wget unzip" >&2;; \
			linux) \
				if [ "$(HOST_ARCH)" = "arm64" ]; then \
					echo "Install (Debian/Ubuntu ARM64):" >&2; \
					echo "  sudo apt update && sudo apt install -y \\" >&2; \
					echo "    make cmake pkg-config golang-go gcc g++ \\" >&2; \
					echo "    wget unzip libsdl2-dev libegl1-mesa-dev" >&2; \
				else \
					echo "Install (Debian/Ubuntu):" >&2; \
					echo "  sudo apt update && sudo apt install -y \\" >&2; \
					echo "    make cmake pkg-config golang-go gcc g++ nasm \\" >&2; \
					echo "    wget unzip" >&2; \
				fi;; \
			darwin) \
				echo "Install with Homebrew:" >&2; \
				echo "  brew install make cmake pkg-config go nasm wget" >&2;; \
		esac; \
		exit 1; \
	fi
	@# Safe path check for MSYS2/Cygwin
	@case "$(HOST_OS)" in \
		windows) \
			case "$(CURDIR)" in \
				*[!A-Za-z0-9._/:-]*) \
					echo "ERROR: Repository path contains characters unsafe for MSYS2/autotools:" >&2; \
					echo "  $(CURDIR)" >&2; \
					echo "Use only letters, digits, ':', '_', '-', '.'" >&2; \
					exit 1;; \
			esac;; \
	esac
	@echo "    All dependencies found."

check-go-env:
	@go version >/dev/null 2>&1 || \
		{ echo "ERROR: 'go version' failed." >&2; exit 1; }

# ============================================================================
# SDL2 Static Build (CMake)
# URL defined above as $(SDL2_URL)
# ============================================================================

# SDL2 CMake flags — platform-specific
SDL2_CMAKE_FLAGS := \
	-DCMAKE_INSTALL_PREFIX="$(BUILD_PREFIX)" \
	-DBUILD_SHARED_LIBS=OFF \
	-DSDL_SHARED=OFF \
	-DSDL_STATIC=ON \
	-DSDL_TEST=OFF \
	-DSDL_TESTS=OFF \
	-DSDL_INSTALL_TESTS=OFF

# CMake generator — "MSYS Makefiles" generates MSYS2-compatible Makefiles
# (forward-slash paths). "MinGW Makefiles" generates Windows backslash paths
# that only work with mingw32-make. We set WIN32=TRUE explicitly to exclude
# Unix-specific sources (src/core/unix/*.c).
SDL2_CMAKE_GENERATOR :=

ifeq ($(HOST_OS),windows)
  SDL2_CMAKE_GENERATOR := -G "MSYS Makefiles" -DWIN32=TRUE
  SDL2_CMAKE_FLAGS += \
	-DSDL_OPENGL=ON \
	-DSDL_OPENGLES=OFF \
	-DSDL_RENDER_D3D=OFF \
	-DSDL_VULKAN=OFF \
	-DSDL_DIRECTX=OFF \
	-DSDL_OFFSCREEN=OFF \
	-DSDL_DUMMYVIDEO=OFF \
	-DSDL_WAYLAND=OFF \
	-DSDL_X11=OFF \
	-DSDL_COCOA=OFF
else ifeq ($(HOST_OS),linux)
  SDL2_CMAKE_FLAGS += \
	-DSDL_OPENGL=ON \
	-DSDL_OPENGLES=OFF \
	-DSDL_X11=ON \
	-DSDL_WAYLAND=OFF \
	-DSDL_COCOA=OFF \
	-DSDL_VULKAN=OFF
else ifeq ($(HOST_OS),darwin)
  SDL2_CMAKE_FLAGS += \
	-DSDL_OPENGL=ON \
	-DSDL_OPENGLES=OFF \
	-DSDL_COCOA=ON \
	-DSDL_METAL=ON \
	-DSDL_VULKAN=OFF \
	-DSDL_X11=OFF \
	-DSDL_WAYLAND=OFF
endif

# SDL2: built from source on Windows, system lib on Linux/macOS.
# NOTE: No ifeq/else/endif around targets — GNU Make 4.2.1 + .ONESHELL
# peeks at tab-indented lines inside false conditionals and chokes.
$(BUILD_PREFIX)/lib/libSDL2.a:
	@echo "==> Building static SDL2 for $(HOST_OS)..."
	mkdir -p $(BUILDDIR)
	if [ ! -d "$(SDL2_SRCDIR)" ]; then
		echo "==> Downloading $(SDL2_URL)..."
		if [ ! -f "$(BUILDDIR)/SDL2.zip" ]; then
			wget -q "$(SDL2_URL)" -O "$(BUILDDIR)/SDL2.zip"
		else
			echo "==> Using existing zip: $(BUILDDIR)/SDL2.zip"
		fi
		tmp="$(BUILDDIR)/SDL2.zip-extract"
		rm -rf "$$tmp"
		mkdir -p "$$tmp"
		unzip -q "$(BUILDDIR)/SDL2.zip" -d "$$tmp"
		subdir="$$(find "$$tmp" -mindepth 1 -maxdepth 1 -type d | head -1)"
		rm -rf "$(SDL2_SRCDIR)"
		mkdir -p "$(SDL2_SRCDIR)"
		shopt -s dotglob
		mv "$$subdir"/* "$(SDL2_SRCDIR)"/ 2>/dev/null || true
		rm -rf "$$tmp" "$(BUILDDIR)/SDL2.zip"
	fi
	cmake -S "$(SDL2_SRCDIR)" -B "$(SDL2_BUILDDIR)" \
		$(SDL2_CMAKE_GENERATOR) \
		$(SDL2_CMAKE_FLAGS)
	cmake --build "$(SDL2_BUILDDIR)" --parallel
	cmake --install "$(SDL2_BUILDDIR)"
	case "$(HOST_OS)" in \
		windows) \
			sed -i 's/-lSDL2\b/-l:libSDL2.a/g' "$(BUILD_PREFIX)/lib/pkgconfig/sdl2.pc"; \
			rm -f "$(BUILD_PREFIX)/lib/libSDL2.dll.a"; \
			cp "$(BUILD_PREFIX)/lib/libSDL2.a" "$(BUILD_PREFIX)/lib/libSDL2_windows_$(GOARCH).a"; \
			cp "$(BUILD_PREFIX)/lib/libSDL2main.a" "$(BUILD_PREFIX)/lib/libSDL2main_windows_$(GOARCH).a";; \
	esac
	@echo "==> SDL2 static library installed to: $(BUILD_PREFIX)"

sdl2:
	@case "$(HOST_OS)" in \
		windows) \
			$(MAKE) -s $(BUILD_PREFIX)/lib/libSDL2.a; \
			echo "    Local SDL2 $$(PKG_CONFIG_PATH="$(BUILD_PREFIX)/lib/pkgconfig" $(PKG_CONFIG) --modversion sdl2) found";; \
		*) \
			pkg-config --exists sdl2 || { \
				echo "ERROR: SDL2 development library not found." >&2; \
				echo "  Install with: sudo apt install libsdl2-dev" >&2; \
				exit 1; \
			}; \
			echo "    System SDL2 $$(PKG_CONFIG_PATH="/usr/lib/pkgconfig:/usr/local/lib/pkgconfig" $(PKG_CONFIG) --modversion sdl2) found";; \
	esac

# ============================================================================
# FFmpeg Static Build (autotools)
# URL defined above as $(FFMPEG_URL)
# ============================================================================

ffmpeg: $(FFMPEG_LIBS)
	@echo "    FFmpeg $$(PKG_CONFIG_PATH="$(BUILD_PREFIX)/lib/pkgconfig" $(PKG_CONFIG) --modversion libavformat) found"

$(FFMPEG_LIBS):
	@echo "==> Building static FFmpeg for $(HOST_OS)..."
	mkdir -p $(BUILDDIR)
	if [ ! -d "$(FFMPEG_SRCDIR)" ]; then
		echo "==> Downloading $(FFMPEG_URL)..."
		if [ ! -f "$(BUILDDIR)/FFmpeg.zip" ]; then
			wget -q "$(FFMPEG_URL)" -O "$(BUILDDIR)/FFmpeg.zip"
		else
			echo "==> Using existing zip: $(BUILDDIR)/FFmpeg.zip"
		fi
		tmp="$(BUILDDIR)/FFmpeg.zip-extract"
		rm -rf "$$tmp"
		mkdir -p "$$tmp"
		unzip -q "$(BUILDDIR)/FFmpeg.zip" -d "$$tmp"
		subdir="$$(find "$$tmp" -mindepth 1 -maxdepth 1 -type d | head -1)"
		rm -rf "$(FFMPEG_SRCDIR)"
		mkdir -p "$(FFMPEG_SRCDIR)"
		shopt -s dotglob
		mv "$$subdir"/* "$(FFMPEG_SRCDIR)"/ 2>/dev/null || true
		rm -rf "$$tmp" "$(BUILDDIR)/FFmpeg.zip"
	fi
	cd "$(FFMPEG_SRCDIR)" && \
		./configure \
			--prefix="$(BUILD_PREFIX)" \
			--enable-static --disable-shared \
			--disable-gpl --disable-nonfree \
			--disable-debug --disable-doc --disable-programs --disable-everything \
			--disable-autodetect --disable-avdevice --disable-pthreads \
			$(if $(filter arm64,$(HOST_ARCH)),--disable-x86asm,) \
			--enable-avformat --enable-avcodec --enable-avutil \
			--enable-swresample --enable-swscale \
			--enable-avfilter --enable-filter=buffer,buffersink,format,scale,pad,crop \
			--enable-protocol=file \
			--enable-demuxer=matroska,webm \
			--enable-decoder=vp8,vp9,opus,vorbis \
			--enable-parser=vp8,vp9,opus,vorbis \
			--cc="$(CC)" \
			--pkg-config="$$(which pkg-config)" && \
		make -j2 && \
		make install
	@# Verify local FFmpeg libraries were installed — fail immediately if not,
	@# otherwise pkg-config falls back to the system FFmpeg (ggml, whisper,
	@# shaderc, ...) and the static link breaks.
	@test -f "$(BUILD_PREFIX)/lib/libavformat.a" && \
		test -f "$(BUILD_PREFIX)/lib/libavcodec.a" && \
		test -f "$(BUILD_PREFIX)/lib/libavutil.a" || \
		{ echo "ERROR: FFmpeg install failed — local .a files missing in $(BUILD_PREFIX)/lib/" >&2; exit 1; }
	@echo "==> FFmpeg static libraries installed to: $(BUILD_PREFIX)"

# ============================================================================
# XMP Static Library Build (CMake)
# URL defined above as $(XMP_URL)
# ============================================================================

XMP_LIB    := $(BUILD_PREFIX)/lib/libxmp.a

xmp: $(XMP_LIB)
	@# Remove any shared import lib (Windows-specific, no-op elsewhere).
	@rm -f "$(BUILD_PREFIX)/lib/libxmp.dll.a"
	@echo "    XMP $$(PKG_CONFIG_PATH="$(BUILD_PREFIX)/lib/pkgconfig" $(PKG_CONFIG) --modversion libxmp) found"

$(XMP_LIB):
	@echo "==> Building static libxmp for $(HOST_OS)..."
	mkdir -p $(BUILDDIR)
	if [ ! -d "$(XMP_SRCDIR)" ]; then
		echo "==> Downloading $(XMP_URL)..."
		if [ ! -f "$(BUILDDIR)/libxmp.zip" ]; then
			wget -q "$(XMP_URL)" -O "$(BUILDDIR)/libxmp.zip"
		else
			echo "==> Using existing zip: $(BUILDDIR)/libxmp.zip"
		fi
		tmp="$(BUILDDIR)/libxmp.zip-extract"
		rm -rf "$$tmp"
		mkdir -p "$$tmp"
		unzip -q "$(BUILDDIR)/libxmp.zip" -d "$$tmp"
		subdir="$$(find "$$tmp" -mindepth 1 -maxdepth 1 -type d | head -1)"
		rm -rf "$(XMP_SRCDIR)"
		mkdir -p "$(XMP_SRCDIR)"
		shopt -s dotglob
		mv "$$subdir"/* "$(XMP_SRCDIR)"/ 2>/dev/null || true
		rm -rf "$$tmp" "$(BUILDDIR)/libxmp.zip"
	fi
	cmake -S "$(XMP_SRCDIR)" -B "$(XMP_BUILDDIR)" \
		-DCMAKE_INSTALL_PREFIX="$(BUILD_PREFIX)" \
		-DBUILD_SHARED=OFF \
		-DWITH_UNIT_TESTS=OFF
	cmake --build "$(XMP_BUILDDIR)" --parallel
	cmake --install "$(XMP_BUILDDIR)"
	@echo "==> XMP static library installed to: $(BUILD_PREFIX)"

# ============================================================================
# Windows Resources (icon + manifest + version) — Windows only
# ============================================================================
# Targets defined unconditionally to avoid .ONESHELL + Make 4.2.1 parse bug.
# Recipes bail out on non-Windows via shell guard.

# FORCE ensures the .rc is regenerated on every build so version info is fresh.
$(WINRES_DIR)/Ikemen_GO.rc: FORCE
	@# Windows resource generation — no-op on non-Windows
	@[ "$(HOST_OS)" = "windows" ] || exit 0
	@echo "==> Generating Windows version resources..."
	mkdir -p $(WINRES_DIR)
	SXS="$(SXS_VERSION)"; \
	VMAJ="$(_sxs_major)"; \
	VMIN="$(_sxs_minor)"; \
	VPAT="$(_sxs_patch)"; \
	VREV="$(_sxs_build)"; \
	YEAR="$(APP_BUILDTIME)"; YEAR="$${YEAR%%-*}"; \
	COPY="$(APP_COPYRIGHT)"; \
	cat > $(WINRES_DIR)/Ikemen_GO.exe.manifest <<-MANEOF
	<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
	<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
	  <assemblyIdentity type="win32" name="Ikemen_GO" version="$${SXS}" processorArchitecture="$(ASM_ARCH)"/>
	  <dependency>
	    <dependentAssembly>
	      <assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls"
	        version="6.0.0.0" processorArchitecture="*" publicKeyToken="6595b64144ccf1df" language="*"/>
	    </dependentAssembly>
	  </dependency>
	</assembly>
	MANEOF
	cat > $(WINRES_DIR)/Ikemen_GO.rc <<-RCEOF
	#include <windows.h>
	#include <winver.h>
	1 ICON "Ikemen_Cylia_V2.ico"
	1 RT_MANIFEST "Ikemen_GO.exe.manifest"

	VS_VERSION_INFO VERSIONINFO
	 FILEVERSION $${VMAJ},$${VMIN},$${VPAT},$${VREV}
	 PRODUCTVERSION $${VMAJ},$${VMIN},$${VPAT},$${VREV}
	 FILEFLAGSMASK 0x3fL
	 FILEFLAGS 0x0L
	 FILEOS 0x4L
	 FILETYPE 0x1L
	 FILESUBTYPE 0x0L
	BEGIN
	    BLOCK "StringFileInfo"
	    BEGIN
	        BLOCK "040904B0"
	        BEGIN
	            VALUE "CompanyName", "Ikemen GO\0"
	            VALUE "FileDescription", "Ikemen GO\0"
	            VALUE "FileVersion", "$${SXS}\0"
	            VALUE "ProductName", "Ikemen GO\0"
	            VALUE "ProductVersion", "$${SXS}\0"
	            VALUE "OriginalFilename", "$(BINNAME)\0"
	            VALUE "InternalName", "Ikemen_GO\0"
	            VALUE "BuildDate", "$(APP_BUILDTIME)\0"
	            VALUE "LegalCopyright", "$${COPY}\0"
	        END
	    END
	    BLOCK "VarFileInfo"
	    BEGIN
	        VALUE "Translation", 0x0409, 1200
	    END
	END
	RCEOF

# Windows resource embedding — produces the .syso object for the linker.
# The shell guard makes this a no-op on non-Windows.
$(SRC_SYSO): $(WINRES_DIR)/Ikemen_GO.rc
	@[ "$(HOST_OS)" = "windows" ] || exit 0
	@echo "==> Embedding Windows resources (icon + manifest)..."
	mkdir -p src
	"$(WINDRES)" --use-temp-file --target=$(WRTARGET) \
		-I $(WINRES_DIR) -I external/icons \
		-i $(WINRES_DIR)/Ikemen_GO.rc \
		-O coff -o $(SRC_SYSO)

.PHONY: winres
winres: $(SRC_SYSO)

# ============================================================================
# CGo Flags — computed inside the recipe at build time (after ffmpeg/xmp
# install their .pc files). PKG_CONFIG_LIBDIR is set per-call so that only
# our local .pc files are found — blocking system FFmpeg (ggml, whisper,
# shaderc, rsvg, ...) from leaking into the static link line.
# ============================================================================

_CGO_PKGS := libavformat libavcodec libavutil libswscale libswresample libavfilter libxmp

# ============================================================================
# Go Binary
# ============================================================================

# Forwarding phony — `make binary` still works as before.
.PHONY: binary
binary: $(BINARY)

# Real file target — only rebuilds when Go sources, libraries, or resources
# have actually changed.  The `go build` command leverages Go's own build
# cache so unchanged packages are not recompiled.
$(BINARY): $(GO_SOURCES) $(XMP_LIB) $(FFMPEG_LIBS)
	@go version >/dev/null 2>&1 || \
		{ echo "ERROR: 'go version' failed." >&2; exit 1; }
	@echo "==> Building $(BINNAME) ($(CONFIG), GOOS=$(GOOS) GOARCH=$(GOARCH))..."
	@echo "    Go build tags: $(GO_TAGS) LDFLAGS: $(LDFLAGS_GO) CGO_CFLAGS: $(CGO_CFLAGS) CGO_LDFLAGS: $(CGO_LDFLAGS)"
	case "$(HOST_OS)" in \
		windows) \
			_PC_WINPATH="$$(cygpath -m "$(BUILD_PREFIX)/lib/pkgconfig")" ; \
			_CGO_CFLAGS=$$( $(PKG_CONFIG) --with-path="$${_PC_WINPATH}" --cflags $(_CGO_PKGS) ) ; \
			_CGO_LDFLAGS="-L$(BUILD_PREFIX)/lib $$( $(PKG_CONFIG) --with-path="$${_PC_WINPATH}" --static --libs $(_CGO_PKGS) )" ; \
			CGO_CFLAGS="-DLIBXMP_STATIC $$_CGO_CFLAGS" \
			CGO_LDFLAGS="$$_CGO_LDFLAGS" \
			go build -trimpath -tags "$(GO_TAGS)" \
				-ldflags "$(LDFLAGS_GO)" \
				-o "$(BINARY)" ./src;; \
		*) \
			_CGO_CFLAGS=$$( PKG_CONFIG_LIBDIR= PKG_CONFIG_PATH="$(BUILD_PREFIX)/lib/pkgconfig:/usr/lib/pkgconfig:/usr/local/lib/pkgconfig$(if $(PKG_CONFIG_PATH),:$(PKG_CONFIG_PATH),)" $(PKG_CONFIG) --cflags $(_CGO_PKGS) ) ; \
			_CGO_LDFLAGS="-L$(BUILD_PREFIX)/lib $$( PKG_CONFIG_LIBDIR= PKG_CONFIG_PATH="$(BUILD_PREFIX)/lib/pkgconfig:/usr/lib/pkgconfig:/usr/local/lib/pkgconfig$(if $(PKG_CONFIG_PATH),:$(PKG_CONFIG_PATH),)" $(PKG_CONFIG) --static --libs $(_CGO_PKGS) )" ; \
			PKG_CONFIG_PATH="$(BUILD_PREFIX)/lib/pkgconfig:/usr/lib/pkgconfig:/usr/local/lib/pkgconfig$(if $(PKG_CONFIG_PATH),:$(PKG_CONFIG_PATH),)" \
			PKG_CONFIG_LIBDIR= \
			CGO_CFLAGS="-DLIBXMP_STATIC $$_CGO_CFLAGS" \
			CGO_LDFLAGS="$$_CGO_LDFLAGS" \
			go build -trimpath -tags "$(GO_TAGS)" \
				-ldflags "$(LDFLAGS_GO)" \
				-o "$(BINARY)" ./src;; \
	esac
	@# Clean up Windows resource object after build
	rm -f $(SRC_SYSO) 2>/dev/null || true

# On Windows, the binary also requires SDL2 (built from source) and the
# resource .syso object (icon + manifest + version info).
ifeq ($(HOST_OS),windows)
$(BINARY): $(BUILD_PREFIX)/lib/libSDL2_windows_$(GOARCH).a $(SRC_SYSO)
endif

# ============================================================================
# Install — assemble a runnable distribution
# ============================================================================
# Depends on `screenpack` which downloads/extracts the Elecbyte screenpack zip.
# Merges screenpack directories (chars, stages, sound, video, data, external,
# font) with engine data and copies the binary into $(INSTALLDIR).

install: deps-check screenpack $(BINARY)
	@echo "==> Installing to $(INSTALLDIR)/..."
	mkdir -p "$(INSTALLDIR)"
	@echo "==> Copying engine data: data font external"
	cp -r data font external "$(INSTALLDIR)/"
	@echo "==> Copying binary $(BINNAME)..."
	cp -f "$(BINARY)" "$(INSTALLDIR)/"
	@echo "==> Install complete: $(INSTALLDIR)/"

# ============================================================================
# macOS App Bundle
# ============================================================================
# Creates I.K.E.M.E.N-Go.app from the built binary, Info.plist, and
# bundle_run.sh. Called from CI after the binary is built:
#   make appbundle BINNAME=Ikemen_GO_MacOSARM
#
# The app bundle structure:
#   I.K.E.M.E.N-Go.app/
#     Contents/
#       Info.plist
#       MacOS/
#         bundle_run.sh
#         <binary>

APPDIR := I.K.E.M.E.N-Go.app

appbundle:
	@echo "==> Creating macOS app bundle: $(APPDIR)..."
	rm -rf "$(APPDIR)"
	mkdir -p "$(APPDIR)/Contents/MacOS"
	mkdir -p "$(APPDIR)/Contents/Resources"
	cp tools/Info.plist "$(APPDIR)/Contents/Info.plist"
	cp tools/bundle_run.sh "$(APPDIR)/Contents/MacOS/bundle_run.sh"
	chmod +x "$(APPDIR)/Contents/MacOS/bundle_run.sh"
	cp -f "$(BINARY)" "$(APPDIR)/Contents/MacOS/$(notdir $(BINARY))"
	chmod +x "$(APPDIR)/Contents/MacOS/$(notdir $(BINARY))"
	@echo "==> App bundle created: $(APPDIR)"
	@echo "    Binary: $(APPDIR)/Contents/MacOS/$(notdir $(BINARY))"

# ============================================================================
# Screenpack Download / Extract
# ============================================================================
# Downloads the Elecbyte screenpack as a zip archive and extracts it directly
# into $(INSTALLDIR). The `install` target then overlays engine data and the
# binary on top — no separate merge step needed.
# URL defined above as $(SCREENPACK_URL).

screenpack:
	@echo "==> Downloading Elecbyte screenpack..."
	mkdir -p $(BUILDDIR)
	if [ ! -d "$(INSTALLDIR)" ]; then
		echo "==> Downloading $(SCREENPACK_URL)..."
		if [ ! -f "$(BUILDDIR)/screenpack.zip" ]; then
			wget -q "$(SCREENPACK_URL)" -O "$(BUILDDIR)/screenpack.zip"
		else
			echo "==> Using existing zip: $(BUILDDIR)/screenpack.zip"
		fi
		tmp="$(BUILDDIR)/screenpack.zip-extract"
		rm -rf "$$tmp"
		mkdir -p "$$tmp"
		unzip -q "$(BUILDDIR)/screenpack.zip" -d "$$tmp"
		subdir="$$(find "$$tmp" -mindepth 1 -maxdepth 1 -type d | head -1)"
		rm -rf "$(INSTALLDIR)"
		mkdir -p "$(INSTALLDIR)"
		shopt -s dotglob
		mv "$$subdir"/* "$(INSTALLDIR)"/ 2>/dev/null || true
		rm -rf "$$tmp" "$(BUILDDIR)/screenpack.zip"
	fi
	@echo "==> Screenpack ready in $(INSTALLDIR)"

# ============================================================================
# Clean
# ============================================================================

clean:
	@echo "==> Cleaning build artifacts..."
	rm -rf $(BUILDDIR) 2>/dev/null || true
	rm -rf $(APPDIR) 2>/dev/null || true
	@echo "==> Clean done."

distclean: clean
	@echo "==> Deep cleaning — removing all build artifacts and external lib sources..."
	rm -rf $(BUILDDIR) 2>/dev/null || true
	rm -rf $(INSTALLDIR) 2>/dev/null || true
	@echo "==> Distclean done."

# ============================================================================
# FORCE — ensures targets depending on it always rebuild
# ============================================================================
FORCE:

# ============================================================================
# Help
# ============================================================================
help:
	@echo 'Ikemen-GO Build — $(HOST_OS) ($(HOST_ARCH))'
	@echo ''
	@echo 'Targets:'
	@echo '  all / release  Build release binary'
	@echo '  debug          Debug build (console + memory instrumentation)'
	@echo '  ffmpeg         Build static FFmpeg libraries'
	@echo '  xmp            Build static XMP library'
	@echo '  sdl2           Build SDL2 library (static on Windows, system lib on Linux/macOS)'
	@echo '  screenpack     Clone/update Elecbyte screenpack'
	@echo '  install        Assemble runnable build in deploy/ (screenpack + binary)'
	@echo '  appbundle      Create macOS .app bundle (I.K.E.M.E.N-Go.app)'
	@echo '  clean          Remove build artifacts'
	@echo '  distclean      Remove artifacts + external library sources'
	@echo '  deps-check     Verify required tools are installed'
	@echo '  help           Show this help'
	@echo ''
	@echo 'Options:'
	@echo '  ARCH=<arch>        Target architecture (default: native)'
	@echo '                       Windows: amd64 (default) or 386'
	@echo '                       Linux:   amd64 (default) or arm64'
	@echo '                       macOS:   arm64 (Apple Silicon) or amd64 (Intel)'
	@echo '  CONFIG=debug       Debug build + memory instrumentation (default: release)'
	@echo '  APP_VERSION=X.Y    Set version string (default: nightly)'
	@echo '  APP_BUILDTIME=X    Set build timestamp'
	@echo ''
	@echo 'Platform notes:'
	@echo '  SDL2: Built from source (Windows) or system lib (Linux/macOS).'
	@echo '  FFmpeg/XMP: Built from source on all platforms.'
	@echo '  Windows: Fully static binary (no external DLLs at runtime).'
	@echo '  Linux:   SDL2/FFmpeg/XMP compiled in; system libs dynamic.'
	@echo '  macOS:   SDL2/FFmpeg/XMP compiled in; system frameworks dynamic.'
	@echo ''
	@echo 'Examples:'
	@echo '  make                          # Native release'
	@echo '  make debug                    # Native debug'
	@echo '  make APP_VERSION=v1.0.0       # Tagged build'
	@echo '  make APP_VERSION=v1.0.0 CONFIG=debug'
	@echo '  make install                  # Build + assemble runnable deploy/'
