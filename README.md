# Ikemen GO

Ikemen GO is an open source fighting game engine that supports resources from the [M.U.G.E.N](https://en.wikipedia.org/wiki/Mugen_(game_engine)) engine, written in Google’s programming language, [Go](https://go.dev/). It is a complete rewrite of a prior engine known simply as Ikemen.

```
git clone -b SDL2 https://github.com/leonkasovan/Ikemen-GO.git
sudo apt install gcc-mingw-w64-x86-64-posix
export PATH="/opt/host/bin:/usr/local/go/bin:$PATH"
make

export PATH="/opt/host/bin:/usr/local/go/bin:$PATH"
export CC=aarch64-buildroot-linux-gnu-gcc
export CXX=aarch64-buildroot-linux-gnu-g++
make rg353p

export PATH="/opt/host/bin:/usr/local/go/bin:$PATH"
make steamdeck
```

