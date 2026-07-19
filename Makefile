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
#   make install            # Assemble runnable build in install/
#   make help               # Show all targets and options
#
# Prerequisites:
#   Windows (MSYS2 MINGW64 shell):
#     pacman -Syu --noconfirm
#     pacman -S --noconfirm git make mingw-w64-x86_64-pkg-config \
#       mingw-w64-x86_64-go mingw-w64-x86_64-toolchain \
#       mingw-w64-x86_64-nasm mingw-w64-x86_64-cmake
#     pacman -S --noconfirm wget unzip
#   Linux (Debian/Ubuntu):
#     sudo apt update && sudo apt install -y \
#       git make cmake pkg-config golang-go gcc g++ nasm \
#       wget unzip libx11-dev libxext-dev libxrandr-dev \
#       libxcursor-dev libxi-dev libxinerama-dev libxss-dev \
#       libxxf86vm-dev libasound2-dev libgl1-mesa-dev
#   macOS (Homebrew):
#     brew install git make cmake pkg-config go nasm wget \
#       molten-vk
# ============================================================================

SHELL       := /bin/bash
.SHELLFLAGS := -euo pipefail -c
.ONESHELL:

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
  export PATH    := /mingw64/bin:$(PATH)
  export GOROOT ?= /mingw64/lib/go
  # Default GOPATH for environments where it isn't set (MSYS2, CI, etc.)
  export GOPATH  ?= $(HOME)/go
  # Go build cache location — needed because %LocalAppData% may be unset
  # in non-interactive MSYS2 shells.
  export GOCACHE ?= $(HOME)/.cache/go-build
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
OUTDIR        := .
WINRES_DIR    := $(BUILDDIR)/winres

# External library source directories
SDL2_SRCDIR   := $(BUILDDIR)/SDL-release-2.32.10
FFMPEG_SRCDIR := $(BUILDDIR)/FFmpeg-n7.1
XMP_SRCDIR    := $(BUILDDIR)/libxmp-libxmp-4.7.1

# External library build directories (separate from source)
SDL2_BUILDDIR    := $(BUILDDIR)/build-sdl2
FFMPEG_BUILDDIR  := $(BUILDDIR)/build-ffmpeg
XMP_BUILDDIR     := $(BUILDDIR)/build-xmp

# FFmpeg static library targets
FFMPEG_LIBS := $(addprefix $(BUILD_PREFIX)/lib/, \
  libavformat.a libavcodec.a libavutil.a \
  libswscale.a libswresample.a libavfilter.a)

# Install directory
INSTALLDIR ?= install

# ============================================================================
# Toolchain
# ============================================================================

export GOEXPERIMENT := arenas
export CGO_ENABLED  := 1

PKG_CONFIG ?= pkg-config

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

STATIC_PKGS := libavformat libavcodec libavutil libswscale libswresample libavfilter libxmp

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
    GO_TAGS := -tags "static debug"
  else
    GO_TAGS := -tags static
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
    GO_TAGS := -tags debug
  else
    GO_TAGS :=
  endif
  EXTLDFLAGS :=
  ifeq ($(IS_DEBUG),1)
    LDFLAGS_GO := -extldflags '$(EXTLDFLAGS)'
  else
    LDFLAGS_GO := -s -w $(LDFLAGS_BASE) -extldflags '$(EXTLDFLAGS)'
  endif
endif

# ============================================================================
# SxS Version Sanitization (Windows only)
# ============================================================================

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

# ============================================================================
# Derived File Targets
# ============================================================================

BINARY   := $(OUTDIR)/$(BINNAME)

ifeq ($(HOST_OS),windows)
  SRC_SYSO := src/rsrc_windows.syso
endif

# ============================================================================
# Phony Targets
# ============================================================================

.PHONY: all release debug help \
        deps-check check-go-env \
        ffmpeg xmp sdl2 winres binary install \
        screenpack \
        android android-apk check-android-tools clean-android \
        clean distclean FORCE

# ============================================================================
# Default Target
# ============================================================================

all: release

# ============================================================================
# Release Build
# ============================================================================

release: deps-check xmp ffmpeg sdl2 binary
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
	for tool in git make cmake pkg-config gcc g++ nasm go unzip wget; do \
		command -v $$tool >/dev/null 2>&1 || missing="$$missing $$tool"; \
	done; \
	if [ -n "$$missing" ]; then \
		echo "ERROR: Missing tools:$$missing" >&2; \
		case "$(HOST_OS)" in \
			windows) \
				echo "Install from the MINGW64 shell:" >&2; \
				echo "  pacman -Syu --noconfirm" >&2; \
				echo "  pacman -S --noconfirm git make mingw-w64-x86_64-pkg-config \\" >&2; \
				echo "    mingw-w64-x86_64-go mingw-w64-x86_64-toolchain \\" >&2; \
				echo "    mingw-w64-x86_64-nasm mingw-w64-x86_64-cmake" >&2; \
				echo "  pacman -S --noconfirm wget unzip" >&2;; \
			linux) \
				echo "Install (Debian/Ubuntu):" >&2; \
				echo "  sudo apt update && sudo apt install -y \\" >&2; \
				echo "    git make cmake pkg-config golang-go gcc g++ nasm \\" >&2; \
				echo "    wget unzip" >&2;; \
			darwin) \
				echo "Install with Homebrew:" >&2; \
				echo "  brew install git make cmake pkg-config go nasm wget" >&2;; \
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
# source: https://github.com/libsdl-org/SDL/archive/refs/tags/release-2.32.10.zip
# ============================================================================

SDL2_SOURCE := https://github.com/libsdl-org/SDL/archive/refs/tags/release-2.32.10.zip

# SDL2 CMake flags — platform-specific
SDL2_CMAKE_FLAGS := \
	-DCMAKE_INSTALL_PREFIX="$(BUILD_PREFIX)" \
	-DBUILD_SHARED_LIBS=OFF \
	-DSDL_SHARED=OFF \
	-DSDL_STATIC=ON \
	-DSDL_TEST=OFF \
	-DSDL_TESTS=OFF \
	-DSDL_INSTALL_TESTS=OFF

ifeq ($(HOST_OS),windows)
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

sdl2: $(BUILD_PREFIX)/lib/libSDL2.a
	@# Post-install steps per platform:
	@# Windows: copy .a to arch-specific name expected by sdl_cgo_static.go.
	@# Linux/macOS: patch sdl2.pc to include private deps so pkg-config (used
	@# by sdl_cgo.go with !static tag) produces a complete link line.
	@case "$(HOST_OS)" in \
		windows) \
			# Patch sdl2.pc to use the static library directly.
			sed -i 's/-lSDL2\b/-l:libSDL2.a/g' "$(BUILD_PREFIX)/lib/pkgconfig/sdl2.pc"; \
			# Remove shared import lib so downstream doesn't pull SDL2.dll.
			rm -f "$(BUILD_PREFIX)/lib/libSDL2.dll.a"; \
			# Copy with arch-specific names expected by sdl_cgo_static.go.
			cp "$(BUILD_PREFIX)/lib/libSDL2.a" "$(BUILD_PREFIX)/lib/libSDL2_windows_$(GOARCH).a"; \
			cp "$(BUILD_PREFIX)/lib/libSDL2main.a" "$(BUILD_PREFIX)/lib/libSDL2main_windows_$(GOARCH).a";; \
		linux|darwin) \
			# CGo runs 'pkg-config --libs sdl2' (without --static). Move
			# Libs.private into Libs so the link line is complete.
			pc="$(BUILD_PREFIX)/lib/pkgconfig/sdl2.pc"; \
			priv="$$(grep '^Libs.private:' "$$pc" | sed 's/^Libs.private: *//')"; \
			if [ -n "$$priv" ]; then \
				sed -i "s/^\(Libs:.*\)/\1 $$priv/" "$$pc"; \
				sed -i '/^Libs.private:/d' "$$pc"; \
			fi;; \
	esac

$(BUILD_PREFIX)/lib/libSDL2.a:
	@echo "==> Building static SDL2 for $(HOST_OS)..."
	mkdir -p $(BUILDDIR)
	if [ ! -d "$(SDL2_SRCDIR)" ]; then \
		echo "==> Downloading SDL2 source ..."; \
		wget "$(SDL2_SOURCE)" -O "$(BUILDDIR)/SDL-release-2.32.10.zip"; \
		unzip "$(BUILDDIR)/SDL-release-2.32.10.zip" -d "$(BUILDDIR)"; \
	fi
	cmake -S "$(SDL2_SRCDIR)" -B "$(SDL2_BUILDDIR)" \
		$(SDL2_CMAKE_FLAGS)
	cmake --build "$(SDL2_BUILDDIR)" --parallel
	cmake --install "$(SDL2_BUILDDIR)"
	@echo "==> SDL2 static library installed to: $(BUILD_PREFIX)"

# ============================================================================
# FFmpeg Static Build (autotools)
# source: https://github.com/FFmpeg/FFmpeg/archive/refs/tags/n7.1.zip
# ============================================================================

FFMPEG_SOURCE := https://github.com/FFmpeg/FFmpeg/archive/refs/tags/n7.1.zip

ffmpeg: $(FFMPEG_LIBS)

$(FFMPEG_LIBS):
	@echo "==> Building static FFmpeg for $(HOST_OS)..."
	mkdir -p $(BUILDDIR)
	if [ ! -d "$(FFMPEG_SRCDIR)" ]; then \
		echo "==> Downloading FFmpeg source ..."; \
		wget "$(FFMPEG_SOURCE)" -O "$(BUILDDIR)/FFmpeg-n7.1.zip"; \
		unzip "$(BUILDDIR)/FFmpeg-n7.1.zip" -d "$(BUILDDIR)"; \
	fi
	cd $(FFMPEG_SRCDIR) && \
		./configure \
			--prefix="$(BUILD_PREFIX)" \
			--enable-static --disable-shared \
			--disable-gpl --disable-nonfree \
			--disable-debug --disable-doc --disable-programs --disable-everything \
			--disable-autodetect --disable-avdevice --disable-pthreads \
			--enable-avformat --enable-avcodec --enable-avutil \
			--enable-swresample --enable-swscale \
			--enable-avfilter --enable-filter=buffer,buffersink,format,scale,pad,crop \
			--enable-protocol=file \
			--enable-demuxer=matroska,webm \
			--enable-decoder=vp8,vp9,opus,vorbis \
			--enable-parser=vp8,vp9,opus,vorbis \
			--cc="$(CC)" \
			--pkg-config="$$(which pkg-config)" && \
		make -j"$$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 2)" && \
		make install
	@echo "==> FFmpeg static libraries installed to: $(BUILD_PREFIX)"

# ============================================================================
# XMP Static Library Build (CMake)
# source: https://github.com/libxmp/libxmp/archive/refs/tags/libxmp-4.7.1.zip
# ============================================================================

XMP_SOURCE := https://github.com/libxmp/libxmp/archive/refs/tags/libxmp-4.7.1.zip
XMP_LIB    := $(BUILD_PREFIX)/lib/libxmp.a

xmp: $(XMP_LIB)
	# Remove any shared import lib (Windows-specific, no-op elsewhere).
	rm -f "$(BUILD_PREFIX)/lib/libxmp.dll.a"

$(XMP_LIB):
	@echo "==> Building static libxmp for $(HOST_OS)..."
	mkdir -p $(BUILDDIR)
	if [ ! -d "$(XMP_SRCDIR)" ]; then \
		echo "==> Downloading XMP source ..."; \
		wget "$(XMP_SOURCE)" -O "$(BUILDDIR)/libxmp-4.7.1.zip"; \
		unzip "$(BUILDDIR)/libxmp-4.7.1.zip" -d "$(BUILDDIR)"; \
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

ifeq ($(HOST_OS),windows)

# FORCE ensures the .rc is regenerated on every build so version info is fresh.
$(WINRES_DIR)/Ikemen_GO.rc: FORCE
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

.PHONY: winres
winres: $(SRC_SYSO)

$(SRC_SYSO): $(WINRES_DIR)/Ikemen_GO.rc
	@echo "==> Embedding Windows resources (icon + manifest)..."
	mkdir -p src
	"$(WINDRES)" --use-temp-file --target=$(WRTARGET) \
		-I $(WINRES_DIR) -I external/icons \
		-i $(WINRES_DIR)/Ikemen_GO.rc \
		-O coff -o $@

endif # HOST_OS == windows

# ============================================================================
# Go Binary
# ============================================================================

binary: ffmpeg xmp sdl2 check-go-env
ifeq ($(HOST_OS),windows)
binary: winres
endif
	@echo "==> Building $(BINNAME) ($(CONFIG), GOOS=$(GOOS) GOARCH=$(GOARCH))..."
	@# Windows: pkgconf.exe splits paths on ';' and mishandles ':' (drive
	@# letters), so pin PKG_CONFIG_LIBDIR to ONLY our local dir — this both
	@# fixes the separator issue and blocks the system MSYS2 FFmpeg (ggml,
	@# whisper, shaderc, ...) from leaking into the static link line.
	@# Linux/macOS: prepend our dir to PKG_CONFIG_PATH (Unix ':' separator).
ifeq ($(HOST_OS),windows)
	@export PKG_CONFIG_LIBDIR="$(BUILD_PREFIX)/lib/pkgconfig"; \
	CGO_CFLAGS="-DLIBXMP_STATIC $$( $(PKG_CONFIG) --cflags $(STATIC_PKGS) )" \
	CGO_LDFLAGS="-L$(BUILD_PREFIX)/lib \
		$$( $(PKG_CONFIG) --static --libs $(STATIC_PKGS) )" \
	go build -trimpath -v $(GO_TAGS) \
		-ldflags "$(LDFLAGS_GO)" \
		-o "$(BINARY)" ./src
else
	@export PKG_CONFIG_PATH="$(BUILD_PREFIX)/lib/pkgconfig$(if $(PKG_CONFIG_PATH),:$(PKG_CONFIG_PATH),)"; \
	CGO_CFLAGS="-DLIBXMP_STATIC $$( $(PKG_CONFIG) --cflags $(STATIC_PKGS) )" \
	CGO_LDFLAGS="-L$(BUILD_PREFIX)/lib \
		$$( $(PKG_CONFIG) --static --libs $(STATIC_PKGS) )" \
	go build -trimpath -v $(GO_TAGS) \
		-ldflags "$(LDFLAGS_GO)" \
		-o "$(BINARY)" ./src
endif
	@# Clean up Windows resource object after build
	rm -f $(SRC_SYSO) 2>/dev/null || true

# ============================================================================
# Install — assemble a runnable distribution
# ============================================================================
# Depends on `screenpack` which clones/updates the Elecbyte screenpack git repo.
# Merges screenpack assets (chars, stages, sound, video) with engine data
# (data, font, external) and copies the binary into $(INSTALLDIR).

install: deps-check screenpack binary
	@echo "==> Installing to $(INSTALLDIR)/..."
	rm -rf "$(INSTALLDIR)"
	mkdir -p "$(INSTALLDIR)"
	@echo "==> Copying engine data: data font external"
	cp -r data font external "$(INSTALLDIR)/"
	@echo "==> Merging screenpack assets on top: chars stages sound video data"
	for d in chars stages sound video data; do \
		if [ -d "$(SCREENPACK_DIR)/$$d" ]; then \
			mkdir -p "$(INSTALLDIR)/$$d"; \
			cp -r "$(SCREENPACK_DIR)/$$d/." "$(INSTALLDIR)/$$d/" 2>/dev/null || true; \
		fi; \
	done
	@echo "==> Copying binary $(BINNAME)..."
	cp -f "$(BINARY)" "$(INSTALLDIR)/"
	@echo "==> Install complete: $(INSTALLDIR)/"

# ============================================================================
# Screenpack Clone / Update
# ============================================================================

SCREENPACK_REPO ?= https://github.com/ikemen-engine/Ikemen-GO-Screenpack.git
SCREENPACK_REF  ?= master
SCREENPACK_DIR   = $(BUILDDIR)/screenpack

screenpack:
	@echo "==> Ensuring Elecbyte screenpack..."
	mkdir -p $(BUILDDIR)
	if [ ! -d "$(SCREENPACK_DIR)/.git" ]; then
		rm -rf $(SCREENPACK_DIR)
		git clone --depth=1 -b "$(SCREENPACK_REF)" "$(SCREENPACK_REPO)" "$(SCREENPACK_DIR)"
	else
		cd "$(SCREENPACK_DIR)" && \
			git fetch --depth=1 origin "$(SCREENPACK_REF)" && \
			git checkout -f FETCH_HEAD
	fi
	@echo "==> Screenpack ready in $(SCREENPACK_DIR)"

# ============================================================================
# Android — arm64 shared library (libmain.so) via NDK + Go c-shared
# ============================================================================
# Builds android/app/libs/arm64-v8a/libmain.so, loaded by the Android app
# (Java/JNI) at runtime. Uses the Android NDK's clang cross-compilers and a
# prebuilt SDL2 for android-30 living under $(ANDROID_DEPS_PATH).
#
# Prerequisites (one-time host setup):
#   - Android NDK r21+          (e.g. sdkmanager "ndk;27.1.12297006")
#   - Android platform API 30   (sdkmanager "platforms;android-30")
#   - JDK 8 recommended for the surrounding Gradle/APK build
#   - SDL2 built for arm64-v8a / android-30 and installed into
#     $(ANDROID_DEPS_PATH) (lib/, lib/pkgconfig/, include/, include/SDL2/):
#       git clone --depth 1 --branch SDL2 https://github.com/libsdl-org/SDL.git SDL2
#       cd SDL2 && mkdir build-android && cd build-android
#       cmake -G "Unix Makefiles" .. \
#         -DCMAKE_TOOLCHAIN_FILE="$(ANDROID_NDK_HOME)/build/cmake/android.toolchain.cmake" \
#         -DANDROID_ABI="arm64-v8a" -DANDROID_PLATFORM=android-30 \
#         -DCMAKE_INSTALL_PREFIX="$(ANDROID_DEPS_PATH)" \
#         -DSDL_ANDROID_PACKAGE_NAME=org.ikemen_engine.ikemen_go \
#         -DSDL_STATIC=OFF -DSDL_SHARED=ON -DCMAKE_BUILD_TYPE=Release \
#         -DCMAKE_SHARED_LINKER_FLAGS="-Wl,-z,max-page-size=16384"
#       cmake --build . -- -j8 && cmake --build . --target install
#
# Override paths on the command line if your NDK lives elsewhere, e.g.:
#   make android ANDROID_NDK_HOME=/path/to/ndk
#
# Note: the android target is self-contained — it sets its own GOOS/GOARCH/CC
# via target-specific assignments and does NOT use the native SDL2/FFmpeg/XMP
# static libs built by `make release`.

# --- Android configuration (override on the command line as needed) ---------
ANDROID_NDK_HOME  ?= C:/Android/SDK/ndk/27.1.12297006
ANDROID_DEPS_PATH ?= $(abspath $(BUILDDIR)/android-deps)
ANDROID_HOST_TAG  ?= windows-x86_64
ANDROID_API       ?= 30
ANDROID_TARGET    := aarch64-linux-android$(ANDROID_API)
ANDROID_TOOLCHAIN := $(ANDROID_NDK_HOME)/toolchains/llvm/prebuilt/$(ANDROID_HOST_TAG)
ANDROID_CC        := $(ANDROID_TOOLCHAIN)/bin/$(ANDROID_TARGET)-clang
ANDROID_CXX       := $(ANDROID_TOOLCHAIN)/bin/$(ANDROID_TARGET)-clang++
ANDROID_OUTDIR    := android/app/libs/arm64-v8a
ANDROID_BINARY    := $(ANDROID_OUTDIR)/libmain.so

# --- Android APK packaging (ikemen-droid Gradle project) --------------------
# The APK is produced from the external ikemen-droid app project (Gradle +
# Java/JNI wrapper). ANDROID_APK_REPO may be a git URL (default) or a local
# path to an existing checkout. ANDROID_SDK_ROOT must point at an Android SDK
# with cmdline-tools + platform-tools installed (Gradle needs it).
ANDROID_APK_REPO ?= https://github.com/Jesuszilla/ikemen-droid.git
ANDROID_APK_REF  ?= main
ANDROID_APK_DIR  ?= $(abspath $(BUILDDIR)/android-apk/ikemen-droid)
ANDROID_APK_OUT  ?= $(abspath bin/ikemen-go.apk)
ANDROID_SDK_ROOT ?= $(ANDROID_HOME)
# gamecontrollerdb.txt is referenced by the Android app manifest.
ANDROID_GCDB_URL ?= https://raw.githubusercontent.com/mdqinc/SDL_GameControllerDB/refs/heads/master/gamecontrollerdb.txt

android:
	@echo "==> Building Android shared library (libmain.so, arm64-v8a)..."
	@if [ ! -d "$(ANDROID_TOOLCHAIN)" ]; then \
		echo "ERROR: NDK toolchain not found: $(ANDROID_TOOLCHAIN)" >&2; \
		echo "  Set ANDROID_NDK_HOME (and ANDROID_HOST_TAG for non-Windows hosts)." >&2; \
		exit 1; \
	fi
	@if [ ! -d "$(ANDROID_DEPS_PATH)/lib" ]; then \
		echo "ERROR: SDL2 android deps not found: $(ANDROID_DEPS_PATH)" >&2; \
		echo "  Build SDL2 for android-30 first (see comments above the android target)." >&2; \
		exit 1; \
	fi
	mkdir -p $(ANDROID_OUTDIR)
	CGO_ENABLED=1 GOOS=android GOARCH=arm64 GOEXPERIMENT=arenas \
	CC="$(ANDROID_CC)" CXX="$(ANDROID_CXX)" \
	PKG_CONFIG_LIBDIR="$(ANDROID_DEPS_PATH)/lib/pkgconfig" \
	PKG_CONFIG_SYSROOT_DIR="$(ANDROID_DEPS_PATH)" \
	PKG_CONFIG_PATH= \
	CGO_CFLAGS="-I$(ANDROID_DEPS_PATH)/include -I$(ANDROID_DEPS_PATH)/include/SDL2" \
	CGO_LDFLAGS="-L$(ANDROID_DEPS_PATH)/lib -lSDL2 -lGLESv2 -lOpenSLES -llog -Wl,-z,max-page-size=16384" \
	go build -buildmode=c-shared -trimpath -v -tags "mugen lite android gles2" \
		-ldflags "-s -w $(LDFLAGS_BASE) -X 'runtime.godebugDefault=asyncpreemptoff=1,sigaltstack=0'" \
		-o "$(ANDROID_BINARY)" ./src
	@echo "==> Android build successful: $(ANDROID_BINARY)"

# --- Verify the host has everything needed for a full APK build -------------
check-android-tools:
	@echo "==> Checking Android APK build tools..."
	@ok=1; \
	if [ ! -d "$(ANDROID_TOOLCHAIN)" ]; then \
		echo "  [X] NDK toolchain not found: $(ANDROID_TOOLCHAIN)" >&2; \
		echo "      Set ANDROID_NDK_HOME (install via: sdkmanager \"ndk;27.1.12297006\")." >&2; \
		ok=0; \
	else echo "  [ok] NDK toolchain: $(ANDROID_TOOLCHAIN)"; fi; \
	if [ ! -d "$(ANDROID_DEPS_PATH)/lib" ]; then \
		echo "  [X] SDL2 android deps not found: $(ANDROID_DEPS_PATH)" >&2; \
		echo "      Build SDL2 for android-30 first (see comments above the android target)." >&2; \
		ok=0; \
	else echo "  [ok] Android deps:  $(ANDROID_DEPS_PATH)"; fi; \
	sdk="$(ANDROID_SDK_ROOT)"; \
	if [ -z "$$sdk" ]; then sdk="$(ANDROID_HOME)"; fi; \
	if [ -z "$$sdk" ] || [ ! -d "$$sdk" ]; then \
		echo "  [X] Android SDK not found (set ANDROID_SDK_ROOT or ANDROID_HOME)." >&2; \
		echo "      Needs cmdline-tools + platform-tools + platforms;android-$(ANDROID_API) + build-tools." >&2; \
		ok=0; \
	else echo "  [ok] Android SDK:   $$sdk"; fi; \
	if ! command -v java >/dev/null 2>&1; then \
		echo "  [X] java not found on PATH (JDK 17 recommended; 8 for older AGP)." >&2; \
		ok=0; \
	else echo "  [ok] java:          $$(java -version 2>&1 | head -n1)"; fi; \
	for tool in git go; do \
		command -v $$tool >/dev/null 2>&1 || { echo "  [X] $$tool not found on PATH." >&2; ok=0; }; \
	done; \
	if [ "$$ok" != "1" ]; then \
		echo "ERROR: Missing Android APK prerequisites (see above)." >&2; \
		exit 1; \
	fi; \
	echo "    All Android APK tools found."

# --- Full APK build: lib + ikemen-droid Gradle project (no Docker) ----------
# Clones/updates the ikemen-droid app project, stages libmain.so + dep .so
# files into jniLibs/, stages engine assets per the app manifest.txt (with
# screenpack fallback), then runs Gradle 'assembleDebug' and copies the APK to
# $(ANDROID_APK_OUT). Requires the Android SDK (ANDROID_SDK_ROOT) + a JDK.
android-apk: check-android-tools android screenpack
	@echo "==> Building Android APK (no Docker)..."
	@# Resolve the SDK root (ANDROID_SDK_ROOT preferred, else ANDROID_HOME).
	sdk="$(ANDROID_SDK_ROOT)"; [ -n "$$sdk" ] || sdk="$(ANDROID_HOME)"; \
	echo "==> Using Android SDK: $$sdk"; \
	\
	echo "==> Syncing ikemen-droid ($(ANDROID_APK_REF))..."; \
	mkdir -p "$$(dirname "$(ANDROID_APK_DIR)")"; \
	if [ -d "$(ANDROID_APK_REPO)" ]; then \
		src="$$(cd "$(ANDROID_APK_REPO)" && pwd -P)"; \
		if [ "$$src" != "$(ANDROID_APK_DIR)" ]; then \
			rm -rf "$(ANDROID_APK_DIR)"; \
			echo "    Using local checkout: $$src"; \
			cp -a "$$src" "$(ANDROID_APK_DIR)"; \
		fi; \
	elif [ ! -d "$(ANDROID_APK_DIR)/.git" ]; then \
		rm -rf "$(ANDROID_APK_DIR)"; \
		git clone --depth=1 -b "$(ANDROID_APK_REF)" "$(ANDROID_APK_REPO)" "$(ANDROID_APK_DIR)"; \
	else \
		( cd "$(ANDROID_APK_DIR)" && \
			git fetch --depth=1 origin "$(ANDROID_APK_REF)" && \
			git checkout -f FETCH_HEAD ); \
	fi; \
	git config --global --add safe.directory "$(ANDROID_APK_DIR)" >/dev/null 2>&1 || true; \
	\
	echo "==> Ensuring runtime assets referenced by the app manifest..."; \
	if [ ! -f "external/gamecontrollerdb.txt" ]; then \
		echo "    Downloading gamecontrollerdb.txt..."; \
		wget -q "$(ANDROID_GCDB_URL)" -O "external/gamecontrollerdb.txt"; \
	fi; \
	if [ ! -f "data/system.base.def" ]; then \
		echo "    Generating data/system.base.def from defaultMotif.ini..."; \
		mkdir -p data; \
		cp -a "src/resources/defaultMotif.ini" "data/system.base.def"; \
	fi; \
	\
	app_dir="$(ANDROID_APK_DIR)/app"; \
	abi_dir="$$app_dir/src/main/jniLibs/arm64-v8a"; \
	echo "==> Staging native libs into: $$abi_dir"; \
	mkdir -p "$$abi_dir"; \
	rm -f "$$abi_dir"/*.so* 2>/dev/null || true; \
	cp -av "$(abspath $(ANDROID_BINARY))" "$$abi_dir/"; \
	cp -av "$(ANDROID_DEPS_PATH)/lib/"*.so* "$$abi_dir/" 2>/dev/null || true; \
	\
	assets_dir="$$app_dir/src/main/assets"; \
	manifest="$$assets_dir/manifest.txt"; \
	if [ ! -f "$$manifest" ]; then \
		echo "ERROR: ikemen-droid manifest not found at: $$manifest" >&2; exit 1; \
	fi; \
	echo "==> Staging assets into: $$assets_dir (from manifest.txt)"; \
	find "$$assets_dir" -mindepth 1 -maxdepth 1 ! -name "manifest.txt" -exec rm -rf {} + 2>/dev/null || true; \
	for p in $$(tr -s '[:space:]' ' ' < "$$manifest"); do \
		[ -z "$$p" ] && continue; \
		src="$(CURDIR)/$$p"; dst="$$assets_dir/$$p"; \
		if [ ! -e "$$src" ] && [ -e "$(SCREENPACK_DIR)/$$p" ]; then src="$(SCREENPACK_DIR)/$$p"; fi; \
		if [ -d "$$src" ]; then \
			mkdir -p "$$dst"; cp -a "$$src/." "$$dst/" 2>/dev/null || true; \
		elif [ -f "$$src" ]; then \
			mkdir -p "$$(dirname "$$dst")"; cp -a "$$src" "$$dst" 2>/dev/null || true; \
		else \
			echo "    WARNING: asset path missing: $$p" >&2; \
		fi; \
	done; \
	\
	echo "==> Running Gradle (assembleDebug)..."; \
	( cd "$(ANDROID_APK_DIR)" && \
		printf "sdk.dir=%s\n" "$$sdk" > local.properties && \
		chmod +x ./gradlew 2>/dev/null || true; \
		./gradlew --no-daemon clean assembleDebug ); \
	apk_src="$(ANDROID_APK_DIR)/app/build/outputs/apk/debug/app-debug.apk"; \
	if [ ! -f "$$apk_src" ]; then \
		echo "ERROR: Gradle finished but APK not found at: $$apk_src" >&2; exit 1; \
	fi; \
	mkdir -p "$$(dirname "$(ANDROID_APK_OUT)")"; \
	cp -av "$$apk_src" "$(ANDROID_APK_OUT)"; \
	echo "==> APK ready: $(ANDROID_APK_OUT)"

clean-android:
	@echo "==> Cleaning Android artifacts..."
	rm -f $(ANDROID_BINARY) 2>/dev/null || true
	rm -f $(ANDROID_OUTDIR)/libmain.h 2>/dev/null || true
	rm -f $(ANDROID_APK_OUT) 2>/dev/null || true
	@echo "==> Android clean done."

# ============================================================================
# Clean
# ============================================================================

clean:
	@echo "==> Cleaning build artifacts..."
	rm -f $(BINARY) 2>/dev/null || true
	rm -f $(OUTDIR)/Ikemen_GO* 2>/dev/null || true
	rm -f $(SRC_SYSO) 2>/dev/null || true
	rm -rf $(WINRES_DIR) 2>/dev/null || true
	rm -f $(ANDROID_BINARY) $(ANDROID_OUTDIR)/libmain.h 2>/dev/null || true
	@echo "==> Clean done."

distclean: clean
	@echo "==> Deep cleaning..."
	rm -rf $(FFMPEG_SRCDIR) 2>/dev/null || true
	rm -rf $(FFMPEG_BUILDDIR) 2>/dev/null || true
	rm -rf $(BUILD_PREFIX) 2>/dev/null || true
	rm -rf $(XMP_SRCDIR) 2>/dev/null || true
	rm -rf $(XMP_BUILDDIR) 2>/dev/null || true
	rm -rf $(SDL2_SRCDIR) 2>/dev/null || true
	rm -rf $(SDL2_BUILDDIR) 2>/dev/null || true
	rm -rf $(SCREENPACK_DIR) 2>/dev/null || true
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
	@echo '  sdl2           Build static SDL2 library'
	@echo '  screenpack     Clone/update Elecbyte screenpack'
	@echo '  install        Assemble runnable build in install/ (screenpack + binary)'
	@echo '  android        Build Android arm64 shared library (libmain.so)'
	@echo '  android-apk    Build full Android APK (no Docker; needs SDK+JDK+NDK)'
	@echo '  check-android-tools  Verify Android APK build prerequisites'
	@echo '  clean-android  Remove Android build artifacts'
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
	@echo '  ANDROID_NDK_HOME=  Path to Android NDK (for the android target)'
	@echo '  ANDROID_DEPS_PATH= Path to prebuilt SDL2 android deps (default: build/android-deps)'
	@echo '  ANDROID_SDK_ROOT=  Path to Android SDK (for android-apk; else uses ANDROID_HOME)'
	@echo '  ANDROID_APK_REPO=  ikemen-droid git URL or local path (for android-apk)'
	@echo '  ANDROID_APK_OUT=   Output APK path (default: bin/ikemen-go.apk)'
	@echo ''
	@echo 'Platform notes:'
	@echo '  SDL2, FFmpeg, and XMP are built from source on all platforms.'
	@echo '  Windows: Fully static binary (no external DLLs at runtime).'
	@echo '  Linux:   SDL2/FFmpeg/XMP compiled in; system libs dynamic.'
	@echo '  macOS:   SDL2/FFmpeg/XMP compiled in; system frameworks dynamic.'
	@echo ''
	@echo 'Examples:'
	@echo '  make                          # Native release'
	@echo '  make debug                    # Native debug'
	@echo '  make APP_VERSION=v1.0.0       # Tagged build'
	@echo '  make APP_VERSION=v1.0.0 CONFIG=debug'
	@echo '  make android                  # Android arm64 libmain.so'
	@echo '  make android-apk              # Full Android APK (no Docker)'
