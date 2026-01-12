# Miltech Telegram Bot

A Telegram bot that fetches and manages job listings from Dwarf Engineering and DefTech career pages.

## Features

- **Job Sources**:
  - Dwarf Engineering (PeopleForce + DOU.ua RSS)
  - DefTech (DOU.ua)

- **Commands**:
  - `/dwarf_engineering` - Get Dwarf Engineering jobs (simple list)
  - `/deftech_all` - Get all DefTech jobs with hide/show controls
  - `/deftech` - Get visible DefTech jobs only
  - `/truncate` - Clear all jobs from database

- **Database Features**:
  - SQLite database with migrations
  - Job persistence and deduplication
  - Hide/show functionality for DefTech jobs
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

### Commands

- `/start` - Welcome message and command overview
- `/help` - Show available commands
- `/dwarf_engineering` - Fetch jobs from Dwarf Engineering (PeopleForce + DOU.ua)
- `/deftech_all` - Fetch all DefTech jobs with interactive hide/show links
- `/deftech` - Show only visible (non-hidden) DefTech jobs
- `/truncate` - Clear all jobs from database (admin only)

### Hide/Show Functionality

DefTech commands (`/deftech_all`, `/deftech`) include interactive links to hide or show individual jobs. Click the links to toggle job visibility. Hidden jobs won't appear in `/deftech` command results.

## Project Structure

```
miltech-tgbot/
├── internal/bot/bot.go    # Bot logic, handlers, and database operations
├── migrations/           # Database schema migrations
├── main.go              # Application entry point
├── go.mod               # Go module dependencies
├── .env                 # Environment configuration
├── jobs.db              # SQLite database (auto-created)
└── bin/                 # Compiled binaries
```
