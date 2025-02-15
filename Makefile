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
Ikemen_GO.exe: ${srcFiles}
	cd ./build && bash ./build.sh Win64

# Windows 64-bit target (GL 3.2)
Ikemen_GO_GL32.exe: ${srcFiles}
	cd ./build && bash ./build.sh Win64GL32

# Windows 32-bit target
Ikemen_GO_86.exe: ${srcFiles}
	cd ./build && bash ./build.sh Win32

# Linux target
Ikemen_GO_Linux: ${srcFiles}
	cd ./build && ./build.sh Linux

# Linux target (GL 3.2)
Ikemen_GO_Linux_GL32: ${srcFiles}
	cd ./build && ./build.sh LinuxGL32

# Linux ARM target
Ikemen_GO_LinuxARM: ${srcFiles}
	cd ./build && ./build.sh LinuxARM

# Linux ARM target (GL 3.2)
Ikemen_GO_LinuxARM_GL32: ${srcFiles}
	cd ./build && ./build.sh LinuxARMGL32

# MacOS x64 target
Ikemen_GO_MacOS: ${srcFiles}
	cd ./build && bash ./build.sh MacOS

# MacOS app bundle
appbundle:
	mkdir -p I.K.E.M.E.N-Go.app
	mkdir -p I.K.E.M.E.N-Go.app/Contents
	mkdir -p I.K.E.M.E.N-Go.app/Contents/MacOS
	mkdir -p I.K.E.M.E.N-Go.app/Contents/Resources
	cp bin/Ikemen_GO_MacOS I.K.E.M.E.N-Go.app/Contents/MacOS/Ikemen_GO_MacOS
	cp ./build/Info.plist I.K.E.M.E.N-Go.app/Contents/Info.plist
	cp ./build/bundle_run.sh I.K.E.M.E.N-Go.app/Contents/MacOS/bundle_run.sh
	chmod +x I.K.E.M.E.N-Go.app/Contents/MacOS/bundle_run.sh
	chmod +x I.K.E.M.E.N-Go.app/Contents/MacOS/Ikemen_GO_MacOS
	cd ./build && mkdir -p ./icontmp/icon.iconset && \
	cp ../external/icons/IkemenCylia_256.png ./icontmp/icon.iconset/icon_256x256.png && \
	iconutil -c icns ./icontmp/icon.iconset && \
	cp icontmp/icon.icns ../I.K.E.M.E.N-Go.app/Contents/Resources/icon.icns && \
	rm -rf icontmp

clean_appbundle:
	rm -rf I.K.E.M.E.N-Go.app

# Tag usage:
# 	OpenGL ES: gles2
#	OpenGL: <default>
#
# 	Linux DRM: kmsdrm
#	Linux X11: x11
#	Linux Wayland: wayland
#	Mac OS: darwin
#	Windows: <default>

windows: ${srcFiles} src/assets.zip
	go build -trimpath -ldflags="-s -w -H=windowsgui" -v -o ./bin/ikemen_win.exe ./src

# Steamdeck (SteamOS X11)
steamdeck: ${srcFiles} src/assets.zip
	export CGO_ENABLED=1 && go build -tags="x11" -trimpath -ldflags="-s -w" -v -o ./bin/ikemen_steamdeck ./src

# Retroid Pocket 5 (Rocknix Linux)
rp5: ${srcFiles} src/assets.zip
	export CGO_ENABLED=1 && go build -tags="wayland" -trimpath -ldflags="-s -w" -v -o ./bin/ikemen_rp5 ./src

# Raspberry Pi 4 (Raspberry OS)
rpi4: ${srcFiles} src/assets.zip
	export CGO_ENABLED=1 && go build -tags="kmsdrm,gles2" -trimpath -ldflags="-s -w" -v -o ./bin/ikemen_rpi4 ./src
#	export CGO_ENABLED=1 && go build -tags="kmsdrm" -trimpath -ldflags="-s -w" -v -o ./bin/ikemen_rpi4 ./src

# Anbernic RG353 variant (Batocera, Recalbox, EmuELEC, ArkOS)
rg353_drm: ${srcFiles} src/assets.zip
	export CGO_ENABLED=1 && go build -tags="kmsdrm,gles2" -trimpath -ldflags="-s -w" -v -o ./bin/ikemen_rg353_drm ./src

# Anbernic RG353 variant (Rocknix Linux)
rg353_wayland: ${srcFiles} src/assets.zip
	export CGO_ENABLED=1 && go build -tags="wayland,gles2" -trimpath -ldflags="-s -w" -v -o ./bin/ikemen_rg353_wayland ./src

# Generic Linux Wayland
wayland: ${srcFiles} src/assets.zip
	export CGO_ENABLED=1 && go build -tags="wayland,gles2" -trimpath -ldflags="-s -w" -v -o ./bin/ikemen_wayland ./src
	sudo cp ./bin/ikemen_wayland /usr/bin
#	cp ./bin/ikemen_wayland /home/deck/Downloads/Famicom\ Fighters\ \(MUGEN\)
#	cp ./bin/ikemen_wayland /home/deck/Downloads/MK1
#	cp ./bin/ikemen_wayland /home/deck/Downloads/MK2\ Remix\ MUGEN
#	cp ./bin/ikemen_wayland /home/deck/Downloads/MugenDreamVer2
#	cp ./bin/ikemen_wayland /home/deck/Downloads/Disney\ Mugen
#	cp ./bin/ikemen_wayland /home/deck/Downloads/mugen_original/MK1
#	cp ./bin/ikemen_wayland /home/deck/Downloads/mugen_original/rmm-3-21
#	cp ./bin/ikemen_wayland /home/deck/Downloads/mugen_original/The\ Queen\ Of\ Fighters

# Generic Linux KMS DRM
drm: ${srcFiles} src/assets.zip
	export CGO_ENABLED=1 && go build -tags="kmsdrm,gles2" -trimpath -ldflags="-s -w" -v -o ./bin/ikemen_drm ./src
#	cp ./bin/ikemen_drm /home/deck/Projects/PortMaster/hyperdbz/HyperDBZIndigo/Hyper\ DBZ\ 5.0d

src/assets.zip: data/* external/* font/*
	zip -r src/assets.zip data external font
