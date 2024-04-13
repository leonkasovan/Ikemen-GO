# Ikemen GO

Ikemen GO is an open source fighting game engine that supports resources from the [M.U.G.E.N](https://en.wikipedia.org/wiki/Mugen_(game_engine)) engine, written in Google’s programming language, [Go](https://go.dev/). It is a complete rewrite of a prior engine known simply as Ikemen.

## Ikemen Resource
Download 3D stages from here:   
https://www.mediafire.com/folder/syh6wacfmskeg/MBTL_Stages  

## How to make 3D stages
https://mugenguild.com/forum/topics/3d-stages-are-here-fairly-comprehensive-guide-198642.0.html  

### Building
You can find instructions for building Ikemen GO on our wiki. Instructions are available for [Windows](https://github.com/ikemen-engine/Ikemen-GO/wiki/Building,-Installing-and-Distributing#building-on-windows), [macOS](https://github.com/ikemen-engine/Ikemen-GO/wiki/Building,-Installing-and-Distributing#building-on-macos), and [Linux](https://github.com/ikemen-engine/Ikemen-GO/wiki/Building,-Installing-and-Distributing#building-on-linux).
```
git clone -b develop https://github.com/leonkasovan/Ikemen-GO.git
make
```

### Cross-compiling binaries with WSL2 Ubuntu
1. Download and install gcc toolchain for cross compile
```shell
wget https://github.com/leonkasovan/RG353P/releases/download/recalbox-9.1/rg353p-recalbox-toolchain.tar.gz
tar -C /opt -xvzf rg353p-recalbox-toolchain.tar.gz
```
2. Download and install golang
```shell
wget https://go.dev/dl/go1.22.0.linux-amd64.tar.gz
rm -rf /usr/local/go && tar -C /usr/local -xzf go1.22.0.linux-amd64.tar.gz
```
3. Set environment variable for cross compiling
```shell
export CC=aarch64-buildroot-linux-gnu-gcc
export CXX=aarch64-buildroot-linux-gnu-g++
export GOARCH=arm64
export CGO_ENABLED=1
export PATH="/home/ark/recalbox-rg353x/output/host/bin:/home/ark/recalbox-rg353x/output/host/aarch64-buildroot-linux-gnu/sysroot/usr/bin:/usr/local/go/bin:$PATH"

go build -tags=gles2,rg353p -trimpath -v -trimpath -ldflags="-s -w" -o ./bin/Ikemen_GO_Linux_RG353P ./src
```
