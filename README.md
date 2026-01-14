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
  - `/truncate` - Clear all vacancies from database
  - `/test_post` - Post a test message to the configured Telegram group

- **Automatic Posting**:
  - Configurable periodic fetching and posting of **new** DefTech job listings to Telegram group
  - Only posts when new jobs are discovered; logs otherwise
  - Set `INTERVAL` in minutes to enable automatic job updates

- **Database Features**:
  - SQLite database with migrations
  - Job persistence and deduplication
  - Hide/show functionality for DefTech jobs
  - Centralized admin authorization via middleware

- **Security**:
  - All bot commands require admin authorization
  - Centralized middleware checks `ADMIN_ID` for all requests
  - Unauthorized users receive access denied messages

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
# Required: BOT_TOKEN from @BotFather
# Required: ADMIN_ID - Your Telegram user ID (get from @userinfobot)
# Optional: GROUP_ID - Telegram group ID for automatic posting
# Optional: INTERVAL - Posting interval in minutes
# Optional: DB_NAME - Database file path (default: ./deftech-tgbot.db)
# Optional: VACANCIES_TABLE - Database table name (default: vacancies)
```

## Usage

Build and run:
```bash
go build -o bin/deftech-tgbot .
./bin/deftech-tgbot
```

### Commands

**Note**: All commands require admin authorization. Configure `ADMIN_ID` in your `.env` file.

- `/start` - Welcome message and command overview
- `/help` - Show available commands
- `/dwarf_engineering` - Fetch jobs from Dwarf Engineering (PeopleForce + DOU.ua)
- `/deftech_all` - Fetch all DefTech jobs with interactive hide/show links
- `/deftech` - Show only visible (non-hidden) DefTech jobs
- `/truncate` - Clear all jobs from database (admin only)
- `/test_post` - Post a test message to the configured group

### Automatic Posting

If `INTERVAL` is set in the `.env` file (in minutes), the bot will automatically fetch DefTech job listings at the specified interval. It will only post to the configured Telegram group when **new jobs are discovered**. If no new jobs are found, it will simply log the check without posting anything. The `/deftech` command can still be used for manual fetching of all visible jobs.

### Admin Authorization

All bot commands require admin authorization. The bot uses centralized middleware to check the `ADMIN_ID` environment variable against the user's Telegram ID for every request:

- **Required**: Set `ADMIN_ID` in your `.env` file to your Telegram user ID
- **How to get your ID**: Send `/start` to [@userinfobot](https://t.me/userinfobot)
- **Access Control**: Unauthorized users will receive "Sorry, you are not authorized to use this bot." messages
- **Development**: If `ADMIN_ID` is not set, all users are allowed (for development purposes)

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
├── deftech-tgbot.db     # SQLite database (auto-created, configurable via DB_NAME)
└── bin/                 # Compiled binaries
```

---

*This code is generated via GrokCodeFast1 model*
