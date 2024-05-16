# Set Bash as the shell.
SHELL=/bin/bash

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

# Windows 64-bit target
win64: ${srcFiles}
	cd ./build && bash ./build.sh win64

# Windows 32-bit target
win32: ${srcFiles}
	cd ./build && bash ./build.sh win32

# Linux target
Ikemen_GO_Linux: ${srcFiles}
	cd ./build && ./build.sh Linux

# MacOS x64 target
Ikemen_GO_MacOS: ${srcFiles}
	cd ./build && bash ./build.sh MacOS

# Anbernic RG353P (Recalbox) target
rg353p: ${srcFiles}
	cd ./build && ./build.sh rg353p

# Steamdeck (SteamOS) target
steamdeck: ${srcFiles}
	cd ./build && ./build.sh steamdeck

# PSC target
psc: ${srcFiles}
	cd ./build && ./build.sh psc
