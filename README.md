# Ikemen GO

Ikemen GO is an open source fighting game engine that supports resources from the [M.U.G.E.N](https://en.wikipedia.org/wiki/Mugen_(game_engine)) engine, written in Google’s programming language, [Go](https://go.dev/). It is a complete rewrite of a prior engine known simply as Ikemen.

## Ikemen Resource
Download stages from here:   
https://www.mediafire.com/folder/syh6wacfmskeg/MBTL_Stages  

### Building
You can find instructions for building Ikemen GO on our wiki. Instructions are available for [Windows](https://github.com/ikemen-engine/Ikemen-GO/wiki/Building,-Installing-and-Distributing#building-on-windows), [macOS](https://github.com/ikemen-engine/Ikemen-GO/wiki/Building,-Installing-and-Distributing#building-on-macos), and [Linux](https://github.com/ikemen-engine/Ikemen-GO/wiki/Building,-Installing-and-Distributing#building-on-linux).
```
git clone -b rg35xx https://github.com/leonkasovan/Ikemen-GO.git
make
```

### Cross-compiling binaries with WSL2 Ubuntu
1. Download and install gcc toolchain for cross compile
```shell
sudo apt install p7zip
wget https://github.com/leonkasovan/RG35XX
7zr x rg35xx -o
export CC=aarch64-buildroot-linux-gnu-gcc
export CXX=aarch64-buildroot-linux-gnu-g++
export PATH="/opt/host/bin:/opt/host/aarch64-buildroot-linux-gnu/sysroot/usr/bin:/usr/local/go/bin:$PATH"
```
2. Download and install golang
```shell
wget https://go.dev/dl/go1.22.0.linux-amd64.tar.gz
rm -rf /usr/local/go && tar -C /usr/local -xzf go1.22.0.linux-amd64.tar.gz
export PATH="/usr/local/go/bin:$PATH"
```
3. Building
```shell
CGO_CFLAGS="-Os -marm -march=armv7-a -mtune=cortex-a9 -mfpu=neon-fp16 -mfloat-abi=hard" \
GOOS=linux \
GOARCH=arm \
CGO_ENABLED=1 \
go build -x -tags=rg35xx -trimpath -v -trimpath -ldflags="-s -w" -o Ikemen_GO_Linux_RG35XX ./src
```
