# Miltech Telegram Bot

A Telegram bot that fetches and displays job listings from Dwarf Engineering's career pages on PeopleForce and DOU.ua.

## Features

- 📋 `/list` - Fetches job titles from PeopleForce careers pages (pages 1 and 2)
- 📋 `/list_dou` - Fetches job titles from DOU.ua RSS feed
- 🎯 Real-time job listings
- 🔄 Automatic updates from multiple sources
- 💬 User-friendly command interface

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

The bot requires the following environment variable:

- `BOT_TOKEN` - Your Telegram bot token obtained from BotFather

You can set this in the `.env` file or as a system environment variable. The `.env` file takes precedence if it exists.

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

Alternatively, you can use the `cmd` directory version:

```bash
go run cmd/main.go
```

### Commands

- `/start` - Start the bot and see welcome message
- `/help` - Show available commands
- `/list` - Get job listings from PeopleForce
- `/list_dou` - Get job listings from DOU.ua

## Project Structure

```
miltech-tgbot/
├── cmd/
│   └── main.go          # Alternative entry point
├── internal/
│   └── bot/
│       └── bot.go       # Bot implementation and handlers
├── pkg/
│   └── utils/
│       └── utils.go     # Utility functions
├── bin/                 # Compiled binaries
├── main.go              # Main entry point
├── go.mod               # Go module file
├── go.sum               # Go checksums
├── .env.example         # Environment variables template
├── .gitignore           # Git ignore rules
└── README.md            # This file
```

## Dependencies

- [gopkg.in/telebot.v3](https://github.com/tucnak/telebot) - Telegram bot framework
- [github.com/joho/godotenv](https://github.com/joho/godotenv) - Environment variable management
- [golang.org/x/net](https://golang.org/x/net) - HTML and XML parsing

## How It Works

### PeopleForce Integration (`/list`)

The bot fetches job listings from:
- https://dwarfengineering.peopleforce.io/careers (page 1)
- https://dwarfengineering.peopleforce.io/careers?page=2 (page 2)

It extracts job titles from `<a>` tags with class `stretched-link tw-text-black` and combines results from both pages, removing duplicates.

### DOU.ua Integration (`/list_dou`)

The bot fetches job listings from the RSS feed:
- https://jobs.dou.ua/vacancies/dwarf-engineering/feeds/

It parses the RSS XML feed and extracts job titles from `<title>` tags within `<item>` elements.

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
