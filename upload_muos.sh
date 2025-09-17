#!/bin/bash

# File to upload
FILE="port/ikemen.zip"
# FILE="bin/ikemen_linux"

# Remote server info
USER="root"
HOST="192.168.1.15"
REMOTE_DIR="/mnt/mmc/MUOS/PortMaster/autoinstall/"
# REMOTE_DIR="/mnt/union/ports/ikemen/"
PASSWORD="root"

# Upload using sshpass
sshpass -p "$PASSWORD" scp "$FILE" "${USER}@${HOST}:${REMOTE_DIR}"

# Check result
if [ $? -eq 0 ]; then
    echo "Upload $FILE successful."
else
    echo "Upload $FILE failed."
fi
