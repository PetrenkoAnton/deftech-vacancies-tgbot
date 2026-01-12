package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"miltech-tgbot/internal/bot"
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

	// Get admin ID from environment variable (optional)
	adminID := os.Getenv("ADMIN_ID")

	// Initialize bot
	log.Println("Initializing Telegram bot...")
	telegramBot, err := bot.New(botToken, adminID)
	if err != nil {
		log.Fatalf("Failed to initialize bot: %v", err)
	}

	// Start bot in a goroutine
	go func() {
		log.Println("Starting Telegram bot...")
		telegramBot.Start()
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down bot...")
	telegramBot.Stop()
}
