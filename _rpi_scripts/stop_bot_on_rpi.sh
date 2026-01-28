#!/bin/bash

cd "$(dirname "$0")"
source ../.prod.env

# Script to stop the bot on Raspberry Pi

# Print status message
echo "Stopping bot..."

# Execute command on Raspberry Pi via SSH
ssh -i "$PI_KEY" -q -T "$PI_USER@$PI_HOST" "sudo systemctl stop deftech-tgbot"

# Print success message
echo "Bot stopped."