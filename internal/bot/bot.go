package bot

import (
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html"
	telebot "gopkg.in/telebot.v3"
)

// Bot represents the Telegram bot instance
type Bot struct {
	telebot *telebot.Bot
}

// New creates a new bot instance
func New(token string) (*Bot, error) {
	pref := telebot.Settings{
		Token:  token,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
	}

	b, err := telebot.NewBot(pref)
	if err != nil {
		return nil, err
	}

	bot := &Bot{
		telebot: b,
	}

	// Register handlers
	bot.registerHandlers()

	return bot, nil
}

// registerHandlers registers all bot command and message handlers
func (b *Bot) registerHandlers() {
	// Start command handler
	b.telebot.Handle("/start", b.handleStart)

	// Help command handler
	b.telebot.Handle("/help", b.handleHelp)

	// Get list command handler
	b.telebot.Handle("/list", b.handleGetList)

	// Get list DOU command handler
	b.telebot.Handle("/list_dou", b.handleGetListDOU)

	// Default message handler
	b.telebot.Handle(telebot.OnText, b.handleText)
}

// handleStart handles the /start command
func (b *Bot) handleStart(c telebot.Context) error {
	return c.Send("Hello! Welcome to the bot. Use /help to see available commands.")
}

// handleHelp handles the /help command
func (b *Bot) handleHelp(c telebot.Context) error {
	helpText := "Available commands:\n" +
		"/start - Start the bot\n" +
		"/help - Show this help message\n" +
		"/list - Get list from PeopleForce\n" +
		"/list_dou - Get list from DOU.ua"
	return c.Send(helpText)
}

// handleGetList handles the /list command
func (b *Bot) handleGetList(c telebot.Context) error {
	// Show loading message
	c.Send("Fetching job listings...")

	// Fetch job titles from the careers page
	jobTitles, err := b.fetchJobTitles()
	if err != nil {
		log.Printf("Error fetching job titles: %v", err)
		return c.Send(fmt.Sprintf("Error fetching job listings: %v", err))
	}

	if len(jobTitles) == 0 {
		return c.Send("No job listings found.")
	}

	// Format and send the list
	message := "📋 *Available Job Positions:*\n\n"
	for i, title := range jobTitles {
		message += fmt.Sprintf("%d. %s\n", i+1, title)
	}

	return c.Send(message, telebot.ModeMarkdown)
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

		// Add unique job titles
		for _, title := range jobTitles {
			if !seen[title] {
				seen[title] = true
				allJobTitles = append(allJobTitles, title)
			}
		}
	}

	return allJobTitles, nil
}

// fetchJobTitlesFromPage fetches and parses job titles from a specific page
func (b *Bot) fetchJobTitlesFromPage(page int) ([]string, error) {
	url := fmt.Sprintf("https://dwarfengineering.peopleforce.io/careers?page=%d", page)
	if page == 1 {
		url = "https://dwarfengineering.peopleforce.io/careers"
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Fetch the page
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch page %d: %w", page, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code for page %d: %d", page, resp.StatusCode)
	}

	// Parse HTML
	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML for page %d: %w", page, err)
	}

	// Extract job titles
	var jobTitles []string
	b.extractJobTitles(doc, &jobTitles)

	return jobTitles, nil
}

// extractJobTitles recursively extracts job titles from HTML nodes
func (b *Bot) extractJobTitles(n *html.Node, jobTitles *[]string) {
	if n.Type == html.ElementNode {
		// Look for <a> tags with class "stretched-link tw-text-black"
		if n.Data == "a" {
			// Check if this link has the required class
			hasRequiredClass := false
			for _, attr := range n.Attr {
				if attr.Key == "class" {
					classes := strings.Fields(attr.Val)
					hasStretchedLink := false
					hasTwTextBlack := false
					for _, class := range classes {
						if class == "stretched-link" {
							hasStretchedLink = true
						}
						if class == "tw-text-black" {
							hasTwTextBlack = true
						}
					}
					if hasStretchedLink && hasTwTextBlack {
						hasRequiredClass = true
						break
					}
				}
			}

			if hasRequiredClass {
				// Extract text content
				text := b.extractText(n)
				text = strings.TrimSpace(text)

				// Only add if it's not empty
				if text != "" {
					// Check if we haven't already added this title
					found := false
					for _, existing := range *jobTitles {
						if existing == text {
							found = true
							break
						}
					}
					if !found {
						*jobTitles = append(*jobTitles, text)
					}
				}
			}
		}
	}

	// Recursively process child nodes
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.extractJobTitles(c, jobTitles)
	}
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

// handleGetListDOU handles the /list_dou command
func (b *Bot) handleGetListDOU(c telebot.Context) error {
	// Show loading message
	c.Send("Fetching job listings from DOU.ua...")

	// Fetch job titles from the DOU.ua page
	jobTitles, err := b.fetchJobTitlesFromDOU()
	if err != nil {
		log.Printf("Error fetching job titles from DOU: %v", err)
		return c.Send(fmt.Sprintf("Error fetching job listings: %v", err))
	}

	if len(jobTitles) == 0 {
		return c.Send("No job listings found.")
	}

	// Format and send the list
	message := "📋 *Available Job Positions (DOU.ua):*\n\n"
	for i, title := range jobTitles {
		message += fmt.Sprintf("%d. %s\n", i+1, title)
	}

	return c.Send(message, telebot.ModeMarkdown)
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

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Fetch the RSS feed
	resp, err := client.Get(url)
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
			}
		}
	}

	return jobTitles, nil
}

// handleText handles text messages
func (b *Bot) handleText(c telebot.Context) error {
	// Echo the message back
	return c.Send("You said: " + c.Text())
}

// Start starts the bot
func (b *Bot) Start() {
	log.Println("Bot started successfully")
	b.telebot.Start()
}

// Stop stops the bot gracefully
func (b *Bot) Stop() {
	log.Println("Bot stopped")
	b.telebot.Stop()
}
