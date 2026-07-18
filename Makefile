# ============================================================================
# Ikemen-GO — Combined Windows Makefile (MSYS2 / MinGW64)
#
# Merged from deepseek-flash, qwen3.7-plus, and mimo Makefiles.
# Covers Win64 (default) and Win32 builds.
#
# Usage:
#   make                    # Win64 release build (default)
#   make debug              # Win64 debug build -> Ikemen_GO_debug.exe (console + -tags debug memory instrumentation)
#   make win32              # Win32 (x86) build
#   make IKEMEN_CPP=1       # C++ backend build
#   make clean              # Remove build artifacts
#   make distclean          # Remove build artifacts + FFmpeg + screenpack
#   make screenpack         # Clone/update Elecbyte screenpack
#   make ffmpeg             # Build/check FFmpeg dependencies
#   make install             # Assemble runnable build in install/ (screenpack + binary)
#   make help               # Show all targets and options
#
# Prerequisites (MSYS2 MINGW64 shell):
#   pacman -Syu --noconfirm
#   pacman -S --noconfirm git make diffutils mingw-w64-x86_64-pkg-config \
#     mingw-w64-x86_64-go mingw-w64-x86_64-toolchain \
#     mingw-w64-x86_64-nasm mingw-w64-x86_64-yasm \
#     mingw-w64-x86_64-tools-git mingw-w64-x86_64-libxmp \
#     mingw-w64-x86_64-SDL2
# ============================================================================

SHELL       := /bin/bash
.SHELLFLAGS := -euo pipefail -c
.ONESHELL:

# ─── MSYS2 / MinGW64 Environment ────────────────────────────────────────────
export PATH    := /mingw64/bin:$(PATH)
export GOROOT ?= /mingw64/lib/go

# ─── Project Metadata (overridable by CI) ───────────────────────────────────
APP_VERSION     ?= nightly
APP_BUILDTIME   ?= $(shell date '+%Y.%m.%d')
COPY_START_YEAR ?= 2016

# ─── Architecture Selection ──────────────────────────────────────────────────
ARCH ?= amd64

ifeq ($(ARCH),386)
  GOARCH       := 386
  CC           ?= i686-w64-mingw32-gcc
  CXX          ?= i686-w64-mingw32-g++
  BINNAME      := Ikemen_GO_x86.exe
  WRTARGET     := pe-i386
  ASM_ARCH     := x86
else
  GOARCH       := amd64
  CC           ?= x86_64-w64-mingw32-gcc
  CXX          ?= x86_64-w64-mingw32-g++
  ifeq ($(IKEMEN_CPP),1)
    BINNAME    := Ikemen_CPP.exe
  else
    BINNAME    := Ikemen_GO.exe
  endif
  WRTARGET     := pe-x86-64
  ASM_ARCH     := amd64
endif

GOOS := windows

# ─── Directories ─────────────────────────────────────────────────────────────
BUILDDIR      := build
BUILD_PREFIX := $(abspath $(BUILDDIR)/output)
FFMPEG_REV    ?= release/7.1
XMP_SRCDIR    := $(BUILDDIR)/xmp-src
OUTDIR        := .
WINRES_DIR    := $(BUILDDIR)/winres
# FFmpeg installs into the SAME tree as XMP ($(BUILD_PREFIX)), so a
# single PKG_CONFIG_PATH / -L covers both. Mirrors the proven static build.
FFMPEG_PREFIX := $(BUILD_PREFIX)
FFMPEG_LIBS   := $(addprefix $(BUILD_PREFIX)/lib/,libavformat.a libavcodec.a libavutil.a libswscale.a libswresample.a libavfilter.a)

# ─── Install / Screenpack Distribution ────────────────────────────────────────
INSTALLDIR         ?= install
SCREENPACK_TAG     ?= 20260715
SCREENPACK_URL     ?= https://github.com/leonkasovan/Ikemen-GO-Screenpack/archive/refs/tags/$(SCREENPACK_TAG).zip
SCREENPACK_DL      := $(BUILDDIR)/screenpack-$(SCREENPACK_TAG).zip
SCREENPACK_EXTRACT := Ikemen-GO-Screenpack-$(SCREENPACK_TAG)

# ─── Build Options ───────────────────────────────────────────────────────────
DEBUG_BUILD   ?= 0
IKEMEN_CPP    ?= 0

# ─── Toolchain ───────────────────────────────────────────────────────────────
export GOEXPERIMENT := arenas
export CGO_ENABLED  := 1
export GOOS GOARCH CC CXX

PKG_CONFIG ?= pkg-config
WINDRES     := $(shell command -v x86_64-w64-mingw32-windres 2>/dev/null || command -v windres 2>/dev/null || echo windres)

# ─── pkg-config Packages ─────────────────────────────────────────────────────
# Static libraries (FFmpeg + XMP) are linked directly into the binary.
# SDL2 is linked statically via -tags static (vendored _libs), so it is
# not a pkg-config dependency at link time.
STATIC_PKGS := libavformat libavcodec libavutil libswscale libswresample libavfilter libxmp

# Local FFmpeg/XMP install into $(BUILD_PREFIX)/lib/pkgconfig. The
# `binary`/`test*` recipes set PKG_CONFIG_PATH at run time to that dir
# first and /mingw64 last (fallback for the shared SDL2 implib), so
# pkg-config never resolves libav* to the system's full FFmpeg.
ifneq ($(wildcard $(BUILD_PREFIX)/lib/pkgconfig),)
  export PKG_CONFIG_PATH := $(BUILD_PREFIX)/lib/pkgconfig:$(PKG_CONFIG_PATH)
endif

# ─── SxS Version Sanitization ───────────────────────────────────────────────
# Strip leading v/V, keep digits+dots only, pad/clamp to 4 numeric parts.
_sxs_clean = $(shell v='$(strip $(subst v,,$(subst V,,$(1))))'; \
  [[ "$$v" =~ ^[0-9.]+$$ ]] || { echo "0.0.0.0"; exit; }; \
  IFS='.' read -ra p <<<"$$v"; \
  for ((i=$${#p[@]}; i<4; i++)); do p+=("0"); done; \
  out=(); for x in "$${p[@]:0:4}"; do \
    [[ "$$x" =~ ^[0-9]+$$ ]] || x=0; ((x<0)) && x=0; ((x>65535)) && x=65535; \
    out+=("$$x"); \
  done; IFS='.'; echo "$${out[*]}")
SXS_VERSION := $(call _sxs_clean,$(APP_VERSION))

_sxs_major := $(word 1,$(subst ., ,$(SXS_VERSION)))
_sxs_minor := $(word 2,$(subst ., ,$(SXS_VERSION)))
_sxs_patch := $(word 3,$(subst ., ,$(SXS_VERSION)))
_sxs_build := $(word 4,$(subst ., ,$(SXS_VERSION)))

BUILD_YEAR    := $(subst -,,$(firstword $(subst ., ,$(APP_BUILDTIME))))
APP_COPYRIGHT ?= (c) $(COPY_START_YEAR)-$(BUILD_YEAR) Ikemen GO team (MIT)

# ─── Go Build Flags ──────────────────────────────────────────────────────────
# The `static` Go build tag activates the vendored static SDL2 in
# packages/go-sdl2/sdl (sdl_cgo_static.go): it links -lSDL2_windows_amd64
# from packages/go-sdl2/_libs plus the Win32 deps (setupapi/imm32/version/
# oleaut32...), so SDL2 is compiled in — no SDL2.dll ships.
ifeq ($(IKEMEN_CPP),1)
  GO_TAGS := -tags ikemen_cpp static
else
  GO_TAGS := -tags static
endif
ifeq ($(DEBUG_BUILD),1)
  # The `debug` target enables the memory-analysis instrumentation (see
  # src/memdebug.go / src/memdebug_off.go, gated by the `debug` Go build tag).
  GO_TAGS += -tags debug
endif

# Fully static external link: -static pulls the MinGW runtime .a implibs
# (libwinpthread.a, libgcc_eh.a, libstdc++.a) so no libwinpthread-1.dll /
# libgcc_s_seh-1.dll / libstdc++-6.dll ships. --defsym aliases the old-MinGW
# __ms_vsscanf (referenced by the prebuilt static SDL2) to current __mingw_vsscanf.
EXTLDFLAGS := -static -Wl,--defsym,__ms_vsscanf=__mingw_vsscanf

# Version stamping is common to both builds.
LDFLAGS_BASE := \
  -X 'main.Version=$(APP_VERSION)' \
  -X 'main.BuildTime=$(APP_BUILDTIME)'

ifeq ($(DEBUG_BUILD),1)
  # Debug build: keep symbols/DWARF for debugging; console subsystem.
  LDFLAGS_GO := $(LDFLAGS_BASE) -extldflags '$(EXTLDFLAGS)'
  BUILD_TYPE := debug
else
  # Release build: GUI subsystem, strip symbols (-s) and DWARF (-w).
  LDFLAGS_GO := -H windowsgui -s -w $(LDFLAGS_BASE) -extldflags '$(EXTLDFLAGS)'
  BUILD_TYPE := release
endif

# ─── Derived File Targets ────────────────────────────────────────────────────
BINARY       := $(OUTDIR)/$(BINNAME)
SRC_SYSO     := src/rsrc_windows.syso

# ─── Phony Targets ───────────────────────────────────────────────────────────
.PHONY: all release debug win32 help \
        deps-check check-go-env \
        ffmpeg xmp winres binary install \
        screenpack \
        test test-debug test-bench \
        clean distclean FORCE

# ─── Default Target ──────────────────────────────────────────────────────────
all: release

release: deps-check xmp ffmpeg binary
	@echo "==> Build successful"
	@echo "    Binary: $(BINARY)"
	@echo "    Fully static: only Windows system DLLs at runtime."

# ===========================================================================
# Convenience Targets
# ===========================================================================

debug:
	$(MAKE) release DEBUG_BUILD=1 BINNAME=Ikemen_GO_debug.exe

# ===========================================================================
# Test — run Go tests with the same build environment as `make debug`.
# The GOEXPERIMENT=arenas flag is required because the engine uses Go's
# experimental arena package for rollback state cloning.
# ===========================================================================

.PHONY: test test-debug test-bench

# Static libs resolve against the LOCAL pkgconfig only (never the
# system FFmpeg). SDL2 is linked statically via -tags static (vendored
# _libs), so no shared SDL2 entry is needed here.
TEST_CGO = CGO_CFLAGS="$$( $(PKG_CONFIG) --cflags $(STATIC_PKGS) )" \
	CGO_LDFLAGS="-L$(BUILD_PREFIX)/lib \
		$$( $(PKG_CONFIG) --static --libs $(STATIC_PKGS) )"

test: deps-check check-go-env xmp ffmpeg
	@echo "==> Running unit tests..."
	IKEMEN_SKIP_DLL_CHECK=1 \
	PKG_CONFIG_PATH="$(BUILD_PREFIX)/lib/pkgconfig$(if $(PKG_CONFIG_PATH),:$(PKG_CONFIG_PATH),)" \
	$(TEST_CGO) \
	go test -v $(GO_TAGS) -count=1 ./src -run 'Test'

test-debug: deps-check check-go-env xmp ffmpeg
	@echo "==> Running unit tests with -tags debug (memory instrumentation)..."
	IKEMEN_SKIP_DLL_CHECK=1 \
	PKG_CONFIG_PATH="$(BUILD_PREFIX)/lib/pkgconfig$(if $(PKG_CONFIG_PATH),:$(PKG_CONFIG_PATH),)" \
	$(TEST_CGO) \
	go test -v $(GO_TAGS) -tags debug -count=1 ./src -run 'Test'

test-bench: deps-check check-go-env xmp ffmpeg
	@echo "==> Running benchmarks..."
	IKEMEN_SKIP_DLL_CHECK=1 \
	PKG_CONFIG_PATH="$(BUILD_PREFIX)/lib/pkgconfig$(if $(PKG_CONFIG_PATH),:$(PKG_CONFIG_PATH),)" \
	$(TEST_CGO) \
	go test -v $(GO_TAGS) -bench=. -benchmem -count=1 ./src

win32:
	$(MAKE) release ARCH=386

# ===========================================================================
# Dependency Checks
# ===========================================================================

deps-check:
	@echo "==> Checking build dependencies..."
	@missing=""; \
	for tool in git make pkg-config gcc g++ nasm go unzip; do \
		command -v $$tool >/dev/null 2>&1 || missing="$$missing $$tool"; \
	done; \
	if [ -n "$$missing" ]; then \
		echo "ERROR: Missing tools:$$missing" >&2; \
		echo "Install from the MINGW64 shell:" >&2; \
		echo "  pacman -Syu --noconfirm" >&2; \
		echo "  pacman -S --noconfirm git make diffutils mingw-w64-x86_64-pkg-config \\" >&2; \
		echo "    mingw-w64-x86_64-go mingw-w64-x86_64-toolchain \\" >&2; \
		echo "    mingw-w64-x86_64-nasm \\" >&2; \
		echo "    mingw-w64-x86_64-tools-git" >&2; \
		exit 1; \
	fi
	@# Safe path check for MSYS2/Cygwin
	echo "Safe path check: $(CURDIR)" >&2;
	@case "$$(uname -s 2>/dev/null)" in \
		MINGW* | MSYS* | CYGWIN*) \
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
		{ echo "ERROR: 'go version' failed. Set GOROOT=/mingw64/lib/go" >&2; exit 1; }

# SDL2 static build, installs into $(BUILD_PREFIX)
# source: https://github.com/libsdl-org/SDL/archive/refs/tags/release-2.32.10.zip

SDL2_SOURCE := https://github.com/libsdl-org/SDL/archive/refs/tags/release-2.32.10.zip
SDL2_SRCDIR := $(BUILDDIR)/SDL-release-2.32.10

sdl2: $(BUILD_PREFIX)/lib/libSDL2.a

$(BUILD_PREFIX)/lib/libSDL2.a:
	@echo "==> Building static SDL2 ..."
	mkdir -p $(BUILDDIR)
	if [ ! -d "$(SDL2_SRCDIR)" ]; then \
		echo "==> Downloading SDL2 source ..."; \
		wget "$(SDL2_SOURCE)" -O "$(BUILDDIR)/SDL-release-2.32.10.zip"; \
		unzip "$(BUILDDIR)/SDL-release-2.32.10.zip" -d "$(BUILDDIR)"; \
	fi
	cd $(SDL2_SRCDIR) && \
		./configure --prefix="$(BUILD_PREFIX)" --enable-static --disable-shared && \
		make -j"$$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 2)" && \
		make install
	@echo "==> SDL2 static library installed to: $(BUILD_PREFIX)"

# ===========================================================================
# FFmpeg Static Build
# ===========================================================================
# FFmpeg Static Build — installs into $(BUILD_PREFIX) (shared with XMP)
# source: https://github.com/FFmpeg/FFmpeg/archive/refs/tags/n7.1.zip
# ===========================================================================

FFMPEG_SOURCE := https://github.com/FFmpeg/FFmpeg/archive/refs/tags/n7.1.zip
FFMPEG_SRCDIR := $(BUILDDIR)/FFmpeg-n7.1

.PHONY: ffmpeg

ffmpeg: $(FFMPEG_LIBS)

$(FFMPEG_LIBS):
	@echo "==> Building static FFmpeg ..."
	mkdir -p $(BUILDDIR)
	if [ ! -d "$(FFMPEG_SRCDIR)" ]; then \
		echo "==> Downloading FFmpeg source ..."; \
		wget "$(FFMPEG_SOURCE)" -O "$(BUILDDIR)/FFmpeg-n7.1.zip"; \
		unzip "$(BUILDDIR)/FFmpeg-n7.1.zip" -d "$(BUILDDIR)"; \
	fi
	cd $(FFMPEG_SRCDIR) && \
		./configure \
			--prefix="$(BUILD_PREFIX)" \
			--install-name-dir=@rpath \
			--enable-static --disable-shared \
			--disable-gpl --disable-nonfree \
			--disable-debug --disable-doc --disable-programs --disable-everything \
			--disable-autodetect --disable-avdevice --disable-pthreads \
			--enable-avformat --enable-avcodec --enable-avutil --enable-swresample --enable-swscale \
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

# ===========================================================================
# Windows Resources (icon + manifest + version) — FORCE ensures fresh version
# ===========================================================================

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
	            VALUE "CompanyName", "Ikemen GO\\0"
	            VALUE "FileDescription", "Ikemen GO\\0"
	            VALUE "FileVersion", "$${SXS}\\0"
	            VALUE "ProductName", "Ikemen GO\\0"
	            VALUE "ProductVersion", "$${SXS}\\0"
	            VALUE "OriginalFilename", "$(BINNAME)\\0"
	            VALUE "InternalName", "Ikemen_GO\\0"
	            VALUE "BuildDate", "$(APP_BUILDTIME)\\0"
	            VALUE "LegalCopyright", "$${COPY}\\0"
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
	"$(WINDRES)" --target=$(WRTARGET) \
		-I $(WINRES_DIR) -I external/icons \
		-i $(WINRES_DIR)/Ikemen_GO.rc \
		-O coff -o $@

# ===========================================================================
# Go Binary — statically links FFmpeg, XMP, the MinGW runtime
# (pthread/gcc/stdc++), and SDL2 (vendored _libs). So no
# libwinpthread-1.dll, libgcc_s_seh-1.dll, libstdc++-6.dll, or
# SDL2.dll ship — `ldd $(BINARY)` lists only Windows system DLLs.
# ===========================================================================

binary: ffmpeg xmp check-go-env $(SRC_SYSO)
	@echo "==> Building $(BINNAME) ($(BUILD_TYPE), GOARCH=$(GOARCH))..."
	@export PKG_CONFIG_PATH="$(BUILD_PREFIX)/lib/pkgconfig$(if $(PKG_CONFIG_PATH),:$(PKG_CONFIG_PATH),)"; \
	CGO_CFLAGS="$$( $(PKG_CONFIG) --cflags $(STATIC_PKGS) )" \
	CGO_LDFLAGS="-L$(BUILD_PREFIX)/lib \
		$$( $(PKG_CONFIG) --static --libs $(STATIC_PKGS) )" \
	go build -trimpath -v $(GO_TAGS) \
		-ldflags "$(LDFLAGS_GO)" \
		-o "$(BINARY)" ./src
	rm -f $(SRC_SYSO)

# ===========================================================================
# Install — assemble a runnable distribution in $(INSTALLDIR)
# ===========================================================================

install: binary deps-check
	@echo "==> Installing to $(INSTALLDIR)/..."
	@if [ ! -f "$(SCREENPACK_DL)" ]; then \
		echo "==> Downloading screenpack (tag $(SCREENPACK_TAG))..."; \
		curl -L -o "$(SCREENPACK_DL)" "$(SCREENPACK_URL)" || { echo "ERROR: screenpack download failed." >&2; exit 1; }; \
	fi
	@echo "==> Extracting screenpack..."
	rm -rf "$(INSTALLDIR)/$(SCREENPACK_EXTRACT)"
	mkdir -p "$(INSTALLDIR)"
	unzip -o -q "$(SCREENPACK_DL)" -d "$(INSTALLDIR)"
	@echo "==> Copying screenpack dirs: chars data font sound stages video"
	for d in chars data font sound stages video; do \
		rm -rf "$(INSTALLDIR)/$$d"; \
		cp -r "$(INSTALLDIR)/$(SCREENPACK_EXTRACT)/$$d" "$(INSTALLDIR)/$$d"; \
	done
	@echo "==> Merging engine dirs from repo root: data font external"
	for d in data font external; do \
		cp -rT "$$d" "$(INSTALLDIR)/$$d"; \
	done
	@echo "==> Removing extracted screenpack folder..."
	rm -rf "$(INSTALLDIR)/$(SCREENPACK_EXTRACT)"
	@echo "==> Copying binary $(BINNAME)..."
	cp -f "$(BINARY)" "$(INSTALLDIR)/"
	@echo "==> Install complete: $(INSTALLDIR)/"

# ===========================================================================
# Screenpack Clone / Update
# ===========================================================================

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

# ===========================================================================
# Clean
# ===========================================================================

clean:
	@echo "==> Cleaning build artifacts..."
	rm -f $(BINARY) 2>/dev/null || true
	rm -f $(OUTDIR)/Ikemen_GO.exe $(OUTDIR)/Ikemen_GO_x86.exe $(OUTDIR)/Ikemen_CPP.exe $(OUTDIR)/Ikemen_GO_debug.exe 2>/dev/null || true
	rm -f $(SRC_SYSO) 2>/dev/null || true
	rm -rf $(WINRES_DIR) 2>/dev/null || true
	@echo "==> Clean done."

distclean: clean
	@echo "==> Deep cleaning..."
	rm -rf $(FFMPEG_SRCDIR) 2>/dev/null || true
	rm -rf $(FFMPEG_PREFIX) 2>/dev/null || true
	rm -rf $(XMP_SRCDIR) 2>/dev/null || true
	rm -rf $(SCREENPACK_DIR) 2>/dev/null || true
	@echo "==> Distclean done."

# ===========================================================================
# FORCE — ensures targets depending on it always rebuild
# ===========================================================================
FORCE:

# ===========================================================================
# Help
# ===========================================================================	help:
# 	@echo 'Ikemen-GO Windows Build (MSYS2 / MinGW64)'
# 	@echo ''
# 	@echo 'Targets:'
# 	@echo '  all / release  Win64 release build (default, GUI subsystem)'
# 	@echo '  debug          Win64 debug build -> Ikemen_GO_debug.exe (console + memory instrumentation)'
# 	@echo '  win32          Win32 (x86) build'
# 	@echo '  ffmpeg         Build static FFmpeg libraries'
# 	@echo '  xmp            Build static XMP library from local source'
# 	@echo '  screenpack     Clone/update Elecbyte screenpack'
# 	@echo '  install        Assemble runnable build in install/ (screenpack + binary)'
# 	@echo '  clean          Remove build artifacts'
# 	@echo '  distclean      Remove artifacts + FFmpeg + XMP + screenpack'
# 	@echo '  deps-check     Verify required tools are installed'
# 	@echo '  help           Show this help'
# 	@echo ''
# 	@echo 'Note: FFmpeg, XMP, the MinGW runtime, and SDL2 are ALL linked'
# 	@echo '      statically; the exe needs only Windows system DLLs at runtime.'
# 	@echo ''
# 	@echo 'Options:'
# 	@echo '  ARCH=386           Build 32-bit (default: amd64)'
# 	@echo '  DEBUG_BUILD=1      Debug build (console subsystem) + memory instrumentation'
# 	@echo '  IKEMEN_CPP=1       Enable C++ backend (Go build tags)'
# 	@echo '  APP_VERSION=X.Y    Set version string (default: nightly)'
# 	@echo '  APP_BUILDTIME=X    Set build timestamp'
# 	@echo ''
# 	@echo 'Examples:'
# 	@echo '  make                          # Win64 release'
# 	@echo '  make debug                    # Win64 debug'
# 	@echo '  make win32                    # Win32 release'
# 	@echo '  make IKEMEN_CPP=1             # C++ backend'
# 	@echo '  make APP_VERSION=v1.0.0       # Tagged build'
# 	@echo '  make APP_VERSION=v1.0.0 DEBUG_BUILD=1 IKEMEN_CPP=1'

# ===========================================================================
# XMP Static Library Build
# Builds libxmp from local source (build/xmp-src/) as a static library
# and creates a pkg-config file so the linker can find it.
# ===========================================================================

XMP_LIB  := $(BUILD_PREFIX)/lib/libxmp.a
XMP_PC   := $(BUILD_PREFIX)/lib/pkgconfig/libxmp.pc

xmp: $(XMP_LIB) $(XMP_PC)

$(XMP_LIB): $(XMP_SRCDIR)/Makefile
	@echo "==> Building libxmp statically from local source..."
	mkdir -p $(BUILD_PREFIX)
	cd $(XMP_SRCDIR) && \
		$(MAKE) CC="$(CC)" AR="$(AR)" RANLIB="$(RANLIB)" OUTPUT_DIR="$(BUILD_PREFIX)" && \
		$(MAKE) CC="$(CC)" AR="$(AR)" RANLIB="$(RANLIB)" OUTPUT_DIR="$(BUILD_PREFIX)" install

$(XMP_PC): $(XMP_LIB)
	@echo "==> Creating libxmp.pc for pkg-config..."
	mkdir -p $(BUILD_PREFIX)/lib/pkgconfig
	cat > $(XMP_PC) <<-PCEOF
	prefix=$(BUILD_PREFIX)
	exec_prefix=\$${prefix}
	libdir=\$${exec_prefix}/lib
	includedir=\$${prefix}/include

	Name: libxmp
	Description: XMP (Extended Module Player) static library
	Version: 4.6.2
	Libs: -L\$${libdir} -lxmp
	Libs.private: -lm
	Cflags: -I\$${includedir} -DLIBXMP_STATIC
	PCEOF
	@echo "==> libxmp.pc created."