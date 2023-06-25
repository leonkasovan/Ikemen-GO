#!/bin/bash
cd ..
export CGO_ENABLED=1

echo "Downloading dependencies..."
echo ""

if [ ! -f ./go.mod ]; then
	go mod init github.com/ikemen-engine/Ikemen-GO
	echo ""
fi
