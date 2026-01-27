#!/bin/bash

source ../.env

# Script to view logs from Raspberry Pi

# Print status message
echo "Fetching last 100 lines of current logs from Raspberry Pi..."

# Execute command on Raspberry Pi via SSH
ssh -i "$PI_KEY" -q -T "$PI_USER@$PI_HOST" "tail -n 100 $PI_ROOT_PATH/$PI_LOG_FILE"