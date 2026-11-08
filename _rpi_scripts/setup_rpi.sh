#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")"
source ../.prod.env

: "${PI_KEY:?PI_KEY is not set}"
: "${PI_USER:?PI_USER is not set}"
: "${PI_HOST:?PI_HOST is not set}"
: "${PI_ROOT_PATH:?PI_ROOT_PATH is not set}"
: "${PI_BINARY:?PI_BINARY is not set}"
: "${PI_LOG_FILE:?PI_LOG_FILE is not set}"

SSH=(ssh -i "$PI_KEY" -q -T "$PI_USER@$PI_HOST")
SCP=(scp -i "$PI_KEY")
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Setting up Raspberry Pi for deftech-tgbot..."

cat > "$tmp/deftech-tgbot.service" << EOF
[Unit]
Description=Deftech Vacancies Telegram Bot
After=network.target

[Service]
Type=simple
User=$PI_USER
WorkingDirectory=$PI_ROOT_PATH
ExecStartPre=/bin/sleep 10
ExecStart=$PI_ROOT_PATH/$PI_BINARY
StandardOutput=append:$PI_ROOT_PATH/$PI_LOG_FILE
StandardError=append:$PI_ROOT_PATH/$PI_LOG_FILE
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

cat > "$tmp/deftech-tgbot-restart.service" << EOF
[Unit]
Description=Restart deftech-tgbot
After=network.target

[Service]
Type=oneshot
ExecStart=/bin/systemctl restart deftech-tgbot
EOF

cat > "$tmp/deftech-tgbot-restart.timer" << EOF
[Unit]
Description=Restart deftech-tgbot every 2 hours

[Timer]
OnBootSec=2h
OnUnitActiveSec=2h
Persistent=true

[Install]
WantedBy=timers.target
EOF

echo "Copying setup files..."
"${SCP[@]}" ../logrotate.conf \
  "$tmp/deftech-tgbot.service" \
  "$tmp/deftech-tgbot-restart.service" \
  "$tmp/deftech-tgbot-restart.timer" \
  "$PI_USER@$PI_HOST:/tmp/"

echo "Installing logrotate, systemd service, and restart timer..."
"${SSH[@]}" "\
set -euo pipefail
sudo mv /tmp/logrotate.conf /etc/logrotate.d/deftech-tgbot
sudo chown root:root /etc/logrotate.d/deftech-tgbot
sudo mv /tmp/deftech-tgbot.service /etc/systemd/system/
sudo mv /tmp/deftech-tgbot-restart.service /etc/systemd/system/
sudo mv /tmp/deftech-tgbot-restart.timer /etc/systemd/system/
chmod +x '$PI_ROOT_PATH/$PI_BINARY' || true
if systemctl list-unit-files | grep -q '^deftech-tgbot.timer'; then
  sudo systemctl disable --now deftech-tgbot.timer || true
  sudo rm -f /etc/systemd/system/deftech-tgbot.timer
fi
sudo systemctl daemon-reload
sudo systemctl enable deftech-tgbot
sudo systemctl enable --now deftech-tgbot-restart.timer
"

echo "Raspberry Pi setup complete. The bot will auto-start on reboot and restart every 2 hours."
