package main

import (
	"go-notification-tg-bot/internal/alteg"
	"go-notification-tg-bot/internal/bot"
	"go-notification-tg-bot/internal/config"
	"go-notification-tg-bot/internal/loader"
	"go-notification-tg-bot/internal/notifier"
	"go-notification-tg-bot/internal/storage"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}

	api, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		log.Fatalf("failed to create Telegram bot: %v", err)
	}
	log.Printf("Authorized on account %s", api.Self.UserName)

	altegClient := alteg.NewClient(cfg.BearerToken).WithPageSize(cfg.PageSize)
	sender := bot.NewSender(api, cfg.ChatID)
	store, err := storage.New(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to open storage: %v", err)
	}
	defer func(store *storage.Storage) {
		_ = store.Close()
	}(store)

	// Loader: keeps the local cache in sync with the Alteg API.
	ld := loader.New(altegClient, store, sender, cfg.NearInterval, cfg.LongTermInterval)

	// Notifier: watches the cache and posts diff notifications to Telegram.
	nf := notifier.New(store, sender, cfg.NearInterval)

	go nf.ListenCommands()
	go nf.Run()
	ld.Run() // blocks
}
