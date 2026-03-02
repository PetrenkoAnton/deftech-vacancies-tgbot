# Copilot Instructions for Deftech Vacancies Telegram Bot

## AI Assistant Guidelines
- **DONT AUTO COMMIT** - Always wait for explicit user instruction before committing changes
- **WRITE SHORT COMMIT MESSAGE** - Keep commit messages concise and relevant when asked to commit and push
- **CP SHORTCUT** - "CP" is recognized as shortcut for "Commit and push current changes"
- **IGNORE SENSITIVE DATA IN .env FILE** - Never view, modify, or reference sensitive data like BOT_TOKEN, ADMIN_ID, or other credentials in the .env file

## Architecture Overview
Go-based Telegram bot with SQLite persistence that scrapes job vacancies from deftech.dou.ua and Dwarf Engineering sources (PeopleForce and djinni.co). Built with SOLID principles and clean architecture using dependency injection. The bot uses interface-based design with separated services for maintainability and testability.

## Key Components
- **main.go**: Environment loading, dependency injection setup, bot initialization, graceful shutdown
- **app/bot.go**: Clean architecture with SOLID principles (~1900 lines)
  - **Interfaces**: `MessageFormatter`, `VacancyRepository`, `CompanyRepository`, `VacancyFetcher`, `KeyboardBuilder`, `MessageService`
  - **Services**: `DefaultMessageFormatter`, `SQLiteVacancyRepository`, `SQLiteCompanyRepository`, `DefaultVacancyFetcher`, `DefaultKeyboardBuilder`, `DefaultMessageService`
- **migrations/001_initial.sql**: Companies/vacancies schema with foreign key constraints
- **build_and_run.sh**: Build script that auto-builds to `_bin/` folder and tests briefly before stopping

## Critical Workflows
- **Build & Run**: `go build -o ./_bin/deftech-tgbot . && ./_bin/deftech-tgbot` or `./build_and_run.sh` (auto-builds to `_bin/`, tests 5s, then stops)
- **Database**: Auto-migrates on startup from `migrations/*.sql`; uses configurable table names via repositories
- **Scraping**: HTML parsing with `golang.org/x/net/html`; deftech uses complex link pairing logic (see `DefaultVacancyFetcher.FetchJobTitlesFromDeftech`)
- **Periodic Posting**: Goroutine-based ticker posts only new vacancies when `INTERVAL` env var set, now includes common action buttons

## Project Conventions
- **SOLID Architecture**: Strict adherence to SOLID principles with dependency injection
- **Admin Auth**: Global middleware checks `ADMIN_ID` env var; logs unauthorized attempts with user details
- **Vacancy Storage**: Repository pattern checks existence by title before saving; uses company foreign keys
- **Message Formatting**: Service-based formatting with Markdown links for vacancies; inline keyboards for commands; hide/show buttons separated by |
- **Error Handling**: Logs errors but continues operation; user-facing messages use `telebot.Silent`
- **Configuration**: All settings via environment variables; `.env` loaded with `godotenv`
- **Common Buttons**: New vacancies (both manual and automatic) now include common action buttons for immediate interaction

## Integration Patterns
- **Telegram API**: `telebot.v3` framework with long poller; handlers registered in `registerHandlers()`
- **Database**: Repository pattern with SQLite prepared statements; indexes on `created_at` and `company_id`
- **Web Scraping**: HTTP client with 10s timeout via `VacancyFetcher` interface; parses HTML for job links and company associations
- **External Sources**: Multiple Dwarf Engineering endpoints combined into single response (PeopleForce HTML, djinni.co HTML with views/applies metadata in "title | views / applies" format)
- **Dependency Injection**: All services injected via constructor for testability and flexibility

## Recent Updates
- **SOLID Refactoring**: Complete architectural overhaul with dependency injection, interfaces, and service separation
- **Common Buttons**: Automatic new vacancy postings now include action buttons (Hide all, Get saved, etc.)
- **Button Renaming**: "Companies" button renamed to "Get companies" for consistency
- **Clean Architecture**: Interface-based design enabling easy testing and extension
