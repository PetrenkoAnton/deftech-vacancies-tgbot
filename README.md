# Miltech Telegram Bot

A Telegram bot that fetches job listings from Dwarf Engineering and DefTech career pages.

## Features

- `/dwarf_engineering` - Get jobs from PeopleForce and DOU.ua
- `/list_deftech` - Get jobs from DefTech DOU.ua
- SQLite database for job persistence
- Admin-only access control

## Prerequisites

- Go 1.24.0+
- Telegram Bot Token from [@BotFather](https://t.me/BotFather)

## Setup

1. Clone and install:
```bash
git clone <repository-url>
cd miltech-tgbot
go mod download
```

2. Configure `.env`:
```bash
cp .env.example .env
# Add BOT_TOKEN and optional ADMIN_ID
```

## Usage

Build and run:
```bash
go build -o bin/miltech-tgbot .
./bin/miltech-tgbot
```

Commands:
- `/start` - Welcome and command list
- `/help` - Show commands
- `/dwarf_engineering` - Dwarf Engineering jobs
- `/list_deftech` - DefTech jobs

## Project Structure

```
miltech-tgbot/
├── internal/bot/bot.go    # Bot logic and handlers
├── main.go               # Entry point
├── go.mod                # Dependencies
├── .env                  # Configuration
└── jobs.db               # SQLite database
```
