#!/bin/bash

cd "$(dirname "$0")"
source ../.prod.env

# Script to view logs from Raspberry Pi

# Print status message
echo "Viewing logs live from Raspberry Pi (press Ctrl+C to stop)..."

# Execute command on Raspberry Pi via SSH
ssh -i "$PI_KEY" -q -T "$PI_USER@$PI_HOST" "tail -f $PI_ROOT_PATH/$PI_LOG_FILE"