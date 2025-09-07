#!/bin/bash

# File to upload
FILE="port/ikemen.zip"
# FILE="bin/ikemen_linux"

# Remote server info
USER="root"
HOST="192.168.1.40"
REMOTE_DIR="/storage/roms/ports/PortMaster/autoinstall"
# REMOTE_DIR="/storage/roms/ports/ikemen"
PASSWORD="amberelec"

# Upload using sshpass
sshpass -p "$PASSWORD" scp "$FILE" "${USER}@${HOST}:${REMOTE_DIR}"

# Check result
if [ $? -eq 0 ]; then
    echo "Upload $FILE successful."
else
    echo "Upload $FILE failed."
fi
