# Deftech Vacancies Telegram Bot

[![CI](https://github.com/PetrenkoAnton/deftech-vacancies-tgbot/actions/workflows/ci.yml/badge.svg)](https://github.com/PetrenkoAnton/deftech-vacancies-tgbot/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/go-1.24-blue)](https://golang.org/)

A Telegram bot that fetches and manages vacancies from [deftech.dou.ua/jobs](https://deftech.dou.ua/jobs) and Dwarf Engineering company. Features interactive deep links for vacancy management with automatic message cleanup.

## Features

- **Job Sources**:
  - [deftech.dou.ua/jobs](https://deftech.dou.ua/jobs)
  - Dwarf Engineering ([dwarfengineering.peopleforce.io/careers](https://dwarfengineering.peopleforce.io/careers), [jobs.dou.ua/companies/dwarf-engineering/vacancies](https://jobs.dou.ua/companies/dwarf-engineering/vacancies), and [djinni.co/jobs/company-dwarf-engineering](https://djinni.co/jobs/company-dwarf-engineering/))
  - Buntar Aerospace ([jobs.dou.ua/vacancies/buntar-aerospace](https://jobs.dou.ua/vacancies/buntar-aerospace/))

- **Commands**:
- `/get_saved_visible` - Get visible saved vacancies only (from database)
- `/fetch_newest` - Fetch and post only new deftech vacancies (from first 2 pages)
- `/fetch_latest` - Fetch and display latest deftech vacancies (from first 2 pages, no saving)
  - `/dwarf_engineering` - Get Dwarf Engineering vacancies (djinni.co listings include views/applies in format: title | views / applies)
  - `/buntar_aerospace` - Get Buntar Aerospace vacancies
  - `/get_saved_latest` - Get all saved vacancies with hide/show controls
  - `/hide_all` - Hide all visible vacancies
  - `/clear_saved` - Clear hidden vacancies from database
  - `/build_version` - Show current build version

- **Automatic Posting**:
  - Configurable periodic fetching and posting of **new** deftech vacancies to the admin
  - Only posts when new vacancies are discovered; logs otherwise
  - Set `INTERVAL` in minutes to enable automatic vacancy updates
  - Fetches from the first 2 pages of job listings for comprehensive coverage

- **Message Batching**:
  - Long vacancy lists are automatically split into messages containing up to 50 vacancies each
  - Helps manage Telegram's message length limits and improves readability

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
BOT_TOKEN=  # From @BotFather
ADMIN_ID=   # Your Telegram user ID (get from @userinfobot)
INTERVAL=1  # Posting interval in minutes
DEFTECH_URL="https://deftech.dou.ua/jobs/?city=%D0%9A%D0%B8%D1%97%D0%B2" # Deftech vacancies URL
DB_NAME="deftech-tgbot.db"
LIMIT=50    # Max vacancies to display in saved lists (20-200), lists are batched into messages of up to 50 vacancies each
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
PI_HOST=       # e.g., "192.168.1.100"
PI_USER=       # e.g., "pi"
PI_KEY=        # e.g., "$HOME/.ssh/id_rsa"
PI_ROOT_PATH=  # e.g., "/home/pi/deftech-tgbot"
PI_LOG_FILE=   # e.g., "deftech-tgbot.log"
PI_BINARY=     # e.g., "deftech-tgbot-rpi"
```

For deploying the bot on a Raspberry Pi (ARM64), follow these steps:

### Prerequisites
- Raspberry Pi with SSH access
- SSH key pair configured for passwordless login
- Go installed on the local machine for cross-compilation

### Configuration
Update the Raspberry Pi connection details in `.env`:
```bash
PI_HOST=       # e.g., "192.168.1.100"
PI_USER=       # e.g., "pi"
PI_KEY=        # e.g., "$HOME/.ssh/id_rsa"
PI_ROOT_PATH=  # e.g., "/home/pi/deftech-tgbot"
PI_LOG_FILE=   # e.g., "deftech-tgbot.log"
PI_BINARY=     # e.g., "deftech-tgbot-rpi"
```

For production deployment, create a `.prod.env` file by copying `.env` and updating it with production values (e.g., production BOT_TOKEN, ADMIN_ID, etc.). The deployment scripts will use `.prod.env` if available, otherwise `.env`.

### Deployment Steps
1. **Build for ARM64**:
   ```bash
   ./_rpi_scripts/build_for_rpi.sh
   ```

2. **Copy binary and environment file to Raspberry Pi**:
   ```bash
   ./_rpi_scripts/copy_bin_to_rpi.sh
   ```

3. **Set up Raspberry Pi (logrotate and auto-start service)**:
   ```bash
   ./_rpi_scripts/setup_rpi.sh
   ```

4. **Start the bot** (initial start, or restart later):
   ```bash
   ./_rpi_scripts/start_bot_on_rpi.sh
   ```

5. **Stop the bot**:
   ```bash
   ./_rpi_scripts/stop_bot_on_rpi.sh
   ```

6. **View logs** (live updates):
   ```bash
   ./_rpi_scripts/view_logs_rpi.sh
   ```

### Database Management
- **Copy database to Pi**:
  ```bash
   ./_rpi_scripts/copy_db_to_rpi.sh
  ```

- **Copy database from Pi to local** (with timestamp prefix):
  ```bash
   ./_rpi_scripts/copy_db_from_rpi.sh
  ```

### Notes
- All scripts source the `.env` file for configuration.
- Ensure SSH key authentication is set up between your local machine and the Raspberry Pi.
- The `setup_rpi.sh` script configures logrotate for 10 MB log limits (using `logrotate.conf` as template) and sets up a systemd service for auto-start on reboot.
- Logs are written to `deftech-tgbot.log` on the Pi.

For production deployment, create a `.prod.env` file by copying `.env` and updating it with production values (e.g., production BOT_TOKEN, ADMIN_ID, etc.). The deployment scripts will use `.prod.env` if available, otherwise `.env`.

### Deployment Steps
1. **Build for ARM64**:
   ```bash
   ./_rpi_scripts/build_for_rpi.sh
   ```

2. **Copy binary and environment file to Raspberry Pi**:
   ```bash
   ./_rpi_scripts/copy_bin_to_rpi.sh
   ```

3. **Set up Raspberry Pi (logrotate and auto-start service)**:
   ```bash
   ./_rpi_scripts/setup_rpi.sh
   ```

4. **Start the bot** (initial start, or restart later):
   ```bash
   ./_rpi_scripts/start_bot_on_rpi.sh
   ```

5. **Stop the bot**:
   ```bash
   ./_rpi_scripts/stop_bot_on_rpi.sh
   ```

6. **View logs** (live updates):
   ```bash
   ./_rpi_scripts/view_logs_rpi.sh
   ```

### Database Management
- **Copy database to Pi**:
  ```bash
   ./_rpi_scripts/copy_db_to_rpi.sh
  ```

- **Copy database from Pi to local** (with timestamp prefix):
  ```bash
   ./_rpi_scripts/copy_db_from_rpi.sh
  ```

### Notes
- All scripts source the `.env` file for configuration.
- Ensure SSH key authentication is set up between your local machine and the Raspberry Pi.
- The `setup_rpi.sh` script configures logrotate for 10 MB log limits (using `logrotate.conf` as template) and sets up a systemd service for auto-start on reboot.
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
- `/get_saved_visible` - Show only visible (non-hidden) saved jobs from database
- `/fetch_newest` - Fetch and post only new DefTech jobs (from first 2 pages)
- `/fetch_latest` - Fetch and display latest DefTech jobs without saving (from first 2 pages)
- `/dwarf_engineering` - Fetch jobs from Dwarf Engineering (PeopleForce + DOU.ua + djinni.co)
- `/buntar_aerospace` - Fetch jobs from Buntar Aerospace (DOU.ua RSS)
- `/get_saved_latest` - Fetch all saved jobs with interactive hide/show links (shows total count of all saved vacancies)
- `/hide_all` - Hide all visible vacancies (admin only)
- `/build_version` - Show current build version
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
│   ├── copy_db_from_rpi.sh
│   ├── copy_db_to_rpi.sh
│   ├── setup_rpi.sh
│   ├── start_bot_on_rpi.sh
│   ├── stop_bot_on_rpi.sh
│   ├── update_rpi.sh
│   └── view_logs_rpi.sh
├── build_and_run.sh      # Build and run script
├── logrotate.conf        # Logrotate configuration for RPi logs
├── .env                  # Dev environment configuration
├── .prod.env             # Production environment configuration
├── .env.example          # Environment configuration template
├── VERSION               # Bot version file
├── go.mod                # Go module dependencies
├── go.sum                # Go module checksums
├── deftech-tgbot.db      # SQLite database (auto-created, configurable via DB_NAME)
└── _bin/                 # Compiled binaries
```