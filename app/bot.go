package bot

import (
	"database/sql"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/net/html"
	telebot "gopkg.in/telebot.v3"
)

const (
	commandsText = "Available commands:\n" +
		"/start - Start the bot\n" +
		"/help - Show this help message\n\n" +
		"/get_saved_latest - Get latest saved vacancies\n" +
		"/get_saved_visible - Get visible saved vacancies\n\n" +
		"/fetch_newest - Fetch newest vacancies from deftech.dou.ua\n" +
		"/fetch_latest - Fetch latest vacancies from deftech.dou.ua\n\n" +
		"/dwarf_engineering - Fetch Dwarf Engineering vacancies\n\n" +
		"/clear_saved - Clear hidden vacancies"

	// Company names
	dwarfEngineeringCompany = "Dwarf Engineering"

	// Action prefixes for deep links
	ignorePrefix   = "ignore"
	unignorePrefix = "unignore"
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

// Bot represents the Telegram bot instance
type Bot struct {
	telebot        *telebot.Bot
	db             *sql.DB
	httpClient     *http.Client
	adminID        string
	dbName         string
	vacanciesTable string
	deftechURL     string
	limit          int
}

// New creates a new bot instance
func New(token string, adminID string, intervalStr string, dbName string, vacanciesTable string, deftechURL string, limit int) (*Bot, error) {
	pref := telebot.Settings{
		Token:  token,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
	}

	b, err := telebot.NewBot(pref)
	if err != nil {
		return nil, err
	}

	// Initialize HTTP client
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Initialize database
	db, err := sql.Open("sqlite3", dbName)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Run database migrations
	if err := runMigrations(db); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	bot := &Bot{
		telebot:        b,
		db:             db,
		httpClient:     httpClient,
		adminID:        adminID,
		dbName:         dbName,
		vacanciesTable: vacanciesTable,
		deftechURL:     deftechURL,
		limit:          limit,
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

// tableName returns the vacancies table name
func (b *Bot) tableName() string {
	return b.vacanciesTable
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
	btnDwarf := markup.Data("Fetch Dwarf Engineering", "/dwarf_engineering")
	markup.Inline(
		markup.Row(btnGetSavedVisible, btnGetSavedLatest),
		markup.Row(btnFetchNewest, btnFetchLatest),
		markup.Row(btnDwarf),
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
	query := fmt.Sprintf(`INSERT INTO %s (title, url, company_id, created_at, is_hidden) VALUES (?, ?, ?, ?, FALSE)`, b.tableName())
	_, err = b.db.Exec(query, title, url, companyID, time.Now())
	return err
}

// vacancyExists checks if a vacancy with the given title already exists
func (b *Bot) vacancyExists(title string) bool {
	var count int
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE title = ?", b.tableName())
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

// getCompanyName safely gets the company name for a vacancy
func (b *Bot) getCompanyName(vacancy Vacancy) string {
	if vacancy.CompanyID != nil {
		if name, err := b.getCompanyNameByID(*vacancy.CompanyID); err == nil {
			return name
		}
	}
	return "Unknown"
}

// getTotalVacancyCountExcludingDwarf gets the total count of vacancies excluding Dwarf Engineering
func (b *Bot) getTotalVacancyCountExcludingDwarf() (int, error) {
	query := fmt.Sprintf(`
		SELECT COUNT(*) FROM %s v
		LEFT JOIN companies c ON v.company_id = c.id
		WHERE c.name != ? OR c.name IS NULL
	`, b.tableName())

	var count int
	err := b.db.QueryRow(query, dwarfEngineeringCompany).Scan(&count)
	return count, err
}

// getLatestVacancies gets the latest N vacancies from database
func (b *Bot) getLatestVacancies(limit int) ([]Vacancy, error) {
	query := fmt.Sprintf(`SELECT v.id, v.title, v.url, v.company_id, v.is_hidden, v.created_at FROM %s v
LEFT JOIN companies c ON v.company_id = c.id
WHERE c.name != ? OR c.name IS NULL
ORDER BY v.created_at DESC LIMIT %d`, b.tableName(), limit)

	rows, err := b.db.Query(query, dwarfEngineeringCompany)
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
	query := fmt.Sprintf("SELECT id, is_hidden FROM %s WHERE title = ?", b.tableName())
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
	return fmt.Sprintf("%d. [%s](%s) @ %s | [%s](https://t.me/%s?start=%s_%d)\n",
		index+1, vacancy.Title, vacancy.URL, company, action, botUsername, prefix, vacancy.ID)
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
	return fmt.Sprintf("%d. [%s](%s) @ %s | [%s](https://t.me/%s?start=%s_%d)\n",
		index+1, vacancyInfo.Title, vacancyInfo.URL, company, action, botUsername, prefix, id)
}

// setVacancyHidden sets the hidden status of a vacancy by ID
func (b *Bot) setVacancyHidden(id int, hidden bool) error {
	query := fmt.Sprintf("UPDATE %s SET is_hidden = ? WHERE id = ?", b.tableName())
	_, err := b.db.Exec(query, hidden, id)
	return err
}

// getVacancyTitleByID gets the vacancy title by ID
func (b *Bot) getVacancyTitleByID(id int) (string, error) {
	var title string
	query := fmt.Sprintf("SELECT title FROM %s WHERE id = ?", b.tableName())
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

	// Format and send the list
	var message strings.Builder
	message.WriteString(fmt.Sprintf("Total vacancies: %d\n\n", totalCount))

	for i, vacancy := range vacancies {
		message.WriteString(b.formatVacancyMessage(i, vacancy, c.Bot().Me.Username))
	}

	return c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview, b.getCommandKeyboard(), telebot.Silent)
}

// getVisibleVacancies retrieves all non-hidden vacancies from the database
func (b *Bot) getVisibleVacancies() ([]Vacancy, error) {
	query := fmt.Sprintf(`SELECT v.id, v.title, v.url, v.company_id, v.is_hidden, v.created_at FROM %s v
LEFT JOIN companies c ON v.company_id = c.id
WHERE v.is_hidden = FALSE AND (c.name != ? OR c.name IS NULL)
ORDER BY v.created_at ASC`, b.tableName())

	rows, err := b.db.Query(query, dwarfEngineeringCompany)
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
	query := fmt.Sprintf(`SELECT id, title, url, company_id, is_hidden, created_at FROM %s ORDER BY created_at ASC`, b.tableName())

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

// registerHandlers registers all bot command and message handlers
func (b *Bot) registerHandlers() {
	// Start command handler
	b.telebot.Handle("/start", b.handleStart)

	// Help command handler
	b.telebot.Handle("/help", b.handleHelp)

	// Get Dwarf Engineering vacancies command handler
	b.telebot.Handle("/dwarf_engineering", b.handleGetDwarfEngineering)

	// Get list deftech command handler
	b.telebot.Handle("/fetch_newest", b.handleFetchNewest)

	// Fetch latest deftech command handler
	b.telebot.Handle("/fetch_latest", b.handleFetchLatest)

	// Get visible deftech vacancies command handler
	b.telebot.Handle("/get_saved_visible", b.handleGetSavedVisible)

	// Get saved vacancies command handler
	b.telebot.Handle("/get_saved_latest", b.handleGetSavedLatest)

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
		message.WriteString(fmt.Sprintf("%d. [%s](%s) @ %s [%s](https://t.me/%s?start=%s_%d)\n", i+1, vacancyInfo.Title, vacancyInfo.URL, company, action, b.telebot.Me.Username, prefix, id))
	}

	_, err = b.telebot.Send(chat, message.String(), telebot.ModeMarkdown, telebot.NoPreview)
	if err != nil {
		return fmt.Errorf("error sending new vacancies: %w", err)
	}

	log.Printf("Posted %d new vacancies", len(newVacancyInfos))
	return nil
}

// handleStart handles the /start command
func (b *Bot) handleStart(c telebot.Context) error {
	log.Printf("Command /start received")

	// Check if this is an ignore/unignore command via deep link
	payload := strings.TrimSpace(c.Message().Payload)
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
		err = b.setVacancyHidden(id, true)
		if err != nil {
			log.Printf("Error hiding vacancy %d: %v", id, err)
			return c.Send("Error hiding vacancy", b.getCommandKeyboard(), telebot.Silent)
		}
		return c.Send(fmt.Sprintf("%s is hidden", title), b.getCommandKeyboard(), telebot.Silent)
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
		err = b.setVacancyHidden(id, false)
		if err != nil {
			log.Printf("Error showing vacancy %d: %v", id, err)
			return c.Send("Error showing job", b.getCommandKeyboard(), telebot.Silent)
		}
		return c.Send(fmt.Sprintf("%s is shown", title), b.getCommandKeyboard(), telebot.Silent)
	}

	startText := "Hello! Welcome to the bot.\n\n" + commandsText
	// Add version if available
	if version, err := os.ReadFile("VERSION"); err == nil {
		startText = fmt.Sprintf("Hello! Welcome to the bot (v%s).\n\n", strings.TrimSpace(string(version))) + commandsText
	}
	return c.Send(startText, b.getCommandKeyboard(), telebot.Silent)
}

// handleHelp handles the /help command
func (b *Bot) handleHelp(c telebot.Context) error {
	log.Printf("Command /help received")
	return c.Send(commandsText, b.getCommandKeyboard(), telebot.Silent)
}

// fetchJobTitles fetches and parses vacancy titles from all careers pages
func (b *Bot) fetchJobTitles() ([]string, error) {
	var allJobTitles []string
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

		// Add unique vacancy titles and save to DB
		for _, job := range vacancies {
			if !seen[job.Title] {
				seen[job.Title] = true
				allJobTitles = append(allJobTitles, job.Title)
				// Save to database with correct URL
				if err := b.saveVacancy(job.Title, job.URL, "Dwarf Engineering"); err != nil {
					log.Printf("Error saving vacancy to DB: %v", err)
				}
			}
		}
	}

	return allJobTitles, nil
}

// fetchJobTitlesFromPage fetches and parses vacancy titles from a specific page
func (b *Bot) fetchJobTitlesFromPage(page int) ([]VacancyInfo, error) {
	url := fmt.Sprintf("https://dwarfengineering.peopleforce.io/careers?page=%d", page)

	resp, err := b.httpClient.Get(url)
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

// extractText extracts text content from a node
func (b *Bot) extractText(n *html.Node) string {
	var text strings.Builder
	b.collectText(n, &text)
	return text.String()
}

// collectText recursively collects text from a node
func (b *Bot) collectText(n *html.Node, text *strings.Builder) {
	if n.Type == html.TextNode {
		text.WriteString(n.Data)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.collectText(c, text)
	}
}

// findJobTitles finds vacancy titles and URLs in the HTML document
func (b *Bot) findJobTitles(n *html.Node) []VacancyInfo {
	var vacancies []VacancyInfo
	if n.Type == html.ElementNode && n.Data == "h4" {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "a" {
				title := strings.TrimSpace(b.extractText(c))
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

	return c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview, b.getCommandKeyboard(), telebot.Silent)
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

	// Format and send the list
	var message strings.Builder

	for i, vacancy := range vacancies {
		message.WriteString(b.formatVacancyMessage(i, vacancy, c.Bot().Me.Username))
	}

	return c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview, b.getCommandKeyboard(), telebot.Silent)
}

// handleGetSavedLatest handles the /get_saved_latest command
func (b *Bot) handleGetSavedLatest(c telebot.Context) error {
	log.Printf("Command /get_saved_latest received")
	// Show loading message
	c.Send(fmt.Sprintf("Getting %d latest saved vacancies from db →", b.limit), telebot.ModeMarkdown, telebot.Silent)

	totalCount, err := b.getTotalVacancyCountExcludingDwarf()
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

// handleGetDwarfEngineering handles the /dwarf_engineering command
func (b *Bot) handleGetDwarfEngineering(c telebot.Context) error {
	log.Printf("Command /dwarf_engineering received")
	// Show loading message
	c.Send("Fetching Dwarf Engineering vacancies →", telebot.Silent)

	var peopleforceTitles, douTitles []string
	var djinniVacancies []DjinniVacancyInfo

	// Fetch from PeopleForce
	peopleforceTitlesRaw, err := b.fetchJobTitles()
	if err != nil {
		log.Printf("Error fetching vacancies from https://dwarfengineering.peopleforce.io/careers/: %v", err)
		// Continue even if one source fails
	} else {
		peopleforceTitles = peopleforceTitlesRaw
	}

	// Fetch from DOU.ua RSS
	douTitlesRaw, err := b.fetchJobTitlesFromDOU()
	if err != nil {
		log.Printf("Error fetching vacancies from jobs.dou.ua: %v", err)
		// Continue even if one source fails
	} else {
		douTitles = douTitlesRaw
	}

	// Fetch from djinni.co
	_, djinniVacanciesRaw, err := b.fetchJobTitlesFromDjinni()
	if err != nil {
		log.Printf("Error fetching vacancies from djinni.co: %v", err)
		// Continue even if one source fails
	} else {
		djinniVacancies = djinniVacanciesRaw
	}

	if len(peopleforceTitles) == 0 && len(douTitles) == 0 && len(djinniVacancies) == 0 {
		return c.Send("No vacancies found from any source.", b.getCommandKeyboard(), telebot.Silent)
	}

	// Format and send the list
	var message strings.Builder

	if len(peopleforceTitles) > 0 {
		message.WriteString("**[dwarfengineering.peopleforce.io/careers](https://dwarfengineering.peopleforce.io/careers):**\n")
		for i, title := range peopleforceTitles {
			// Get vacancy URL from database
			var url string
			err := b.db.QueryRow(fmt.Sprintf("SELECT url FROM %s WHERE title = ?", b.tableName()), title).Scan(&url)
			if err != nil {
				log.Printf("Error getting URL for vacancy %s: %v", title, err)
				url = "#"
			}
			message.WriteString(fmt.Sprintf("%d. [%s](%s)\n", i+1, title, url))
		}
		message.WriteString("\n")
	}

	if len(douTitles) > 0 {
		message.WriteString("**[jobs.dou.ua/companies/dwarf-engineering/vacancies](https://jobs.dou.ua/companies/dwarf-engineering/vacancies/):**\n")
		for i, title := range douTitles {
			// Get job URL from database
			var url string
			err := b.db.QueryRow(fmt.Sprintf("SELECT url FROM %s WHERE title = ?", b.tableName()), title).Scan(&url)
			if err != nil {
				log.Printf("Error getting URL for vacancy %s: %v", title, err)
				url = "#"
			}
			message.WriteString(fmt.Sprintf("%d. [%s](%s)\n", i+1, title, url))
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
	query := fmt.Sprintf("DELETE FROM %s WHERE is_hidden = 1", b.tableName())
	_, err := b.db.Exec(query)
	if err != nil {
		log.Printf("Error clearing hidden vacancies: %v", err)
		return c.Send("Error clearing hidden vacancies", b.getCommandKeyboard(), telebot.Silent)
	}
	return c.Send("Hidden vacancies cleared successfully", b.getCommandKeyboard(), telebot.Silent)
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

// fetchJobTitlesFromDOU fetches and parses job titles from DOU.ua RSS feed
func (b *Bot) fetchJobTitlesFromDOU() ([]string, error) {
	url := "https://jobs.dou.ua/vacancies/dwarf-engineering/feeds/"

	// Fetch the RSS feed
	resp, err := b.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch RSS feed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse XML/RSS feed
	var feed RSSFeed
	err = xml.Unmarshal(body, &feed)
	if err != nil {
		return nil, fmt.Errorf("failed to parse RSS feed: %w", err)
	}

	// Extract vacancy titles from RSS items
	var vacancyTitles []string
	for _, item := range feed.Channel.Items {
		if item.Title != "" {
			// Clean up the title - remove location suffix if present (e.g., " в Dwarf Engineering, Київ")
			title := strings.TrimSpace(item.Title)
			// Remove the " в Dwarf Engineering, Київ" suffix if it exists
			if idx := strings.Index(title, " в Dwarf Engineering"); idx != -1 {
				title = title[:idx]
			}
			if title != "" {
				vacancyTitles = append(vacancyTitles, title)
				// Save to database
				if err := b.saveVacancy(title, item.Link, "Dwarf Engineering"); err != nil {
					log.Printf("Error saving vacancy to DB: %v", err)
				}
			}
		}
	}

	return vacancyTitles, nil
}

// fetchJobTitlesFromDjinni fetches and parses job titles from djinni.co Dwarf Engineering page
func (b *Bot) fetchJobTitlesFromDjinni() ([]string, []DjinniVacancyInfo, error) {
	url := "https://djinni.co/jobs/company-dwarf-engineering/"

	resp, err := b.httpClient.Get(url)
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
		if node.Type == html.ElementNode && node.Data == "li" {
			// Check if this is a job item
			for _, attr := range node.Attr {
				if attr.Key == "id" && strings.HasPrefix(attr.Val, "job-item-") {
					// Found a job item, extract information
					vacancy := b.extractDjinniVacancyInfo(node)
					if vacancy.Title != "" && vacancy.URL != "" && !seen[vacancy.Title] {
						seen[vacancy.Title] = true
						vacancyTitles = append(vacancyTitles, vacancy.Title)
						vacancies = append(vacancies, vacancy)
						// Save to database
						if err := b.saveVacancy(vacancy.Title, vacancy.URL, "Dwarf Engineering"); err != nil {
							log.Printf("Error saving vacancy to DB: %v", err)
						}
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
			// Extract title and URL from h2 element
			if n.Data == "h2" {
				for _, attr := range n.Attr {
					if attr.Key == "class" && strings.Contains(attr.Val, "fs-3") {
						for c := n.FirstChild; c != nil; c = c.NextSibling {
							if c.Type == html.ElementNode && c.Data == "a" {
								vacancy.Title = strings.TrimSpace(b.extractText(c))
								for _, a := range c.Attr {
									if a.Key == "href" {
										vacancy.URL = a.Val
										if !strings.HasPrefix(vacancy.URL, "http") {
											vacancy.URL = "https://djinni.co" + vacancy.URL
										}
										break
									}
								}
								break
							}
						}
					}
				}
			}

			// Extract views and applies from the metadata section
			if n.Data == "div" {
				for _, attr := range n.Attr {
					if attr.Key == "class" && strings.Contains(attr.Val, "text-secondary") {
						text := strings.TrimSpace(b.extractText(n))
						// Look for patterns like "46 переглядів" (views) and "X відгуків" (applies/responses)
						if strings.Contains(text, "переглядів") {
							// Extract number before "переглядів"
							parts := strings.Fields(text)
							for i, part := range parts {
								if part == "переглядів" && i > 0 {
									vacancy.Views = parts[i-1]
									break
								}
							}
						}
						if strings.Contains(text, "відгуків") || strings.Contains(text, "відгук") {
							// Extract number before "відгуків" or "відгук"
							parts := strings.Fields(text)
							for i, part := range parts {
								if (part == "відгуків" || part == "відгук") && i > 0 {
									vacancy.Applies = parts[i-1]
									break
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

// FetchJobTitlesFromDeftech fetches and parses job titles from deftech.dou.ua page
func (b *Bot) FetchJobTitlesFromDeftech() ([]VacancyInfo, error) {
	url := b.deftechURL

	resp, err := b.httpClient.Get(url)
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

	vacancyInfos := findJobTitlesDeftech(doc)
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

// collectText recursively collects text from a node
func collectText(n *html.Node, text *strings.Builder) {
	if n.Type == html.TextNode {
		text.WriteString(n.Data)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectText(c, text)
	}
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
	case "/dwarf_engineering":
		err := b.handleGetDwarfEngineering(c)
		c.Respond(&telebot.CallbackResponse{})
		return err
	case "/get_saved_latest":
		err := b.handleGetSavedLatest(c)
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
