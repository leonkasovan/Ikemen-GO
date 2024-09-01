```
git clone -b SDL2 https://github.com/leonkasovan/Ikemen-GO.git
chmod a+x build/build.sh
make steamdeck

git clone -b PSC https://github.com/leonkasovan/Ikemen-GO.git
chmod a+x build/build.sh
make psc

# BUILD FOR WINDOWS 64 (Using MSYS2 https://www.msys2.org/)
pacman -Syy
pacman -S mingw-w64-ucrt-x86_64-go
git clone -b SDL2 https://github.com/leonkasovan/Ikemen-GO.git
chmod a+x build/build.sh
go mod tidy
make win64
```

https://mugenguild.com/forum/  

Character  
Pots Collection: https://www.mediafire.com/folder/6bji8e36narp6/Characters  
Karma Charizard Collection: https://www.mediafire.com/folder/f4qxixm5h39cu  

3D Stages  
https://www.mediafire.com/folder/syh6wacfmskeg/MBTL_Stages  
https://www.mediafire.com/folder/4yrw405s2eeal/StagePacks  
https://www.mediafire.com/folder/w6wgk5xo7sraz/Stages  

Library for v.0.98.2
```
sudo pacman -S libsysprof-capture pcre openal pcre2 libffi freetype2 xcb libpng brotli graphite util-linux-libs
fribidi libthai libdatrie fontconfig expat libxcb libxau libxdmcp libxft pixman libjpeg-turbo libtiff zstd liblzma
xz shared-mime-info libxcomposite libxdamage wayland libxkbcommon libexpoxy libepoxy libcloudproviders dbus libxtst
```
