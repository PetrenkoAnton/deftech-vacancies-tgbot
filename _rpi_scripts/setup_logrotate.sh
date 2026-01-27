#!/bin/bash

cd "$(dirname "$0")"
source ../.env

# Script to copy logrotate config to Raspberry Pi

# Print status message
echo "Copying logrotate config to Raspberry Pi..."

# Copy the logrotate config to Raspberry Pi using SCP
scp -i "$PI_KEY" ../logrotate.conf "$PI_USER@$PI_HOST":/tmp/logrotate.conf

# Install it on the Pi
ssh -i "$PI_KEY" -q -T "$PI_USER@$PI_HOST" "sudo mv /tmp/logrotate.conf /etc/logrotate.d/deftech-tgbot && sudo chown root:root /etc/logrotate.d/deftech-tgbot"

# Print success message
echo "Logrotate config installed successfully."