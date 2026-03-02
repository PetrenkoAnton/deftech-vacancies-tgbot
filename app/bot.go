package bot

import (
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
	telebot "gopkg.in/telebot.v3"
	_ "modernc.org/sqlite"
)

const (
	commandsText = "Available commands:\n" +
		"/start - Start the bot\n" +
		"/help - Show this help message\n\n" +
		"/get_saved_latest - Get latest saved vacancies\n" +
		"/get_saved_visible - Get visible saved vacancies\n\n" +
		"/fetch_newest - Fetch newest vacancies from deftech.dou.ua\n" +
		"/fetch_latest - Fetch latest vacancies from deftech.dou.ua\n\n" +
		"/fetch_dwarf_engineering - Fetch Dwarf Engineering vacancies\n\n" +
		"/get_companies - Show all companies with vacancy counts\n\n" +
		"/hide_all - Hide all visible vacancies\n\n" +
		"/clear_saved - Clear hidden vacancies\n\n" +
		"/build_version - Show current build version"

	// Company names
	// dwarfEngineeringCompany = "Dwarf Engineering"

	// Action prefixes for deep links
	ignorePrefix   = "ignore"
	unignorePrefix = "unignore"
	companyPrefix  = "company"
)

// Vacancy represents a vacancy listing
type Vacancy struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	CompanyID *int      `json:"company_id"`
	IsHidden  bool      `json:"is_hidden"`
	CreatedAt time.Time `json:"created_at"`
}

// Company represents a company
type Company struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// VacancyInfo represents vacancy information with company
type VacancyInfo struct {
	Title   string
	Company string
	URL     string
}

// DjinniVacancyInfo represents vacancy information from djinni.co with additional metadata
type DjinniVacancyInfo struct {
	Title   string
	URL     string
	Views   string
	Applies string
}

// XHRResponse represents the response from xhr-load endpoint
type XHRResponse struct {
	HTML string `json:"html"`
	Last bool   `json:"last"`
	Num  int    `json:"num"`
}

// Bot represents the Telegram bot instance
type Bot struct {
	telebot      *telebot.Bot
	db           *sql.DB
	httpClient   *http.Client
	adminID      string
	dbName       string
	deftechURL   string
	limit        int
	version      string
	buildVersion string
}

// New creates a new bot instance
func New(token string, adminID string, intervalStr string, dbName string, deftechURL string, limit int, version string, buildVersion string) (*Bot, error) {
	pref := telebot.Settings{
		Token:  token,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
		Client: &http.Client{Timeout: 60 * time.Second},
	}

	b, err := telebot.NewBot(pref)
	if err != nil {
		return nil, err
	}

	// Initialize HTTP client with cookie jar
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %w", err)
	}
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Jar:     jar,
	}

	// Initialize database
	db, err := sql.Open("sqlite", dbName)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Run database migrations
	if err := runMigrations(db); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	bot := &Bot{
		telebot:      b,
		db:           db,
		httpClient:   httpClient,
		adminID:      adminID,
		dbName:       dbName,
		deftechURL:   deftechURL,
		limit:        limit,
		version:      version,
		buildVersion: buildVersion,
	}

	// Apply admin middleware globally
	bot.telebot.Use(bot.adminMiddleware)

	// Register handlers
	bot.registerHandlers()

	// Start periodic posting if interval is set
	if intervalStr != "" {
		interval, err := strconv.Atoi(intervalStr)
		if err != nil {
			return nil, fmt.Errorf("invalid INTERVAL: %w", err)
		}
		go bot.startPeriodicPosting(time.Duration(interval) * time.Minute)
	}

	return bot, nil
}

// isAdmin checks if the user is authorized to use the bot
func (b *Bot) isAdmin(userID int64) bool {
	return fmt.Sprintf("%d", userID) == b.adminID
}

// adminMiddleware checks if the user is authorized to use the bot
func (b *Bot) adminMiddleware(next telebot.HandlerFunc) telebot.HandlerFunc {
	return func(c telebot.Context) error {
		if !b.isAdmin(c.Sender().ID) {
			user := c.Sender()
			fullName := user.FirstName
			if user.LastName != "" {
				fullName += " " + user.LastName
			}
			log.Printf("Unauthorized access attempt - User: %s (@%s) ID: %d", fullName, user.Username, user.ID)
			return c.Send("Sorry, you are not authorized to use this bot.", b.getCommandKeyboard(), telebot.Silent)
		}
		return next(c)
	}
}

// getCommandKeyboard creates an inline keyboard with command buttons
func (b *Bot) getCommandKeyboard() *telebot.ReplyMarkup {
	markup := &telebot.ReplyMarkup{}
	btnGetSavedVisible := markup.Data("Get saved (visible)", "/get_saved_visible")
	btnGetSavedLatest := markup.Data("Get saved (latest)", "/get_saved_latest")
	btnFetchNewest := markup.Data("Fetch newest", "/fetch_newest")
	btnFetchLatest := markup.Data("Fetch latest", "/fetch_latest")
	btnCompanies := markup.Data("Companies", "/get_companies")
	btnDwarf := markup.Data("Fetch Dwarf Engineering", "/fetch_dwarf_engineering")
	markup.Inline(
		markup.Row(btnGetSavedVisible, btnGetSavedLatest),
		markup.Row(btnFetchNewest, btnFetchLatest),
		markup.Row(btnCompanies),
		markup.Row(btnDwarf),
	)
	return markup
}

// getCommandKeyboardWithHideAll creates an inline keyboard with command buttons including "Hide all"
func (b *Bot) getCommandKeyboardWithHideAll() *telebot.ReplyMarkup {
	markup := &telebot.ReplyMarkup{}
	btnHideAll := markup.Data("Hide all", "/hide_all")
	btnGetSavedVisible := markup.Data("Get saved (visible)", "/get_saved_visible")
	btnGetSavedLatest := markup.Data("Get saved (latest)", "/get_saved_latest")
	btnFetchNewest := markup.Data("Fetch newest", "/fetch_newest")
	btnFetchLatest := markup.Data("Fetch latest", "/fetch_latest")
	btnDwarf := markup.Data("Fetch Dwarf Engineering", "/fetch_dwarf_engineering")
	btnCompanies := markup.Data("Companies", "/get_companies")
	markup.Inline(
		markup.Row(btnHideAll),
		markup.Row(btnGetSavedVisible, btnGetSavedLatest),
		markup.Row(btnFetchNewest, btnFetchLatest),
		markup.Row(btnDwarf),
		markup.Row(btnCompanies),
	)
	return markup
}

// runMigrations runs database migrations
func runMigrations(db *sql.DB) error {
	files, err := filepath.Glob("migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		_, err = db.Exec(string(content))
		if err != nil {
			return err
		}
	}
	return nil
}

// saveVacancy saves a vacancy to the database
func (b *Bot) saveVacancy(title, url, companyName string) error {
	if b.vacancyExists(title) {
		return nil // already exists
	}
	companyID, err := b.getOrCreateCompany(companyName)
	if err != nil {
		return err
	}
	query := `INSERT INTO vacancies (title, url, company_id, created_at, is_hidden) VALUES (?, ?, ?, ?, FALSE)`
	_, err = b.db.Exec(query, title, url, companyID, time.Now())
	return err
}

// vacancyExists checks if a vacancy with the given title already exists
func (b *Bot) vacancyExists(title string) bool {
	var count int
	query := "SELECT COUNT(*) FROM vacancies WHERE title = ?"
	err := b.db.QueryRow(query, title).Scan(&count)
	return err == nil && count > 0
}

// getOrCreateCompany gets the company ID by name, creating it if it doesn't exist
func (b *Bot) getOrCreateCompany(name string) (int, error) {
	var id int
	query := "SELECT id FROM companies WHERE name = ?"
	err := b.db.QueryRow(query, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	// Create new company
	query = "INSERT INTO companies (name, created_at) VALUES (?, ?)"
	result, err := b.db.Exec(query, name, time.Now())
	if err != nil {
		return 0, err
	}
	id64, err := result.LastInsertId()
	return int(id64), err
}

// getCompanyNameByID gets the company name by ID
func (b *Bot) getCompanyNameByID(id int) (string, error) {
	var name string
	query := "SELECT name FROM companies WHERE id = ?"
	err := b.db.QueryRow(query, id).Scan(&name)
	return name, err
}

// getCompanyIDByName gets the company ID by name
func (b *Bot) getCompanyIDByName(name string) (int, error) {
	var id int
	query := "SELECT id FROM companies WHERE name = ?"
	err := b.db.QueryRow(query, name).Scan(&id)
	return id, err
}

// getCompanyName safely gets the company name for a vacancy
func (b *Bot) getCompanyName(vacancy Vacancy) string {
	if vacancy.CompanyID != nil {
		if name, err := b.getCompanyNameByID(*vacancy.CompanyID); err == nil {
			return name
		}
	}
	return "Unknown"
}

// getTotalVacancyCount gets the total count of vacancies
func (b *Bot) getTotalVacancyCount() (int, error) {
	query := `SELECT COUNT(*) FROM vacancies`

	var count int
	err := b.db.QueryRow(query).Scan(&count)
	return count, err
}

// getLatestVacancies gets the latest N vacancies from database
func (b *Bot) getLatestVacancies(limit int) ([]Vacancy, error) {
	query := fmt.Sprintf(`SELECT v.id, v.title, v.url, v.company_id, v.is_hidden, v.created_at FROM vacancies v
ORDER BY v.created_at DESC LIMIT %d`, limit)

	rows, err := b.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var vacancies []Vacancy
	for rows.Next() {
		var vacancy Vacancy
		err := rows.Scan(&vacancy.ID, &vacancy.Title, &vacancy.URL, &vacancy.CompanyID, &vacancy.IsHidden, &vacancy.CreatedAt)
		if err != nil {
			log.Printf("Error scanning vacancy: %v", err)
			continue
		}
		vacancies = append(vacancies, vacancy)
	}

	return vacancies, rows.Err()
}

// getVacancyIDAndHiddenByTitle gets the vacancy ID and hidden status by title
func (b *Bot) getVacancyIDAndHiddenByTitle(title string) (int, bool, error) {
	var id int
	var hidden bool
	query := "SELECT id, is_hidden FROM vacancies WHERE title = ?"
	err := b.db.QueryRow(query, title).Scan(&id, &hidden)
	return id, hidden, err
}

// formatVacancyMessage formats a single vacancy for display
func (b *Bot) formatVacancyMessage(index int, vacancy Vacancy, botUsername string) string {
	action := "hide"
	prefix := ignorePrefix
	if vacancy.IsHidden {
		action = "show"
		prefix = unignorePrefix
	}
	company := b.getCompanyName(vacancy)

	// Make company name clickable if we have a company ID
	companyText := company
	if vacancy.CompanyID != nil {
		companyText = fmt.Sprintf("[%s](https://t.me/%s?start=%s_%d)", company, botUsername, companyPrefix, *vacancy.CompanyID)
	}

	return fmt.Sprintf("%d. [%s](%s) @ %s | [%s](https://t.me/%s?start=%s_%d)\n",
		index+1, vacancy.Title, vacancy.URL, companyText, action, botUsername, prefix, vacancy.ID)
}

// formatVacancyInfoMessage formats a vacancy info for display (used for fetched vacancies)
func (b *Bot) formatVacancyInfoMessage(index int, vacancyInfo VacancyInfo, id int, hidden bool, botUsername string) string {
	action := "hide"
	prefix := ignorePrefix
	if hidden {
		action = "show"
		prefix = unignorePrefix
	}
	company := vacancyInfo.Company
	if company == "" {
		company = "-"
	}

	// Try to make company name clickable by looking up company ID
	companyText := company
	if companyID, err := b.getCompanyIDByName(company); err == nil && companyID != 0 {
		companyText = fmt.Sprintf("[%s](https://t.me/%s?start=%s_%d)", company, botUsername, companyPrefix, companyID)
	}

	return fmt.Sprintf("%d. [%s](%s) @ %s | [%s](https://t.me/%s?start=%s_%d)\n",
		index+1, vacancyInfo.Title, vacancyInfo.URL, companyText, action, botUsername, prefix, id)
}

// setVacancyHidden sets the hidden status of a vacancy by ID
func (b *Bot) setVacancyHidden(id int, hidden bool) error {
	query := "UPDATE vacancies SET is_hidden = ? WHERE id = ?"
	_, err := b.db.Exec(query, hidden, id)
	return err
}

// getVacancyTitleByID gets the vacancy title by ID
func (b *Bot) getVacancyTitleByID(id int) (string, error) {
	var title string
	query := "SELECT title FROM vacancies WHERE id = ?"
	err := b.db.QueryRow(query, id).Scan(&title)
	return title, err
}

// sendVacancyList sends a formatted list of vacancies
func (b *Bot) sendVacancyList(c telebot.Context, vacancies []Vacancy, totalCount, limit int) error {
	if len(vacancies) == 0 {
		if totalCount == 0 {
			return c.Send("No saved vacancies found.", b.getCommandKeyboard(), telebot.Silent)
		} else {
			return c.Send(fmt.Sprintf("No vacancies to display (showing latest %d of %d total).", limit, totalCount), b.getCommandKeyboard(), telebot.Silent)
		}
	}

	const batchSize = 25

	// Send the total count in the first message
	firstMessage := fmt.Sprintf("Total vacancies: %d\n\n", totalCount)
	err := c.Send(firstMessage, telebot.ModeMarkdown, telebot.NoPreview, telebot.Silent)
	if err != nil {
		return err
	}

	// Send vacancies in batches of 50
	for i := 0; i < len(vacancies); i += batchSize {
		end := i + batchSize
		if end > len(vacancies) {
			end = len(vacancies)
		}

		var message strings.Builder
		for j, vacancy := range vacancies[i:end] {
			message.WriteString(b.formatVacancyMessage(i+j, vacancy, c.Bot().Me.Username))
		}

		// Add keyboard only to the last message
		if end == len(vacancies) {
			err = c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview, b.getCommandKeyboard(), telebot.Silent)
		} else {
			err = c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview, telebot.Silent)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

// getVisibleVacancies retrieves all non-hidden vacancies from the database
func (b *Bot) getVisibleVacancies() ([]Vacancy, error) {
	query := `SELECT v.id, v.title, v.url, v.company_id, v.is_hidden, v.created_at FROM vacancies v
WHERE v.is_hidden = FALSE
ORDER BY v.created_at ASC`

	rows, err := b.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var vacancies []Vacancy
	for rows.Next() {
		var vacancy Vacancy
		err := rows.Scan(&vacancy.ID, &vacancy.Title, &vacancy.URL, &vacancy.CompanyID, &vacancy.IsHidden, &vacancy.CreatedAt)
		if err != nil {
			return nil, err
		}
		vacancies = append(vacancies, vacancy)
	}

	return vacancies, rows.Err()
}

// getAllVacancies retrieves all vacancies from the database
func (b *Bot) getAllVacancies() ([]Vacancy, error) {
	query := `SELECT id, title, url, company_id, is_hidden, created_at FROM vacancies ORDER BY created_at ASC`

	rows, err := b.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var vacancies []Vacancy
	for rows.Next() {
		var vacancy Vacancy
		err := rows.Scan(&vacancy.ID, &vacancy.Title, &vacancy.URL, &vacancy.CompanyID, &vacancy.IsHidden, &vacancy.CreatedAt)
		if err != nil {
			return nil, err
		}
		vacancies = append(vacancies, vacancy)
	}

	return vacancies, rows.Err()
}

// getVacanciesByCompanyID retrieves all vacancies for a specific company
func (b *Bot) getVacanciesByCompanyID(companyID int) ([]Vacancy, error) {
	query := `SELECT id, title, url, company_id, is_hidden, created_at FROM vacancies WHERE company_id = ? ORDER BY created_at DESC`

	rows, err := b.db.Query(query, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var vacancies []Vacancy
	for rows.Next() {
		var vacancy Vacancy
		err := rows.Scan(&vacancy.ID, &vacancy.Title, &vacancy.URL, &vacancy.CompanyID, &vacancy.IsHidden, &vacancy.CreatedAt)
		if err != nil {
			return nil, err
		}
		vacancies = append(vacancies, vacancy)
	}

	return vacancies, rows.Err()
}

// getCompaniesWithVacancyCounts retrieves all companies with their vacancy counts
func (b *Bot) getCompaniesWithVacancyCounts() ([]struct {
	ID    int
	Name  string
	Count int
}, error) {
	query := `SELECT c.id, c.name, COUNT(v.id) as vacancy_count
FROM companies c
LEFT JOIN vacancies v ON c.id = v.company_id
GROUP BY c.id, c.name
ORDER BY c.name ASC`

	rows, err := b.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var companies []struct {
		ID    int
		Name  string
		Count int
	}
	for rows.Next() {
		var company struct {
			ID    int
			Name  string
			Count int
		}
		err := rows.Scan(&company.ID, &company.Name, &company.Count)
		if err != nil {
			return nil, err
		}
		companies = append(companies, company)
	}

	return companies, rows.Err()
}

// registerHandlers registers all bot command and message handlers
func (b *Bot) registerHandlers() {
	// Start command handler
	b.telebot.Handle("/start", b.handleStart)

	// Help command handler
	b.telebot.Handle("/help", b.handleHelp)

	// Get Dwarf Engineering vacancies command handler
	b.telebot.Handle("/fetch_dwarf_engineering", b.handleGetDwarfEngineering)

	// Get list deftech command handler
	b.telebot.Handle("/fetch_newest", b.handleFetchNewest)

	// Fetch latest deftech command handler
	b.telebot.Handle("/fetch_latest", b.handleFetchLatest)

	// Get visible deftech vacancies command handler
	b.telebot.Handle("/get_saved_visible", b.handleGetSavedVisible)

	// Get saved vacancies command handler
	b.telebot.Handle("/get_saved_latest", b.handleGetSavedLatest)

	// Hide all command handler
	b.telebot.Handle("/hide_all", b.handleHideAll)

	// Build version command handler
	b.telebot.Handle("/build_version", b.handleBuildVersion)

	// Clear saved command handler
	b.telebot.Handle("/clear_saved", b.handleClearSaved)

	// Inline button callback handler
	b.telebot.Handle(telebot.OnCallback, b.handleCallback)

	// Default message handler
	b.telebot.Handle(telebot.OnText, b.handleText)
}

// startPeriodicPosting starts a goroutine that posts messages every interval
func (b *Bot) startPeriodicPosting(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("Starting periodic new deftech vacancies posting every %v", interval)

	for {
		select {
		case <-ticker.C:
			if err := b.postDeftechVacancies(); err != nil {
				log.Printf("Error in periodic new deftech vacancies posting: %v", err)
			}
		}
	}
}

// postDeftechVacancies fetches deftech vacancies and posts them to the admin only if there are new vacancies
func (b *Bot) postDeftechVacancies() error {
	if b.adminID == "" {
		return fmt.Errorf("ADMIN_ID not set")
	}

	adminIDInt, err := strconv.ParseInt(b.adminID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid ADMIN_ID: %w", err)
	}

	chat := &telebot.Chat{ID: adminIDInt}

	log.Println("Fetching deftech vacancies for periodic posting →")

	// Fetch vacancy titles from the deftech.dou.ua/jobs page
	vacancyInfos, err := b.FetchJobTitlesFromDeftech()
	if err != nil {
		return fmt.Errorf("error fetching vacancy titles from deftech: %w", err)
	}

	// Check for new jobs and save them
	var newVacancyInfos []VacancyInfo
	for _, vacancyInfo := range vacancyInfos {
		if !b.vacancyExists(vacancyInfo.Title) {
			// This is a new vacancy
			err := b.saveVacancy(vacancyInfo.Title, vacancyInfo.URL, vacancyInfo.Company)
			if err != nil {
				log.Printf("Error saving new vacancy to DB: %v", err)
			} else {
				newVacancyInfos = append(newVacancyInfos, vacancyInfo)
			}
		}
	}

	// If no new vacancies, just log and return
	if len(newVacancyInfos) == 0 {
		log.Println("No new vacancies found")
		return nil
	}

	log.Printf("Found %d new vacancies", len(newVacancyInfos))

	// Format the message with only new vacancies
	var message strings.Builder
	message.WriteString("**New vacancies:**\n\n")

	for i, vacancyInfo := range newVacancyInfos {
		id, hidden, err := b.getVacancyIDAndHiddenByTitle(vacancyInfo.Title)
		if err != nil {
			log.Printf("Error getting vacancy ID for %s: %v", vacancyInfo.Title, err)
			continue
		}
		action := "hide"
		prefix := "ignore"
		if hidden {
			action = "show"
			prefix = "unignore"
		}
		company := vacancyInfo.Company
		if company == "" {
			company = "-"
		}
		message.WriteString(fmt.Sprintf("%d. [%s](%s) @ %s | [%s](https://t.me/%s?start=%s_%d)\n", i+1, vacancyInfo.Title, vacancyInfo.URL, company, action, b.telebot.Me.Username, prefix, id))
	}

	// Send with retry logic
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		_, err = b.telebot.Send(chat, message.String(), telebot.ModeMarkdown, telebot.NoPreview)
		if err == nil {
			break
		}
		log.Printf("Attempt %d failed to send new vacancies: %v", attempt, err)
		if attempt < maxRetries {
			time.Sleep(time.Duration(attempt) * 5 * time.Second) // Exponential backoff: 5s, 10s, 15s
		}
	}
	if err != nil {
		return fmt.Errorf("error sending new vacancies after %d attempts: %w", maxRetries, err)
	}

	log.Printf("Posted %d new vacancies", len(newVacancyInfos))
	return nil
}

// handleStart handles the /start command
func (b *Bot) handleStart(c telebot.Context) error {
	log.Printf("Command /start received")

	// Check if this is a deep link command
	payload := strings.TrimSpace(c.Message().Payload)

	// Check if this is a company selection via deep link
	if strings.HasPrefix(payload, companyPrefix+"_") {
		companyIDStr := strings.TrimPrefix(payload, companyPrefix+"_")
		companyID, err := strconv.Atoi(companyIDStr)
		if err != nil {
			return c.Send("Invalid company ID", b.getCommandKeyboard(), telebot.Silent)
		}
		// Delete the /start command message to keep chat clean
		go func() {
			time.Sleep(50 * time.Millisecond) // Small delay to ensure processing completes
			c.Delete()
		}()
		return b.handleCompanyVacancies(c, companyID)
	}

	// Check if this is a companies list request via deep link
	if payload == "companies" {
		// Delete the /start command message to keep chat clean
		go func() {
			time.Sleep(50 * time.Millisecond) // Small delay to ensure processing completes
			c.Delete()
		}()
		return b.handleCompanies(c)
	}

	// Check if this is an ignore/unignore command via deep link
	if strings.HasPrefix(payload, "ignore_") {
		idStr := strings.TrimPrefix(payload, "ignore_")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			return c.Send("Invalid ignore ID", b.getCommandKeyboard(), telebot.Silent)
		}
		title, err := b.getVacancyTitleByID(id)
		if err != nil {
			log.Printf("Error getting title for vacancy %d: %v", id, err)
			return c.Send("Error ignoring job", b.getCommandKeyboard(), telebot.Silent)
		}
		// Get vacancy URL and company for clickable link
		var url, company string
		err = b.db.QueryRow("SELECT v.url, COALESCE(c.name, 'Unknown') FROM vacancies v LEFT JOIN companies c ON v.company_id = c.id WHERE v.id = ?", id).Scan(&url, &company)
		if err != nil {
			log.Printf("Error getting URL and company for vacancy %d: %v", id, err)
			url = "#"
			company = "Unknown"
		}
		err = b.setVacancyHidden(id, true)
		if err != nil {
			log.Printf("Error hiding vacancy %d: %v", id, err)
			return c.Send("Error hiding vacancy", b.getCommandKeyboard(), telebot.Silent)
		}
		// Delete the /start command message to keep chat clean
		go func() {
			time.Sleep(50 * time.Millisecond) // Small delay to ensure processing completes
			c.Delete()
		}()
		// Send status message
		err = c.Send(fmt.Sprintf("[%s](%s) @ %s is hidden", title, url, company), telebot.ModeMarkdown, telebot.Silent, telebot.NoPreview)
		if err != nil {
			log.Printf("Error sending hide status message: %v", err)
			return nil
		}

		// Send updated visible vacancies list (like /get_saved_visible)
		visibleVacancies, err := b.getVisibleVacancies()
		if err != nil {
			log.Printf("Error getting visible vacancies after hide: %v", err)
			return nil // Don't return error as the hide operation succeeded
		}

		if len(visibleVacancies) == 0 {
			return c.Send("No visible vacancies found.", b.getCommandKeyboardWithHideAll(), telebot.Silent)
		}

		// Send vacancies in chunks to avoid message length limit
		const maxVacanciesPerMessage = 50
		for i := 0; i < len(visibleVacancies); i += maxVacanciesPerMessage {
			end := i + maxVacanciesPerMessage
			if end > len(visibleVacancies) {
				end = len(visibleVacancies)
			}
			chunk := visibleVacancies[i:end]

			var message strings.Builder
			for j, vacancy := range chunk {
				message.WriteString(b.formatVacancyMessage(i+j, vacancy, c.Bot().Me.Username))
			}

			// Add keyboard only to the last message
			if end == len(visibleVacancies) {
				c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview, b.getCommandKeyboardWithHideAll(), telebot.Silent)
			} else {
				c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview, telebot.Silent)
			}
		}
		return nil
	}
	if strings.HasPrefix(payload, "unignore_") {
		idStr := strings.TrimPrefix(payload, "unignore_")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			return c.Send("Invalid unignore ID", b.getCommandKeyboard(), telebot.Silent)
		}
		title, err := b.getVacancyTitleByID(id)
		if err != nil {
			log.Printf("Error getting title for vacancy %d: %v", id, err)
			return c.Send("Error showing job", b.getCommandKeyboard(), telebot.Silent)
		}
		// Get vacancy URL and company for clickable link
		var url, company string
		err = b.db.QueryRow("SELECT v.url, COALESCE(c.name, 'Unknown') FROM vacancies v LEFT JOIN companies c ON v.company_id = c.id WHERE v.id = ?", id).Scan(&url, &company)
		if err != nil {
			log.Printf("Error getting URL and company for vacancy %d: %v", id, err)
			url = "#"
			company = "Unknown"
		}
		err = b.setVacancyHidden(id, false)
		if err != nil {
			log.Printf("Error showing vacancy %d: %v", id, err)
			return c.Send("Error showing job", b.getCommandKeyboard(), telebot.Silent)
		}
		// Delete the /start command message to keep chat clean
		go func() {
			time.Sleep(50 * time.Millisecond) // Small delay to ensure processing completes
			c.Delete()
		}()
		// Send status message
		err = c.Send(fmt.Sprintf("[%s](%s) @ %s is shown", title, url, company), telebot.ModeMarkdown, telebot.Silent, telebot.NoPreview)
		if err != nil {
			log.Printf("Error sending show status message: %v", err)
			return nil
		}

		// Send updated visible vacancies list (like /get_saved_visible)
		visibleVacancies, err := b.getVisibleVacancies()
		if err != nil {
			log.Printf("Error getting visible vacancies after show: %v", err)
			return nil // Don't return error as the show operation succeeded
		}

		if len(visibleVacancies) == 0 {
			return c.Send("No visible vacancies found.", b.getCommandKeyboardWithHideAll(), telebot.Silent)
		}

		// Send vacancies in chunks to avoid message length limit
		const maxVacanciesPerMessage = 25
		for i := 0; i < len(visibleVacancies); i += maxVacanciesPerMessage {
			end := i + maxVacanciesPerMessage
			if end > len(visibleVacancies) {
				end = len(visibleVacancies)
			}
			chunk := visibleVacancies[i:end]

			var message strings.Builder
			for j, vacancy := range chunk {
				message.WriteString(b.formatVacancyMessage(i+j, vacancy, c.Bot().Me.Username))
			}

			// Add keyboard only to the last message
			if end == len(visibleVacancies) {
				c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview, b.getCommandKeyboardWithHideAll(), telebot.Silent)
			} else {
				c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview, telebot.Silent)
			}
		}
		return nil
	}

	startText := "Hello! Welcome to the bot.\n\n" + commandsText
	// Add version if available
	if b.version != "" {
		startText = fmt.Sprintf("Hello! Welcome to the bot (v%s).\n\n", b.version) + commandsText
	}
	return c.Send(startText, b.getCommandKeyboard(), telebot.Silent)
}

// handleHelp handles the /help command
func (b *Bot) handleHelp(c telebot.Context) error {
	log.Printf("Command /help received")
	return c.Send(commandsText, b.getCommandKeyboard(), telebot.Silent)
}

// fetchJobTitles fetches and parses vacancy titles from all careers pages
func (b *Bot) fetchJobTitles() ([]VacancyInfo, error) {
	var allVacancies []VacancyInfo
	seen := make(map[string]bool) // To avoid duplicates

	// Fetch pages 1 and 2
	pages := []int{1, 2}
	for _, page := range pages {
		vacancies, err := b.fetchJobTitlesFromPage(page)
		if err != nil {
			log.Printf("Error fetching page %d: %v", page, err)
			// Continue with other pages even if one fails
			continue
		}

		// Add unique vacancy titles
		for _, job := range vacancies {
			if !seen[job.Title] {
				seen[job.Title] = true
				allVacancies = append(allVacancies, job)
			}
		}
	}

	return allVacancies, nil
}

// fetchJobTitlesFromPage fetches and parses vacancy titles from a specific page
func (b *Bot) fetchJobTitlesFromPage(page int) ([]VacancyInfo, error) {
	url := fmt.Sprintf("https://dwarfengineering.peopleforce.io/careers?page=%d", page)

	resp, err := b.getWithUserAgent(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	vacancies := b.findJobTitles(doc)
	return vacancies, nil
}

// getWithUserAgent performs a GET request with a User-Agent header
func (b *Bot) getWithUserAgent(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Bot/1.0)")
	return b.httpClient.Do(req)
}

// fetchXHRPage fetches additional vacancies via AJAX
func (b *Bot) fetchXHRPage(xhrURL, csrfToken string, count int) (*XHRResponse, error) {
	data := url.Values{}
	data.Set("csrfmiddlewaretoken", csrfToken)
	data.Set("count", strconv.Itoa(count))

	req, err := http.NewRequest("POST", xhrURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Bot/1.0)")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", b.deftechURL)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var xhrResp XHRResponse
	if err := json.NewDecoder(resp.Body).Decode(&xhrResp); err != nil {
		return nil, fmt.Errorf("failed to decode JSON: %w", err)
	}

	return &xhrResp, nil
}

// collectText recursively collects text from a node
func collectText(n *html.Node, text *strings.Builder) {
	if n.Type == html.TextNode {
		text.WriteString(n.Data)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectText(c, text)
	}
}

// findJobTitles finds vacancy titles and URLs in the HTML document
func (b *Bot) findJobTitles(n *html.Node) []VacancyInfo {
	var vacancies []VacancyInfo
	if n.Type == html.ElementNode && n.Data == "h4" {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "a" {
				title := strings.TrimSpace(extractText(c))
				var url string
				for _, attr := range c.Attr {
					if attr.Key == "href" {
						url = attr.Val
						// Make URL absolute if it's relative
						if strings.HasPrefix(url, "/") {
							url = "https://dwarfengineering.peopleforce.io" + url
						} else if !strings.HasPrefix(url, "http") {
							url = "https://dwarfengineering.peopleforce.io/" + url
						}
						break
					}
				}
				if title != "" && url != "" {
					vacancies = append(vacancies, VacancyInfo{Title: title, URL: url})
				}
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		vacancies = append(vacancies, b.findJobTitles(c)...)
	}
	return vacancies
}

// handleFetchNewest handles the /fetch_newest command
func (b *Bot) handleFetchNewest(c telebot.Context) error {
	log.Printf("Command /fetch_newest received")
	// Show loading message
	c.Send(fmt.Sprintf("Fetching new vacancies from [%s](%s) →", b.deftechURL, b.deftechURL), telebot.ModeMarkdown, telebot.Silent)

	// Fetch vacancy titles from the deftech.dou.ua page
	vacancyInfos, err := b.FetchJobTitlesFromDeftech()
	if err != nil {
		log.Printf("Error fetching vacancy titles from deftech.dou.ua: %v", err)
		return c.Send(fmt.Sprintf("Error fetching vacancies: %v", err), b.getCommandKeyboard(), telebot.Silent)
	}

	// Check for new jobs and save them
	var newVacancyInfos []VacancyInfo
	for _, vacancyInfo := range vacancyInfos {
		if !b.vacancyExists(vacancyInfo.Title) {
			// This is a new vacancy
			err := b.saveVacancy(vacancyInfo.Title, vacancyInfo.URL, vacancyInfo.Company)
			if err != nil {
				log.Printf("Error saving new vacancy to DB: %v", err)
			} else {
				newVacancyInfos = append(newVacancyInfos, vacancyInfo)
			}
		}
	}

	// If no new vacancies, send message and return
	if len(newVacancyInfos) == 0 {
		return c.Send("No new vacancies found.", b.getCommandKeyboard(), telebot.Silent)
	}

	log.Printf("Found %d new vacancies", len(newVacancyInfos))

	// Format the message with only new vacancies
	var message strings.Builder
	message.WriteString("**New vacancies:**\n\n")

	for i, vacancyInfo := range newVacancyInfos {
		id, hidden, err := b.getVacancyIDAndHiddenByTitle(vacancyInfo.Title)
		if err != nil {
			log.Printf("Error getting vacancy ID for %s: %v", vacancyInfo.Title, err)
			continue
		}
		message.WriteString(b.formatVacancyInfoMessage(i, vacancyInfo, id, hidden, c.Bot().Me.Username))
	}

	return c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview, b.getCommandKeyboardWithHideAll(), telebot.Silent)
}

// handleFetchLatest handles the /fetch_latest command
func (b *Bot) handleFetchLatest(c telebot.Context) error {
	log.Printf("Command /fetch_latest received")
	// Show loading message
	c.Send(fmt.Sprintf("Fetching latest vacancies from [%s](%s) →", b.deftechURL, b.deftechURL), telebot.ModeMarkdown, telebot.Silent)

	// Fetch vacancy titles from the deftech.dou.ua page
	vacancyInfos, err := b.FetchJobTitlesFromDeftech()
	if err != nil {
		log.Printf("Error fetching vacancy titles from deftech.dou.ua: %v", err)
		return c.Send(fmt.Sprintf("Error fetching vacancies: %v", err), b.getCommandKeyboard(), telebot.Silent)
	}

	if len(vacancyInfos) == 0 {
		return c.Send("No vacancies found.", b.getCommandKeyboard(), telebot.Silent)
	}

	// Format and send the list
	var message strings.Builder

	for i, vacancyInfo := range vacancyInfos {
		company := vacancyInfo.Company
		if company == "" {
			company = "-"
		}
		message.WriteString(fmt.Sprintf("%d. [%s](%s) @ %s\n", i+1, vacancyInfo.Title, vacancyInfo.URL, company))
	}

	return c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview, b.getCommandKeyboard(), telebot.Silent)
}

// handleGetSavedVisible handles the /get_saved_visible command
func (b *Bot) handleGetSavedVisible(c telebot.Context) error {
	log.Printf("Command /get_saved_visible received")
	// Show loading message
	c.Send(fmt.Sprintf("Getting visible vacancies from db →"), telebot.ModeMarkdown, telebot.Silent)

	// Get visible vacancies from database
	vacancies, err := b.getVisibleVacancies()
	if err != nil {
		log.Printf("Error getting visible vacancies: %v", err)
		return c.Send("Error getting visible vacancies", b.getCommandKeyboard(), telebot.Silent)
	}

	if len(vacancies) == 0 {
		return c.Send("No visible vacancies found.", b.getCommandKeyboard(), telebot.Silent)
	}

	// Send vacancies in chunks to avoid message length limit
	const maxVacanciesPerMessage = 25
	for i := 0; i < len(vacancies); i += maxVacanciesPerMessage {
		end := i + maxVacanciesPerMessage
		if end > len(vacancies) {
			end = len(vacancies)
		}
		chunk := vacancies[i:end]

		var message strings.Builder
		for j, vacancy := range chunk {
			message.WriteString(b.formatVacancyMessage(i+j, vacancy, c.Bot().Me.Username))
		}

		// Add keyboard only to the last message
		if end == len(vacancies) {
			c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview, b.getCommandKeyboardWithHideAll(), telebot.Silent)
		} else {
			c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview, telebot.Silent)
		}
	}

	return nil
}

// handleGetSavedLatest handles the /get_saved_latest command
func (b *Bot) handleGetSavedLatest(c telebot.Context) error {
	log.Printf("Command /get_saved_latest received")
	// Show loading message
	c.Send(fmt.Sprintf("Getting %d latest saved vacancies from db →", b.limit), telebot.ModeMarkdown, telebot.Silent)

	totalCount, err := b.getTotalVacancyCount()
	if err != nil {
		log.Printf("Error getting total count: %v", err)
		return c.Send("Error getting saved vacancies", b.getCommandKeyboard(), telebot.Silent)
	}

	vacancies, err := b.getLatestVacancies(b.limit)
	if err != nil {
		log.Printf("Error getting latest vacancies: %v", err)
		return c.Send("Error getting saved vacancies", b.getCommandKeyboard(), telebot.Silent)
	}

	return b.sendVacancyList(c, vacancies, totalCount, b.limit)
}

// handleGetDwarfEngineering handles the /fetch_dwarf_engineering command
func (b *Bot) handleGetDwarfEngineering(c telebot.Context) error {
	log.Printf("Command /fetch_dwarf_engineering received")
	// Show loading message
	c.Send("Fetching Dwarf Engineering vacancies →", telebot.Silent)

	var peopleforceVacancies []VacancyInfo
	var djinniVacancies []DjinniVacancyInfo

	// Fetch from PeopleForce
	peopleforceVacanciesRaw, err := b.fetchJobTitles()
	if err != nil {
		log.Printf("Error fetching vacancies from https://dwarfengineering.peopleforce.io/careers/: %v", err)
		// Continue even if one source fails
	} else {
		peopleforceVacancies = peopleforceVacanciesRaw
		log.Printf("DEBUG: Fetched %d vacancies from PeopleForce", len(peopleforceVacancies))
	}

	// Fetch from djinni.co
	_, djinniVacanciesRaw, err := b.fetchJobTitlesFromDjinni()
	if err != nil {
		log.Printf("Error fetching vacancies from djinni.co: %v", err)
		// Continue even if one source fails
	} else {
		djinniVacancies = djinniVacanciesRaw
		log.Printf("DEBUG: Fetched %d vacancies from djinni.co", len(djinniVacancies))
	}

	if len(peopleforceVacancies) == 0 && len(djinniVacancies) == 0 {
		return c.Send("No vacancies found from any source.", b.getCommandKeyboard(), telebot.Silent)
	}

	// Format and send the list
	var message strings.Builder

	if len(peopleforceVacancies) > 0 {
		message.WriteString("**[dwarfengineering.peopleforce.io/careers](https://dwarfengineering.peopleforce.io/careers):**\n")
		for i, vacancy := range peopleforceVacancies {
			message.WriteString(fmt.Sprintf("%d. [%s](%s)\n", i+1, vacancy.Title, vacancy.URL))
		}
		message.WriteString("\n")
	}

	if len(djinniVacancies) > 0 {
		message.WriteString("**[djinni.co/jobs/company-dwarf-engineering](https://djinni.co/jobs/company-dwarf-engineering/):**\n")
		for i, vacancy := range djinniVacancies {
			// Format with views and applies info after the link
			statsInfo := ""
			if vacancy.Views != "" || vacancy.Applies != "" {
				views := vacancy.Views
				if views == "" {
					views = "0"
				}
				applies := vacancy.Applies
				if applies == "" {
					applies = "0"
				}
				statsInfo = fmt.Sprintf(" | %s / %s", views, applies)
			}
			message.WriteString(fmt.Sprintf("%d. [%s](%s)%s\n", i+1, vacancy.Title, vacancy.URL, statsInfo))
		}
	}

	return c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview, b.getCommandKeyboard(), telebot.Silent)
}

// handleClearSaved handles the /clear_saved command
func (b *Bot) handleClearSaved(c telebot.Context) error {
	log.Printf("Command /clear_saved received")
	query := "DELETE FROM vacancies WHERE is_hidden = 1"
	_, err := b.db.Exec(query)
	if err != nil {
		log.Printf("Error clearing hidden vacancies: %v", err)
		return c.Send("Error clearing hidden vacancies", b.getCommandKeyboard(), telebot.Silent)
	}
	return c.Send("Hidden vacancies cleared successfully", b.getCommandKeyboard(), telebot.Silent)
}

// handleHideAll handles the /hide_all command
func (b *Bot) handleHideAll(c telebot.Context) error {
	log.Printf("Command /hide_all received")

	// Hide all visible vacancies
	query := "UPDATE vacancies SET is_hidden = 1 WHERE is_hidden = 0"
	result, err := b.db.Exec(query)
	if err != nil {
		log.Printf("Error hiding all vacancies: %v", err)
		return c.Send("Error hiding vacancies", b.getCommandKeyboard(), telebot.Silent)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("Error getting rows affected: %v", err)
		return c.Send("Error hiding vacancies", b.getCommandKeyboard(), telebot.Silent)
	}

	if rowsAffected == 0 {
		return c.Send("No visible vacancies to hide", b.getCommandKeyboard(), telebot.Silent)
	}

	return c.Send(fmt.Sprintf("Hidden %d vacancies successfully", rowsAffected), b.getCommandKeyboard(), telebot.Silent)
}

// handleBuildVersion handles the /build_version command
func (b *Bot) handleBuildVersion(c telebot.Context) error {
	log.Printf("Command /build_version received")

	// Use embedded build version
	version := b.buildVersion
	if version == "" {
		version = "unknown"
	}
	return c.Send(fmt.Sprintf("Current build version: %s", version), b.getCommandKeyboard(), telebot.Silent)
}

// handleCompanies handles the /get_companies command
func (b *Bot) handleCompanies(c telebot.Context) error {
	log.Printf("Command /get_companies received")

	companies, err := b.getCompaniesWithVacancyCounts()
	if err != nil {
		log.Printf("Error getting companies: %v", err)
		return c.Send("Error getting companies", b.getCommandKeyboard(), telebot.Silent)
	}

	if len(companies) == 0 {
		return c.Send("No companies found.", b.getCommandKeyboard(), telebot.Silent)
	}

	var message strings.Builder
	message.WriteString("**Companies with vacancy counts:**\n\n")
	for i, company := range companies {
		message.WriteString(fmt.Sprintf("%d. [%s](https://t.me/%s?start=%s_%d) (%d)\n",
			i+1, company.Name, c.Bot().Me.Username, companyPrefix, company.ID, company.Count))
	}

	return c.Send(message.String(), telebot.ModeMarkdown, b.getCommandKeyboard(), telebot.Silent)
}

// handleCompanyVacancies handles showing vacancies for a specific company
func (b *Bot) handleCompanyVacancies(c telebot.Context, companyID int) error {
	log.Printf("Command /company_%d received", companyID)

	// Get company name
	companyName, err := b.getCompanyNameByID(companyID)
	if err != nil {
		log.Printf("Error getting company name for ID %d: %v", companyID, err)
		return c.Send("Error getting company information", b.getCommandKeyboard(), telebot.Silent)
	}

	// Show loading message
	c.Send(fmt.Sprintf("Getting %s vacancies →", companyName), telebot.Silent)

	// Get all vacancies for this company
	vacancies, err := b.getVacanciesByCompanyID(companyID)
	if err != nil {
		log.Printf("Error getting vacancies for company %d: %v", companyID, err)
		return c.Send("Error getting company vacancies", b.getCommandKeyboard(), telebot.Silent)
	}

	if len(vacancies) == 0 {
		return c.Send(fmt.Sprintf("**%s**\n\nNo vacancies found for this company.", companyName), telebot.ModeMarkdown, b.getCommandKeyboard(), telebot.Silent)
	}

	// Send vacancies in chunks to avoid message length limit
	const maxVacanciesPerMessage = 25
	for i := 0; i < len(vacancies); i += maxVacanciesPerMessage {
		end := i + maxVacanciesPerMessage
		if end > len(vacancies) {
			end = len(vacancies)
		}
		chunk := vacancies[i:end]

		var message strings.Builder
		for j, vacancy := range chunk {
			message.WriteString(b.formatVacancyMessage(i+j, vacancy, c.Bot().Me.Username))
		}

		// Add back link only to the last message
		if end == len(vacancies) {
			c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview, b.getCommandKeyboard(), telebot.Silent)
		} else {
			c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview, telebot.Silent)
		}
	}

	return nil
}

// handleBackToMenu handles the back to menu callback
func (b *Bot) handleBackToMenu(c telebot.Context) error {
	log.Printf("Command /back_to_menu received")
	return c.Send("Main menu:", b.getCommandKeyboard(), telebot.Silent)
}

// RSSFeed represents the RSS feed structure
type RSSFeed struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Items []RSSItem `xml:"item"`
	} `xml:"channel"`
}

// RSSItem represents an item in the RSS feed
type RSSItem struct {
	Title string `xml:"title"`
	Link  string `xml:"link"`
}

// fetchJobTitlesFromDjinni fetches and parses job titles from djinni.co Dwarf Engineering page
func (b *Bot) fetchJobTitlesFromDjinni() ([]string, []DjinniVacancyInfo, error) {
	url := "https://djinni.co/jobs/company-dwarf-engineering/"

	resp, err := b.getWithUserAgent(url)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch djinni page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	titles, vacancies := b.findJobTitlesFromDjinni(doc)
	return titles, vacancies, nil
}

// findJobTitlesFromDjinni finds job titles in the djinni.co HTML document
func (b *Bot) findJobTitlesFromDjinni(n *html.Node) ([]string, []DjinniVacancyInfo) {
	var vacancyTitles []string
	var vacancies []DjinniVacancyInfo
	var seen = make(map[string]bool)

	var findTitles func(*html.Node)
	findTitles = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "div" {
			// Check if this is a job item
			for _, attr := range node.Attr {
				if attr.Key == "id" && strings.HasPrefix(attr.Val, "job-item-") {
					// Found a job item, extract information
					vacancy := b.extractDjinniVacancyInfo(node)
					if vacancy.Title != "" && vacancy.URL != "" && !seen[vacancy.Title] {
						seen[vacancy.Title] = true
						vacancyTitles = append(vacancyTitles, vacancy.Title)
						vacancies = append(vacancies, vacancy)
					}
					break
				}
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			findTitles(c)
		}
	}

	findTitles(n)
	return vacancyTitles, vacancies
}

// extractDjinniVacancyInfo extracts vacancy information from a djinni.co job item
func (b *Bot) extractDjinniVacancyInfo(node *html.Node) DjinniVacancyInfo {
	vacancy := DjinniVacancyInfo{}

	var extractInfo func(*html.Node)
	extractInfo = func(n *html.Node) {
		if n.Type == html.ElementNode {
			// Extract title and URL from a element
			if n.Data == "a" {
				for _, attr := range n.Attr {
					if attr.Key == "class" && strings.Contains(attr.Val, "job_item__header-link") {
						// Get URL
						for _, a := range n.Attr {
							if a.Key == "href" {
								vacancy.URL = a.Val
								if !strings.HasPrefix(vacancy.URL, "http") {
									vacancy.URL = "https://djinni.co" + vacancy.URL
								}
								break
							}
						}
						// Find h2 inside for title
						var findH2 func(*html.Node)
						findH2 = func(nn *html.Node) {
							if nn.Type == html.ElementNode && nn.Data == "h2" {
								vacancy.Title = strings.TrimSpace(extractText(nn))
							}
							for cc := nn.FirstChild; cc != nil; cc = cc.NextSibling {
								findH2(cc)
							}
						}
						findH2(n)
						break
					}
				}
			}

			// Extract views and applies from the metadata section
			if n.Data == "div" {
				for _, attr := range n.Attr {
					if attr.Key == "class" && strings.Contains(attr.Val, "text-secondary") {
						text := strings.TrimSpace(extractText(n))
						// Split by · to get individual metadata parts
						parts := strings.Split(text, "·")
						for _, part := range parts {
							part = strings.TrimSpace(part)
							if strings.Contains(part, "переглядів") || strings.Contains(part, "перегляд") {
								// Extract number before "переглядів" or "перегляд"
								if idx := strings.Index(part, "перегляд"); idx > 0 {
									vacancy.Views = strings.TrimSpace(part[:idx])
								}
							}
							if strings.Contains(part, "відгуків") || strings.Contains(part, "відгук") {
								// Extract number before "відгуків" or "відгук"
								if idx := strings.Index(part, "відгук"); idx > 0 {
									vacancy.Applies = strings.TrimSpace(part[:idx])
								}
							}
						}
					}
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extractInfo(c)
		}
	}

	extractInfo(node)
	return vacancy
}

// FetchJobTitlesFromDeftech fetches and parses job titles from deftech.dou.ua page with pagination
func (b *Bot) FetchJobTitlesFromDeftech() ([]VacancyInfo, error) {
	pageURL := b.deftechURL

	resp, err := b.getWithUserAgent(pageURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	// Extract CSRF token from cookies
	u, _ := url.Parse(pageURL)
	cookies := b.httpClient.Jar.Cookies(u)
	var csrfToken string
	for _, cookie := range cookies {
		if cookie.Name == "csrftoken" {
			csrfToken = cookie.Value
			break
		}
	}
	if csrfToken == "" {
		return nil, fmt.Errorf("failed to extract CSRF token from cookies")
	}

	vacancyInfos := findJobTitlesDeftech(doc)
	count := len(vacancyInfos)
	log.Printf("Fetched %d initial vacancies from deftech", count)

	// Load more pages via AJAX, up to 1 additional page (total pages 1-2)
	xhrURL := strings.Replace(pageURL, "/jobs/", "/jobs/xhr-load/", 1)
	maxAdditionalPages := 1
	pageCount := 0
	for pageCount < maxAdditionalPages {
		xhrResp, err := b.fetchXHRPage(xhrURL, csrfToken, count)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch xhr page: %w", err)
		}

		if xhrResp.Last {
			break
		}

		// Parse the additional HTML
		additionalDoc, err := html.Parse(strings.NewReader(xhrResp.HTML))
		if err != nil {
			return nil, fmt.Errorf("failed to parse additional HTML: %w", err)
		}

		additionalVacancies := findJobTitlesDeftech(additionalDoc)
		log.Printf("Fetched %d additional vacancies from page %d", len(additionalVacancies), pageCount+2)
		vacancyInfos = append(vacancyInfos, additionalVacancies...)
		count += xhrResp.Num
		pageCount++
	}

	log.Printf("Total vacancies fetched: %d", len(vacancyInfos))
	return vacancyInfos, nil
}

// findJobTitlesDeftech finds job titles in the deftech.dou.ua/jobs HTML document
func findJobTitlesDeftech(n *html.Node) []VacancyInfo {
	var allLinks []struct {
		text string
		url  string
	}

	// First pass: collect all relevant links in document order
	collectLinks(n, &allLinks)

	// Second pass: pair job titles with companies
	var vacancyInfos []VacancyInfo
	for i := 0; i < len(allLinks); i++ {
		link := allLinks[i]
		if strings.Contains(link.url, "/jobs/companies/") && strings.Contains(link.url, "/vacancies/") {
			// Check if it's a job link (has a number after /vacancies/)
			parts := strings.Split(link.url, "/vacancies/")
			if len(parts) > 1 && len(parts[1]) > 0 && (parts[1][0] >= '0' && parts[1][0] <= '9') {
				vacancyInfo := VacancyInfo{Title: link.text, URL: link.url}

				// Look for the next company link
				for j := i + 1; j < len(allLinks) && j < i+3; j++ { // Look up to 2 links ahead
					nextLink := allLinks[j]
					if strings.Contains(nextLink.url, "jobs.dou.ua/companies/") && strings.Contains(nextLink.url, "/vacancies/") {
						parts := strings.Split(nextLink.url, "/vacancies/")
						if len(parts) > 1 && (parts[1] == "" || strings.HasPrefix(parts[1], "?")) {
							vacancyInfo.Company = nextLink.text
							break
						}
					}
				}

				vacancyInfos = append(vacancyInfos, vacancyInfo)
			}
		}
	}

	return vacancyInfos
}

// collectLinks collects all relevant links from the HTML document in document order
func collectLinks(n *html.Node, links *[]struct {
	text string
	url  string
}) {
	if n.Type == html.ElementNode && n.Data == "a" {
		for _, attr := range n.Attr {
			if attr.Key == "href" && strings.Contains(attr.Val, "/companies/") && strings.Contains(attr.Val, "/vacancies/") {
				text := strings.TrimSpace(extractText(n))
				if text != "" && !strings.Contains(text, "Більше вакансій") && len(text) < 100 {
					*links = append(*links, struct {
						text string
						url  string
					}{text: text, url: attr.Val})
				}
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectLinks(c, links)
	}
}

// extractText extracts text content from a node
func extractText(n *html.Node) string {
	var text strings.Builder
	collectText(n, &text)
	return text.String()
}

// handleText handles text messages
func (b *Bot) handleText(c telebot.Context) error {
	log.Printf("Text message received: %s", c.Text())
	// Echo the message back
	return c.Send("You said: "+c.Text(), b.getCommandKeyboard())
}

// handleCallback handles inline button callbacks
func (b *Bot) handleCallback(c telebot.Context) error {
	data := strings.TrimSpace(c.Callback().Data)
	log.Printf("Callback received: %s", data)

	switch data {
	case "/get_saved_visible":
		err := b.handleGetSavedVisible(c)
		c.Respond(&telebot.CallbackResponse{})
		return err
	case "/fetch_newest":
		err := b.handleFetchNewest(c)
		c.Respond(&telebot.CallbackResponse{})
		return err
	case "/fetch_latest":
		err := b.handleFetchLatest(c)
		c.Respond(&telebot.CallbackResponse{})
		return err
	case "/fetch_dwarf_engineering":
		err := b.handleGetDwarfEngineering(c)
		c.Respond(&telebot.CallbackResponse{})
		return err
	case "/get_saved_latest":
		err := b.handleGetSavedLatest(c)
		c.Respond(&telebot.CallbackResponse{})
		return err
	case "/hide_all":
		err := b.handleHideAll(c)
		c.Respond(&telebot.CallbackResponse{})
		return err
	case "/build_version":
		err := b.handleBuildVersion(c)
		c.Respond(&telebot.CallbackResponse{})
		return err
	case "/get_companies":
		err := b.handleCompanies(c)
		c.Respond(&telebot.CallbackResponse{})
		return err
	case "/back_to_menu":
		err := b.handleBackToMenu(c)
		c.Respond(&telebot.CallbackResponse{})
		return err
	default:
		// For now, just acknowledge the callback without functionality
		log.Printf("Unknown callback data: %s", data)
		return c.Respond(&telebot.CallbackResponse{Text: "Not implemented yet."})
	}
}

// Start starts the bot
func (b *Bot) Start() {
	log.Println("Bot started successfully")
	b.telebot.Start()
}

// Stop stops the bot gracefully
func (b *Bot) Stop() {
	log.Println("Bot stopped")
	if b.db != nil {
		b.db.Close()
	}
	b.telebot.Stop()
}
