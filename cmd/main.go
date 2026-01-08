package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"miltech-tgbot/internal/bot"
)

func main() {
	// Get bot token from environment variable
	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		log.Fatal("BOT_TOKEN environment variable is not set")
	}

	// Initialize and start bot
	telegramBot, err := bot.New(botToken)
	if err != nil {
		log.Fatalf("Failed to initialize bot: %v", err)
	}

	log.Println("Starting Telegram bot...")
	if err := telegramBot.Start(); err != nil {
		log.Fatalf("Failed to start bot: %v", err)
	}

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down bot...")
	telegramBot.Stop()
}

