package main

import (
	"go-notification-tg-bot/internal/alteg"
	"go-notification-tg-bot/internal/bot"
	"go-notification-tg-bot/internal/config"
	"go-notification-tg-bot/internal/notifier"
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

	altegClient := alteg.NewClient(cfg.BearerToken)
	sender := bot.NewSender(api, cfg.ChatID)
	n := notifier.New(altegClient, sender, cfg.Interval)

	n.Run()
}
