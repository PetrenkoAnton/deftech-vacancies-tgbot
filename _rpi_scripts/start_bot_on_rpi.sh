#!/bin/bash

cd "$(dirname "$0")"
source ../.prod.env

# Script to stop previous bot and start the current one on Raspberry Pi

# Print status message
echo "Stopping previous bot and starting new one..."

# Execute commands on Raspberry Pi via SSH
ssh -i "$PI_KEY" -q -T "$PI_USER@$PI_HOST" "sudo systemctl restart deftech-tgbot"
 
# Print success message
echo "Bot started."