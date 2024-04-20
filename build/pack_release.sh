#!/bin/bash
cd ..

if [ ! -d ./bin ]; then
	exit
fi

cd bin

if [ ! -d ./release ]; then
	mkdir release
fi

7zr a ./release/Ikemen_GO.7z ../external ../data ../font 'IkemenGO_x86.exe' 'Ikemen_GO.exe' Ikemen_GO_RG353P Ikemen_GO_Steamdeck
# cp ./release/Ikemen_GO.7z /mnt/c/PortableApps/
cp ./release/Ikemen_GO.7z ~/Applications/IkemenGo/