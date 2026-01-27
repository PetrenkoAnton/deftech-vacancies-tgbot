#!/bin/bash

cd "$(dirname "$0")"
source ../.prod.env

# Script to stop the bot on Raspberry Pi

# Print status message
echo "Stopping bot..."

# Execute command on Raspberry Pi via SSH
ssh -i "$PI_KEY" -q -T "$PI_USER@$PI_HOST" "pkill -f $PI_BINARY || true"

# Print success message
echo "Bot stopped."