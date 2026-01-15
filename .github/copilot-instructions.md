# Cursor Rules for Miltech Telegram Bot

## Project Overview
This is a Telegram bot written in Go that fetches and manages job vacancies from:
- deftech.dou.ua/jobs
- Dwarf Engineering (dwarfengineering.peopleforce.io/careers and jobs.dou.ua/companies/dwarf-engineering/vacancies)

## Tech Stack
- **Language**: Go
- **Database**: SQLite
- **Bot Framework**: telebot (gopkg.in/telebot.v3)
- **Web Scraping**: Standard library + html package
- **Configuration**: Environment variables (.env file)

## Project Structure
```
/
├── main.go                 # Application entry point
├── internal/bot/bot.go     # Main bot logic and handlers
├── migrations/             # Database migration files
│   └── 001_initial.sql     # Initial schema
├── build_and_run.sh        # Build and run script
└── .env                    # Environment configuration
```

## Database Schema
- **companies**: id, name, created_at
- **vacancies**: id, title, url, company_id (NOT NULL, CASCADE DELETE), is_hidden, created_at

## Bot Commands
- `/start` - Initialize bot
- `/help` - Show available commands
- `/get_saved_visible` - Show visible deftech vacancies from database (excludes Dwarf Engineering)
- `/fetch_newest` - Fetch newest vacancies from deftech.dou.ua (only new ones)
- `/fetch_latest` - Fetch latest vacancies from deftech.dou.ua (all, no saving)
- `/dwarf_engineering` - Fetch Dwarf Engineering vacancies
- `/get_saved_all` - List all saved vacancies (excludes Dwarf Engineering)
- `/clear_saved` - Clear hidden vacancies

## Key Functions
- `handleGetSavedVisible` - Display visible deftech vacancies from database
- `handleFetchNewest` - Scrape and save only new deftech vacancies, send message if any new
- `handleFetchLatest` - Fetch and display latest deftech vacancies without saving
- `handleGetDwarfEngineering` - Fetch from multiple Dwarf sources
- `handleGetSavedAll` - Display filtered saved vacancies with total count
- `saveVacancy` - Insert vacancy if not exists
- `getAllVacancies` - Retrieve all vacancies from DB

## Coding Conventions
- Follow Go standard conventions
- Use meaningful variable names
- Add comments for complex logic
- Handle errors appropriately with logging
- Use telebot's Silent mode for non-intrusive messages
- Format messages in Markdown when using links

## Development Notes
- Use `go build -o ./bin/deftech-tgbot .` to build
- Run with `./bin/deftech-tgbot` or use `build_and_run.sh`
- Database file: `deftech-tgbot.db`
- Environment variables in `.env` (copy from `.env.example`)
- Bot token required for Telegram API

## AI Assistant Guidelines
- When modifying bot commands, update both handler registration and callback cases
- Database changes require migration updates
- Test web scraping functions carefully (sites may change)
- Maintain consistent error handling and user feedback
- Use absolute paths when referencing files in the workspace
- Do not auto commit and push changes
- Always write as short as posible commit message
- Always update README and actualize .github/copilot-instructions.md before committing changes