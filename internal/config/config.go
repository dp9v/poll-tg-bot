package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

const defaultInterval = 5 * time.Minute

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	BotToken    string
	ChatID      int64
	BearerToken string
	Interval    time.Duration
	StoragePath string
}

// Load reads configuration from environment variables and returns a Config.
// Returns an error if any required variable is missing or invalid.
func Load() (*Config, error) {
	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		return nil, fmt.Errorf("BOT_TOKEN environment variable is not set")
	}

	chatIDStr := os.Getenv("CHAT_ID")
	if chatIDStr == "" {
		return nil, fmt.Errorf("CHAT_ID environment variable is not set")
	}
	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("CHAT_ID is not a valid integer: %w", err)
	}

	bearerToken := os.Getenv("BEARER_TOKEN")
	if bearerToken == "" {
		return nil, fmt.Errorf("BEARER_TOKEN environment variable is not set")
	}

	storagePath := os.Getenv("STORAGE_PATH")
	if storagePath == "" {
		storagePath = "/data/tgbot.db"
	}

	return &Config{
		BotToken:    botToken,
		ChatID:      chatID,
		BearerToken: bearerToken,
		Interval:    defaultInterval,
		StoragePath: storagePath,
	}, nil
}
