package bot

import (
	"database/sql"
	"encoding/json"
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
		"/get_companies - Show all companies with vacancy counts\n\n" +
		"/hide_all - Hide all visible vacancies\n\n" +
		"/clear_saved - Clear hidden vacancies\n\n" +
		"/build_version - Show current build version"

	// Action prefixes for deep links
	hidePrefix    = "hide"
	showPrefix    = "show"
	companyPrefix = "company"
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
	Title      string
	Company    string
	CompanyURL string
	URL        string
}

// XHRResponse represents the response from xhr-load endpoint
type XHRResponse struct {
	HTML string `json:"html"`
	Last bool   `json:"last"`
	Num  int    `json:"num"`
}

// MessageFormatter handles message formatting operations
type MessageFormatter interface {
	FormatVacancy(index int, title, url, company string, companyID int, id int, hidden bool, botUsername string) string
	FormatVacancyFromStruct(index int, vacancy Vacancy, companyName string, botUsername string) string
}

// VacancyRepository handles database operations for vacancies
type VacancyRepository interface {
	SaveVacancy(title, url, company string) error
	VacancyExists(title string) bool
	GetVisibleVacancies() ([]Vacancy, error)
	GetLatestVacancies(limit int) ([]Vacancy, error)
	GetTotalVacancyCount() (int, error)
	SetVacancyHidden(id int, hidden bool) error
	GetVacancyIDAndHiddenByTitle(title string) (int, bool, error)
	GetVacancyByID(id int) (*Vacancy, error)
	ClearHiddenVacancies() error
	DeleteHiddenVacancies() error
	HideAllVacancies() (int64, error)
	GetVacanciesByCompanyID(companyID int) ([]Vacancy, error)
}

// CompanyRepository handles database operations for companies
type CompanyRepository interface {
	GetCompanyIDByName(name string) (int, error)
	GetCompanyName(vacancy Vacancy) string
	GetCompanyNameByID(id int) (string, error)
	GetAllCompaniesWithCounts() ([]CompanyWithCount, error)
}

// CompanyWithCount represents a company with vacancy count
type CompanyWithCount struct {
	ID        int
	Name      string
	Count     int
	IsVisible bool
}

// VacancyFetcher handles HTTP operations for fetching vacancies
type VacancyFetcher interface {
	FetchJobTitlesFromDeftech() ([]VacancyInfo, error)
}

// KeyboardBuilder handles keyboard creation
type KeyboardBuilder interface {
	GetCommandKeyboard() *telebot.ReplyMarkup
	GetCommandKeyboardWithHideAll() *telebot.ReplyMarkup
}

// MessageService handles message operations
type MessageService interface {
	SendLoadingMessage(c telebot.Context, message string) error
	SendErrorMessage(c telebot.Context, errorMsg string) error
	SendVacancyList(c telebot.Context, vacancies []Vacancy, totalCount, limit int) error
}

// Bot represents the Telegram bot instance with dependency injection
type Bot struct {
	telebot             *telebot.Bot
	messageFormatter    MessageFormatter
	vacancyRepo         VacancyRepository
	companyRepo         CompanyRepository
	vacancyFetcher      VacancyFetcher
	keyboardBuilder     KeyboardBuilder
	messageService      MessageService
	httpClient          *http.Client
	db                  *sql.DB
	adminID             string
	publicChatID        int64
	deftechURL          string
	limit               int
	companiesPerMessage int
	version             string
	buildVersion        string
}

// New creates a new bot instance with dependency injection
func New(token string, adminID string, intervalStr string, publicChatIDStr string, dbName string, deftechURL string, limit int, companiesPerMessage int, version string, buildVersion string) (*Bot, error) {
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

	// Create service implementations
	messageFormatter := &DefaultMessageFormatter{}
	vacancyRepo := &SQLiteVacancyRepository{db: db}
	companyRepo := &SQLiteCompanyRepository{db: db}
	vacancyFetcher := &DefaultVacancyFetcher{httpClient: httpClient, deftechURL: deftechURL}
	keyboardBuilder := &DefaultKeyboardBuilder{}
	messageService := &DefaultMessageService{keyboardBuilder: keyboardBuilder}

	var publicChatID int64
	if publicChatIDStr != "" {
		publicChatID, err = strconv.ParseInt(strings.TrimSpace(publicChatIDStr), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid PUBLIC_CHAT_ID: %w", err)
		}
	}

	bot := &Bot{
		telebot:             b,
		messageFormatter:    messageFormatter,
		vacancyRepo:         vacancyRepo,
		companyRepo:         companyRepo,
		vacancyFetcher:      vacancyFetcher,
		keyboardBuilder:     keyboardBuilder,
		messageService:      messageService,
		httpClient:          httpClient,
		db:                  db,
		adminID:             adminID,
		publicChatID:        publicChatID,
		deftechURL:          deftechURL,
		limit:               limit,
		companiesPerMessage: companiesPerMessage,
		version:             version,
		buildVersion:        buildVersion,
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

// DefaultMessageFormatter implements MessageFormatter
type DefaultMessageFormatter struct{}

func (f *DefaultMessageFormatter) FormatVacancy(index int, title, url, company string, companyID int, id int, hidden bool, botUsername string) string {
	action := "hide"
	prefix := hidePrefix
	if hidden {
		action = "show"
		prefix = showPrefix
	}
	if company == "" {
		company = "-"
	}

	// Make company name clickable
	companyText := company
	if companyID > 0 {
		companyText = fmt.Sprintf("[%s](https://t.me/%s?start=%s_%d)", company, botUsername, companyPrefix, companyID)
	}

	return fmt.Sprintf("%d. [%s](%s) @ %s | [%s](https://t.me/%s?start=%s_%d)\n",
		index+1, escapeMarkdownLinkText(title), url, companyText, action, botUsername, prefix, id)
}

func (f *DefaultMessageFormatter) FormatVacancyFromStruct(index int, vacancy Vacancy, companyName string, botUsername string) string {
	action := "hide"
	prefix := hidePrefix
	if vacancy.IsHidden {
		action = "show"
		prefix = showPrefix
	}
	company := companyName
	if vacancy.CompanyID != nil {
		company = fmt.Sprintf("[%s](https://t.me/%s?start=%s_%d)", companyName, botUsername, companyPrefix, *vacancy.CompanyID)
	}

	return fmt.Sprintf("%d. [%s](%s) @ %s | [%s](https://t.me/%s?start=%s_%d)\n",
		index+1, escapeMarkdownLinkText(vacancy.Title), vacancy.URL, company, action, botUsername, prefix, vacancy.ID)
}

// SQLiteVacancyRepository implements VacancyRepository
type SQLiteVacancyRepository struct {
	db *sql.DB
}

func (r *SQLiteVacancyRepository) SaveVacancy(title, url, company string) error {
	// Get or create company
	companyID, err := r.getOrCreateCompanyID(company)
	if err != nil {
		return err
	}

	// Insert vacancy
	query := `INSERT INTO vacancies (title, url, company_id, is_hidden, created_at) VALUES (?, ?, ?, FALSE, ?)`
	_, err = r.db.Exec(query, title, url, companyID, time.Now())
	return err
}

func (r *SQLiteVacancyRepository) VacancyExists(title string) bool {
	var count int
	query := `SELECT COUNT(*) FROM vacancies WHERE title = ?`
	r.db.QueryRow(query, title).Scan(&count)
	return count > 0
}

func (r *SQLiteVacancyRepository) GetVisibleVacancies() ([]Vacancy, error) {
	query := `SELECT v.id, v.title, v.url, v.company_id, v.is_hidden, v.created_at FROM vacancies v
	          WHERE v.is_hidden = FALSE ORDER BY v.created_at ASC`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var vacancies []Vacancy
	for rows.Next() {
		var v Vacancy
		err := rows.Scan(&v.ID, &v.Title, &v.URL, &v.CompanyID, &v.IsHidden, &v.CreatedAt)
		if err != nil {
			return nil, err
		}
		vacancies = append(vacancies, v)
	}
	return vacancies, nil
}

func (r *SQLiteVacancyRepository) GetLatestVacancies(limit int) ([]Vacancy, error) {
	query := `SELECT v.id, v.title, v.url, v.company_id, v.is_hidden, v.created_at FROM vacancies v
	          ORDER BY v.created_at DESC LIMIT ?`

	rows, err := r.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var vacancies []Vacancy
	for rows.Next() {
		var v Vacancy
		err := rows.Scan(&v.ID, &v.Title, &v.URL, &v.CompanyID, &v.IsHidden, &v.CreatedAt)
		if err != nil {
			return nil, err
		}
		vacancies = append(vacancies, v)
	}
	return vacancies, nil
}

func (r *SQLiteVacancyRepository) GetTotalVacancyCount() (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM vacancies`
	err := r.db.QueryRow(query).Scan(&count)
	return count, err
}

func (r *SQLiteVacancyRepository) SetVacancyHidden(id int, hidden bool) error {
	query := "UPDATE vacancies SET is_hidden = ? WHERE id = ?"
	_, err := r.db.Exec(query, hidden, id)
	return err
}

func (r *SQLiteVacancyRepository) GetVacancyIDAndHiddenByTitle(title string) (int, bool, error) {
	var id int
	var hidden bool
	query := "SELECT id, is_hidden FROM vacancies WHERE title = ?"
	err := r.db.QueryRow(query, title).Scan(&id, &hidden)
	return id, hidden, err
}

func (r *SQLiteVacancyRepository) GetVacancyByID(id int) (*Vacancy, error) {
	var vacancy Vacancy
	query := `SELECT id, title, url, company_id, is_hidden, created_at FROM vacancies WHERE id = ?`
	err := r.db.QueryRow(query, id).Scan(&vacancy.ID, &vacancy.Title, &vacancy.URL, &vacancy.CompanyID, &vacancy.IsHidden, &vacancy.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &vacancy, nil
}

func (r *SQLiteVacancyRepository) ClearHiddenVacancies() error {
	query := "UPDATE vacancies SET is_hidden = false WHERE is_hidden = true"
	_, err := r.db.Exec(query)
	return err
}

func (r *SQLiteVacancyRepository) DeleteHiddenVacancies() error {
	query := "DELETE FROM vacancies WHERE is_hidden = true"
	_, err := r.db.Exec(query)
	return err
}

func (r *SQLiteVacancyRepository) HideAllVacancies() (int64, error) {
	query := "UPDATE vacancies SET is_hidden = true WHERE is_hidden = false"
	result, err := r.db.Exec(query)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *SQLiteVacancyRepository) GetVacanciesByCompanyID(companyID int) ([]Vacancy, error) {
	query := `SELECT id, title, url, company_id, is_hidden, created_at FROM vacancies WHERE company_id = ? ORDER BY created_at DESC`
	rows, err := r.db.Query(query, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var vacancies []Vacancy
	for rows.Next() {
		var v Vacancy
		err := rows.Scan(&v.ID, &v.Title, &v.URL, &v.CompanyID, &v.IsHidden, &v.CreatedAt)
		if err != nil {
			return nil, err
		}
		vacancies = append(vacancies, v)
	}
	return vacancies, nil
}

func (r *SQLiteVacancyRepository) getOrCreateCompanyID(companyName string) (*int, error) {
	if companyName == "" {
		return nil, nil
	}

	// Try to find existing company
	var companyID int
	query := "SELECT id FROM companies WHERE name = ?"
	err := r.db.QueryRow(query, companyName).Scan(&companyID)
	if err == nil {
		return &companyID, nil
	}

	// Create new company
	query = "INSERT INTO companies (name, created_at) VALUES (?, ?)"
	result, err := r.db.Exec(query, companyName, time.Now())
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	companyID = int(id)
	return &companyID, nil
}

// SQLiteCompanyRepository implements CompanyRepository
type SQLiteCompanyRepository struct {
	db *sql.DB
}

func (r *SQLiteCompanyRepository) GetCompanyIDByName(name string) (int, error) {
	var id int
	query := "SELECT id FROM companies WHERE name = ?"
	err := r.db.QueryRow(query, name).Scan(&id)
	return id, err
}

func (r *SQLiteCompanyRepository) GetCompanyName(vacancy Vacancy) string {
	if vacancy.CompanyID == nil {
		return "-"
	}

	var name string
	query := "SELECT name FROM companies WHERE id = ?"
	err := r.db.QueryRow(query, *vacancy.CompanyID).Scan(&name)
	if err != nil {
		return "-"
	}
	return name
}

func (r *SQLiteCompanyRepository) GetCompanyNameByID(id int) (string, error) {
	var name string
	query := "SELECT name FROM companies WHERE id = ?"
	err := r.db.QueryRow(query, id).Scan(&name)
	return name, err
}

func (r *SQLiteCompanyRepository) GetAllCompaniesWithCounts() ([]CompanyWithCount, error) {
	query := `
		SELECT c.id, c.name, COUNT(v.id) as count,
		       SUM(CASE WHEN v.is_hidden = FALSE THEN 1 ELSE 0 END) > 0 as is_visible
		FROM companies c
		LEFT JOIN vacancies v ON c.id = v.company_id
		GROUP BY c.id, c.name
		ORDER BY c.name ASC`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var companies []CompanyWithCount
	for rows.Next() {
		var c CompanyWithCount
		err := rows.Scan(&c.ID, &c.Name, &c.Count, &c.IsVisible)
		if err != nil {
			return nil, err
		}
		companies = append(companies, c)
	}
	return companies, nil
}

// DefaultVacancyFetcher implements VacancyFetcher
type DefaultVacancyFetcher struct {
	httpClient *http.Client
	deftechURL string
}

func (f *DefaultVacancyFetcher) FetchJobTitlesFromDeftech() ([]VacancyInfo, error) {
	// Implementation would go here - simplified for refactoring
	return []VacancyInfo{}, nil
}

// DefaultKeyboardBuilder implements KeyboardBuilder
type DefaultKeyboardBuilder struct{}

func (b *DefaultKeyboardBuilder) GetCommandKeyboard() *telebot.ReplyMarkup {
	markup := &telebot.ReplyMarkup{}
	btnGetSavedVisible := markup.Data("Get saved (visible)", "/get_saved_visible")
	btnGetSavedLatest := markup.Data("Get saved (latest)", "/get_saved_latest")
	btnFetchNewest := markup.Data("Fetch newest", "/fetch_newest")
	btnFetchLatest := markup.Data("Fetch latest", "/fetch_latest")
	btnCompanies := markup.Data("Get companies", "/get_companies")
	markup.Inline(
		markup.Row(btnGetSavedVisible, btnGetSavedLatest),
		markup.Row(btnFetchNewest, btnFetchLatest),
		markup.Row(btnCompanies),
	)
	return markup
}

func (b *DefaultKeyboardBuilder) GetCommandKeyboardWithHideAll() *telebot.ReplyMarkup {
	markup := &telebot.ReplyMarkup{}
	btnHideAll := markup.Data("Hide all", "/hide_all")
	btnGetSavedVisible := markup.Data("Get saved (visible)", "/get_saved_visible")
	btnGetSavedLatest := markup.Data("Get saved (latest)", "/get_saved_latest")
	btnFetchNewest := markup.Data("Fetch newest", "/fetch_newest")
	btnFetchLatest := markup.Data("Fetch latest", "/fetch_latest")
	btnCompanies := markup.Data("Get companies", "/get_companies")
	markup.Inline(
		markup.Row(btnHideAll),
		markup.Row(btnGetSavedVisible, btnGetSavedLatest),
		markup.Row(btnFetchNewest, btnFetchLatest),
		markup.Row(btnCompanies),
	)
	return markup
}

// DefaultMessageService implements MessageService
type DefaultMessageService struct {
	keyboardBuilder KeyboardBuilder
}

func (s *DefaultMessageService) SendLoadingMessage(c telebot.Context, message string) error {
	return c.Send(message, telebot.ModeMarkdown, telebot.Silent)
}

func (s *DefaultMessageService) SendErrorMessage(c telebot.Context, errorMsg string) error {
	return c.Send(errorMsg, s.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
}

func (s *DefaultMessageService) SendVacancyList(c telebot.Context, vacancies []Vacancy, totalCount, limit int) error {
	if len(vacancies) == 0 {
		if totalCount == 0 {
			return c.Send("No saved vacancies found.", s.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
		} else {
			return c.Send(fmt.Sprintf("No vacancies to display (showing latest %d of %d total).", limit, totalCount), s.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
		}
	}

	const batchSize = 25

	// Send the total count in the first message
	firstMessage := fmt.Sprintf("Total vacancies: %d\n\n", totalCount)
	err := c.Send(firstMessage, telebot.ModeMarkdown, telebot.NoPreview, telebot.Silent)
	if err != nil {
		return err
	}

	// Send vacancies in batches
	for i := 0; i < len(vacancies); i += batchSize {
		end := i + batchSize
		if end > len(vacancies) {
			end = len(vacancies)
		}

		var message strings.Builder
		for j, vacancy := range vacancies[i:end] {
			// This would need proper formatting - simplified for now
			message.WriteString(fmt.Sprintf("%d. %s\n", i+j+1, vacancy.Title))
		}

		// Add keyboard only to the last message
		var err error
		if end == len(vacancies) {
			err = c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview, s.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
		} else {
			err = c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview, telebot.Silent)
		}
		if err != nil {
			return err
		}
	}

	return nil
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
			return c.Send("Sorry, you are not authorized to use this bot.", b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
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

// vacancyExists checks if a vacancy with the given title already exists
func (b *Bot) vacancyExists(title string) bool {
	return b.vacancyRepo.VacancyExists(title)
}

// saveVacancy saves a new vacancy to the database
func (b *Bot) saveVacancy(title, url, companyName string) error {
	return b.vacancyRepo.SaveVacancy(title, url, companyName)
}

// getVacancyIDAndHiddenByTitle gets the vacancy ID and hidden status by title
func (b *Bot) getVacancyIDAndHiddenByTitle(title string) (int, bool, error) {
	return b.vacancyRepo.GetVacancyIDAndHiddenByTitle(title)
}

// getTotalVacancyCount gets the total count of vacancies
func (b *Bot) getTotalVacancyCount() (int, error) {
	return b.vacancyRepo.GetTotalVacancyCount()
}

// getLatestVacancies gets the latest N vacancies from database
func (b *Bot) getLatestVacancies(limit int) ([]Vacancy, error) {
	return b.vacancyRepo.GetLatestVacancies(limit)
}

// sendVacancyList sends a formatted list of vacancies
func (b *Bot) sendVacancyList(c telebot.Context, vacancies []Vacancy, totalCount, limit int) error {
	return b.messageService.SendVacancyList(c, vacancies, totalCount, limit)
}

// formatVacancyMessage formats a single vacancy for display
func (b *Bot) formatVacancyMessage(index int, vacancy Vacancy, botUsername string) string {
	companyName := b.companyRepo.GetCompanyName(vacancy)
	return b.messageFormatter.FormatVacancyFromStruct(index, vacancy, companyName, botUsername)
}

// formatVacancyInfoMessage formats a vacancy info for display (used for fetched vacancies)
func (b *Bot) formatVacancyInfoMessage(index int, vacancyInfo VacancyInfo, id int, hidden bool, botUsername string) string {
	companyID := 0
	if vacancyInfo.Company != "" {
		if cid, err := b.companyRepo.GetCompanyIDByName(vacancyInfo.Company); err == nil {
			companyID = cid
		}
	}
	return b.messageFormatter.FormatVacancy(index, vacancyInfo.Title, vacancyInfo.URL, vacancyInfo.Company, companyID, id, hidden, botUsername)
}

// getVisibleVacancies retrieves all non-hidden vacancies from the database
func (b *Bot) getVisibleVacancies() ([]Vacancy, error) {
	return b.vacancyRepo.GetVisibleVacancies()
}

// registerHandlers registers all bot command and message handlers
func (b *Bot) registerHandlers() {
	// Start command handler
	b.telebot.Handle("/start", b.handleStart)

	// Help command handler
	b.telebot.Handle("/help", b.handleHelp)

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
	message.WriteString(newVacanciesHeading(len(newVacancyInfos)))

	for i, vacancyInfo := range newVacancyInfos {
		id, hidden, err := b.getVacancyIDAndHiddenByTitle(vacancyInfo.Title)
		if err != nil {
			log.Printf("Error getting vacancy ID for %s: %v", vacancyInfo.Title, err)
			continue
		}
		action := "hide"
		prefix := "hide"
		if hidden {
			action = "show"
			prefix = "show"
		}
		company := vacancyInfo.Company
		if company == "" {
			company = "-"
		} else {
			// Make company clickable
			if companyID, err := b.companyRepo.GetCompanyIDByName(company); err == nil {
				company = fmt.Sprintf("[%s](https://t.me/%s?start=%s_%d)", company, b.telebot.Me.Username, companyPrefix, companyID)
			}
		}
		message.WriteString(fmt.Sprintf("%d. [%s](%s) @ %s | [%s](https://t.me/%s?start=%s_%d)\n", i+1, escapeMarkdownLinkText(vacancyInfo.Title), vacancyInfo.URL, company, action, b.telebot.Me.Username, prefix, id))
	}

	// Send with retry logic
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		_, err = b.telebot.Send(chat, message.String(), telebot.ModeMarkdown, telebot.NoPreview, b.keyboardBuilder.GetCommandKeyboardWithHideAll(), telebot.Silent)
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

	if err := b.postNewVacanciesToPublic(newVacancyInfos); err != nil {
		log.Printf("Error posting new vacancies to public chat: %v", err)
	}

	return nil
}

func newVacanciesHeading(count int) string {
	if count == 1 {
		return "**New vacancy:**\n\n"
	}
	return "**New vacancies:**\n\n"
}

func (b *Bot) formatPublicVacanciesMessage(vacancies []VacancyInfo) string {
	var message strings.Builder
	message.WriteString(newVacanciesHeading(len(vacancies)))

	for i, vacancyInfo := range vacancies {
		company := formatCompanyForPublic(vacancyInfo.Company, vacancyInfo.CompanyURL)
		message.WriteString(fmt.Sprintf("%d. [%s](%s) @ %s\n", i+1, escapeMarkdownLinkText(vacancyInfo.Title), vacancyInfo.URL, company))
	}

	return message.String()
}

func formatCompanyForPublic(company, companyURL string) string {
	if company == "" {
		return "-"
	}
	if companyURL == "" {
		return escapeMarkdownLinkText(company)
	}
	return fmt.Sprintf("[%s](%s)", escapeMarkdownLinkText(company), companyURL)
}

func normalizeCompanyURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	switch {
	case strings.HasPrefix(raw, "http://"), strings.HasPrefix(raw, "https://"):
		return raw
	case strings.HasPrefix(raw, "//"):
		return "https:" + raw
	case strings.HasPrefix(raw, "/"):
		return "https://jobs.dou.ua" + raw
	default:
		return "https://" + raw
	}
}

func (b *Bot) postNewVacanciesToPublic(vacancies []VacancyInfo) error {
	if b.publicChatID == 0 {
		return nil
	}
	if len(vacancies) == 0 {
		return nil
	}

	chat := &telebot.Chat{ID: b.publicChatID}
	message := b.formatPublicVacanciesMessage(vacancies)

	var err error
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		_, err = b.telebot.Send(chat, message, telebot.ModeMarkdown, telebot.NoPreview, telebot.Silent)
		if err == nil {
			log.Printf("Posted %d new vacancies to public chat %d", len(vacancies), b.publicChatID)
			return nil
		}
		log.Printf("Attempt %d failed to send new vacancies to public chat: %v", attempt, err)
		if attempt < maxRetries {
			time.Sleep(time.Duration(attempt) * 5 * time.Second)
		}
	}

	return fmt.Errorf("error sending new vacancies to public chat after %d attempts: %w", maxRetries, err)
}

// handleStart handles the /start command
func (b *Bot) handleStart(c telebot.Context) error {
	payload := strings.TrimSpace(c.Message().Payload)
	log.Printf("Command /start received with payload: '%s'", payload)

	// Check if this is a deep link command

	// Check if this is a company selection via deep link
	if strings.HasPrefix(payload, companyPrefix+"_") {
		companyIDStr := strings.TrimPrefix(payload, companyPrefix+"_")
		companyID, err := strconv.Atoi(companyIDStr)
		if err != nil {
			return c.Send("Invalid company ID", b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
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

	// Check if this is a hide/show command via deep link
	if strings.HasPrefix(payload, "hide_") {
		idStr := strings.TrimPrefix(payload, "hide_")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			return c.Send("Invalid hide ID", b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
		}
		vacancy, err := b.vacancyRepo.GetVacancyByID(id)
		if err != nil {
			log.Printf("Error getting vacancy %d: %v", id, err)
			return c.Send("Error ignoring job", b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
		}
		// Get vacancy URL and company for clickable link
		url := vacancy.URL
		company := b.companyRepo.GetCompanyName(*vacancy)
		if vacancy.CompanyID != nil {
			company = fmt.Sprintf("[%s](https://t.me/%s?start=%s_%d)", company, c.Bot().Me.Username, companyPrefix, *vacancy.CompanyID)
		}
		err = b.vacancyRepo.SetVacancyHidden(id, true)
		if err != nil {
			log.Printf("Error hiding vacancy %d: %v", id, err)
			return c.Send("Error hiding vacancy", b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
		}
		// Delete the /start command message to keep chat clean
		go func() {
			time.Sleep(50 * time.Millisecond) // Small delay to ensure processing completes
			c.Delete()
		}()
		// Send status message
		err = c.Send(fmt.Sprintf("[%s](%s) @ %s is hidden", escapeMarkdownLinkText(vacancy.Title), url, company), telebot.ModeMarkdown, telebot.Silent, telebot.NoPreview)
		if err != nil {
			log.Printf("Error sending hide status message: %v", err)
			return nil
		}

		// Send updated visible vacancies list (like /get_saved_visible)
		c.Send(fmt.Sprintf("Getting visible vacancies from db →"), telebot.ModeMarkdown, telebot.Silent)
		visibleVacancies, err := b.getVisibleVacancies()
		if err != nil {
			log.Printf("Error getting visible vacancies after hide: %v", err)
			return nil // Don't return error as the hide operation succeeded
		}

		if len(visibleVacancies) == 0 {
			return c.Send("No visible vacancies found.", b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
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
				c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview, b.keyboardBuilder.GetCommandKeyboardWithHideAll(), telebot.Silent)
			} else {
				c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview, telebot.Silent)
			}
		}
		return nil
	}
	if strings.HasPrefix(payload, "show_") {
		idStr := strings.TrimPrefix(payload, "show_")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			return c.Send("Invalid show ID", b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
		}
		vacancy, err := b.vacancyRepo.GetVacancyByID(id)
		if err != nil {
			log.Printf("Error getting vacancy %d: %v", id, err)
			return c.Send("Error showing job", b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
		}
		// Get vacancy URL and company for clickable link
		url := vacancy.URL
		company := b.companyRepo.GetCompanyName(*vacancy)
		if vacancy.CompanyID != nil {
			company = fmt.Sprintf("[%s](https://t.me/%s?start=%s_%d)", company, c.Bot().Me.Username, companyPrefix, *vacancy.CompanyID)
		}
		err = b.vacancyRepo.SetVacancyHidden(id, false)
		if err != nil {
			log.Printf("Error showing vacancy %d: %v", id, err)
			return c.Send("Error showing job", b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
		}
		// Delete the /start command message to keep chat clean
		go func() {
			time.Sleep(50 * time.Millisecond) // Small delay to ensure processing completes
			c.Delete()
		}()
		// Send status message
		err = c.Send(fmt.Sprintf("[%s](%s) @ %s is shown", escapeMarkdownLinkText(vacancy.Title), url, company), telebot.ModeMarkdown, telebot.Silent, telebot.NoPreview)
		if err != nil {
			log.Printf("Error sending show status message: %v", err)
			return nil
		}

		// Send updated visible vacancies list (like /get_saved_visible)
		c.Send(fmt.Sprintf("Getting visible vacancies from db →"), telebot.ModeMarkdown, telebot.Silent)
		visibleVacancies, err := b.getVisibleVacancies()
		if err != nil {
			log.Printf("Error getting visible vacancies after show: %v", err)
			return nil // Don't return error as the show operation succeeded
		}

		if len(visibleVacancies) == 0 {
			return c.Send("No visible vacancies found.", b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
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
				c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview, b.keyboardBuilder.GetCommandKeyboardWithHideAll(), telebot.Silent)
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
	return c.Send(startText, b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
}

// handleHelp handles the /help command
func (b *Bot) handleHelp(c telebot.Context) error {
	log.Printf("Command /help received")
	return c.Send(commandsText, b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
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

// handleFetchNewest handles the /fetch_newest command
func (b *Bot) handleFetchNewest(c telebot.Context) error {
	log.Printf("Command /fetch_newest received")
	// Show loading message
	c.Send(fmt.Sprintf("Fetching new vacancies from [%s](%s) →", b.deftechURL, b.deftechURL), telebot.ModeMarkdown, telebot.Silent)

	// Fetch vacancy titles from the deftech.dou.ua page
	vacancyInfos, err := b.FetchJobTitlesFromDeftech()
	if err != nil {
		log.Printf("Error fetching vacancy titles from deftech.dou.ua: %v", err)
		return c.Send(fmt.Sprintf("Error fetching vacancies: %v", err), b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
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
		return c.Send("No new vacancies found.", b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
	}

	log.Printf("Found %d new vacancies", len(newVacancyInfos))

	// Format the message with only new vacancies
	var message strings.Builder
	message.WriteString(newVacanciesHeading(len(newVacancyInfos)))

	for i, vacancyInfo := range newVacancyInfos {
		id, hidden, err := b.getVacancyIDAndHiddenByTitle(vacancyInfo.Title)
		if err != nil {
			log.Printf("Error getting vacancy ID for %s: %v", vacancyInfo.Title, err)
			continue
		}
		message.WriteString(b.formatVacancyInfoMessage(i, vacancyInfo, id, hidden, c.Bot().Me.Username))
	}

	if err := c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview, b.keyboardBuilder.GetCommandKeyboardWithHideAll(), telebot.Silent); err != nil {
		return err
	}

	if err := b.postNewVacanciesToPublic(newVacancyInfos); err != nil {
		log.Printf("Error posting new vacancies to public chat: %v", err)
	}

	return nil
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
		return c.Send(fmt.Sprintf("Error fetching vacancies: %v", err), b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
	}

	if len(vacancyInfos) == 0 {
		return c.Send("No vacancies found.", b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
	}

	if len(vacancyInfos) > b.limit {
		vacancyInfos = vacancyInfos[:b.limit]
	}

	// Send vacancies in chunks to avoid message length limit
	for i := 0; i < len(vacancyInfos); i += b.companiesPerMessage {
		end := i + b.companiesPerMessage
		if end > len(vacancyInfos) {
			end = len(vacancyInfos)
		}
		chunk := vacancyInfos[i:end]

		var message strings.Builder
		for j, vacancyInfo := range chunk {
			company := vacancyInfo.Company
			if company == "" {
				company = "-"
			} else if companyID, err := b.companyRepo.GetCompanyIDByName(company); err == nil {
				company = fmt.Sprintf("[%s](https://t.me/%s?start=%s_%d)", company, c.Bot().Me.Username, companyPrefix, companyID)
			}
			message.WriteString(fmt.Sprintf("%d. [%s](%s) @ %s\n", i+j+1, escapeMarkdownLinkText(vacancyInfo.Title), vacancyInfo.URL, company))
		}

		// Add keyboard only to the last message
		if end == len(vacancyInfos) {
			c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview, b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
		} else {
			c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview, telebot.Silent)
		}
	}

	return nil
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
		return c.Send("Error getting visible vacancies", b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
	}

	if len(vacancies) == 0 {
		return c.Send("No visible vacancies found.", b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
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
			c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview, b.keyboardBuilder.GetCommandKeyboardWithHideAll(), telebot.Silent)
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
		return c.Send("Error getting saved vacancies", b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
	}

	vacancies, err := b.getLatestVacancies(b.limit)
	if err != nil {
		log.Printf("Error getting latest vacancies: %v", err)
		return c.Send("Error getting saved vacancies", b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
	}

	if len(vacancies) == 0 {
		return c.Send("No saved vacancies found.", b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
	}

	// Send total count
	firstMessage := fmt.Sprintf("Total vacancies: %d\n\n", totalCount)
	err = c.Send(firstMessage, telebot.ModeMarkdown, telebot.NoPreview, telebot.Silent)
	if err != nil {
		return err
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
			c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview, b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
		} else {
			c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview, telebot.Silent)
		}
	}

	return nil
}

// handleClearSaved handles the /clear_saved command
func (b *Bot) handleClearSaved(c telebot.Context) error {
	log.Printf("Command /clear_saved received")
	err := b.vacancyRepo.DeleteHiddenVacancies()
	if err != nil {
		log.Printf("Error clearing hidden vacancies: %v", err)
		return c.Send("Error clearing hidden vacancies", b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
	}
	return c.Send("Hidden vacancies cleared successfully", b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
}

// handleHideAll handles the /hide_all command
func (b *Bot) handleHideAll(c telebot.Context) error {
	log.Printf("Command /hide_all received")

	// Hide all visible vacancies
	rowsAffected, err := b.vacancyRepo.HideAllVacancies()
	if err != nil {
		log.Printf("Error hiding all vacancies: %v", err)
		return c.Send("Error hiding vacancies", b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
	}
	if err != nil {
		log.Printf("Error getting rows affected: %v", err)
		return c.Send("Error hiding vacancies", b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
	}

	if rowsAffected == 0 {
		return c.Send("No visible vacancies to hide", b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
	}

	return c.Send(fmt.Sprintf("Hidden %d vacancies successfully", rowsAffected), b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
}

// handleBuildVersion handles the /build_version command
func (b *Bot) handleBuildVersion(c telebot.Context) error {
	log.Printf("Command /build_version received")

	// Use embedded build version
	version := b.buildVersion
	if version == "" {
		version = "unknown"
	}
	return c.Send(fmt.Sprintf("Current build version: %s", version), b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
}

// handleCompanies handles the /get_companies command
func (b *Bot) handleCompanies(c telebot.Context) error {
	log.Printf("Command /get_companies received")

	// Show loading message
	c.Send("Getting saved companies →", telebot.Silent)

	companies, err := b.companyRepo.GetAllCompaniesWithCounts()
	if err != nil {
		log.Printf("Error getting companies: %v", err)
		return c.Send("Error getting companies", b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
	}

	if len(companies) == 0 {
		return c.Send("No companies found.", b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
	}

	// Split companies into chunks
	for i := 0; i < len(companies); i += b.companiesPerMessage {
		end := i + b.companiesPerMessage
		if end > len(companies) {
			end = len(companies)
		}

		chunk := companies[i:end]

		var message strings.Builder
		// No header - start directly with company list

		for j, company := range chunk {
			companyLink := fmt.Sprintf("[%s](https://t.me/%s?start=%s_%d) | %d", company.Name, c.Bot().Me.Username, companyPrefix, company.ID, company.Count)
			message.WriteString(fmt.Sprintf("%d. %s\n", i+j+1, companyLink))
		}

		// Send with keyboard only on the last message
		isLastMessage := i+b.companiesPerMessage >= len(companies)
		if isLastMessage {
			c.Send(message.String(), telebot.ModeMarkdown, b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
		} else {
			c.Send(message.String(), telebot.ModeMarkdown, telebot.Silent)
		}
	}

	return nil
}

// handleCompanyVacancies handles showing vacancies for a specific company
func (b *Bot) handleCompanyVacancies(c telebot.Context, companyID int) error {
	log.Printf("Command /company_%d received", companyID)

	// Get company name
	companyName, err := b.companyRepo.GetCompanyNameByID(companyID)
	if err != nil {
		log.Printf("Error getting company name for ID %d: %v", companyID, err)
		return c.Send("Error getting company information", b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
	}

	// Show loading message
	c.Send(fmt.Sprintf("Getting %s vacancies →", companyName), telebot.Silent)

	// Get all vacancies for this company
	vacancies, err := b.vacancyRepo.GetVacanciesByCompanyID(companyID)
	if err != nil {
		log.Printf("Error getting vacancies for company %d: %v", companyID, err)
		return c.Send("Error getting company vacancies", b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
	}

	if len(vacancies) == 0 {
		return c.Send(fmt.Sprintf("**%s**\n\nNo vacancies found for this company.", companyName), telebot.ModeMarkdown, b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
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
			c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview, b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
		} else {
			c.Send(message.String(), telebot.ModeMarkdown, telebot.NoPreview, telebot.Silent)
		}
	}

	return nil
}

// handleBackToMenu handles the back to menu callback
func (b *Bot) handleBackToMenu(c telebot.Context) error {
	log.Printf("Command /back_to_menu received")
	return c.Send("Main menu:", b.keyboardBuilder.GetCommandKeyboard(), telebot.Silent)
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
							vacancyInfo.CompanyURL = normalizeCompanyURL(nextLink.url)
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

func escapeMarkdownLinkText(text string) string {
	// Telegram Markdown cannot escape ] inside [text](url), so replace brackets.
	text = strings.ReplaceAll(text, "[", "(")
	text = strings.ReplaceAll(text, "]", ")")
	return text
}

// handleText handles text messages
func (b *Bot) handleText(c telebot.Context) error {
	log.Printf("Text message received: %s", c.Text())
	// Echo the message back
	return c.Send("You said: "+c.Text(), b.keyboardBuilder.GetCommandKeyboard())
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
