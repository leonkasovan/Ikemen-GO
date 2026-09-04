# ============================================================================
# Ikemen-GO — Cross-Platform Makefile (Windows / Linux / macOS)
#
# FFmpeg, libvpx (VP8/VP9 for WebM alpha) and libxmp are built from source and
# linked statically. SDL2 is built from source on Windows (static); on
# Linux/macOS the system SDL2 is used via pkg-config, with a dynamic-from-source
# fallback on Linux. On Windows the MinGW runtime is also linked statically; on
# Linux/macOS system libraries (glibc, X11, etc.) are linked dynamically.
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
#   Windows (w64devkit — self-contained, no package manager needed):
#     Everything needed is already in w64devkit (gcc, make, cmake, go, ...).
#     Optional: put yasm (or nasm) on PATH to enable FFmpeg x86 SIMD
#     (accelerated swscale color conversion); without any assembler FFmpeg
#     falls back to --disable-x86asm.
#   Linux (Debian/Ubuntu):
#     sudo apt update && sudo apt install -y \
#       make cmake pkg-config gcc g++ nasm \
#       wget unzip libsdl2-dev \
#   Linux Go: >= 1.22 required — `make check-go-env` auto-installs the latest
#     Go to /usr/local/go when `go` is missing or too old (needs sudo unless
#     GO_INSTALL_DIR points at a writable path). No need for apt golang-go.
#       libx11-dev libxext-dev libxrandr-dev \
#       libxcursor-dev libxi-dev libxinerama-dev libxss-dev \
#       libxxf86vm-dev libasound2-dev libgl1-mesa-dev \
#       libgtk-3-dev  (required by sqweek/dialog error dialog)
#   macOS (Homebrew):
#     brew install make cmake pkg-config go nasm wget \
#       molten-vk
# ============================================================================

# Build configuration: release (default) or debug. `config=` is canonical;
# the legacy uppercase `CONFIG=` is still accepted so old command lines and
# scripts don't silently build the wrong configuration.
config ?= $(if $(CONFIG),$(CONFIG),release)
ifdef CONFIG
$(warning CONFIG= is deprecated - use config= instead)
endif
# Recipes run under a real bash on MSYS2/Linux/macOS. Native Windows make
# (e.g. w64devkit) cannot resolve /bin/bash and silently falls back to the
# first sh on PATH (a POSIX-only busybox ash in w64devkit), so ALL recipe
# shell code must stay POSIX-sh compatible — no bashisms ([[ ]], shopt,
# read -ra, <<<, C-style for loops, ...).
SHELL       := /bin/bash
.SHELLFLAGS := -euo pipefail -c
.ONESHELL:

# Library source URLs (downloaded as zip archives from GitHub)
SDL2_URL    := https://github.com/libsdl-org/SDL/archive/refs/tags/release-2.32.10.zip
FFMPEG_URL  := https://github.com/FFmpeg/FFmpeg/archive/refs/tags/n7.1.zip
LIBVPX_URL  := https://github.com/webmproject/libvpx/archive/refs/tags/v1.15.2.zip
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
# Windows Toolchain Identification — MSYS2 MinGW64 vs w64devkit
# ============================================================================
# Both environments provide a MinGW-w64 compiler, but they differ in how make
# and its recipe shell behave, which changes PATH/Go handling, the CMake
# generator, and the FFmpeg x86 assembler:
#
#   MSYS2 MINGW64  — Cygwin-based MSYS2 runtime with its own POSIX tools
#                    (/usr/bin/cygpath, MSYSTEM env, /mingw64 package prefix).
#                    `make` is MSYS2's GNU make and resolves POSIX paths.
#   w64devkit      — Self-contained native MinGW-w64 toolchain. `make` is the
#                    native Windows GNU make; recipes run under a POSIX-only
#                    busybox ash; no cygpath. (Note: MSYSTEM can leak into this
#                    shell when MSYS2 is also installed, so it is NOT used for
#                    detection — only the wildcard checks below are.)
#
# Detection: MSYS2 always ships /usr/bin/cygpath.exe and /mingw64/bin (the
# MINGW64 package prefix); w64devkit has neither. $(wildcard) resolves POSIX
# paths under MSYS2's make but returns empty under native Windows make (there
# is no C:\usr\...), which is exactly the split we need. Note: the MSYSTEM
# env var is NOT reliable on its own — it leaks into a w64devkit shell when
# MSYS2 is also installed.
#
# Override with `make WIN_TOOLCHAIN=msys2` / `make WIN_TOOLCHAIN=w64devkit`.
ifeq ($(HOST_OS),windows)
  ifndef WIN_TOOLCHAIN
    ifneq ($(wildcard /usr/bin/cygpath),)
      WIN_TOOLCHAIN := msys2
    else ifneq ($(wildcard /mingw64/bin),)
      WIN_TOOLCHAIN := msys2
    else
      WIN_TOOLCHAIN := w64devkit
    endif
  endif
endif

# ============================================================================
# Environment — Windows-specific PATH and Go env
# ============================================================================

ifeq ($(HOST_OS),windows)
  # MSYS2 shell: put /usr/bin first so MSYS2's make.exe (which handles
  # .ONESHELL + POSIX env propagation correctly) wins over the MinGW native
  # make.exe in /mingw64/bin, and point Go at the MSYS2 toolchain.
  #
  # Guarded on the toolchain being MSYS2: native Windows make (w64devkit)
  # cannot use POSIX-style PATH entries — a colon-joined PATH would be
  # unreadable by Windows executables like go.exe — so its environment is
  # left untouched (the user's own PATH already provides gcc, go,
  # pkg-config, ...).
  ifeq ($(WIN_TOOLCHAIN),msys2)
    export PATH    := /usr/bin:/mingw64/bin:$(PATH)
    export GOROOT ?= /mingw64/lib/go
    # Default GOPATH for environments where it isn't set (MSYS2, CI, etc.)
    export GOPATH  ?= $(HOME)/go
    # Go build cache location — needed because %LocalAppData% may be unset
    # in non-interactive MSYS2 shells.
    export GOCACHE ?= $(HOME)/.cache/go-build
  endif
else ifeq ($(HOST_OS),linux)
  # /usr/local/go/bin goes FIRST on Linux: a toolchain auto-installed by
  # `make check-go-env` (or manually placed at /usr/local/go) must take
  # precedence over an older distro Go in /usr/bin.
  export PATH := /usr/local/go/bin:$(PATH)
else
  # macOS — keep system paths first (Homebrew Go is usually current)
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
  else
    GOARCH      := amd64
    CC          ?= x86_64-w64-mingw32-gcc
    CXX         ?= x86_64-w64-mingw32-g++
    WRTARGET    := pe-x86-64
    ASM_ARCH    := amd64
  endif
  BINEXT  := .exe
else ifeq ($(HOST_OS),linux)
  GOOS    := linux
  GOARCH  := $(ARCH)
  CC      ?= gcc
  CXX     ?= g++
  BINEXT  :=
else ifeq ($(HOST_OS),darwin)
  GOOS    := darwin
  GOARCH  := $(ARCH)
  CC      ?= clang
  CXX     ?= clang++
  BINEXT  :=
endif

# Output binary name: Windows and Linux embed the arch in the name
# (Ikemen_GO.amd64.exe / Ikemen_GO.386.exe / Ikemen_GO.amd64 / Ikemen_GO.arm64);
# macOS keeps a plain Ikemen_GO (no arch suffix).
ifeq ($(HOST_OS),darwin)
  BINBASE := Ikemen_GO
else
  BINBASE := Ikemen_GO.$(GOARCH)
endif
BINNAME := $(BINBASE)$(BINEXT)

export GOOS GOARCH CC CXX

# ============================================================================
# Directories
# ============================================================================

# Per-platform build tree: every OS/ARCH combination gets its own isolated
# directory under build/ (e.g. build/windows_amd64, build/linux_arm64,
# build/linux_amd64, build/windows_386, build/darwin_arm64). This lets you
# build for multiple targets on one machine without one platform's artifacts
# (SDL2/FFmpeg/XMP libs, winres, binary) clobbering another's.
#   make clean / distclean only remove the CURRENT platform's directory.
BUILDDIR      := build/$(GOOS)_$(GOARCH)
BUILD_PREFIX  := $(abspath $(BUILDDIR)/output)
OUTDIR        := $(BUILDDIR)
WINRES_DIR    := $(BUILDDIR)/winres

# External library source directories
SDL2_SRCDIR   := $(BUILDDIR)/SDL-release-2.32.10
FFMPEG_SRCDIR := $(BUILDDIR)/FFmpeg-n7.1
LIBVPX_SRCDIR := $(BUILDDIR)/libvpx-v1.15.2
XMP_SRCDIR    := $(BUILDDIR)/libxmp-libxmp-4.7.1

# External library build directories (separate from source, used by CMake builds)
SDL2_BUILDDIR    := $(BUILDDIR)/build-sdl2
LIBVPX_BUILDDIR  := $(BUILDDIR)/build-libvpx
XMP_BUILDDIR     := $(BUILDDIR)/build-xmp

# Static library targets
LIBVPX_LIB  := $(BUILD_PREFIX)/lib/libvpx.a
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

# GOEXPERIMENT=arenas is required to compile the stdlib 'arena' package (used
# by the rollback system), but only Go 1.20+ toolchains know the experiment.
# It is decided at BUILD TIME in the $(BINARY) recipe — a parse-time probe
# would run against the toolchain on the ORIGINAL PATH (e.g. Ubuntu's go 1.18),
# not the one check-go-env may have just auto-installed.
export CGO_ENABLED  := 1

PKG_CONFIG ?= pkg-config

# Tools required for building — nasm is an x86 assembler, not available/needed on ARM64
# On Linux, `go` is intentionally NOT required here: `make check-go-env`
# auto-installs a supported toolchain when `go` is missing or too old.
# Neither nasm nor yasm is required: FFmpeg falls back to --disable-x86asm
# when no assembler is found, so assemblers are intentionally left out of
# BUILD_TOOLS (w64devkit doesn't ship them; MSYS2 provides nasm).
BUILD_TOOLS := make cmake pkg-config gcc g++ unzip wget
ifneq ($(HOST_OS),linux)
  BUILD_TOOLS += go
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
ifeq ($(config),debug)
  IS_DEBUG := 1
else
  IS_DEBUG :=
endif

# Debug builds get a distinct binary name (Ikemen_GO.amd64_debug.exe / Ikemen_GO.amd64_debug)
# so they never overwrite the release binary.
ifeq ($(IS_DEBUG),1)
  BINNAME := $(BINBASE)_debug$(BINEXT)
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

# `desktop` build tag — set by default for regular desktop builds
# (Windows / Linux / macOS, excluding the armdevice variant). The mugen
# build uses its own `mugen` tag instead and strips `desktop` (see
# $(MUGEN_GO_TAGS)); android is tagged by GOOS + its own build script.
ifneq ($(filter armdevice,$(GO_TAGS)),)
  # armdevice variant — do not tag as desktop
else
  GO_TAGS += desktop
endif

# Verbose Go build — VERBOSE=1 adds -x -v to `go build` so you can watch
# exactly what the Go tool does during a build:
#   -x  print every command it runs (compile, link, cache hits)
#   -v  print the name of each package as it is compiled
# With a warm cache you'll see only the changed package(s) recompiled and
# everything else reported as cached.  `go build -work` (add to GO_VERBOSE
# manually) additionally keeps the temporary work directory.
VERBOSE ?=
GO_VERBOSE := $(if $(VERBOSE),-x -v,)

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

# POSIX-sh version of the SxS sanitizer. It runs at parse time via
# $(shell), which native Windows make executes with the first sh on PATH
# (a POSIX-only busybox ash in w64devkit). Two hard constraints:
#   1. No bashisms ([[ ]], shopt, read -ra, <<<, C-style for loops, ...).
#   2. No ')' characters in the script — make's own parser closes the
#      $(shell ...) function at the first unbalanced ')' it sees (e.g. a
#      shell `case` pattern terminator), truncating the script.
# grep (instead of case) keeps the script free of ')'.
_sxs_clean = $(shell v='$(strip $(subst v,,$(subst V,,$(1))))'; \
  IFS='.'; set -- $$v; \
  p1=$${1:-0}; p2=$${2:-0}; p3=$${3:-0}; p4=$${4:-0}; \
  ok=1; \
  for x in "$$p1" "$$p2" "$$p3" "$$p4"; do \
    if printf '%s' "$$x" | grep -qvE '^[0-9]+$$'; then ok=0; fi; \
  done; \
  if [ "$$ok" = 0 ]; then echo 0.0.0.0; exit 0; fi; \
  [ "$$p1" -gt 65535 ] && p1=65535; \
  [ "$$p2" -gt 65535 ] && p2=65535; \
  [ "$$p3" -gt 65535 ] && p3=65535; \
  [ "$$p4" -gt 65535 ] && p4=65535; \
  echo "$$p1.$$p2.$$p3.$$p4")

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
        deps-check check-go-env vet \
        ffmpeg libvpx xmp sdl2 mugen winres install install-remote fetch-log appbundle \
        screenpack \
        clean distclean FORCE

# ============================================================================
# Default Target
# ============================================================================

all: release

# ============================================================================
# Release Build
# ============================================================================

release: check-go-env deps-check xmp libvpx ffmpeg sdl2 $(BINARY)
	@echo "==> Build successful"
	@echo "    Binary: $(BINARY)"

# ============================================================================
# Convenience Targets
# ============================================================================

debug:
	$(MAKE) release config=debug

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

# --- Go toolchain management ---
# Minimum Go required to build (go.mod declares go 1.20; Ubuntu's golang-go is
# 1.18, so we auto-install a modern toolchain on Linux instead of failing).
#
# On Linux, `make check-go-env` installs the LATEST Go release from go.dev into
# $(GO_INSTALL_DIR) whenever `go` is missing or older than $(GO_MIN_VERSION):
#   - version is resolved dynamically from https://go.dev/VERSION (fallback:
#     $(GO_VERSION) if that query fails)
#   - `sudo` is used automatically when the install dir is not writable
#   - override GO_INSTALL_DIR to skip sudo (e.g. a user-writable path)
GO_MIN_VERSION  ?= 1.22
GO_VERSION      ?= go1.26.5
GO_INSTALL_DIR  ?= /usr/local/go

check-go-env:
	@case "$(HOST_OS)" in \
		linux) \
			ver=""; \
			GO_CMD="$$(command -v go || true)"; \
			if [ -n "$$GO_CMD" ]; then \
				ver="$$($$GO_CMD version 2>/dev/null | awk '{print $$3}' | sed 's/^go//' || true)"; \
			fi; \
			if [ -n "$$ver" ] && [ "$$(printf '%s\n' "$$ver" "$(GO_MIN_VERSION)" | sort -V | head -1)" = "$(GO_MIN_VERSION)" ]; then \
				echo "    Go $$ver found (>= $(GO_MIN_VERSION))"; \
			else \
				if [ -z "$$ver" ]; then \
					echo "==> Go toolchain not found on PATH."; \
				else \
					echo "==> Go $$ver is older than the required $(GO_MIN_VERSION)."; \
				fi; \
				echo "==> Installing latest Go from https://go.dev/dl/ to $(GO_INSTALL_DIR) ..."; \
				latest="$$(curl -fsSL --max-time 20 'https://go.dev/VERSION?m=text' 2>/dev/null | head -1 || true)"; \
				[ -n "$$latest" ] || latest="$(GO_VERSION)"; \
				url="https://go.dev/dl/$${latest}.linux-$(GOARCH).tar.gz"; \
				echo "==> Downloading $$url ..."; \
				tmp="$(BUILDDIR)/go-install"; \
				rm -rf "$$tmp"; mkdir -p "$$tmp"; \
				( wget -q "$$url" -O "$$tmp/go.tgz" 2>/dev/null || curl -fsSL "$$url" -o "$$tmp/go.tgz" ) || { \
					echo "ERROR: failed to download $$url" >&2; exit 1; }; \
				dest="$(dir $(GO_INSTALL_DIR))"; \
				sudo_cmd=""; [ -w "$$dest" ] || sudo_cmd="sudo"; \
				if [ -d "$(GO_INSTALL_DIR)" ]; then $$sudo_cmd rm -rf "$(GO_INSTALL_DIR)"; fi; \
				$$sudo_cmd tar -C "$$dest" -xzf "$$tmp/go.tgz" || { \
					echo "ERROR: failed to extract Go into $$dest (try 'sudo', or set GO_INSTALL_DIR to a writable path)" >&2; \
					exit 1; }; \
				if [ -d "$$dest/go" ] && [ "$(GO_INSTALL_DIR)" != "$${dest%/}/go" ]; then \
					$$sudo_cmd mv "$$dest/go" "$(GO_INSTALL_DIR)"; \
				fi; \
				rm -rf "$$tmp"; \
				echo "==> Installed $$latest to $(GO_INSTALL_DIR)"; \
				ver="$$("$(GO_INSTALL_DIR)"/bin/go version 2>/dev/null | awk '{print $$3}' | sed 's/^go//' || true)"; \
				if [ -z "$$ver" ] || [ "$$(printf '%s\n' "$$ver" "$(GO_MIN_VERSION)" | sort -V | head -1)" != "$(GO_MIN_VERSION)" ]; then \
					echo "ERROR: Go install failed — '$(GO_INSTALL_DIR)/bin/go version' reports '$$ver'." >&2; \
					exit 1; \
				fi; \
				echo "    Go $$ver ready (>= $(GO_MIN_VERSION))"; \
			fi;; \
		*) \
			go version >/dev/null 2>&1 || { \
				echo "ERROR: 'go version' failed — install Go >= $(GO_MIN_VERSION) from https://go.dev/dl/." >&2; \
				exit 1; }; \
			echo "    Go $$(go version | awk '{print $$3}') found";; \
	esac

# ============================================================================
# Go Vet — static analysis of the engine sources
# ============================================================================
# Runs `go vet` with the same build tags as the current configuration
# (GO_TAGS, e.g. `static desktop` on Windows; `armdevice` on Linux arm64).
# The stdlib `arena` package requires GOEXPERIMENT=arenas. Override the
# tags with TAGS=..., e.g. `make vet TAGS="mugen static"` for the mugen
# build (no `desktop` tag — the mugen variant uses `mugen` instead).
#
# NOTE: vet needs the local cgo headers, so run it after the external
# libraries are built (`make sdl2 ffmpeg xmp` or any normal build).
vet: check-go-env
	@echo "==> Running go vet on ./src (tags: $(GO_TAGS))..."
	@# CGO_CFLAGS carries the local FFmpeg/XMP include dirs (resolved via
	@# pkg-config against the exported PKG_CONFIG_LIBDIR/PKG_CONFIG_PATH) so
	@# the cgo preambles in packages/reisen and sound_xm.go can find headers.
	@GOEXPERIMENT=arenas CGO_CFLAGS="$$( $(PKG_CONFIG) --cflags $(_CGO_PKGS) 2>/dev/null || true )" \
		go vet -tags "$(GO_TAGS)" ./src
	@echo "==> go vet passed"

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
  # "MSYS Makefiles" is correct inside a real MSYS2 shell (its make/sh
  # understand the /c/... paths cmake emits there). Native Windows make
  # (w64devkit) runs under a POSIX-only ash that cannot execute /c/...
  # paths, so "MinGW Makefiles" + mingw32-make is the right generator for
  # that environment (same WIN_TOOLCHAIN guard as the PATH export above).
  ifeq ($(WIN_TOOLCHAIN),msys2)
    SDL2_CMAKE_GENERATOR := -G "MSYS Makefiles" -DWIN32=TRUE
  else
    SDL2_CMAKE_GENERATOR := -G "MinGW Makefiles" -DWIN32=TRUE
  endif
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

# Dynamic/shared SDL2 build flags — used only by the Linux fallback (the sdl2
# target builds a shared libSDL2.so from source when no system SDL2 is found).
SDL2_CMAKE_FLAGS_SHARED := \
	$(filter-out -DBUILD_SHARED_LIBS=OFF -DSDL_SHARED=OFF -DSDL_STATIC=ON,$(SDL2_CMAKE_FLAGS)) \
	-DBUILD_SHARED_LIBS=ON \
	-DSDL_SHARED=ON \
	-DSDL_STATIC=OFF

# SDL2 build rule — one recipe serves both library types, selected by $@:
#   libSDL2.a   static — Windows (also feeds the arch-specific archive)
#   libSDL2.so  shared — Linux fallback when no system SDL2 is installed
# NOTE: No ifeq/else/endif around targets — GNU Make 4.2.1 + .ONESHELL
# peeks at tab-indented lines inside false conditionals and chokes.
$(BUILD_PREFIX)/lib/libSDL2.a $(BUILD_PREFIX)/lib/libSDL2.so:
	@case "$@" in \
		*.so) \
			echo "==> Building dynamic SDL2 for $(HOST_OS)..."; \
			sdl2_flags="$(SDL2_CMAKE_FLAGS_SHARED)";; \
		*) \
			echo "==> Building static SDL2 for $(HOST_OS)..."; \
			sdl2_flags="$(SDL2_CMAKE_FLAGS)";; \
	esac
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
		# SDL's zip contains symlinks (android-project-ant/ -> ../android-project)
		# that w64devkit's busybox unzip cannot create on Windows (needs the
		# SeCreateSymbolicLinkPrivilege), aborting extraction. Exclude that
		# legacy Android-Ant dir — not needed for a desktop build.
		unzip -q "$(BUILDDIR)/SDL2.zip" -d "$$tmp" -x '*/android-project-ant/*'
		subdir="$$(find "$$tmp" -mindepth 1 -maxdepth 1 -type d | head -1)"
		rm -rf "$(SDL2_SRCDIR)"
		mkdir -p "$(SDL2_SRCDIR)"
		cp -a "$$subdir"/. "$(SDL2_SRCDIR)"/
		rm -rf "$$tmp" "$(BUILDDIR)/SDL2.zip"
	fi
	cmake -S "$(SDL2_SRCDIR)" -B "$(SDL2_BUILDDIR)" \
		$(SDL2_CMAKE_GENERATOR) \
		$$sdl2_flags
	cmake --build "$(SDL2_BUILDDIR)" --parallel
	cmake --install "$(SDL2_BUILDDIR)"
	case "$(HOST_OS)" in \
		windows) \
			sed -i 's/-lSDL2\b/-l:libSDL2.a/g' "$(BUILD_PREFIX)/lib/pkgconfig/sdl2.pc"; \
			rm -f "$(BUILD_PREFIX)/lib/libSDL2.dll.a"; \
			cp "$(BUILD_PREFIX)/lib/libSDL2.a" "$(BUILD_PREFIX)/lib/libSDL2_windows_$(GOARCH).a"; \
			cp "$(BUILD_PREFIX)/lib/libSDL2main.a" "$(BUILD_PREFIX)/lib/libSDL2main_windows_$(GOARCH).a";; \
	esac
	@echo "==> SDL2 installed to: $(BUILD_PREFIX)"

# On Windows the Go linker references the arch-specific archive directly
# (packages/go-sdl2/sdl/sdl_cgo_static.go: -lSDL2_windows_$(GOARCH)), but it
# is only produced as a side effect of the $(BUILD_PREFIX)/lib/libSDL2.a rule
# above. Declaring an explicit rule lets any target that depends on the binary
# (e.g. `install`, `binary`, `install-remote`) trigger the SDL2 build
# automatically, without requiring `sdl2` to have been run first. Defined
# unconditionally with a shell guard, following the .ONESHELL + Make 4.2.1
# conditional limitation used by the winres targets.
$(BUILD_PREFIX)/lib/libSDL2_windows_$(GOARCH).a: $(BUILD_PREFIX)/lib/libSDL2.a
	@[ "$(HOST_OS)" = "windows" ] || exit 0
	cp "$(BUILD_PREFIX)/lib/libSDL2.a" "$@"

# SDL2 policy:
#   Windows: static SDL2 built from source.
#   Linux:   system SDL2 via pkg-config; if not installed, build a dynamic
#            libSDL2.so from source (fallback).
#   macOS:   system SDL2 via pkg-config (Homebrew); error if missing.
sdl2:
	@case "$(HOST_OS)" in \
		windows) \
			$(MAKE) -s $(BUILD_PREFIX)/lib/libSDL2.a; \
			echo "    Local SDL2 $$(PKG_CONFIG_PATH="$(BUILD_PREFIX)/lib/pkgconfig" $(PKG_CONFIG) --modversion sdl2) found";; \
		linux) \
			if pkg-config --exists sdl2; then \
				echo "    SDL2 $$($(PKG_CONFIG) --modversion sdl2) found (system or local)"; \
			else \
				echo "==> System SDL2 not found — building dynamic SDL2 from source..."; \
				$(MAKE) -s $(BUILD_PREFIX)/lib/libSDL2.so; \
				echo "    Local SDL2 $$(PKG_CONFIG_PATH="$(BUILD_PREFIX)/lib/pkgconfig" $(PKG_CONFIG) --modversion sdl2) found"; \
			fi;; \
		*) \
			pkg-config --exists sdl2 || { \
				echo "ERROR: SDL2 development library not found." >&2; \
				echo "  Install with: brew install sdl2" >&2; \
				exit 1; \
			}; \
			echo "    System SDL2 $$($(PKG_CONFIG) --modversion sdl2) found";; \
	esac

# ============================================================================
# libvpx Static Library Build (VP8/VP9 decoder for WebM alpha)
# URL defined above as $(LIBVPX_URL)
# Decoder-only static build — no encoders/tools/docs, ~1.5 MB, linked into
# libavcodec for WebM alpha (second VP8/VP9 payload). Mirrors
# tools/build.sh:build_libvpx (v1.15.2) like libxmp's static CMake build.
# ============================================================================

ifeq ($(GOOS)_$(GOARCH),windows_amd64)
  LIBVPX_TARGET := x86_64-win64-gcc
else ifeq ($(GOOS)_$(GOARCH),windows_386)
  LIBVPX_TARGET := x86-win32-gcc
else ifeq ($(GOOS)_$(GOARCH),darwin_amd64)
  LIBVPX_TARGET := x86_64-darwin20-gcc
else ifeq ($(GOOS)_$(GOARCH),darwin_arm64)
  LIBVPX_TARGET := arm64-darwin20-gcc
else ifeq ($(GOOS)_$(GOARCH),linux_amd64)
  LIBVPX_TARGET := x86_64-linux-gcc
else ifeq ($(GOOS)_$(GOARCH),linux_arm64)
  LIBVPX_TARGET := arm64-linux-gcc
else
  LIBVPX_TARGET := generic-gnu
endif
ifeq ($(LIBVPX_TARGET),generic-gnu)
  LIBVPX_TARGET_OPT :=
else
  LIBVPX_TARGET_OPT := --target="$(LIBVPX_TARGET)"
endif

libvpx: $(LIBVPX_LIB)
	@echo "    libvpx $$(PKG_CONFIG_PATH="$(BUILD_PREFIX)/lib/pkgconfig" $(PKG_CONFIG) --modversion vpx 2>/dev/null || echo v1.15.2) found"

$(LIBVPX_LIB):
	@echo "==> Building static libvpx for $(HOST_OS) ($(GOOS)_$(GOARCH) -> $(LIBVPX_TARGET))..."
	mkdir -p $(BUILDDIR)
	if [ ! -d "$(LIBVPX_SRCDIR)" ]; then \
		echo "==> Downloading $(LIBVPX_URL)..."; \
		if [ ! -f "$(BUILDDIR)/libvpx.zip" ]; then \
			wget -q "$(LIBVPX_URL)" -O "$(BUILDDIR)/libvpx.zip"; \
		else \
			echo "==> Using existing zip: $(BUILDDIR)/libvpx.zip"; \
		fi; \
		tmp="$(BUILDDIR)/libvpx.zip-extract"; \
		rm -rf "$$tmp"; \
		mkdir -p "$$tmp"; \
		unzip -q "$(BUILDDIR)/libvpx.zip" -d "$$tmp"; \
		subdir="$$(find "$$tmp" -mindepth 1 -maxdepth 1 -type d | head -1)"; \
		rm -rf "$(LIBVPX_SRCDIR)"; \
		mkdir -p "$(LIBVPX_SRCDIR)"; \
		cp -a "$$subdir"/. "$(LIBVPX_SRCDIR)"/; \
		rm -rf "$$tmp" "$(BUILDDIR)/libvpx.zip"; \
	fi
	mkdir -p $(LIBVPX_BUILDDIR)
	cd $(LIBVPX_SRCDIR) && \
		CC="$(CC)" CXX="$(CXX)" ./configure \
			--prefix="$(BUILD_PREFIX)" \
			$(LIBVPX_TARGET_OPT) \
			--enable-pic \
			--enable-static --disable-shared \
			--disable-examples --disable-tools --disable-docs --disable-unit-tests \
			--disable-install-bins --disable-install-docs \
			--disable-vp8-encoder --disable-vp9-encoder \
			--enable-vp8-decoder --enable-vp9-decoder \
			--disable-webm-io && \
		make -j2 && \
		make install
	@test -f "$(BUILD_PREFIX)/lib/libvpx.a" && \
		test -f "$(BUILD_PREFIX)/lib/pkgconfig/vpx.pc" || \
		{ echo "ERROR: libvpx install failed — $(BUILD_PREFIX)/lib/libvpx.a or vpx.pc missing" >&2; exit 1; }
	@echo "==> libvpx static library installed to: $(BUILD_PREFIX)"

# ============================================================================
# FFmpeg Static Build (autotools)
# URL defined above as $(FFMPEG_URL)
# ============================================================================

# x86 assembler for FFmpeg's standalone asm (swscale/avcodec SIMD).
# MSYS2 ships nasm (preferred); w64devkit ships neither, but yasm is
# commonly on PATH (e.g. from an Android NDK install) and FFmpeg accepts it
# via --x86asmexe. Prefer nasm, fall back to yasm; if neither is available
# FFmpeg is configured with --disable-x86asm (still builds, just no SIMD).
# This probe runs on every platform, so Linux/macOS also pass --x86asmexe
# explicitly (and would use yasm if nasm were absent) — same result as
# FFmpeg's own auto-detection, just explicit. Override with
# `make X86ASM=</path/to/nasm|yasm>` (empty disables x86asm).
X86ASM := $(shell command -v nasm 2>/dev/null || command -v yasm 2>/dev/null)
# --disable-x86asm is required when building for ARM64 or without an assembler.
NO_X86ASM := $(if $(or $(filter arm64,$(HOST_ARCH)),$(X86ASM)),,--disable-x86asm)

ffmpeg: $(FFMPEG_LIBS)
	@echo "    FFmpeg $$(PKG_CONFIG_PATH="$(BUILD_PREFIX)/lib/pkgconfig" $(PKG_CONFIG) --modversion libavformat) found (libvpx $$(PKG_CONFIG_PATH="$(BUILD_PREFIX)/lib/pkgconfig" $(PKG_CONFIG) --modversion vpx 2>/dev/null || echo none))"

$(FFMPEG_LIBS) &: $(LIBVPX_LIB)
	@echo "==> Building static FFmpeg for $(HOST_OS) (x86 asm: $(if $(X86ASM),$(X86ASM),disabled))..."
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
		cp -a "$$subdir"/. "$(FFMPEG_SRCDIR)"/
		rm -rf "$$tmp" "$(BUILDDIR)/FFmpeg.zip"
	fi
	@# Wrap nasm to silence deprecated $ hex warning (FFmpeg n7.1 yuv2yuvX.asm:128)
	@# nasm 2.16+ warns on `$0x` style; FFmpeg uses it via x86inc.asm macros. Wrapper adds -w.
	if [ -n "$(X86ASM)" ] && echo "$(X86ASM)" | grep -q nasm; then \
		mkdir -p "$(BUILD_PREFIX)/bin"; \
		printf '#!/bin/sh\nexec nasm -w-number-deprecated-hex "$$@"\n' > "$(BUILD_PREFIX)/bin/nasm-wrapper"; \
		chmod +x "$(BUILD_PREFIX)/bin/nasm-wrapper"; \
		X86ASM_WRAPPER="$(BUILD_PREFIX)/bin/nasm-wrapper"; \
	else \
		X86ASM_WRAPPER="$(X86ASM)"; \
	fi
	@# FFmpeg's configure is a plain POSIX sh script. Run it under `sh` so it
	@# works everywhere: MSYS2/Linux/macOS sh is bash/dash (FFmpeg supports
	@# both), and w64devkit's busybox ash handles it fine too — while `bash`
	@# itself does not exist in w64devkit.
	cd "$(FFMPEG_SRCDIR)" && \
		sh ./configure \
			--prefix="$(BUILD_PREFIX)" \
			$(if $(filter windows,$(HOST_OS)),--target-os=mingw32,) \
			--enable-static --disable-shared \
			--disable-gpl --disable-nonfree \
			--disable-debug --disable-doc --disable-programs --disable-everything \
			--disable-autodetect --disable-avdevice --disable-pthreads \
			$(if $(NO_X86ASM),--disable-x86asm,) \
			$(if $(X86ASM),--x86asmexe="$$X86ASM_WRAPPER",) \
			--enable-avformat --enable-avcodec --enable-avutil \
			--enable-swresample --enable-swscale \
			--enable-avfilter --enable-filter=buffer,buffersink,format,scale,pad,crop \
			--enable-protocol=file \
			--enable-demuxer=matroska,webm \
			--enable-libvpx \
			--enable-decoder=libvpx_vp8,libvpx_vp9,opus,vorbis \
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
		cp -a "$$subdir"/. "$(XMP_SRCDIR)"/
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

# Forwarding phony — `make binary` still works as before. check-go-env runs
# first so a missing/old Go on Linux is auto-installed instead of failing at
# the `go version` guard in the $(BINARY) recipe; sdl2 ensures SDL2 is present
# (system lib or dynamic-from-source on Linux) before the cgo link.
.PHONY: binary
binary: check-go-env sdl2 $(BINARY)

# Real file target — only rebuilds when Go sources, libraries, or resources
# have actually changed.  The `go build` command leverages Go's own build
# cache so unchanged packages are not recompiled.
$(BINARY): $(GO_SOURCES) $(XMP_LIB) $(FFMPEG_LIBS)
	@go version >/dev/null 2>&1 || \
		{ echo "ERROR: 'go version' failed. Run 'make check-go-env' to check/auto-install the Go toolchain." >&2; exit 1; }
	@# The stdlib 'arena' package only compiles with GOEXPERIMENT=arenas (Go 1.20+).
	@# Probe at run time: the toolchain on PATH here is the one check-go-env
	@# ensured, which may differ from the toolchain seen at parse time.
	@_GOEXPERIMENT=$$( GOEXPERIMENT=arenas go env GOEXPERIMENT 2>/dev/null | grep -q arenas && echo arenas || true ); \
	echo "    GOEXPERIMENT=$${_GOEXPERIMENT:-<none>}"
	@echo "==> Building $(BINNAME) ($(config), GOOS=$(GOOS) GOARCH=$(GOARCH))..."
	@echo "    Go build tags: $(GO_TAGS) LDFLAGS: $(LDFLAGS_GO) CGO_CFLAGS: $(CGO_CFLAGS) CGO_LDFLAGS: $(CGO_LDFLAGS)"
	case "$(HOST_OS)" in \
		windows) \
			_PC_WINPATH="$$(cygpath -m "$(BUILD_PREFIX)/lib/pkgconfig" 2>/dev/null || echo "$(BUILD_PREFIX)/lib/pkgconfig")" ; \
			_CGO_CFLAGS=$$( $(PKG_CONFIG) --with-path="$${_PC_WINPATH}" --cflags $(_CGO_PKGS) ) ; \
			_CGO_LDFLAGS="-L$(BUILD_PREFIX)/lib $$( $(PKG_CONFIG) --with-path="$${_PC_WINPATH}" --static --libs $(_CGO_PKGS) )" ; \
			GOEXPERIMENT="$$_GOEXPERIMENT" \
			CGO_CFLAGS="-DLIBXMP_STATIC $$_CGO_CFLAGS" \
			CGO_LDFLAGS="$$_CGO_LDFLAGS" \
			go build $(GO_VERBOSE) -trimpath -tags "$(GO_TAGS)" \
				-ldflags "$(LDFLAGS_GO)" \
				-o "$(BINARY)" ./src;; \
		*) \
			_CGO_CFLAGS=$$( PKG_CONFIG_LIBDIR= PKG_CONFIG_PATH="$(BUILD_PREFIX)/lib/pkgconfig:/usr/lib/pkgconfig:/usr/local/lib/pkgconfig$(if $(PKG_CONFIG_PATH),:$(PKG_CONFIG_PATH),)" $(PKG_CONFIG) --cflags $(_CGO_PKGS) ) ; \
			_CGO_LDFLAGS="-L$(BUILD_PREFIX)/lib $$( PKG_CONFIG_LIBDIR= PKG_CONFIG_PATH="$(BUILD_PREFIX)/lib/pkgconfig:/usr/lib/pkgconfig:/usr/local/lib/pkgconfig$(if $(PKG_CONFIG_PATH),:$(PKG_CONFIG_PATH),)" $(PKG_CONFIG) --static --libs $(_CGO_PKGS) )" ; \
			PKG_CONFIG_PATH="$(BUILD_PREFIX)/lib/pkgconfig:/usr/lib/pkgconfig:/usr/local/lib/pkgconfig$(if $(PKG_CONFIG_PATH),:$(PKG_CONFIG_PATH),)" \
			GOEXPERIMENT="$$_GOEXPERIMENT" \
			CGO_CFLAGS="-DLIBXMP_STATIC $$_CGO_CFLAGS" \
			CGO_LDFLAGS="$$_CGO_LDFLAGS" \
			go build $(GO_VERBOSE) -trimpath -tags "$(GO_TAGS)" \
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
# Mugen Build — drop-in engine for existing M.U.G.E.N game folders.
#   - Go build tag: mugen  (embedded assets + data/mugen.cfg config import)
#   - No FFmpeg/XMP dependencies: only SDL2 is linked. Module music
#     (.xm/.mod/.it/.s3m) and stage video backgrounds are stubbed out.
#   - Engine assets (data/external/font) are packaged into src/assets.zip and
#     embedded into the binary; on first run inside a Mugen game folder they
#     are extracted automatically.
# Usage: make mugen
# ============================================================================
MUGEN_BINNAME := Mugen_GO$(BINEXT)
MUGEN_BINARY  := $(OUTDIR)/$(MUGEN_BINNAME)
ASSETS_ZIP    := src/assets.zip
# Mugen builds are their own variant: `mugen` replaces the `desktop` tag
# (motif_desktop.go is desktop-only), so strip `desktop` from GO_TAGS.
MUGEN_GO_TAGS := mugen $(filter-out desktop,$(GO_TAGS))

mugen: check-go-env sdl2 $(ASSETS_ZIP) $(MUGEN_BINARY)
	@echo "==> Mugen build successful"
	@echo "    Binary: $(MUGEN_BINARY)"

$(MUGEN_BINARY): $(GO_SOURCES) $(ASSETS_ZIP)
	@go version >/dev/null 2>&1 || \
		{ echo "ERROR: 'go version' failed. Run 'make check-go-env' to check/auto-install the Go toolchain." >&2; exit 1; }
	@_GOEXPERIMENT=$$( GOEXPERIMENT=arenas go env GOEXPERIMENT 2>/dev/null | grep -q arenas && echo arenas || true ); \
	echo "    GOEXPERIMENT=$${_GOEXPERIMENT:-<none>}"
	@echo "==> Building $(MUGEN_BINNAME) ($(config), GOOS=$(GOOS) GOARCH=$(GOARCH)) [mugen]..."
	@echo "    Go build tags: $(MUGEN_GO_TAGS)"
	case "$(HOST_OS)" in \
		windows) \
			GOEXPERIMENT="$$_GOEXPERIMENT" \
			CGO_CFLAGS="" \
			CGO_LDFLAGS="-L$(BUILD_PREFIX)/lib" \
			go build $(GO_VERBOSE) -trimpath -tags "$(MUGEN_GO_TAGS)" \
				-ldflags "$(LDFLAGS_GO)" \
				-o "$(MUGEN_BINARY)" ./src;; \
		*) \
			_CGO_CFLAGS=$$( PKG_CONFIG_LIBDIR= PKG_CONFIG_PATH="$(BUILD_PREFIX)/lib/pkgconfig:/usr/lib/pkgconfig:/usr/local/lib/pkgconfig$(if $(PKG_CONFIG_PATH),:$(PKG_CONFIG_PATH),)" $(PKG_CONFIG) --cflags sdl2 ) ; \
			CGO_LDFLAGS="-L$(BUILD_PREFIX)/lib $$( PKG_CONFIG_LIBDIR= PKG_CONFIG_PATH="$(BUILD_PREFIX)/lib/pkgconfig:/usr/lib/pkgconfig:/usr/local/lib/pkgconfig$(if $(PKG_CONFIG_PATH),:$(PKG_CONFIG_PATH),)" $(PKG_CONFIG) --static --libs sdl2 )" ; \
			PKG_CONFIG_PATH="$(BUILD_PREFIX)/lib/pkgconfig:/usr/lib/pkgconfig:/usr/local/lib/pkgconfig$(if $(PKG_CONFIG_PATH),:$(PKG_CONFIG_PATH),)" \
			GOEXPERIMENT="$$_GOEXPERIMENT" \
			CGO_CFLAGS="$$_CGO_CFLAGS" \
			CGO_LDFLAGS="$$_CGO_LDFLAGS" \
			go build $(GO_VERBOSE) -trimpath -tags "$(MUGEN_GO_TAGS)" \
				-ldflags "$(LDFLAGS_GO)" \
				-o "$(MUGEN_BINARY)" ./src;; \
	esac
	@rm -f $(SRC_SYSO) 2>/dev/null || true

# On Windows the mugen binary also needs the local static SDL2 archive and the
# resource .syso object (icon + manifest + version info).
ifeq ($(HOST_OS),windows)
$(MUGEN_BINARY): $(BUILD_PREFIX)/lib/libSDL2_windows_$(GOARCH).a $(SRC_SYSO)
endif

# Package engine assets (data/external/font) for embedding into the mugen
# binary. Requires `zip`; falls back to `python -m zipfile` when unavailable.
$(ASSETS_ZIP): data/* external/* font/*
	@echo "==> Packaging engine assets into $(ASSETS_ZIP)..."
	rm -f $@
	if command -v zip >/dev/null 2>&1; then \
		zip -r -q $@ data external font; \
	else \
		python -m zipfile -c $@ data external font; \
	fi

# ============================================================================
# Install — assemble a runnable distribution
# ============================================================================
# Depends on `screenpack` which downloads/extracts the Elecbyte screenpack zip.
# Merges screenpack directories (chars, stages, sound, video, data, external,
# font) with engine data and copies the binary into $(INSTALLDIR).

install: check-go-env deps-check sdl2 screenpack $(BINARY)
	@echo "==> Installing to $(INSTALLDIR)/..."
	mkdir -p "$(INSTALLDIR)"
	@echo "==> Copying engine data: data font external"
	cp -r data font external "$(INSTALLDIR)/"
	@echo "==> Copying binary $(BINNAME)..."
	cp -f "$(BINARY)" "$(INSTALLDIR)/"
	@if [ "$(HOST_OS)" = "linux" ] && [ -f "$(BUILD_PREFIX)/lib/libSDL2.so" ]; then \
		echo "==> Copying dynamic SDL2 library (built from source)..."; \
		cp -f "$(BUILD_PREFIX)"/lib/libSDL2.so* "$(INSTALLDIR)/"; \
	fi
	@echo "==> Install complete: $(INSTALLDIR)/"

# ============================================================================
# Remote Deploy — copy the binary to a device over scp (opt-in)
# ============================================================================
# Separate from `install` so building for a remote host never happens by
# accident. `make install` is always the local deploy/ assembly; use this
# target explicitly when you want to push the binary to a device:
#   make install-remote REMOTE_HOST=ark@192.168.7.2 REMOTE_DIR=/home/ark/ikemen
# scp prompts for the password interactively.
REMOTE_HOST ?= ark@192.168.7.2
REMOTE_DIR  ?= /home/ark/ikemen

install-remote: check-go-env deps-check sdl2 $(BINARY)
	@echo "==> Deploying $(BINARY) to $(REMOTE_HOST):$(REMOTE_DIR)/..."
	scp "$(BINARY)" "$(REMOTE_HOST):$(REMOTE_DIR)/"
	@if [ "$(HOST_OS)" = "linux" ] && [ -f "$(BUILD_PREFIX)/lib/libSDL2.so" ]; then \
		echo "==> Deploying dynamic SDL2 library..."; \
		scp "$(BUILD_PREFIX)"/lib/libSDL2.so* "$(REMOTE_HOST):$(REMOTE_DIR)/"; \
	fi
	@echo "==> Deploy complete."

# ============================================================================
# Remote Log — pull ikemen.log from a device over scp (opt-in)
# ============================================================================
# Fetches the engine log file from the same REMOTE_HOST/REMOTE_DIR used by
# `install-remote`:
#   make fetch-log REMOTE_HOST=ark@rg351mp
# Pulls $(REMOTE_DIR)/ikemen.log into the repo root. Override REMOTE_LOG to
# fetch a different file.
REMOTE_LOG ?= ikemen.log

fetch-log:
	@echo "==> Pulling $(REMOTE_HOST):$(REMOTE_DIR)/$(REMOTE_LOG) ..."
	scp "$(REMOTE_HOST):$(REMOTE_DIR)/$(REMOTE_LOG)" "$(REMOTE_LOG)"
	@echo "==> Fetched $(REMOTE_LOG)"

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
		cp -a "$$subdir"/. "$(INSTALLDIR)"/
		rm -rf "$$tmp" "$(BUILDDIR)/screenpack.zip"
	fi
	@echo "==> Screenpack ready in $(INSTALLDIR)"

# ============================================================================
# Clean
# ============================================================================
# NOTE: build artifacts are per-platform (build/<GOOS>_<GOARCH>), so `clean`
# and `distclean` only remove the CURRENT platform's tree, leaving any other
# platforms' builds intact. Use `rm -rf build/` to wipe every platform.

clean:
	@echo "==> Cleaning build artifacts for $(GOOS)_$(GOARCH)..."
	rm -rf $(BUILDDIR) 2>/dev/null || true
	rm -rf $(APPDIR) 2>/dev/null || true
	@echo "==> Clean done."

distclean: clean
	@echo "==> Deep cleaning — removing build artifacts and external lib sources..."
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
	@echo 'Ikemen-GO Build — $(HOST_OS) ($(HOST_ARCH))$(if $(WIN_TOOLCHAIN), [$(WIN_TOOLCHAIN)],)'
	@echo ''
	@echo 'Targets:'
	@echo '  all / release  Build release binary'
	@echo '  debug          Debug build (console + memory instrumentation)'
	@echo '  ffmpeg         Build static FFmpeg libraries (with libvpx for WebM alpha)'
	@echo '  libvpx         Build static libvpx decoder (VP8/VP9 for WebM alpha)'
	@echo '  xmp            Build static XMP library'
	@echo '  sdl2           Verify/build SDL2: static from source on Windows;'
	@echo '                 system lib via pkg-config on Linux/macOS, with a'
	@echo '                 dynamic-from-source fallback on Linux'
	@echo '  screenpack     Clone/update Elecbyte screenpack'
	@echo '  install        Assemble runnable build in deploy/ (screenpack + binary)'
	@echo '  mugen          Drop-in engine for existing M.U.G.E.N game folders'
	@echo '                 (SDL2 only, no FFmpeg/XMP; embedded assets)'
	@echo '  install-remote  scp binary to a device (REMOTE_HOST/REMOTE_DIR, opt-in)'
	@echo '  fetch-log       scp ikemen.log from a device (REMOTE_HOST/REMOTE_DIR, opt-in)'
	@echo '  appbundle      Create macOS .app bundle (I.K.E.M.E.N-Go.app)'
	@echo '  clean          Remove current platform build dir (build/<GOOS>_<GOARCH>)'
	@echo '  distclean      Remove current platform build dir + deploy/'
	@echo '  deps-check     Verify required tools are installed'
	@echo '  check-go-env   Check Go version; on Linux auto-install latest Go'
	@echo '                 to /usr/local/go when missing or < 1.22'
	@echo '  vet            Run go vet on ./src (same tags as the build;'
	@echo '                 run after a build so cgo headers are available)'
	@echo '  help           Show this help'
	@echo ''
	@echo 'Build tree: artifacts are separated per platform:'
	@echo '  build/<GOOS>_<GOARCH>/   e.g. build/windows_amd64, build/linux_arm64'
	@echo '  Each platform keeps its own SDL2/FFmpeg/XMP libs, winres, and binary.'
	@echo '  make clean / distclean only touch the current platform; rm -rf build/ wipes all.'
	@echo ''
	@echo 'Options:'
	@echo '  ARCH=<arch>        Target architecture (default: native)'
	@echo '                       Windows: amd64 (default) or 386'
	@echo '                       Linux:   amd64 (default) or arm64'
	@echo '                       macOS:   arm64 (Apple Silicon) or amd64 (Intel)'
	@echo '  config=debug       Debug build + memory instrumentation (default: release)'
	@echo '                       (legacy uppercase CONFIG=debug is also accepted)'
	@echo '  APP_VERSION=X.Y    Set version string (default: nightly)'
	@echo '  APP_BUILDTIME=X    Set build timestamp'
	@echo '  VERBOSE=1          Verbose go build (-x -v): show every command and'
	@echo '                     which packages are recompiled vs. from cache'
	@echo '  TAGS=<tags>        Override build tags for make vet, e.g.'
	@echo '                     TAGS="mugen static" (default: current build tags)'
	@echo '  GO_VERSION=<ver>   Go release used by the Linux auto-installer'
	@echo '                     (default: latest from go.dev; fallback: go1.26.5)'
	@echo '  GO_INSTALL_DIR=<d> Install dir for auto-installed Go (default: /usr/local/go)'
	@echo '  GO_MIN_VERSION=<v> Min Go version required; below it on Linux the latest'
	@echo '                     Go is auto-installed (default: 1.22)'
	@echo '  X86ASM=<path>      x86 assembler for FFmpeg SIMD (default: nasm,'
	@echo '                     then yasm; if none found, x86asm is disabled)'
	@echo '  WIN_TOOLCHAIN=<t>  Force Windows toolchain branch: msys2 or w64devkit'
	@echo '                     (default: auto-detected)'
	@echo ''
	@echo 'Platform notes:'
	@echo '  SDL2: Static from source (Windows); system lib via pkg-config'
	@echo '        (Linux/macOS); dynamic from source fallback on Linux.'
	@echo '  FFmpeg/libvpx/XMP: Built from source on all platforms (libvpx decoder-only for WebM alpha).'
	@echo '  Windows: Fully static binary (no external DLLs at runtime).'
	@echo '  Linux:   SDL2/FFmpeg/XMP compiled in; system libs dynamic.'
	@echo '           Go < 1.22 auto-installs the latest Go to /usr/local/go'
	@echo '           (needs sudo, or set GO_INSTALL_DIR to a writable path)'
	@echo '  macOS:   SDL2/FFmpeg/XMP compiled in; system frameworks dynamic.'
	@echo ''
	@echo 'Examples:'
	@echo '  make                          # Native release'
	@echo '  make debug                    # Native debug'
	@echo '  make APP_VERSION=v1.0.0       # Tagged build'
	@echo '  make APP_VERSION=v1.0.0 config=debug'
	@echo '  make install                  # Build + assemble runnable deploy/'
	@echo '  rm -rf build/                 # Wipe ALL platform build trees'

