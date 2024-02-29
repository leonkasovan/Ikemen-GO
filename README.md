# Ikemen GO

Ikemen GO is an open source fighting game engine that supports resources from the [M.U.G.E.N](https://en.wikipedia.org/wiki/Mugen_(game_engine)) engine, written in Google’s programming language, [Go](https://go.dev/). It is a complete rewrite of a prior engine known simply as Ikemen.

## Ikemen Resource
Download stages from here:   
https://www.mediafire.com/folder/syh6wacfmskeg/MBTL_Stages  

### Building
You can find instructions for building Ikemen GO on our wiki. Instructions are available for [Windows](https://github.com/ikemen-engine/Ikemen-GO/wiki/Building,-Installing-and-Distributing#building-on-windows), [macOS](https://github.com/ikemen-engine/Ikemen-GO/wiki/Building,-Installing-and-Distributing#building-on-macos), and [Linux](https://github.com/ikemen-engine/Ikemen-GO/wiki/Building,-Installing-and-Distributing#building-on-linux).
```
git clone -b develop https://github.com/leonkasovan/Ikemen-GO.git
make
```

### Cross-compiling binaries with WSL2 Ubuntu
1. Download and install golang
```shell
wget https://go.dev/dl/go1.22.0.linux-amd64.tar.gz
rm -rf /usr/local/go && tar -C /usr/local -xzf go1.22.0.linux-amd64.tar.gz
```
2. Set environment variable for cross compiling
```shell
export PATH=$PATH:/usr/local/go/bin
export CC=
export CXX=
export CGO_CFLAGS=
export CGO_LDFLAGS=
```
