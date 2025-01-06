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

# Generic aarch64 target (with cache build, faster, compile only what changed)
aarch64:
	export CGO_ENABLED=1 && go build -tags=linux,arm64,kmsdrm,gles2 -trimpath -v -trimpath -ldflags "-s -w" -o ./bin/ikemengo.0.99.0 ./src

# Generic aarch64 target (with no cache build, slower, compile everything)
aarch64_no_cache:	
	export CGO_ENABLED=1 && go build -tags=linux,arm64,kmsdrm,gles2 -trimpath -a -v -trimpath -ldflags "-s -w" -o ./bin/ikemengo.0.99.0 ./src

# Windows 64-bit target
Ikemen_GO.exe: ${srcFiles}
	cd ./build && bash ./build.sh Win64

# Windows 32-bit target
Ikemen_GO_86.exe: ${srcFiles}
	cd ./build && bash ./build.sh Win32

# Linux target
Ikemen_GO_Linux: ${srcFiles}
	cd ./build && ./build.sh Linux

# MacOS x64 target
Ikemen_GO_MacOS: ${srcFiles}
	cd ./build && bash ./build.sh MacOS
