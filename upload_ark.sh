#!/bin/bash

# File to upload
FILE="port/ikemen.zip"
# FILE="bin/ikemen_linux"

# Remote server info
USER="ark"
HOST="192.168.1.32"
REMOTE_DIR="/opt/system/Tools/PortMaster/autoinstall"
# REMOTE_DIR="/roms/ports/ikemen"ssh
PASSWORD="ark"

# Upload using sshpass
sshpass -p "$PASSWORD" scp "$FILE" "${USER}@${HOST}:${REMOTE_DIR}"

# Check result
if [ $? -eq 0 ]; then
    echo "Upload $FILE successful."
else
    echo "Upload $FILE failed."
fi
