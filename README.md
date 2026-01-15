# Deftech Vacancies Telegram Bot

A Telegram bot that fetches and manages vacancies from [deftech.dou.ua/jobs](https://deftech.dou.ua/jobs) and Dwarf Engineering company.

## Features

- **Job Sources**:
  - [deftech.dou.ua/jobs](https://deftech.dou.ua/jobs)
  - Dwarf Engineering ([dwarfengineering.peopleforce.io/careers](https://dwarfengineering.peopleforce.io/careers) and [jobs.dou.ua/companies/dwarf-engineering/vacancies](https://jobs.dou.ua/companies/dwarf-engineering/vacancies))

- **Commands**:
  - `/get_saved_visible` - Get visible deftech vacancies only (from database, excludes Dwarf Engineering)
  - `/fetch_newest` - Fetch and post only new deftech vacancies
  - `/fetch_latest` - Fetch and display latest deftech vacancies (no saving)
  - `/dwarf_engineering` - Get Dwarf Engineering vacancies
  - `/get_saved_latest` - Get all saved vacancies with hide/show controls (excludes Dwarf Engineering)
  - `/clear_saved` - Clear hidden vacancies from database

- **Automatic Posting**:
  - Configurable periodic fetching and posting of **new** deftech vacancies to the admin
  - Only posts when new vacancies are discovered; logs otherwise
  - Set `INTERVAL` in minutes to enable automatic vacancy updates

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
git clone https://github.com/PetrenkoAnton/deftech-vacancies-tgbot
cd deftech-vacancies-tgbot
go mod download
```

2. Configure `.env`:
```bash
cp .env.example .env
# Required: BOT_TOKEN from @BotFather
# Required: ADMIN_ID - Your Telegram user ID (get from @userinfobot)
# Required: INTERVAL - Posting interval in minutes
# Required: DEFTECH_URL - DefTech vacancies URL (default: https://deftech.dou.ua/jobs/?city=%D0%9A%D0%B8%D1%97%D0%B2)
# Optional: DB_NAME - Database file path (default: ./deftech-tgbot.db)
# Optional: VACANCIES_TABLE - Database table name (default: vacancies)
# Required: LIMIT - Maximum vacancies to display in saved lists (1-50)
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
- `/get_saved_visible` - Show only visible (non-hidden) DefTech jobs from database (excludes Dwarf Engineering)
- `/fetch_newest` - Fetch and post only new DefTech jobs
- `/fetch_latest` - Fetch and display latest DefTech jobs without saving
- `/dwarf_engineering` - Fetch jobs from Dwarf Engineering (PeopleForce + DOU.ua)
- `/get_saved_latest` - Fetch all saved jobs with interactive hide/show links (excludes Dwarf Engineering, shows total count of all saved vacancies)
- `/clear_saved` - Clear hidden jobs from database (admin only)

### Automatic Posting

If `INTERVAL` is set in the `.env` file (in minutes), the bot will automatically fetch deftech vacancies at the specified interval. It will only post to the admin when **new vacancies are discovered**. If no new vacancies are found, it will simply log the check without posting anything. The `/get_saved_visible` command can still be used for manual fetching of all visible jobs.

### Admin Authorization

All bot commands require admin authorization. The bot uses centralized middleware to check the `ADMIN_ID` environment variable against the user's Telegram ID for every request:

- **Required**: Set `ADMIN_ID` in your `.env` file to your Telegram user ID
- **How to get your ID**: Send `/start` to [@userinfobot](https://t.me/userinfobot)
- **Access Control**: Unauthorized users will receive "Sorry, you are not authorized to use this bot." messages

### Hide/Show Functionality

Bot commands (`/get_saved_latest`) include interactive links to hide or show individual vacancies. Click the links to toggle vacancy visibility. Hidden vacancies won't appear in `/get_saved_visible` command results.

## Project Structure

```
deftech-vacancies-tgbot/
├── main.go               # Application entry point
├── app/bot.go            # Bot logic, handlers, and database operations
├── migrations/           # Database schema migrations
│   └── 001_initial.sql   # Initial schema
├── build_and_run.sh      # Build and run script
├── .env                  # Environment configuration
├── .env.example          # Environment configuration template
├── VERSION               # Bot version file
├── go.mod                # Go module dependencies
├── go.sum                # Go module checksums
├── deftech-tgbot.db      # SQLite database (auto-created, configurable via DB_NAME)
└── bin/                  # Compiled binaries
```