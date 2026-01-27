#!/bin/bash

source ../.env

# Script to stop previous bot and start the current one on Raspberry Pi

# Print status message
echo "Stopping previous bot and starting new one..."

# Execute commands on Raspberry Pi via SSH
ssh -i "$PI_KEY" -q -T "$PI_USER@$PI_HOST" << EOF
  pkill -f $PI_BINARY || true
  cd $PI_ROOT_PATH
  > $PI_LOG_FILE
  nohup ./$PI_BINARY >> $PI_LOG_FILE 2>&1 &
EOF

# Print success message
echo "Bot started."