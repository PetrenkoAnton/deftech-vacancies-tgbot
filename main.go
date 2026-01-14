package main

import (
	"log"
	"os"
	"os/signal"
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

	// Get group ID from environment variable (required)
	groupID := os.Getenv("GROUP_ID")
	if groupID == "" {
		log.Fatal("GROUP_ID environment variable is not set")
	}

	// Get interval from environment variable (required)
	intervalStr := os.Getenv("INTERVAL")
	if intervalStr == "" {
		log.Fatal("INTERVAL environment variable is not set")
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

	// Initialize bot
	log.Println("Initializing Telegram bot →")
	telegramBot, err := bot.New(botToken, adminID, groupID, intervalStr, dbName, vacanciesTable)
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
