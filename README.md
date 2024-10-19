```
git clone -b SDL2 https://github.com/leonkasovan/Ikemen-GO.git
chmod a+x build/build.sh
make steamdeck

git clone -b PSC https://github.com/leonkasovan/Ikemen-GO.git
chmod a+x build/build.sh
make psc
```

# BUILD FOR WINDOWS 64 (Using MSYS2 https://www.msys2.org/)  
1. Download Golang https://go.dev/dl/
2. Download Git Standalone Installer https://git-scm.com/download/win
3. Download MSys2 Installer https://www.msys2.org

Open Terminal UCRT in MSYS2  
```
sed -i '$a export PATH=$PATH:/c/Program\\ Files/Go/bin:/c/Program\\ Files/Git/cmd' ~/.bashrc
source ~/.bashrc
git clone -b SDL2 https://github.com/leonkasovan/Ikemen-GO.git
chmod a+x build/build.sh
make win64
```

Setting MSYS2 binary in Windows Shell:  
Add `C:\msys64\ucrt64\bin` into environment system variables  
```
git clone -b SDL2 https://github.com/leonkasovan/Ikemen-GO.git
cd Ikemen-GO/build
build.cmd
```

https://mugenguild.com/forum/  

Character  
Pots Collection: https://www.mediafire.com/folder/6bji8e36narp6/Characters  
Karma Charizard Collection: https://www.mediafire.com/folder/f4qxixm5h39cu  
Lessard Collection https://www.mediafire.com/folder/gf9p2w993dwka/LESSARD_MUGEN

3D Stages  
https://www.mediafire.com/folder/syh6wacfmskeg/MBTL_Stages  
https://www.mediafire.com/folder/4yrw405s2eeal/StagePacks  
https://www.mediafire.com/folder/w6wgk5xo7sraz/Stages  
xcheatdeath https://www.mediafire.com/folder/oz0kp2v4juism/3D_STAGES  
bigchungusfartporn https://www.mediafire.com/folder/t9oic8khyxxk2/fighting_game
Library for v.0.98.2
```
sudo pacman -S libsysprof-capture pcre openal pcre2 libffi freetype2 xcb libpng brotli graphite util-linux-libs
fribidi libthai libdatrie fontconfig expat libxcb libxau libxdmcp libxft pixman libjpeg-turbo libtiff zstd liblzma
xz shared-mime-info libxcomposite libxdamage wayland libxkbcommon libexpoxy libepoxy libcloudproviders dbus libxtst
```
