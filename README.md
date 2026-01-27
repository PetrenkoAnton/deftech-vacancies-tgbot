# Deftech Vacancies Telegram Bot

[![CI](https://github.com/PetrenkoAnton/deftech-vacancies-tgbot/actions/workflows/ci.yml/badge.svg)](https://github.com/PetrenkoAnton/deftech-vacancies-tgbot/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/go-1.24-blue)](https://golang.org/)

A Telegram bot that fetches and manages vacancies from [deftech.dou.ua/jobs](https://deftech.dou.ua/jobs) and Dwarf Engineering company. Features interactive deep links for vacancy management with automatic message cleanup.

## Features

- **Job Sources**:
  - [deftech.dou.ua/jobs](https://deftech.dou.ua/jobs)
  - Dwarf Engineering ([dwarfengineering.peopleforce.io/careers](https://dwarfengineering.peopleforce.io/careers), [jobs.dou.ua/companies/dwarf-engineering/vacancies](https://jobs.dou.ua/companies/dwarf-engineering/vacancies), and [djinni.co/jobs/company-dwarf-engineering](https://djinni.co/jobs/company-dwarf-engineering/))

- **Commands**:
  - `/get_saved_visible` - Get visible deftech vacancies only (from database, excludes Dwarf Engineering)
  - `/fetch_newest` - Fetch and post only new deftech vacancies
  - `/fetch_latest` - Fetch and display latest deftech vacancies (no saving)
  - `/dwarf_engineering` - Get Dwarf Engineering vacancies (djinni.co listings include views/applies in format: title | views / applies)
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
git clone https://github.com/PetrenkoAnton/deftech-vacancies-tgbot.git
cd deftech-vacancies-tgbot
go mod download
```

2. Configure `.env`:
```bash
cp .env.example .env
```
```bash
BOT_TOKEN= # from @BotFather
ADMIN_ID= #Your Telegram user ID (get from @userinfobot)
INTERVAL=1 #Posting interval in minutes
DEFTECH_URL=https://deftech.dou.ua/jobs/?city=%D0%9A%D0%B8%D1%97%D0%B2 #Deftech vacancies URL
DB_NAME="./deftech-tgbot.db"
LIMIT=50 #max vacancies to display in saved lists (20-200)
```

## Raspberry Pi Deployment

For deploying the bot on a Raspberry Pi (ARM64), follow these steps:

### Prerequisites
- Raspberry Pi with SSH access
- SSH key pair configured for passwordless login
- Go installed on the local machine for cross-compilation

### Configuration
Update the Raspberry Pi connection details in `.env`:
```bash
PI_HOST=                          # e.g., "192.168.1.100"
PI_USER=                          # e.g., "pi"
PI_KEY=                           # e.g., "$HOME/.ssh/id_rsa"
PI_ROOT_PATH=                     # e.g., "/home/pi/deftech-tgbot"
LOG_PATH=                         # e.g., "/home/pi/deftech-tgbot/deftech-tgbot.log"
PI_BINARY=                        # e.g., "deftech-tg-bot-rpi"
```

### Deployment Steps
1. **Build for ARM64**:
   ```bash
   ./_rpi_scripts/build_for_rpi.sh
   ```

2. **Copy binary and environment file to Raspberry Pi**:
   ```bash
   ./_rpi_scripts/copy_bin_to_rpi.sh
   ```

3. **Start the bot on Raspberry Pi**:
   ```bash
   ./_rpi_scripts/start_bot_on_rpi.sh
   ```

4. **Stop the bot**:
   ```bash
   ./_rpi_scripts/stop_bot_on_rpi.sh
   ```

5. **View logs**:
   ```bash
   ./_rpi_scripts/view_logs_rpi.sh
   ```

### Database Management
- **Copy database to Pi**:
  ```bash
   ./_rpi_scripts/copy_db_to_rpi.sh
  ```

- **Copy database from Pi to local**:
  ```bash
   ./_rpi_scripts/copy_db_from_rpi.sh
  ```

### Notes
- All scripts source the `.env` file for configuration.
- Ensure SSH key authentication is set up between your local machine and the Raspberry Pi.
- The bot runs in the background on the Pi using `nohup`.
- Logs are written to `deftech-tgbot.log` on the Pi.

## Usage

Build and run locally:
```bash
go build -o _bin/deftech-tgbot .
./_bin/deftech-tgbot
```

Or use the convenience script:
```bash
./build_and_run.sh
```

For Raspberry Pi deployment, see the [Raspberry Pi Deployment](#raspberry-pi-deployment) section.

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

Bot commands (`/get_saved_latest`, `/fetch_newest`, `/fetch_latest`) include interactive deep links to hide or show individual vacancies. Clicking the [hide] or [show] links will execute the action via `/start` command with appropriate payload and automatically delete the command message to keep the chat clean. Hidden vacancies won't appear in `/get_saved_visible` command results.

## Project Structure

```
deftech-vacancies-tgbot/
├── main.go               # Application entry point
├── app/bot.go            # Bot logic, handlers, and database operations
├── migrations/           # Database schema migrations
│   └── 001_initial.sql   # Initial schema
├── _rpi_scripts/         # Additional Raspberry Pi management scripts
│   ├── build_for_rpi.sh
│   ├── copy_bin_to_rpi.sh
│   └── copy_db_from_rpi.sh
│   ├── copy_db_to_rpi.sh
│   ├── start_bot_on_rpi.sh
│   ├── stop_bot_on_rpi.sh
│   └── view_logs_rpi.sh
├── build_and_run.sh      # Build and run script
├── .env                  # Environment configuration
├── .env.example          # Environment configuration template
├── VERSION               # Bot version file
├── go.mod                # Go module dependencies
├── go.sum                # Go module checksums
├── deftech-tgbot.db      # SQLite database (auto-created, configurable via DB_NAME)
└── _bin/                  # Compiled binaries
```