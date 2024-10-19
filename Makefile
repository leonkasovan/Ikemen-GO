# Set Bash as the shell.
SHELL=/bin/bash

# Get all Lua files in external/script directory
LUA_FILES := $(wildcard external/script/*.lua)

# Get Ikemen's data files in data/ directory
DATA_FILES := $(wildcard data/*)

# /src files
srcFiles=src/anim.go \
	src/bgdef.go \
	src/bytecode.go \
	src/camera.go \
	src/char.go \
	src/common.go \
	src/compiler.go \
	src/compiler_functions.go \
	src/font.go \
	src/image.go \
	src/input.go \
	src/lifebar.go \
	src/main.go \
	src/render.go \
	src/script.go \
	src/sound.go \
	src/stage.go \
	src/stdout_windows.go \
	src/system.go \
	src/util_desktop.go \
	src/util_js.go

# Target: assets.zip depends on the Lua scripts
assets.zip: $(LUA_FILES) $(DATA_FILES)
	@echo "Zipping Lua files into assets.zip..."
	zip -r src/assets.zip external/ data/ font/
#	rm src/assets.zip
#	mv assets.zip src/

# Windows 64-bit target
Ikemen_GO.exe: ${srcFiles} assets.zip
	cd ./build && bash ./build.sh Win64

# Windows 32-bit target
Ikemen_GO_86.exe: ${srcFiles} assets.zip
	cd ./build && bash ./build.sh Win32

# Linux target
Ikemen_GO_Linux: ${srcFiles} assets.zip
	cd ./build && ./build.sh Linux

# MacOS x64 target
Ikemen_GO_MacOS: ${srcFiles} assets.zip
	cd ./build && bash ./build.sh MacOS

# Anbernic RG353P (Recalbox) target
rg353p: ${srcFiles} assets.zip
	cd ./build && ./build.sh rg353p

# Steamdeck (SteamOS) target
steamdeck: ${srcFiles} assets.zip
	cd ./build && ./build.sh steamdeck	