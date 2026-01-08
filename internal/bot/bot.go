package bot

import (
	"log"
	"time"

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
		"/help - Show this help message"
	return c.Send(helpText)
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

