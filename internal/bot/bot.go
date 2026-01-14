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
		"/help - Show this help message\n" +
		"/test_post - Post a test message to the configured group\n\n" +
		"/deftech - Fetch and show visible deftech vacancies\n\n" +
		"/dwarf_engineering - Get Dwarf Engineering vacancies\n" +
		"/deftech_all - Get vacancies from deftech.dou.ua\n\n" +
		"/truncate - Truncate vacancies table"
)

// Vacancy represents a vacancy listing
type Vacancy struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	IsHidden  bool      `json:"is_hidden"`
	CreatedAt time.Time `json:"created_at"`
}

// VacancyInfo represents vacancy information with company
type VacancyInfo struct {
	Title   string
	Company string
	URL     string
}

// Bot represents the Telegram bot instance
type Bot struct {
	telebot        *telebot.Bot
	db             *sql.DB
	httpClient     *http.Client
	adminID        string
	groupID        string
	dbName         string
	vacanciesTable string
}

// New creates a new bot instance
func New(token string, adminID string, groupID string, intervalStr string, dbName string, vacanciesTable string) (*Bot, error) {
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
		groupID:        groupID,
		dbName:         dbName,
		vacanciesTable: vacanciesTable,
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
			return c.Send("Sorry, you are not authorized to use this bot.")
		}
		return next(c)
	}
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
func (b *Bot) saveVacancy(title, url string) error {
	if b.vacancyExists(title) {
		return nil // already exists
	}
	query := fmt.Sprintf(`INSERT INTO %s (title, url, created_at, is_hidden) VALUES (?, ?, ?, FALSE)`, b.tableName())
	_, err := b.db.Exec(query, title, url, time.Now())
	return err
}

// vacancyExists checks if a vacancy with the given title already exists
func (b *Bot) vacancyExists(title string) bool {
	var count int
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE title = ?", b.tableName())
	err := b.db.QueryRow(query, title).Scan(&count)
	return err == nil && count > 0
}

// getVacancyIDAndHiddenByTitle gets the vacancy ID and hidden status by title
func (b *Bot) getVacancyIDAndHiddenByTitle(title string) (int, bool, error) {
	var id int
	var hidden bool
	query := fmt.Sprintf("SELECT id, is_hidden FROM %s WHERE title = ?", b.tableName())
	err := b.db.QueryRow(query, title).Scan(&id, &hidden)
	return id, hidden, err
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

// getVacancies retrieves vacancies from the database
func (b *Bot) getVacancies() ([]Vacancy, error) {
	query := fmt.Sprintf(`SELECT id, title, url, created_at, is_hidden FROM %s ORDER BY created_at ASC`, b.tableName())

	rows, err := b.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var vacancies []Vacancy
	for rows.Next() {
		var vacancy Vacancy
		err := rows.Scan(&vacancy.ID, &vacancy.Title, &vacancy.URL, &vacancy.CreatedAt, &vacancy.IsHidden)
		if err != nil {
			return nil, err
		}
		vacancies = append(vacancies, vacancy)
	}

	return vacancies, rows.Err()
}

// getVisibleVacancies retrieves all non-hidden vacancies from the database
func (b *Bot) getVisibleVacancies() ([]Vacancy, error) {
	query := fmt.Sprintf(`SELECT id, title, url, created_at, is_hidden FROM %s WHERE is_hidden = FALSE ORDER BY created_at ASC`, b.tableName())

	rows, err := b.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var vacancies []Vacancy
	for rows.Next() {
		var vacancy Vacancy
		err := rows.Scan(&vacancy.ID, &vacancy.Title, &vacancy.URL, &vacancy.CreatedAt, &vacancy.IsHidden)
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
	b.telebot.Handle("/deftech_all", b.handleGetDeftechAll)

	// Get visible deftech vacancies command handler
	b.telebot.Handle("/deftech", b.handleGetDeftech)

	// Truncate command handler
	b.telebot.Handle("/truncate", b.handleTruncate)

	// Test post command handler
	b.telebot.Handle("/test_post", b.handleTestPost)

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

// postMessage posts a test message to the configured group
func (b *Bot) postMessage() error {
	if b.groupID == "" {
		return fmt.Errorf("GROUP_ID not set")
	}

	groupIDInt, err := strconv.ParseInt(b.groupID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid GROUP_ID: %w", err)
	}

	chat := &telebot.Chat{ID: groupIDInt}
	message := "This is a test message from the Deftech Vacancies Bot."

	_, err = b.telebot.Send(chat, message)
	if err != nil {
		return fmt.Errorf("error sending message to group: %w", err)
	}

	log.Println("Manual test message posted to group")
	return nil
}

// postDeftechVacancies fetches deftech vacancies and posts them to the configured group only if there are new vacancies
func (b *Bot) postDeftechVacancies() error {
	if b.groupID == "" {
		return fmt.Errorf("GROUP_ID not set")
	}

	groupIDInt, err := strconv.ParseInt(b.groupID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid GROUP_ID: %w", err)
	}

	chat := &telebot.Chat{ID: groupIDInt}

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
			err := b.saveVacancy(vacancyInfo.Title, vacancyInfo.URL)
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
		return fmt.Errorf("error sending new vacancies to group: %w", err)
	}

	log.Printf("Posted %d new vacancies to group", len(newVacancyInfos))
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
			return c.Send("Invalid ignore ID")
		}
		title, err := b.getVacancyTitleByID(id)
		if err != nil {
			log.Printf("Error getting title for vacancy %d: %v", id, err)
			return c.Send("Error ignoring job")
		}
		err = b.setVacancyHidden(id, true)
		if err != nil {
			log.Printf("Error hiding vacancy %d: %v", id, err)
			return c.Send("Error hiding job")
		}
		return c.Send(fmt.Sprintf("%s is hidden", title))
	}
	if strings.HasPrefix(payload, "unignore_") {
		idStr := strings.TrimPrefix(payload, "unignore_")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			return c.Send("Invalid unignore ID")
		}
		title, err := b.getVacancyTitleByID(id)
		if err != nil {
			log.Printf("Error getting title for vacancy %d: %v", id, err)
			return c.Send("Error showing job")
		}
		err = b.setVacancyHidden(id, false)
		if err != nil {
			log.Printf("Error showing vacancy %d: %v", id, err)
			return c.Send("Error showing job")
		}
		return c.Send(fmt.Sprintf("%s is shown", title))
	}

	startText := "Hello! Welcome to the bot.\n\n" + commandsText
	return c.Send(startText)
}

// handleHelp handles the /help command
func (b *Bot) handleHelp(c telebot.Context) error {
	log.Printf("Command /help received")
	return c.Send(commandsText)
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
				if err := b.saveVacancy(job.Title, job.URL); err != nil {
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

// handleGetDeftechAll handles the /deftech_all command
func (b *Bot) handleGetDeftechAll(c telebot.Context) error {
	log.Printf("Command /deftech_all received")
	// Show loading message
	c.Send("Fetching vacancies from [https://deftech.dou.ua/vacancies/?city=Київ](https://deftech.dou.ua/vacancies/?city=%D0%9A%D0%B8%D1%97%D0%B2) →", telebot.ModeMarkdown)

	// Fetch vacancy titles from the deftech.dou.ua page
	vacancyInfos, err := b.FetchJobTitlesFromDeftech()
	if err != nil {
		log.Printf("Error fetching vacancy titles from deftech.dou.ua: %v", err)
		return c.Send(fmt.Sprintf("Error fetching vacancies: %v", err))
	}

	// Save vacancies to database if not exists
	for _, vacancyInfo := range vacancyInfos {
		err := b.saveVacancy(vacancyInfo.Title, vacancyInfo.URL)
		if err != nil {
			log.Printf("Error saving vacancy to DB: %v", err)
		}
	}

	if len(vacancyInfos) == 0 {
		return c.Send("No vacancies found.")
	}

	// Format and send the list as a simple numbered list
	var message strings.Builder

	for i, vacancyInfo := range vacancyInfos {
		id, hidden, err := b.getVacancyIDAndHiddenByTitle(vacancyInfo.Title)
		if err != nil {
			log.Printf("Error getting job ID for %s: %v", vacancyInfo.Title, err)
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
		message.WriteString(fmt.Sprintf("%d. [%s](%s) @ %s [%s](https://t.me/%s?start=%s_%d)\n", i+1, vacancyInfo.Title, vacancyInfo.URL, company, action, c.Bot().Me.Username, prefix, id))
	}

	return c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview)
}

// handleGetDeftech handles the /deftech command
func (b *Bot) handleGetDeftech(c telebot.Context) error {
	log.Printf("Command /deftech received")
	// Show loading message
	c.Send("Fetching vacancies from [https://deftech.dou.ua/jobs/?city=Київ](https://deftech.dou.ua/jobs/?city=%D0%9A%D0%B8%D1%97%D0%B2) →", telebot.ModeMarkdown)

	// Fetch job titles from the deftech.dou.ua page
	vacancyInfos, err := b.FetchJobTitlesFromDeftech()
	if err != nil {
		log.Printf("Error fetching vacancies from deftech.dou.ua: %v", err)
		return c.Send(fmt.Sprintf("Error fetching vacancies: %v", err))
	}

	// Save vacancies to database if not exists
	for _, vacancyInfo := range vacancyInfos {
		err := b.saveVacancy(vacancyInfo.Title, vacancyInfo.URL)
		if err != nil {
			log.Printf("Error saving vacancy to DB: %v", err)
		}
	}

	// Filter vacancyInfos to only show visible vacancies
	var visibleVacancyInfos []VacancyInfo
	for _, vacancyInfo := range vacancyInfos {
		_, hidden, err := b.getVacancyIDAndHiddenByTitle(vacancyInfo.Title)
		if err != nil {
			// If vacancy doesn't exist in DB yet (just saved), it's visible by default
			visibleVacancyInfos = append(visibleVacancyInfos, vacancyInfo)
		} else if !hidden {
			// Vacancy exists and is not hidden
			visibleVacancyInfos = append(visibleVacancyInfos, vacancyInfo)
		}
	}

	if len(visibleVacancyInfos) == 0 {
		return c.Send("No visible vacancies found.")
	}

	// Format and send the list as a simple numbered list
	var message strings.Builder

	for i, vacancyInfo := range visibleVacancyInfos {
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
		message.WriteString(fmt.Sprintf("%d. [%s](%s) @ %s [%s](https://t.me/%s?start=%s_%d)\n", i+1, vacancyInfo.Title, vacancyInfo.URL, company, action, c.Bot().Me.Username, prefix, id))
	}

	return c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview)
}

// handleGetDwarfEngineering handles the /dwarf_engineering command
func (b *Bot) handleGetDwarfEngineering(c telebot.Context) error {
	log.Printf("Command /dwarf_engineering received")
	// Show loading message
	c.Send("Fetching Dwarf Engineering vacancies →")

	var peopleforceTitles, douTitles []string

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

	if len(peopleforceTitles) == 0 && len(douTitles) == 0 {
		return c.Send("No vacancies found from either source.")
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
	}

	return c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview)
}

// handleTruncate handles the /truncate command
func (b *Bot) handleTruncate(c telebot.Context) error {
	log.Printf("Command /truncate received")
	query := fmt.Sprintf("DELETE FROM %s", b.tableName())
	_, err := b.db.Exec(query)
	if err != nil {
		log.Printf("Error truncating vacancies: %v", err)
		return c.Send("Error truncating vacancies table")
	}
	return c.Send("Vacancies table truncated successfully")
}

// handleTestPost handles the /test_post command
func (b *Bot) handleTestPost(c telebot.Context) error {
	log.Printf("Command /test_post received")

	if err := b.postMessage(); err != nil {
		log.Printf("Error posting message: %v", err)
		return c.Send(fmt.Sprintf("Error posting message: %v", err))
	}

	return c.Send("Test message posted to group successfully")
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
				if err := b.saveVacancy(title, item.Link); err != nil {
					log.Printf("Error saving vacancy to DB: %v", err)
				}
			}
		}
	}

	return vacancyTitles, nil
}

// FetchJobTitlesFromDeftech fetches and parses job titles from deftech.dou.ua page
func (b *Bot) FetchJobTitlesFromDeftech() ([]VacancyInfo, error) {
	url := "https://deftech.dou.ua/jobs/?city=%D0%9A%D0%B8%D1%97%D0%B2"

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
	return c.Send("You said: " + c.Text())
}

// handleCallback handles inline button callbacks
func (b *Bot) handleCallback(c telebot.Context) error {
	log.Printf("Callback received: %s", c.Callback().Data)

	// For now, just acknowledge the callback without functionality
	return c.Respond(&telebot.CallbackResponse{Text: "Ignore functionality not implemented yet"})
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
