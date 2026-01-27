#!/bin/bash

cd "$(dirname "$0")"
source ../.prod.env

# Script to set up Raspberry Pi for the bot: logrotate and systemd service

# Print status message
echo "Setting up Raspberry Pi for deftech-tgbot..."

# Step 1: Set up logrotate
echo "Setting up logrotate..."
scp -i "$PI_KEY" ../logrotate.conf "$PI_USER@$PI_HOST":/tmp/logrotate.conf
ssh -i "$PI_KEY" -q -T "$PI_USER@$PI_HOST" "sudo mv /tmp/logrotate.conf /etc/logrotate.d/deftech-tgbot && sudo chown root:root /etc/logrotate.d/deftech-tgbot"
echo "Logrotate configured."

# Step 2: Set up systemd service
echo "Setting up systemd service..."
cat > /tmp/deftech-tgbot.service << EOF
[Unit]
Description=Deftech Vacancies Telegram Bot
After=network.target

[Service]
Type=simple
User=$PI_USER
WorkingDirectory=$PI_ROOT_PATH
ExecStart=$PI_ROOT_PATH/$PI_BINARY
Restart=always
RestartSec=5
EnvironmentFile=$PI_ROOT_PATH/.env

[Install]
WantedBy=multi-user.target
EOF

scp -i "$PI_KEY" /tmp/deftech-tgbot.service "$PI_USER@$PI_HOST":/tmp/
ssh -i "$PI_KEY" -q -T "$PI_USER@$PI_HOST" "sudo mv /tmp/deftech-tgbot.service /etc/systemd/system/ && sudo systemctl daemon-reload && sudo systemctl enable deftech-tgbot"
echo "Systemd service installed and enabled."

# Print success message
echo "Raspberry Pi setup complete. The bot will auto-start on reboot."