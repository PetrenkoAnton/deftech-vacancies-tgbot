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

// Job represents a job listing
type Job struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	IsHidden  bool      `json:"is_hidden"`
	CreatedAt time.Time `json:"created_at"`
}

// JobInfo represents job information with hot status
type JobInfo struct {
	Title   string
	Company string
	URL     string
}

// Bot represents the Telegram bot instance
type Bot struct {
	telebot    *telebot.Bot
	db         *sql.DB
	httpClient *http.Client
	adminID    string
}

// New creates a new bot instance
func New(token string, adminID string) (*Bot, error) {
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
	db, err := sql.Open("sqlite3", "./jobs.db")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Run database migrations
	if err := runMigrations(db); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	bot := &Bot{
		telebot:    b,
		db:         db,
		httpClient: httpClient,
		adminID:    adminID,
	}

	// Register handlers
	bot.registerHandlers()

	return bot, nil
}

// isAdmin checks if the user is authorized to use the bot
func (b *Bot) isAdmin(userID int64) bool {
	if b.adminID == "" {
		// If no admin ID is set, allow all users (for development)
		return true
	}
	return fmt.Sprintf("%d", userID) == b.adminID
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

// saveJob saves a job to the database
func (b *Bot) saveJob(title, url string) error {
	if b.jobExists(title) {
		return nil // already exists
	}
	query := `INSERT INTO jobs (title, url, created_at, is_hidden) VALUES (?, ?, ?, FALSE)`
	_, err := b.db.Exec(query, title, url, time.Now())
	return err
}

// jobExists checks if a job with the given title already exists
func (b *Bot) jobExists(title string) bool {
	var count int
	err := b.db.QueryRow("SELECT COUNT(*) FROM jobs WHERE title = ?", title).Scan(&count)
	return err == nil && count > 0
}

// getJobIDAndHiddenByTitle gets the job ID and hidden status by title
func (b *Bot) getJobIDAndHiddenByTitle(title string) (int, bool, error) {
	var id int
	var hidden bool
	err := b.db.QueryRow("SELECT id, is_hidden FROM jobs WHERE title = ?", title).Scan(&id, &hidden)
	return id, hidden, err
}

// setJobHidden sets the hidden status of a job by ID
func (b *Bot) setJobHidden(id int, hidden bool) error {
	_, err := b.db.Exec("UPDATE jobs SET is_hidden = ? WHERE id = ?", hidden, id)
	return err
}

// getJobTitleByID gets the job title by ID
func (b *Bot) getJobTitleByID(id int) (string, error) {
	var title string
	err := b.db.QueryRow("SELECT title FROM jobs WHERE id = ?", id).Scan(&title)
	return title, err
}

// getJobs retrieves jobs from the database
func (b *Bot) getJobs() ([]Job, error) {
	query := `SELECT id, title, url, created_at, is_hidden FROM jobs ORDER BY created_at ASC`

	rows, err := b.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var job Job
		err := rows.Scan(&job.ID, &job.Title, &job.URL, &job.CreatedAt, &job.IsHidden)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}

	return jobs, rows.Err()
}

// getVisibleJobs retrieves all non-hidden jobs from the database
func (b *Bot) getVisibleJobs() ([]Job, error) {
	query := `SELECT id, title, url, created_at, is_hidden FROM jobs WHERE is_hidden = FALSE ORDER BY created_at ASC`

	rows, err := b.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var job Job
		err := rows.Scan(&job.ID, &job.Title, &job.URL, &job.CreatedAt, &job.IsHidden)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}

	return jobs, rows.Err()
}

// registerHandlers registers all bot command and message handlers
func (b *Bot) registerHandlers() {
	// Start command handler
	b.telebot.Handle("/start", b.handleStart)

	// Help command handler
	b.telebot.Handle("/help", b.handleHelp)

	// Get Dwarf Engineering jobs command handler
	b.telebot.Handle("/dwarf_engineering", b.handleGetDwarfEngineering)

	// Get list DefTech command handler
	b.telebot.Handle("/deftech_all", b.handleGetDeftechAll)

	// Get visible DefTech jobs command handler
	b.telebot.Handle("/deftech", b.handleGetDeftech)

	// Truncate command handler
	b.telebot.Handle("/truncate", b.handleTruncate)

	// Inline button callback handler
	b.telebot.Handle(telebot.OnCallback, b.handleCallback)

	// Default message handler
	b.telebot.Handle(telebot.OnText, b.handleText)
}

// handleStart handles the /start command
func (b *Bot) handleStart(c telebot.Context) error {
	if !b.isAdmin(c.Sender().ID) {
		log.Printf("Unauthorized access attempt from user %s (ID: %d)", c.Sender().Username, c.Sender().ID)
		return c.Send("Sorry, you are not authorized to use this bot.")
	}

	log.Printf("Command /start received from user %s", c.Sender().Username)

	// Check if this is an ignore/unignore command via deep link
	payload := strings.TrimSpace(c.Message().Payload)
	if strings.HasPrefix(payload, "ignore_") {
		idStr := strings.TrimPrefix(payload, "ignore_")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			return c.Send("Invalid ignore ID")
		}
		title, err := b.getJobTitleByID(id)
		if err != nil {
			log.Printf("Error getting title for job %d: %v", id, err)
			return c.Send("Error ignoring job")
		}
		err = b.setJobHidden(id, true)
		if err != nil {
			log.Printf("Error hiding job %d: %v", id, err)
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
		title, err := b.getJobTitleByID(id)
		if err != nil {
			log.Printf("Error getting title for job %d: %v", id, err)
			return c.Send("Error showing job")
		}
		err = b.setJobHidden(id, false)
		if err != nil {
			log.Printf("Error showing job %d: %v", id, err)
			return c.Send("Error showing job")
		}
		return c.Send(fmt.Sprintf("%s is shown", title))
	}

	startText := "Hello! Welcome to the bot.\n\nAvailable commands:\n" +
		"/start - Start the bot\n" +
		"/help - Show this help message\n\n" +
		"/dwarf_engineering - Get Dwarf Engineering jobs\n" +
		"/deftech - Fetch and show visible DefTech jobs\n\n" +
		"/deftech_all - Get list from DefTech DOU.ua\n\n" +
		"/truncate - Truncate jobs table"
	return c.Send(startText)
}

// handleHelp handles the /help command
func (b *Bot) handleHelp(c telebot.Context) error {
	if !b.isAdmin(c.Sender().ID) {
		log.Printf("Unauthorized access attempt from user %s (ID: %d)", c.Sender().Username, c.Sender().ID)
		return c.Send("Sorry, you are not authorized to use this bot.")
	}

	log.Printf("Command /help received from user %s", c.Sender().Username)
	helpText := "Available commands:\n" +
		"/start - Start the bot\n" +
		"/help - Show this help message\n" +
		"/dwarf_engineering - Get Dwarf Engineering jobs from PeopleForce and DOU.ua\n" +
		"/deftech_all - Get list from DefTech DOU.ua\n" +
		"/deftech - Fetch and show visible DefTech jobs\n" +
		"/truncate - Truncate jobs table"
	return c.Send(helpText)
}

// fetchJobTitles fetches and parses job titles from all careers pages
func (b *Bot) fetchJobTitles() ([]string, error) {
	var allJobTitles []string
	seen := make(map[string]bool) // To avoid duplicates

	// Fetch pages 1 and 2
	pages := []int{1, 2}
	for _, page := range pages {
		jobTitles, err := b.fetchJobTitlesFromPage(page)
		if err != nil {
			log.Printf("Error fetching page %d: %v", page, err)
			// Continue with other pages even if one fails
			continue
		}

		// Add unique job titles and save to DB
		for _, title := range jobTitles {
			if !seen[title] {
				seen[title] = true
				allJobTitles = append(allJobTitles, title)
				// Save to database
				url := fmt.Sprintf("https://dwarfengineering.peopleforce.io/careers?page=%d", page)
				if err := b.saveJob(title, url); err != nil {
					log.Printf("Error saving job to DB: %v", err)
				}
			}
		}
	}

	return allJobTitles, nil
}

// fetchJobTitlesFromPage fetches and parses job titles from a specific page
func (b *Bot) fetchJobTitlesFromPage(page int) ([]string, error) {
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

	jobTitles := b.findJobTitles(doc)
	return jobTitles, nil
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

// findJobTitles finds job titles in the HTML document
func (b *Bot) findJobTitles(n *html.Node) []string {
	var titles []string
	if n.Type == html.ElementNode && n.Data == "h4" {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "a" {
				title := strings.TrimSpace(b.extractText(c))
				if title != "" {
					titles = append(titles, title)
				}
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		titles = append(titles, b.findJobTitles(c)...)
	}
	return titles
}

// handleGetDeftechAll handles the /deftech_all command
func (b *Bot) handleGetDeftechAll(c telebot.Context) error {
	if !b.isAdmin(c.Sender().ID) {
		log.Printf("Unauthorized access attempt from user %s (ID: %d)", c.Sender().Username, c.Sender().ID)
		return c.Send("Sorry, you are not authorized to use this bot.")
	}

	log.Printf("Command /deftech_all received from user %s", c.Sender().Username)
	// Show loading message
	c.Send("Fetching job listings from [https://deftech.dou.ua/jobs/?city=Київ](https://deftech.dou.ua/jobs/?city=%D0%9A%D0%B8%D1%97%D0%B2) ...", telebot.ModeMarkdown)

	// Fetch job titles from the DefTech DOU.ua page
	jobInfos, err := b.FetchJobTitlesFromDeftech()
	if err != nil {
		log.Printf("Error fetching job titles from DefTech: %v", err)
		return c.Send(fmt.Sprintf("Error fetching job listings: %v", err))
	}

	// Save jobs to database if not exists
	for _, jobInfo := range jobInfos {
		err := b.saveJob(jobInfo.Title, jobInfo.URL)
		if err != nil {
			log.Printf("Error saving job to DB: %v", err)
		}
	}

	if len(jobInfos) == 0 {
		return c.Send("No job listings found.")
	}

	// Format and send the list as a simple numbered list
	var message strings.Builder

	for i, jobInfo := range jobInfos {
		id, hidden, err := b.getJobIDAndHiddenByTitle(jobInfo.Title)
		if err != nil {
			log.Printf("Error getting job ID for %s: %v", jobInfo.Title, err)
			continue
		}
		action := "hide"
		prefix := "ignore"
		if hidden {
			action = "show"
			prefix = "unignore"
		}
		company := jobInfo.Company
		if company == "" {
			company = "-"
		}
		message.WriteString(fmt.Sprintf("%d. [%s](%s) (%s) [%s](https://t.me/%s?start=%s_%d)\n", i+1, jobInfo.Title, jobInfo.URL, company, action, c.Bot().Me.Username, prefix, id))
	}

	return c.Send(message.String(), telebot.ModeMarkdown)
}

// handleGetDeftech handles the /deftech command
func (b *Bot) handleGetDeftech(c telebot.Context) error {
	if !b.isAdmin(c.Sender().ID) {
		log.Printf("Unauthorized access attempt from user %s (ID: %d)", c.Sender().Username, c.Sender().ID)
		return c.Send("Sorry, you are not authorized to use this bot.")
	}

	log.Printf("Command /deftech received from user %s", c.Sender().Username)
	// Show loading message
	c.Send("Fetching job listings from [https://deftech.dou.ua/jobs/?city=Київ](https://deftech.dou.ua/jobs/?city=%D0%9A%D0%B8%D1%97%D0%B2) ...", telebot.ModeMarkdown)

	// Fetch job titles from the DefTech DOU.ua page
	jobInfos, err := b.FetchJobTitlesFromDeftech()
	if err != nil {
		log.Printf("Error fetching job titles from DefTech: %v", err)
		return c.Send(fmt.Sprintf("Error fetching job listings: %v", err))
	}

	// Save jobs to database if not exists
	for _, jobInfo := range jobInfos {
		err := b.saveJob(jobInfo.Title, jobInfo.URL)
		if err != nil {
			log.Printf("Error saving job to DB: %v", err)
		}
	}

	// Filter jobInfos to only show visible jobs
	var visibleJobInfos []JobInfo
	for _, jobInfo := range jobInfos {
		_, hidden, err := b.getJobIDAndHiddenByTitle(jobInfo.Title)
		if err != nil {
			// If job doesn't exist in DB yet (just saved), it's visible by default
			visibleJobInfos = append(visibleJobInfos, jobInfo)
		} else if !hidden {
			// Job exists and is not hidden
			visibleJobInfos = append(visibleJobInfos, jobInfo)
		}
	}

	if len(visibleJobInfos) == 0 {
		return c.Send("No visible job listings found.")
	}

	// Format and send the list as a simple numbered list
	var message strings.Builder

	for i, jobInfo := range visibleJobInfos {
		id, hidden, err := b.getJobIDAndHiddenByTitle(jobInfo.Title)
		if err != nil {
			log.Printf("Error getting job ID for %s: %v", jobInfo.Title, err)
			continue
		}
		action := "hide"
		prefix := "ignore"
		if hidden {
			action = "show"
			prefix = "unignore"
		}
		company := jobInfo.Company
		if company == "" {
			company = "-"
		}
		message.WriteString(fmt.Sprintf("%d. [%s](%s) (%s) [%s](https://t.me/%s?start=%s_%d)\n", i+1, jobInfo.Title, jobInfo.URL, company, action, c.Bot().Me.Username, prefix, id))
	}

	return c.Send(message.String(), telebot.ModeMarkdown)
}

// handleGetDwarfEngineering handles the /dwarf_engineering command
func (b *Bot) handleGetDwarfEngineering(c telebot.Context) error {
	if !b.isAdmin(c.Sender().ID) {
		log.Printf("Unauthorized access attempt from user %s (ID: %d)", c.Sender().Username, c.Sender().ID)
		return c.Send("Sorry, you are not authorized to use this bot.")
	}

	log.Printf("Command /dwarf_engineering received from user %s", c.Sender().Username)
	// Show loading message
	c.Send("Fetching Dwarf Engineering job listings...")

	var peopleforceTitles []string
	var douTitles []string

	// Fetch from PeopleForce
	peopleforceTitlesRaw, err := b.fetchJobTitles()
	if err != nil {
		log.Printf("Error fetching PeopleForce job titles: %v", err)
		// Continue even if one source fails
	} else {
		peopleforceTitles = peopleforceTitlesRaw
	}

	// Fetch from DOU.ua RSS
	douTitlesRaw, err := b.fetchJobTitlesFromDOU()
	if err != nil {
		log.Printf("Error fetching DOU.ua job titles: %v", err)
		// Continue even if one source fails
	} else {
		douTitles = douTitlesRaw
	}

	if len(peopleforceTitles) == 0 && len(douTitles) == 0 {
		return c.Send("No job listings found from either source.")
	}

	// Format and send the list
	var message strings.Builder

	if len(peopleforceTitles) > 0 {
		message.WriteString("**[dwarfengineering.peopleforce.io/careers](https://dwarfengineering.peopleforce.io/careers):**\n")
		for i, title := range peopleforceTitles {
			// Get job URL from database
			var url string
			err := b.db.QueryRow("SELECT url FROM jobs WHERE title = ?", title).Scan(&url)
			if err != nil {
				log.Printf("Error getting URL for job %s: %v", title, err)
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
			err := b.db.QueryRow("SELECT url FROM jobs WHERE title = ?", title).Scan(&url)
			if err != nil {
				log.Printf("Error getting URL for job %s: %v", title, err)
				url = "#"
			}
			message.WriteString(fmt.Sprintf("%d. [%s](%s)\n", i+1, title, url))
		}
	}

	return c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview)
}

// handleTruncate handles the /truncate command
func (b *Bot) handleTruncate(c telebot.Context) error {
	if !b.isAdmin(c.Sender().ID) {
		log.Printf("Unauthorized truncate command from user %s (ID: %d)", c.Sender().Username, c.Sender().ID)
		return c.Send("Sorry, you are not authorized to use this bot.")
	}

	log.Printf("Command /truncate received from user %s", c.Sender().Username)
	_, err := b.db.Exec("DELETE FROM jobs")
	if err != nil {
		log.Printf("Error truncating jobs: %v", err)
		return c.Send("Error truncating jobs table")
	}
	return c.Send("Jobs table truncated successfully")
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

	// Extract job titles from RSS items
	var jobTitles []string
	for _, item := range feed.Channel.Items {
		if item.Title != "" {
			// Clean up the title - remove location suffix if present (e.g., " в Dwarf Engineering, Київ")
			title := strings.TrimSpace(item.Title)
			// Remove the " в Dwarf Engineering, Київ" suffix if it exists
			if idx := strings.Index(title, " в Dwarf Engineering"); idx != -1 {
				title = title[:idx]
			}
			if title != "" {
				jobTitles = append(jobTitles, title)
				// Save to database
				if err := b.saveJob(title, item.Link); err != nil {
					log.Printf("Error saving job to DB: %v", err)
				}
			}
		}
	}

	return jobTitles, nil
}

// FetchJobTitlesFromDeftech fetches and parses job titles from DefTech DOU.ua page
func (b *Bot) FetchJobTitlesFromDeftech() ([]JobInfo, error) {
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

	jobInfos := findJobTitlesDeftech(doc)
	return jobInfos, nil
}

// findJobTitlesDeftech finds job titles in the DefTech HTML document
func findJobTitlesDeftech(n *html.Node) []JobInfo {
	var allLinks []struct {
		text string
		url  string
	}

	// First pass: collect all relevant links in document order
	collectLinks(n, &allLinks)

	// Second pass: pair job titles with companies
	var jobInfos []JobInfo
	for i := 0; i < len(allLinks); i++ {
		link := allLinks[i]
		if strings.Contains(link.url, "/jobs/companies/") && strings.Contains(link.url, "/vacancies/") {
			// Check if it's a job link (has a number after /vacancies/)
			parts := strings.Split(link.url, "/vacancies/")
			if len(parts) > 1 && len(parts[1]) > 0 && (parts[1][0] >= '0' && parts[1][0] <= '9') {
				jobInfo := JobInfo{Title: link.text, URL: link.url}

				// Look for the next company link
				for j := i + 1; j < len(allLinks) && j < i+3; j++ { // Look up to 2 links ahead
					nextLink := allLinks[j]
					if strings.Contains(nextLink.url, "jobs.dou.ua/companies/") && strings.Contains(nextLink.url, "/vacancies/") {
						parts := strings.Split(nextLink.url, "/vacancies/")
						if len(parts) > 1 && (parts[1] == "" || strings.HasPrefix(parts[1], "?")) {
							jobInfo.Company = nextLink.text
							break
						}
					}
				}

				jobInfos = append(jobInfos, jobInfo)
			}
		}
	}

	return jobInfos
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
	if !b.isAdmin(c.Sender().ID) {
		log.Printf("Unauthorized text message from user %s (ID: %d)", c.Sender().Username, c.Sender().ID)
		return c.Send("Sorry, you are not authorized to use this bot.")
	}

	log.Printf("Text message received from user %s: %s", c.Sender().Username, c.Text())
	// Echo the message back
	return c.Send("You said: " + c.Text())
}

// handleCallback handles inline button callbacks
func (b *Bot) handleCallback(c telebot.Context) error {
	if !b.isAdmin(c.Sender().ID) {
		log.Printf("Unauthorized callback from user %s (ID: %d)", c.Sender().Username, c.Sender().ID)
		return c.Respond(&telebot.CallbackResponse{Text: "Unauthorized"})
	}

	log.Printf("Callback received from user %s: %s", c.Sender().Username, c.Callback().Data)

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
