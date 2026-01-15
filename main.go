package main

import (
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"miltech-tgbot/internal/bot"

	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables from .env file
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	// Get bot token from environment variable
	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		log.Fatal("BOT_TOKEN environment variable is not set")
	}

	// Get admin ID from environment variable (required)
	adminID := os.Getenv("ADMIN_ID")
	if adminID == "" {
		log.Fatal("ADMIN_ID environment variable is not set")
	}

	// Get interval from environment variable (required)
	intervalStr := os.Getenv("INTERVAL")
	if intervalStr == "" {
		log.Fatal("INTERVAL environment variable is not set")
	}

	// Get deftech URL from environment variable (required)
	deftechURL := os.Getenv("DEFTECH_URL")
	if deftechURL == "" {
		log.Fatal("DEFTECH_URL environment variable is not set")
	}

	// Get database name from environment variable (optional, default to ./deftech-tgbot.db)
	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "./deftech-tgbot.db"
	}

	// Get vacancies table name from environment variable (optional, default to vacancies)
	vacanciesTable := os.Getenv("VACANCIES_TABLE")
	if vacanciesTable == "" {
		vacanciesTable = "vacancies"
	}

	// Get limit from environment variable (required)
	limitStr := os.Getenv("LIMIT")
	if limitStr == "" {
		log.Fatal("LIMIT environment variable is not set")
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		log.Fatalf("Invalid LIMIT value: %v", err)
	}
	if limit <= 0 || limit >= 100 {
		log.Fatalf("LIMIT must be greater than 0 and less than 100, got: %d", limit)
	}

	// Initialize bot
	log.Println("Initializing Telegram bot →")
	telegramBot, err := bot.New(botToken, adminID, intervalStr, dbName, vacanciesTable, deftechURL, limit)
	if err != nil {
		log.Fatalf("Failed to initialize bot: %v", err)
	}

	// Start bot in a goroutine
	go func() {
		log.Println("Starting Telegram bot →")
		telegramBot.Start()
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down bot →")
	telegramBot.Stop()
}
