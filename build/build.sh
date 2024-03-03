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
		[wW][iI][nN]64)
			varWin64
			buildWin
		;;
		[wW][iI][nN]32)
			varWin32
			buildWin
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
		[Ss]teamdeck)
			export GOOS=linux
			binName="Ikemen_GO_Linux_Steamdeck"
			build_Steamdeck
		;;
		[Rr][Pp][Ii]4)
			export GOOS=linux
			binName="Ikemen_GO_Linux_RPi4"
			build_RPi4
		;;
		[Rr][Gg]353[Pp])
			export GOOS=linux
			binName="Ikemen_GO_Linux_RG353P"
			build_RG353P
		;;
		[Rr][Gg]35[Xx][Xx])
			export GOOS=linux
			binName="Ikemen_GO_Linux_RG35XX"
			build_RG35XX
		;;
	esac

	if [[ "${binName}" == "Default" ]]; then
		echo "Invalid target architecture \"${targetOS}\".";
		exit 1
	fi
}

# Export Variables
function varWin32() {
	export GOOS=windows
	export GOARCH=386
	if [[ "${currentOS,,}" != "win32" ]]; then
		export CC=i686-w64-mingw32-gcc
		export CXX=i686-w64-mingw32-g++
	fi
	binName="Ikemen_GO_x86.exe"
}

function varWin64() {
	export GOOS=windows
	export GOARCH=amd64
	if [[ "${currentOS,,}" != "win64" ]]; then
		export CC=x86_64-w64-mingw32-gcc
		export CXX=x86_64-w64-mingw32-g++
	fi
	binName="Ikemen_GO.exe"
}

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
	# go build -trimpath -v -trimpath -o ./bin/$binName ./src	// original with debug
	go build -tags=gles2,sdl -trimpath -v -trimpath -ldflags="-s -w" -o ./bin/$binName ./src
	# go build -tags=gles2 -trimpath -v -trimpath -ldflags="-s -w" -o ./bin/$binName ./src
	# go build -tags=steamdeck -trimpath -v -trimpath -ldflags="-s -w" -o ./bin/$binName ./src
	# go build -trimpath -v -trimpath -ldflags="-s -w" -o ./bin/$binName ./src
}

function build_RPi4() {
	echo "Building for Raspberry Pi 4"
	go build -tags=gles2,sdl -trimpath -v -trimpath -ldflags="-s -w" -o ./bin/$binName ./src
}

function build_Steamdeck() {
	echo "Building for Steamdeck"
	go build -tags=steamdeck -trimpath -v -trimpath -ldflags="-s -w" -o ./bin/$binName ./src
}

function build_RG353P() {
	echo "Building for Anbernic RG353P"
	GOARCH=arm64 CGO_ENABLED=1 go build -x -tags=gles2,rg353p -trimpath -v -trimpath -ldflags="-s -w" -o ./bin/$binName ./src
}

function build_RG35XX() {
	echo "Building for Anbernic RG35XX"
	CGO_CFLAGS="-Os -marm -march=armv7-a -mtune=cortex-a9 -mfpu=neon-fp16 -mfloat-abi=hard" GOARCH=arm CGO_ENABLED=1 go build -x -tags=rg35xx -trimpath -v -trimpath -ldflags="-s -w" -o ./bin/$binName ./src
}

function buildWin() {
	#echo "buildWin"
	#echo "$binName"
	go build -trimpath -v -trimpath -ldflags "-H windowsgui" -o ./bin/$binName ./src
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
