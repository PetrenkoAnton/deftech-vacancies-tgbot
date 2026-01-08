package config

import (
	"os"
)

// Config holds the application configuration
type Config struct {
	BotToken string
	// Add other configuration fields here
}

// Load loads configuration from environment variables
func Load() *Config {
	return &Config{
		BotToken: os.Getenv("BOT_TOKEN"),
	}
}

