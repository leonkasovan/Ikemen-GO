#!/bin/bash

# Exit in case of failure
set -e

# Int vars
binName="Default"
targetOS=$1
currentOS="Unknown"

# Go to the main folder.
cd "$(dirname "$0")/.."

# Main function.
function main() {
	# Enable CGO.
	export CGO_ENABLED=1

	# Create "bin" folder.
	mkdir -p bin

	# Check OS
	checkOS
	# If a build target has not been specified use the current OS.
	if [[ "$1" == "" ]]; then
		targetOS=$currentOS
	fi
	
	# Build
	case "${targetOS}" in
		win64)
			export GOOS=windows
			export GOARCH=amd64
			export CC=x86_64-w64-mingw32-gcc
			export CXX=x86_64-w64-mingw32-g++
			binName="Ikemen_Go.exe"
			echo "Win64 Build Release with GLFW and OpenGL"
			go build -tags=glfw,gl -trimpath -v -trimpath -ldflags "-s -w -H windowsgui" -o ./bin/$binName ./src
			# echo "Win64 Build Release with SDL2 and OpenGL"
			# go build -tags=sdl,static,gl -trimpath -v -trimpath -ldflags "-s -w -H windowsgui" -o ./bin/$binName ./src
			cp bin/$binName /mnt/c/PortableApps/Ikemen_Go\(Dev\)/
		;;
		win32)
			export GOOS=windows
			export GOARCH=386
			export CC=i686-w64-mingw32-gcc
			export CXX=i686-w64-mingw32-g++
			binName="Ikemen_Go_x86.exe"
			echo "Win32 Build Release with GLFW and OpenGL"
			go build -tags=glfw,gl -trimpath -v -trimpath -ldflags "-s -w -H windowsgui" -o ./bin/$binName ./src
			# echo "Win32 Build Release with SDL2 and OpenGL"
			# go build -tags=sdl,static,gl -trimpath -v -trimpath -ldflags "-s -w -H windowsgui" -o ./bin/$binName ./src
			cp bin/$binName /mnt/c/PortableApps/Ikemen_Go\(Dev\)/
		;;
		[mM][aA][cC][oO][sS])
			varMacOS
			build
		;;
		[lL][iI][nN][uU][xX][aA][rR][mM])
			varLinuxARM
			build
		;;
		[lL][iI][nN][uU][xX])
			varLinux
			build
		;;
		rg353p)
			export GOOS=linux
			export GOARCH=arm64
			# export CC=aarch64-buildroot-linux-gnu-gcc
			# export CXX=aarch64-buildroot-linux-gnu-g++
			binName="Ikemen_Go_RG353P"
			echo "Linux Build Release for RG353P(Recalbox) with SDL and GLES"
			go build -tags=sdl,gles2 -trimpath -v -trimpath -ldflags="-s -w" -o ./bin/$binName ./src
		;;
		steamdeck)
			export GOOS=linux
			binName="Ikemen_Go_Steamdeck"
			echo "Linux Build Release for Steamdeck(SteamOS) with GLFW and OpenGL"
			go build -tags=glfw,gl -trimpath -v -trimpath -ldflags="-s -w" -o ./bin/$binName ./src
			cp bin/Ikemen_GO_Steamdeck ~/Applications/IkemenGoDev
		;;
		pi4)
			export GOOS=linux
			export GOARCH=arm64
			binName="Ikemen_Go_Pi4"
			echo "Linux Build Release for Raspberry Pi4 (Raspberry Pi OS 64) with SDL and GLES"
			go build -tags=sdl,gles2 -trimpath -v -trimpath -ldflags="-s -w" -o ./bin/$binName ./src
		;;
	esac

	if [[ "${binName}" == "Default" ]]; then
		echo "Invalid target architecture \"${targetOS}\".";
		exit 1
	fi
}

# Export Variables
function varMacOS() {
	export GOOS=darwin
	case "${currentOS}" in
		[mM][aA][cC][oO][sS])
			export CC=clang
			export CXX=clang++
		;;
		*)
			export CC=o64-clang
			export CXX=o64-clang++
		;;
	esac
	binName="Ikemen_GO_MacOS"
}
function varLinux() {
	export GOOS=linux
	#export CC=gcc
	#export CXX=g++
	binName="Ikemen_GO_Linux"
}
function varLinuxARM() {
	export GOOS=linux
	export GOARCH=arm64
	binName="Ikemen_GO_LinuxARM"
}

# Build functions.
function build() {
	#echo "buildNormal"
	#echo "$binName"

	echo "Linux Build Release with GLFW"
	go build -tags=glfw,gl -trimpath -v -trimpath -ldflags="-s -w" -o ./bin/$binName ./src
}

function buildWin() {
	#echo "buildWin"
	#echo "$binName"

	echo "Win64 Build Release with GLFW and OpenGL"
	go build -tags=glfw,gl -trimpath -v -trimpath -ldflags "-s -w -H windowsgui" -o ./bin/$binName ./src
	
	# echo "Win64 Build Release with SDL2"
	# go build -tags=sdl,static,gles2 -trimpath -v -trimpath -ldflags "-s -w -H windowsgui" -o ./bin/$binName ./src
}

# Determine the target OS.
function checkOS() {
	osArch=`uname -m`
	case "$OSTYPE" in
		darwin*)
			currentOS="MacOS"
		;;
		linux*)
			currentOS="Linux"
		;;
		msys)
			if [[ "$osArch" == "x86_64" ]]; then
				currentOS="Win64"
			else
				currentOS="Win32"
			fi
		;;
		*)
			if [[ "$1" == "" ]]; then
				echo "Unknown system \"${OSTYPE}\".";
				exit 1
			fi
		;;
	esac
}

# Check if "go.mod" exists.
if [ ! -f ./go.mod ]; then
	echo "Missing dependencies, please run \"get.sh\"."
	exit 1
else
	# Exec Main
	main $1 $2
fi
