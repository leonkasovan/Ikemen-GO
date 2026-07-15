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
#   make bundle             # Copy runtime DLLs to lib/
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
OUTDIR        := .
LIBDIR        := lib
DELAYLIB_DIR  := $(BUILDDIR)/delaylib
WINRES_DIR    := $(BUILDDIR)/winres
FFMPEG_SRCDIR := $(BUILDDIR)/ffmpeg-src
FFMPEG_PREFIX ?= $(CURDIR)/$(BUILDDIR)/ffmpeg
FFMPEG_REV    ?= release/7.1

# ─── Install / Screenpack Distribution ────────────────────────────────────────
INSTALLDIR         ?= install
SCREENPACK_TAG     ?= 20260715
SCREENPACK_URL     ?= https://github.com/leonkasovan/Ikemen-GO-Screenpack/archive/refs/tags/$(SCREENPACK_TAG).zip
SCREENPACK_DL      := $(BUILDDIR)/screenpack-$(SCREENPACK_TAG).zip
SCREENPACK_EXTRACT := Ikemen-GO-Screenpack-$(SCREENPACK_TAG)

# ─── Build Options ───────────────────────────────────────────────────────────
DEBUG_BUILD   ?= 0
IKEMEN_CPP    ?= 0
BUILD_FFMPEG  ?= auto   # auto | yes | no

# ─── Toolchain ───────────────────────────────────────────────────────────────
export GOEXPERIMENT := arenas
export CGO_ENABLED  := 1
export GOOS GOARCH CC CXX

PKG_CONFIG ?= pkg-config
WINDRES     := $(shell command -v x86_64-w64-mingw32-windres 2>/dev/null || command -v windres 2>/dev/null || echo windres)

# ─── pkg-config Packages ─────────────────────────────────────────────────────
PKG_PKGS := libavformat libavcodec libavutil libswscale libswresample libavfilter libxmp sdl2

# Prepend local FFmpeg to PKG_CONFIG_PATH if it exists
ifneq ($(wildcard $(FFMPEG_PREFIX)/lib/pkgconfig),)
  export PKG_CONFIG_PATH := $(FFMPEG_PREFIX)/lib/pkgconfig:/mingw64/lib/pkgconfig:$(PKG_CONFIG_PATH)
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
ifeq ($(IKEMEN_CPP),1)
  GO_TAGS := -tags ikemen_cpp
else
  GO_TAGS :=
endif
ifeq ($(DEBUG_BUILD),1)
  # The `debug` target enables the memory-analysis instrumentation (see
  # src/memdebug.go / src/memdebug_off.go, gated by the `debug` Go build tag).
  GO_TAGS += -tags debug
endif

# Version stamping is common to both builds.
LDFLAGS_BASE := \
  -X 'main.Version=$(APP_VERSION)' \
  -X 'main.BuildTime=$(APP_BUILDTIME)'

ifeq ($(DEBUG_BUILD),1)
  # Debug build: keep symbols/DWARF for debugging; console subsystem.
  LDFLAGS_GO := $(LDFLAGS_BASE)
  BUILD_TYPE := debug
else
  # Release build: GUI subsystem, strip symbols (-s) and DWARF (-w).
  LDFLAGS_GO := -H windowsgui -s -w $(LDFLAGS_BASE)
  BUILD_TYPE := release
endif

# ─── Derived File Targets ────────────────────────────────────────────────────
BINARY       := $(OUTDIR)/$(BINNAME)
SRC_SYSO     := src/rsrc_windows.syso
DELAY_STAMP  := $(DELAYLIB_DIR)/.delaylibs_done

# ─── Phony Targets ───────────────────────────────────────────────────────────
.PHONY: all release debug win32 help \
        deps-check check-sdl2 check-libxmp check-go-env \
        ffdeps _build-ffmpeg \
        winres delaylibs binary bundle install \
        screenpack \
        clean distclean FORCE

# ─── Default Target ──────────────────────────────────────────────────────────
all: release

release: deps-check check-sdl2 check-libxmp ffdeps binary bundle
	@echo "==> Build successful"
	@echo "    Binary: $(BINARY)"
	@test -d "$(LIBDIR)" && echo "    Runtime DLLs: $(LIBDIR)/" || true

# ===========================================================================
# Convenience Targets
# ===========================================================================

debug:
	$(MAKE) release DEBUG_BUILD=1 BINNAME=Ikemen_GO_debug.exe

win32:
	$(MAKE) release ARCH=386

# ===========================================================================
# Dependency Checks
# ===========================================================================

deps-check:
	@echo "==> Checking build dependencies..."
	@missing=""; \
	for tool in git make pkg-config gcc g++ nasm go gendef dlltool unzip; do \
		command -v $$tool >/dev/null 2>&1 || missing="$$missing $$tool"; \
	done; \
	if [ -n "$$missing" ]; then \
		echo "ERROR: Missing tools:$$missing" >&2; \
		echo "Install from the MINGW64 shell:" >&2; \
		echo "  pacman -Syu --noconfirm" >&2; \
		echo "  pacman -S --noconfirm git make diffutils mingw-w64-x86_64-pkg-config \\" >&2; \
		echo "    mingw-w64-x86_64-go mingw-w64-x86_64-toolchain \\" >&2; \
		echo "    mingw-w64-x86_64-nasm mingw-w64-x86_64-yasm \\" >&2; \
		echo "    mingw-w64-x86_64-tools-git mingw-w64-x86_64-libxmp \\" >&2; \
		echo "    mingw-w64-x86_64-SDL2" >&2; \
		exit 1; \
	fi
	@# Safe path check for MSYS2/Cygwin
	@case "$$(uname -s 2>/dev/null)" in \
		MINGW* | MSYS* | CYGWIN*) \
			case "$(CURDIR)" in \
				*[!A-Za-z0-9._/-]*) \
					echo "ERROR: Repository path contains characters unsafe for MSYS2/autotools:" >&2; \
					echo "  $(CURDIR)" >&2; \
					echo "Use only letters, digits, '_', '-', '.'" >&2; \
					exit 1;; \
			esac;; \
	esac
	@echo "    All dependencies found."

check-sdl2:
	@if ! $(PKG_CONFIG) --exists sdl2 2>/dev/null; then \
		echo "ERROR: SDL2 dev package not found." >&2; \
		echo "  Install: pacman -S mingw-w64-x86_64-SDL2" >&2; \
		exit 1; \
	fi

check-libxmp:
	@if ! $(PKG_CONFIG) --exists libxmp 2>/dev/null; then \
		echo "ERROR: libxmp dev package not found." >&2; \
		echo "  Install: pacman -S mingw-w64-x86_64-libxmp" >&2; \
		exit 1; \
	fi

check-go-env:
	@go version >/dev/null 2>&1 || \
		{ echo "ERROR: 'go version' failed. Set GOROOT=/mingw64/lib/go" >&2; exit 1; }

# ===========================================================================
# FFmpeg Detection / Build
# ===========================================================================

_FFMPEG_HAS_PC := $(shell $(PKG_CONFIG) --exists libavformat libavcodec libavutil libswresample libswscale libavfilter 2>/dev/null && echo yes)

ifeq ($(BUILD_FFMPEG),yes)
ffdeps: _build-ffmpeg
else ifeq ($(BUILD_FFMPEG),no)
ffdeps:
	@if [ "$(_FFMPEG_HAS_PC)" != "yes" ]; then \
		echo "ERROR: FFmpeg dev libraries not found (BUILD_FFMPEG=no)." >&2; \
		echo "       Install distro dev packages, or re-run with BUILD_FFMPEG=yes" >&2; \
		exit 1; \
	fi
	@echo "==> Using system FFmpeg."
else
ifeq ($(_FFMPEG_HAS_PC),yes)
ffdeps:
	@echo "==> Found FFmpeg via pkg-config; using it."
else
ffdeps: _build-ffmpeg
endif
endif

.PHONY: ffmpeg
ffmpeg: ffdeps

_build-ffmpeg:
	@if [ -d "$(FFMPEG_SRCDIR)" ]; then \
		echo "==> FFmpeg sources exist, skipping build."; \
		exit 0; \
	fi
	@echo "==> Building minimal FFmpeg ($(FFMPEG_REV))..."
	mkdir -p $(BUILDDIR)
	rm -rf $(FFMPEG_SRCDIR)
	git clone --depth=1 -b "$(FFMPEG_REV)" https://github.com/FFmpeg/FFmpeg.git $(FFMPEG_SRCDIR)
	cd $(FFMPEG_SRCDIR) && \
		./configure \
			--prefix="$(FFMPEG_PREFIX)" \
			--install-name-dir=@rpath \
			--enable-shared --disable-static \
			--disable-gpl --disable-nonfree \
			--disable-debug --disable-doc --disable-programs --disable-everything \
			--disable-autodetect \
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
	@echo "==> FFmpeg built and installed to: $(FFMPEG_PREFIX)"

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
# Delay-Load Import Libraries for MinGW
# ===========================================================================

delaylibs: $(DELAY_STAMP)

$(DELAY_STAMP):
	@echo "==> Generating delay-load import libraries..."
	mkdir -p $(DELAYLIB_DIR)
	shopt -s nullglob
	for d in "$(FFMPEG_PREFIX)"/bin/*.dll /mingw64/bin/libxmp*.dll /mingw64/bin/libwinpthread-1.dll /mingw64/bin/SDL2*.dll; do
		[[ -f "$$d" ]] || continue
		base="$$(basename "$$d")"
		name="$${base%.dll}"
		libname="$${name%%-*}"
		libname="$${libname#lib}"
		(cd "$(DELAYLIB_DIR)" && gendef "$$d" >/dev/null 2>&1)
		dlltool --dllname "$$base" \
			--def "$(DELAYLIB_DIR)/$${name}.def" \
			--output-delaylib "$(DELAYLIB_DIR)/lib$${libname}.dll.a"
		rm -f "$(DELAYLIB_DIR)/$${name}.def"
	done
	shopt -u nullglob
	touch $@
	@echo "==> Delay libraries ready."

# ===========================================================================
# Go Binary
# ===========================================================================

binary: check-go-env $(SRC_SYSO) $(DELAY_STAMP)
	@echo "==> Building $(BINNAME) ($(BUILD_TYPE), GOARCH=$(GOARCH))..."
	@_pc_path=""; \
	if [ -d "$(FFMPEG_PREFIX)/lib/pkgconfig" ]; then \
		_pc_path="$(FFMPEG_PREFIX)/lib/pkgconfig:"; \
	fi && \
	export PKG_CONFIG_PATH="$${_pc_path}$${PKG_CONFIG_PATH:-}" && \
	export CGO_CFLAGS="$$($(PKG_CONFIG) --cflags $(PKG_PKGS))" && \
	export CGO_LDFLAGS="-L$$(pwd)/$(DELAYLIB_DIR) $$($(PKG_CONFIG) --libs $(PKG_PKGS))" && \
	go build -trimpath -v $(GO_TAGS) \
		-ldflags "$(LDFLAGS_GO)" \
		-o "$(BINARY)" ./src
	rm -f $(SRC_SYSO)

# ===========================================================================
# Bundle Shared DLLs
# ===========================================================================

.PHONY: bundle
bundle:
	@echo "==> Bundling runtime DLLs to $(LIBDIR)/..."
	mkdir -p $(LIBDIR)
	if [ -d "$(FFMPEG_PREFIX)/bin" ]; then
		cp -av "$(FFMPEG_PREFIX)"/bin/*.dll "$(LIBDIR)/" 2>/dev/null || true
	fi
	for d in \
		/mingw64/bin/libwinpthread-1.dll \
		/mingw64/bin/libgcc_s_seh-1.dll \
		/mingw64/bin/libstdc++-6.dll \
		/mingw64/bin/libxmp*.dll \
		/mingw64/bin/SDL2*.dll; do
		[ -f "$$d" ] && cp -av "$$d" "$(LIBDIR)/" 2>/dev/null || true
	done
	@echo "==> Runtime DLLs bundled in $(LIBDIR)/"

# ===========================================================================
# Install — assemble a runnable distribution in $(INSTALLDIR)
# ===========================================================================

install: binary bundle deps-check
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
	@echo "==> Copying runtime DLLs (lib/)..."
	rm -rf "$(INSTALLDIR)/lib"
	cp -r "$(LIBDIR)" "$(INSTALLDIR)/"
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
	rm -f $(DELAY_STAMP) 2>/dev/null || true
	rm -rf $(DELAYLIB_DIR) 2>/dev/null || true
	rm -rf $(LIBDIR) 2>/dev/null || true
	rm -rf $(WINRES_DIR) 2>/dev/null || true
	@echo "==> Clean done."

distclean: clean
	@echo "==> Deep cleaning..."
	rm -rf $(FFMPEG_SRCDIR) 2>/dev/null || true
	rm -rf $(FFMPEG_PREFIX) 2>/dev/null || true
	rm -rf $(SCREENPACK_DIR) 2>/dev/null || true
	@echo "==> Distclean done."

# ===========================================================================
# FORCE — ensures targets depending on it always rebuild
# ===========================================================================
FORCE:

# ===========================================================================
# Help
# ===========================================================================

help:
	@echo 'Ikemen-GO Windows Build (MSYS2 / MinGW64)'
	@echo ''
	@echo 'Targets:'
	@echo '  all / release  Win64 release build (default, GUI subsystem)'
	@echo '  debug          Win64 debug build -> Ikemen_GO_debug.exe (console + memory instrumentation)'
	@echo '  win32          Win32 (x86) build'
	@echo '  ffmpeg         Build/check FFmpeg dependencies'
	@echo '  screenpack     Clone/update Elecbyte screenpack'
	@echo '  bundle         Copy shared DLLs to lib/'
	@echo '  install        Assemble runnable build in install/ (screenpack + binary)'
	@echo '  clean          Remove build artifacts'
	@echo '  distclean      Remove artifacts + FFmpeg + screenpack'
	@echo '  deps-check     Verify required tools are installed'
	@echo '  help           Show this help'
	@echo ''
	@echo 'Options:'
	@echo '  ARCH=386           Build 32-bit (default: amd64)'
	@echo '  DEBUG_BUILD=1      Debug build (console subsystem) + memory instrumentation'
	@echo '  IKEMEN_CPP=1       Enable C++ backend (Go build tags)'
	@echo '  BUILD_FFMPEG=yes   Force local FFmpeg build'
	@echo '  BUILD_FFMPEG=no    Require system FFmpeg only'
	@echo '  APP_VERSION=X.Y    Set version string (default: nightly)'
	@echo '  APP_BUILDTIME=X    Set build timestamp'
	@echo ''
	@echo 'Examples:'
	@echo '  make                          # Win64 release'
	@echo '  make debug                    # Win64 debug'
	@echo '  make win32                    # Win32 release'
	@echo '  make IKEMEN_CPP=1             # C++ backend'
	@echo '  make BUILD_FFMPEG=yes         # Force local FFmpeg'
	@echo '  make APP_VERSION=v1.0.0       # Tagged build'
	@echo '  make APP_VERSION=v1.0.0 DEBUG_BUILD=1 IKEMEN_CPP=1'
