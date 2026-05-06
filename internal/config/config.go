package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

const (
	defaultNearInterval     = 15 * time.Minute
	defaultLongTermInterval = 60 * time.Minute
	defaultPageSize         = 200
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	BotToken         string
	ChatID           int64
	BearerToken      string
	NearInterval     time.Duration // poll interval for the "near" window (current + 2 weeks)
	LongTermInterval time.Duration // poll interval for the "long-term" window (current + 2 months, excluding near)
	PageSize         int           // page size used for paginated API calls
	DatabaseURL      string
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

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL environment variable is not set")
	}

	nearInterval, err := parseDuration("NEAR_INTERVAL", defaultNearInterval)
	if err != nil {
		return nil, err
	}
	longTermInterval, err := parseDuration("LONGTERM_INTERVAL", defaultLongTermInterval)
	if err != nil {
		return nil, err
	}

	pageSize, err := parsePositiveInt("PAGE_SIZE", defaultPageSize)
	if err != nil {
		return nil, err
	}

	return &Config{
		BotToken:         botToken,
		ChatID:           chatID,
		BearerToken:      bearerToken,
		NearInterval:     nearInterval,
		LongTermInterval: longTermInterval,
		PageSize:         pageSize,
		DatabaseURL:      databaseURL,
	}, nil
}

// parseDuration reads an env var and parses it with time.ParseDuration.
// Returns the default value when the variable is unset or empty.
func parseDuration(name string, def time.Duration) (time.Duration, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return def, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s is not a valid duration: %w", name, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration, got %q", name, raw)
	}
	return d, nil
}

// parsePositiveInt reads an env var and parses it as a positive integer.
// Returns the default value when the variable is unset or empty.
func parsePositiveInt(name string, def int) (int, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return def, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s is not a valid integer: %w", name, err)
	}
	if v <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer, got %q", name, raw)
	}
	return v, nil
}

