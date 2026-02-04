# =============================================================================
# Ikemen GO Integrated Makefile (V16 - Fixed Target Variables & Optimized)
# =============================================================================

# --- Configuration & Defaults ---
SHELL := /bin/bash
.DEFAULT_GOAL := all

# Metadata
APP_VERSION ?= nightly
APP_BUILDTIME ?= $(shell date '+%Y.%m.%d')
COPY_START_YEAR ?= 2016
BUILD_YEAR := $(shell echo $(APP_BUILDTIME) | cut -d. -f1)
APP_COPYRIGHT := (c) $(COPY_START_YEAR)-$(BUILD_YEAR) Ikemen GO team (MIT)

# Output Directories
BUILD_DIR := build
SRC_DIR := src
BUILD_PREFIX := $(abspath $(BUILD_DIR)/output)
FFMPEG_REV ?= release/7.1
ASSETS   := $(SRC_DIR)/assets.zip

# Tools
GO := go
PKG_CONFIG := pkg-config

# --- Auto-Detect OS & Arch ---
OS_DETECT := $(shell uname -s)
RAW_ARCH := $(shell uname -m)

# Normalize Architecture for Go
GOARCH_DETECT := $(RAW_ARCH)
ifeq ($(RAW_ARCH),x86_64)
    GOARCH_DETECT := amd64
endif
ifeq ($(RAW_ARCH),aarch64)
    GOARCH_DETECT := arm64
endif
ifneq (,$(filter i686 i386,$(RAW_ARCH)))
    GOARCH_DETECT := 386
endif

# Initialize System Variables
HOST_OS := unknown
EXE_EXT :=
GO_TAGS :=
CGO_LDFLAGS_EXTRA :=
EXE_OUTPUT_DIR := bin

# Detect Linux
ifneq (,$(findstring Linux,$(OS_DETECT)))
    HOST_OS := linux
    export GOOS := linux
    export GOARCH := $(GOARCH_DETECT)
    EXE_OUTPUT_DIR := .
    CGO_LDFLAGS_EXTRA += -Wl,-rpath,'$$ORIGIN' -Wl,-rpath,'$$ORIGIN/lib'
endif

# Detect Mac
ifneq (,$(findstring Darwin,$(OS_DETECT)))
    HOST_OS := darwin
    export GOOS := darwin
    export GOARCH := $(GOARCH_DETECT)
    EXE_OUTPUT_DIR := bin
    CGO_LDFLAGS_EXTRA += -Wl,-rpath,@executable_path -Wl,-rpath,@executable_path/../Frameworks
    MVK_PATH := $(shell \
        if [ -f /opt/homebrew/lib/libMoltenVK.dylib ]; then echo /opt/homebrew/lib; \
        elif [ -f /usr/local/lib/libMoltenVK.dylib ]; then echo /usr/local/lib; \
        fi)
    ifneq ($(MVK_PATH),)
        CGO_LDFLAGS_EXTRA += -L$(MVK_PATH)
    endif
endif

# Detect Windows (MinGW/MSYS)
ifneq (,$(findstring MINGW,$(OS_DETECT))$(findstring MSYS,$(OS_DETECT))$(findstring CYGWIN,$(OS_DETECT))$(findstring Windows_NT,$(OS_DETECT)))
    HOST_OS := windows
    export GOOS := windows
    export GOARCH := $(GOARCH_DETECT)
    EXE_OUTPUT_DIR := .
    EXE_EXT := .exe
	GO_TAGS += static
endif

# Manual Override
ifdef TARGET_OS
    HOST_OS := $(TARGET_OS)
endif

# --- Binary Name Logic ---
BIN_NAME := Ikemen_GO_$(HOST_OS)$(EXE_EXT)
ifeq ($(HOST_OS),windows)
    ifeq ($(GOARCH),amd64)
        BIN_NAME := Ikemen_GO.exe
        ASM_ARCH := amd64
    else
        BIN_NAME := Ikemen_GO_x86.exe
        ASM_ARCH := x86
    endif
endif

# --- Source Files & Dependencies ---

# PKG_CONFIG_PATH for locating FFmpeg/XMP pkg-config files
export PKG_CONFIG_PATH := $(BUILD_PREFIX)/lib/pkgconfig
export GOEXPERIMENT := arenas

# Go Source Files
GO_SRCS := $(shell find $(SRC_DIR) -name "*.go" 2>/dev/null)

# Windows Resource Object
ifeq ($(HOST_OS),windows)
    GO_SRCS += src/rsrc_windows.syso
endif

# Required packages (Full build)
REQ_PKGS := libavformat libavcodec libavutil libswscale libswresample libavfilter sdl2

ifeq ($(HOST_OS),windows)
	EXTRA_LIBS := -static -lmingw32 -lmingwex -lkernel32 -luser32 -lgdi32 -lwinmm -limm32 -lole32 -loleaut32 -lversion -luuid -ladvapi32 -lshell32 -lsetupapi
endif

export CGO_ENABLED := 1
LD_FLAGS := -s -w -X 'main.Version=$(APP_VERSION)' -X 'main.BuildTime=$(APP_BUILDTIME)'

# --- Build Command Definition ---
# Using a canned sequence to ensure target-specific variables are correctly expanded
define BUILD_GO
	@export PKG_CFLAGS="$$($(PKG_CONFIG) --cflags $(REQ_PKGS) 2>/dev/null)" && \
	export PKG_LIBS="$$($(PKG_CONFIG) --libs --static $(REQ_PKGS) 2>/dev/null) $(EXTRA_LIBS) $(EXTRA_PKG_LIBS)" && \
	export CGO_CFLAGS="$$PKG_CFLAGS" && \
	export CGO_LDFLAGS="$$PKG_LIBS $(CGO_LDFLAGS_EXTRA)" && \
	echo "==> Building $(BIN_NAME) for $(GOOS)/$(GOARCH)..." && \
	echo "  -> Tags:   $(GO_TAGS)" && \
	$(GO) build -trimpath -v -tags "$(GO_TAGS)" -ldflags "$(LD_FLAGS)" -o $(EXE_OUTPUT_DIR)/$(BIN_NAME) ./src
endef

# --- Targets ---
.PHONY: all clean ffmpeg xmp appbundle windows-resources build-core full lite

# Main Entry Point
all: build-core

# Build core target
build-core: windows-resources $(EXE_OUTPUT_DIR)/$(BIN_NAME)

# Mugen build: Mugen compatible build, only requires SDL2, no FFmpeg/XMP dependencies
mugen: GO_TAGS += mugen lite
mugen: REQ_PKGS = sdl2
mugen: EXTRA_PKG_LIBS = -static
mugen: windows-resources $(GO_SRCS) $(ASSETS)
	$(BUILD_GO)	

# Lite build: only requires SDL2, no FFmpeg/XMP dependencies
lite: GO_TAGS += lite
lite: REQ_PKGS = sdl2
lite: EXTRA_PKG_LIBS = -static
lite: windows-resources $(GO_SRCS)
	$(BUILD_GO)

# The REAL Build Rule
$(EXE_OUTPUT_DIR)/$(BIN_NAME): $(GO_SRCS) ffmpeg xmp
	$(BUILD_GO)

# FFmpeg Build
FFMPEG_LIBS := $(addprefix $(BUILD_PREFIX)/lib/,libavformat.a libavcodec.a libavutil.a libswscale.a libswresample.a libavfilter.a)

ffmpeg: $(FFMPEG_LIBS)

$(FFMPEG_LIBS):
	@if [ ! -d "$(BUILD_DIR)/ffmpeg-src" ]; then \
		echo "==> Cloning FFmpeg source code..."; \
		mkdir -p $(BUILD_DIR); \
		git clone --depth=1 -b $(FFMPEG_REV) https://github.com/FFmpeg/FFmpeg.git $(BUILD_DIR)/ffmpeg-src; \
	fi
	@echo "==> Building FFmpeg locally..."
	cd $(BUILD_DIR)/ffmpeg-src && \
	./configure --prefix=$(BUILD_PREFIX) \
         --install-name-dir=@rpath \
         --disable-gpl --disable-nonfree \
         --disable-debug --disable-doc --disable-programs --disable-everything \
         --disable-autodetect --disable-avdevice --disable-pthreads \
         --enable-avformat --enable-avcodec --enable-avutil --enable-swresample --enable-swscale \
         --enable-avfilter --enable-filter=buffer,buffersink,format,scale,pad,crop \
         --enable-protocol=file \
         --enable-demuxer=matroska,webm \
         --enable-decoder=vp8,vp9,opus,vorbis \
         --enable-parser=vp8,vp9,opus,vorbis && \
	make -j$(shell nproc || echo 2) && \
	make install

# XMP Static Library Build
xmp: $(BUILD_PREFIX)/lib/libxmp.a

$(BUILD_PREFIX)/lib/libxmp.a:
	@echo "==> Building XMP locally..."
	cd $(BUILD_DIR)/xmp-src && \
	make -j$(shell nproc || echo 2) && \
	make install

# Windows: Resources & Manifest
ifeq ($(HOST_OS),windows)
windows-resources: src/rsrc_windows.syso
else
windows-resources:
endif

# Generate Windows Resource Files
build/winres/Ikemen_GO.rc: build/winres/Ikemen_GO.exe.manifest
	@echo "==> Generating Windows RC file..."
	@echo '#include <windows.h>' > $@
	@echo '#include <winver.h>' >> $@
	@echo '1 ICON "Ikemen_Cylia_V2.ico"' >> $@
	@echo '1 RT_MANIFEST "Ikemen_GO.exe.manifest"' >> $@
	@echo 'VS_VERSION_INFO VERSIONINFO' >> $@
	@echo 'FILEVERSION 1,0,0,0' >> $@
	@echo 'PRODUCTVERSION 1,0,0,0' >> $@
	@echo 'FILEFLAGSMASK 0x3fL' >> $@
	@echo 'FILEFLAGS 0x0L' >> $@
	@echo 'FILEOS 0x4L' >> $@
	@echo 'FILETYPE 0x1L' >> $@
	@echo 'FILESUBTYPE 0x0L' >> $@
	@echo 'BEGIN' >> $@
	@echo '    BLOCK "StringFileInfo"' >> $@
	@echo '    BEGIN' >> $@
	@echo '        BLOCK "040904B0"' >> $@
	@echo '        BEGIN' >> $@
	@echo '            VALUE "CompanyName", "Ikemen GO\\0"' >> $@
	@echo '            VALUE "FileDescription", "Ikemen GO\\0"' >> $@
	@echo '            VALUE "FileVersion", "$(APP_VERSION)\\0"' >> $@
	@echo '            VALUE "ProductName", "Ikemen GO\\0"' >> $@
	@echo '            VALUE "ProductVersion", "$(APP_VERSION)\\0"' >> $@
	@echo '            VALUE "OriginalFilename", "Ikemen_GO.exe\\0"' >> $@
	@echo '            VALUE "InternalName", "Ikemen_GO\\0"' >> $@
	@echo '            VALUE "BuildDate", "$(APP_BUILDTIME)\\0"' >> $@
	@echo '            VALUE "LegalCopyright", "$(APP_COPYRIGHT)\\0"' >> $@
	@echo '        END' >> $@
	@echo '    END' >> $@
	@echo '    BLOCK "VarFileInfo"' >> $@
	@echo '    BEGIN' >> $@
	@echo '        VALUE "Translation", 0x0409, 1200' >> $@
	@echo '    END' >> $@
	@echo 'END' >> $@

build/winres/Ikemen_GO.exe.manifest:
	@echo "==> Generating Windows manifest..."
	@mkdir -p build/winres
	@echo '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>' > $@
	@echo '<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">' >> $@
	@echo '  <assemblyIdentity type="win32" name="Ikemen_GO" version="1.0.0.0" processorArchitecture="$(ASM_ARCH)"/>' >> $@
	@echo '  <dependency>' >> $@
	@echo '    <dependentAssembly>' >> $@
	@echo '      <assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls"' >> $@
	@echo '        version="6.0.0.0" processorArchitecture="*" publicKeyToken="6595b64144ccf1df" language="*"/>' >> $@
	@echo '    </dependentAssembly>' >> $@
	@echo '  </dependency>' >> $@
	@echo '</assembly>' >> $@

src/rsrc_windows.syso: build/winres/Ikemen_GO.rc
	@echo "==> Compiling Windows resources..."
	@windres -I build/winres -I external/icons -i build/winres/Ikemen_GO.rc -O coff -o $@

# Full Distribution
SCREENPACK_URL := https://github.com/ikemen-engine/Ikemen_GO-Elecbyte-Screenpack/archive/47bf675.zip
# SCREENPACK_URL := https://github.com/ikemen-engine/Ikemen_GO-Elecbyte-Screenpack/archive/60d0b51.zip
SCREENPACK_ZIP := screenpack.zip

$(SCREENPACK_ZIP):
	@echo "==> Downloading screenpack..."
	wget -O $@ $(SCREENPACK_URL)

$(ASSETS): data/* external/* font/*
	@echo "Packaging assets..."
	echo $(BUILD_DATE) > external/script/version
	rm -f $(ASSETS)
	cd $(SRC_DIR) && zip -r assets.zip ../data ../external ../font >/dev/null

full: build-core $(SCREENPACK_ZIP)
	@echo "==> Assembling full Ikemen app..."
	@rm -rf dist
	@mkdir -p dist/tmp_extract
	@echo " -> Extracting Screenpack..."
	@unzip -q $(SCREENPACK_ZIP) -d dist/tmp_extract
	@cp -rf dist/tmp_extract/*/* dist/
	@rm -rf dist/tmp_extract
	@echo " -> Copying Assets..."
	@cp -rf data external font dist/
	@echo " -> Copying Engine Binaries..."
	@cp $(EXE_OUTPUT_DIR)/$(BIN_NAME) dist/
	@echo " -> Copying Tools..."
	@find tool -maxdepth 1 -name "*.exe" -type f -exec cp {} dist/ \;
	@echo " -> Zipping IkemenGoFull.zip..."
	@rm -f IkemenGoFull.zip
	@cd dist && zip -r -q ../IkemenGoFull.zip .
	@rm -rf dist
	@echo "==> Done! Output: IkemenGoFull.zip"

# MacOS Bundle
appbundle: build-core
ifeq ($(HOST_OS),darwin)
	@echo "==> Creating macOS App Bundle..."
	mkdir -p I.K.E.M.E.N-Go.app/Contents/{MacOS,Resources}
	cp $(EXE_OUTPUT_DIR)/$(BIN_NAME) I.K.E.M.E.N-Go.app/Contents/MacOS/$(BIN_NAME)
	[ -f ./build/Info.plist ] && cp ./build/Info.plist I.K.E.M.E.N-Go.app/Contents/Info.plist || true
	mkdir -p build/icontmp/icon.iconset
	cp external/icons/IkemenCylia_256.png build/icontmp/icon.iconset/icon_256x256.png
	iconutil -c icns build/icontmp/icon.iconset -o build/icontmp/icon.icns
	cp build/icontmp/icon.icns I.K.E.M.E.N-Go.app/Contents/Resources/icon.icns
	rm -rf build/icontmp
endif

clean:
	rm -rf $(SCREENPACK_ZIP) $(EXE_OUTPUT_DIR)/$(BIN_NAME) $(BUILD_PREFIX) $(BUILD_DIR)/winres src/*.syso I.K.E.M.E.N-Go.app dist IkemenGoFull.zip *.exe
	@echo "==> Cleaned."