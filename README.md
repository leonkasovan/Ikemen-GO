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
# Build for Steamdeck
Enter desktop mode in Steamdeck and run Konsole:
```
# prepare build environtment
sudo steamos-readonly disable
sudo sed -i '/^SigLevel/ s/.*/SigLevel = Never/' /etc/pacman.conf
sudo pacman -Sy gcc cmake make autoconf binutils pkg-config
sudo pacman -Sy gcc glibc linux-api-headers alsa-lib libx11 xorgproto gtk3 glib2 libxcursor libxrandr pango libxrender libxinerama harfbuzz libxi cairo gdk-pixbuf2 libxext libxfixes at-spi2-core libglvnd sdl2
wget https://go.dev/dl/go1.22.2.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.22.2.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
sed -i '$a\export PATH=$PATH:/usr/local/go/bin' ~/.bashrc
sudo steamos-readonly enable

# clone repo and build it
git clone -b SDL2 https://github.com/leonkasovan/Ikemen-GO.git
cd Ikemen-GO
make steamdeck
ls -l bin/
# cp bin/Ikemen_Go_Steamdeck /home/deck/Games/Ikemen_GO_Default/
```
