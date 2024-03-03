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

# Handheld Anbernic RG353P target
rg35xx:
	@echo "Building for Anbernic RG35XX"
	CGO_CFLAGS="-Os -marm -march=armv7-a -mtune=cortex-a9 -mfpu=neon-fp16 -mfloat-abi=hard" GOOS=linux GOARCH=arm CGO_ENABLED=1 go build -x -tags=rg35xx -trimpath -v -trimpath -ldflags="-s -w" -o ./bin/Ikemen_GO_Linux_RG35XX ./src
	cp bin/Ikemen_GO_Linux_RG35XX /mnt/f/ADB
