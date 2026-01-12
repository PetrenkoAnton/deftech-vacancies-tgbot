# Miltech Telegram Bot

A Telegram bot that fetches and displays job listings from Dwarf Engineering's career pages on PeopleForce and DOU.ua, as well as DefTech job listings.

## Features

- 🏭 `/dwarf_engineering` - Fetches job titles from Dwarf Engineering (PeopleForce + DOU.ua)
- 🏢 `/list_deftech` - Fetches job titles from DefTech DOU.ua
- 🎯 Real-time job listings from multiple sources
- 🔄 Automatic updates and duplicate removal
- 💬 User-friendly command interface
- ⚡ Optimized performance with shared HTTP client
- 💾 SQLite database for job persistence

## Prerequisites

- Go 1.24.0 or higher
- Telegram Bot Token (obtain from [@BotFather](https://t.me/BotFather))

## Installation

1. Clone the repository:
```bash
git clone <repository-url>
cd miltech-tgbot
```

2. Install dependencies:
```bash
go mod download
```

3. Create a `.env` file in the root directory:
```bash
cp .env.example .env
```

4. Add your bot token to the `.env` file:
```
BOT_TOKEN=your_telegram_bot_token_here
```

## Configuration

The bot requires the following environment variables:

- `BOT_TOKEN` - Your Telegram bot token obtained from BotFather
- `ADMIN_ID` - (Optional) Telegram user ID of the admin user. If not set, the bot allows access to all users.

You can set these in the `.env` file or as system environment variables. The `.env` file takes precedence if it exists.

**Note:** For security, it's recommended to set `ADMIN_ID` to restrict bot access to authorized users only.

## Usage

### Build

```bash
go build -o bin/miltech-tgbot .
```

### Run

```bash
./bin/miltech-tgbot
```

Or run directly:

```bash
go run main.go
```

### Commands

- `/start` - Start the bot and see welcome message
- `/help` - Show available commands
- `/dwarf_engineering` - Get Dwarf Engineering jobs from PeopleForce and DOU.ua
- `/list_deftech` - Get job listings from DefTech DOU.ua

## Project Structure

```
miltech-tgbot/
├── internal/
│   └── bot/
│       └── bot.go       # Bot implementation, handlers, and job fetching logic
├── bin/                 # Compiled binaries
├── main.go              # Main entry point
├── go.mod               # Go module file
├── go.sum               # Go checksums
├── jobs.db              # SQLite database (created automatically)
├── .env.example         # Environment variables template
├── .gitignore           # Git ignore rules
└── README.md            # This file
```

## Dependencies

- [gopkg.in/telebot.v3](https://github.com/tucnak/telebot) - Telegram bot framework
- [github.com/joho/godotenv](https://github.com/joho/godotenv) - Environment variable management
- [github.com/mattn/go-sqlite3](https://github.com/mattn/go-sqlite3) - SQLite database driver
- [golang.org/x/net](https://golang.org/x/net) - HTML and XML parsing

## Recent Improvements

### v1.1.0 - Performance Optimizations (January 2026)

- **Shared HTTP Client**: Implemented single HTTP client instance to reduce connection overhead
- **Code Cleanup**: Removed unused handler functions and improved code maintainability
- **String Building Optimization**: Replaced string concatenation with `strings.Builder` for better performance
- **Database Integration**: Added SQLite persistence for job listings with automatic table creation
- **Enhanced Error Handling**: Improved error messages and logging throughout the application
- **DefTech Support**: Added support for fetching jobs from DefTech DOU.ua platform

## How It Works

### Dwarf Engineering Integration (`/dwarf_engineering`)

The bot fetches job listings from multiple sources:

**PeopleForce:**
- https://dwarfengineering.peopleforce.io/careers (page 1)
- https://dwarfengineering.peopleforce.io/careers?page=2 (page 2)

It extracts job titles from `<h4>` elements containing `<a>` tags and combines results from both pages, removing duplicates.

**DOU.ua RSS Feed:**
- https://jobs.dou.ua/vacancies/dwarf-engineering/feeds/

It parses the RSS XML feed and extracts job titles from `<title>` tags within `<item>` elements, cleaning up location suffixes.

### DefTech Integration (`/list_deftech`)

The bot fetches job listings from:
- https://deftech.dou.ua/jobs/?city=Київ

It parses the HTML page to extract job titles, companies, and identifies "hot" job postings with special markers.

### Database Storage

All fetched jobs are stored in a local SQLite database (`jobs.db`) with the following information:
- Job title
- Source (peopleforce, dou, deftech)
- URL
- Fetch timestamp

This allows for tracking job history and avoiding duplicate processing.

### Performance Optimizations

- **Shared HTTP Client**: Single HTTP client instance with 10-second timeout reused across all requests
- **Efficient String Building**: Uses `strings.Builder` for message formatting instead of string concatenation
- **Duplicate Removal**: Intelligent deduplication of job listings across sources
- **Memory Management**: Proper resource cleanup and connection reuse

## Development

### Running Tests

```bash
go test ./...
```

### Building for Different Platforms

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o bin/miltech-tgbot-linux .

# macOS
GOOS=darwin GOARCH=amd64 go build -o bin/miltech-tgbot-macos .

# Windows
GOOS=windows GOARCH=amd64 go build -o bin/miltech-tgbot.exe .
```

## Graceful Shutdown

The bot supports graceful shutdown. When you send `SIGINT` (Ctrl+C) or `SIGTERM`, it will:
1. Stop accepting new updates
2. Finish processing current requests
3. Clean up resources
4. Exit gracefully

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

[Add your license here]

## Support

For issues, questions, or contributions, please open an issue on the GitHub repository.

## Acknowledgments

- Dwarf Engineering for providing the job listings
- PeopleForce and DOU.ua for hosting the career pages
