package bot

import (
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const tokenRequestTimeout = 10 * time.Minute

// TokenDialog asks the user in Telegram to send a new bearer token and waits for the reply.
// It returns the received token, or an empty string if the timeout is reached.
func (s *Sender) TokenDialog() string {
	// Ask the user to provide a new token.
	if err := s.send(
		"🔑 *Bearer token has expired.*\n\n" +
			"Please send the new token as a plain message here.\n" +
			"You can copy it from the browser DevTools → Network tab after opening the booking page.",
	); err != nil {
		log.Printf("failed to send token request message: %v", err)
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := s.api.GetUpdatesChan(u)
	defer s.api.StopReceivingUpdates()

	deadline := time.After(tokenRequestTimeout)

	for {
		select {
		case <-deadline:
			_ = s.send("⏰ Token input timed out. Will retry on the next poll.")
			return ""

		case update, ok := <-updates:
			if !ok {
				return ""
			}
			// Accept only text messages from the configured chat.
			if update.Message == nil {
				continue
			}
			if update.Message.Chat.ID != s.chatID {
				continue
			}

			token := strings.TrimSpace(update.Message.Text)
			if token == "" {
				continue
			}

			_ = s.send("✅ Token updated. Resuming polling...")
			return token
		}
	}
}
