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
rg353p: ${srcFiles}
	cd ./build && ./build.sh rg353p

# Raspberry Pi 4 target
rpi4: ${srcFiles}
	cd ./build && ./build.sh rpi4
	cp bin/Ikemen_GO_Linux_RPi4 /home/pi/Apps/

# Steamdeck target
steamdeck: ${srcFiles}
	cd ./build && ./build.sh steamdeck
	cp bin/Ikemen_GO_Linux_RPi4 /home/pi/Apps/

# Linux target
Ikemen_GO_Linux: ${srcFiles}
	cd ./build && ./build.sh Linux
	cp bin/Ikemen_GO_Linux /home/pi/Apps/Ikemen_GO_Linux_RPi4
	
# Windows 64-bit target
Ikemen_GO.exe: ${srcFiles}
	cd ./build && bash ./build.sh Win64

# Windows 32-bit target
Ikemen_GO_86.exe: ${srcFiles}
	cd ./build && bash ./build.sh Win32

# MacOS x64 target
Ikemen_GO_MacOS: ${srcFiles}
	cd ./build && bash ./build.sh MacOS