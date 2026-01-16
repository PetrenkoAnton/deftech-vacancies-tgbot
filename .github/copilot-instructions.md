# Copilot Instructions for Deftech Vacancies Telegram Bot

## AI Assistant Guidelines
- **DONT AUTO COMMIT** - Always wait for explicit user instruction before committing changes
- **WRITE SHORT COMMIT MESSAGE** - Keep commit messages concise and relevant when asked to commit and push

## Architecture Overview
Go-based Telegram bot with SQLite persistence that scrapes job vacancies from deftech.dou.ua and Dwarf Engineering sources (PeopleForce, DOU.ua RSS, and djinni.co). Single `Bot` struct in `app/bot.go` manages all operations: HTTP scraping, database interactions, and Telegram messaging.

## Key Components
- **main.go**: Environment loading, bot initialization, graceful shutdown
- **app/bot.go**: Core bot logic (1221 lines) - handlers, scraping, DB operations
- **migrations/001_initial.sql**: Companies/vacancies schema with foreign key constraints
- **build_and_run.sh**: Build script that kills existing processes before restart

## Critical Workflows
- **Build & Run**: `go build -o ./bin/deftech-tgbot . && ./bin/deftech-tgbot` or `./build_and_run.sh`
- **Database**: Auto-migrates on startup from `migrations/*.sql`; uses configurable table names
- **Scraping**: HTML parsing with `golang.org/x/net/html`; deftech uses complex link pairing logic (see `findJobTitlesDeftech`)
- **Periodic Posting**: Goroutine-based ticker posts only new vacancies when `INTERVAL` env var set

## Project Conventions
- **Admin Auth**: Global middleware checks `ADMIN_ID` env var; logs unauthorized attempts with user details
- **Vacancy Storage**: Checks existence by title before saving; uses company foreign keys
- **Message Formatting**: Markdown links for vacancies; inline keyboards for commands; hide/show buttons separated by |
- **Error Handling**: Logs errors but continues operation; user-facing messages use `telebot.Silent`
- **Configuration**: All settings via environment variables; `.env` loaded with `godotenv`

## Integration Patterns
- **Telegram API**: `telebot.v3` framework with long poller; handlers registered in `registerHandlers()`
- **Database**: SQLite with prepared statements; indexes on `created_at` and `company_id`
- **Web Scraping**: HTTP client with 10s timeout; parses HTML for job links and company associations
- **External Sources**: Multiple Dwarf Engineering endpoints combined into single response (PeopleForce HTML, DOU.ua RSS, djinni.co HTML with views/applies metadata in "title | views / applies" format)

## Development Notes
- **Testing Scraping**: Sites change frequently; test `FetchJobTitlesFromDeftech()` after modifications
- **Database Changes**: Add new migration files; schema uses CASCADE DELETE for companies
- **Command Updates**: Modify both handler registration and help text constants
- **Bot Commands**: When modifying bot commands, update both handler registration and callback cases
- **Version Tracking**: Update `VERSION` file for releases; referenced in build scripts</content>
<parameter name="filePath">/Users/antonpetrenko/Projects/miltech-tgbot/.github/copilot-instructions.md