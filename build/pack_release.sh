#!/bin/bash
cd ..

if [ ! -d ./bin ]; then
	exit
fi

cd bin

if [ ! -d ./release ]; then
	mkdir release
fi

7zr a ./release/Ikemen_GO.7z ../external ../data ../font License.txt 'IkemenGO_x86.exe' 'Ikemen_GO.exe' Ikemen_GO.command Ikemen_GO_Mac Ikemen_GO_Linux