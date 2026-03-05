package bot

import (
	"context"
	"log"

	"go-notification-tg-bot/internal/alteg"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const BtnShowActivities = "📋 Show available activities"

// ActivitiesFetcher is a function that returns the current list of all activities.
type ActivitiesFetcher func() ([]alteg.Activity, error)

// ListenCommands starts a loop that handles incoming Telegram messages/button presses.
// It blocks until the updates channel is closed.
func (s *Sender) ListenCommands(fetch ActivitiesFetcher) {
	s.listenCommandsCtx(context.Background(), fetch)
}

// listenCommandsCtx is the cancellable implementation used by ListenCommands and tests.
func (s *Sender) listenCommandsCtx(ctx context.Context, fetch ActivitiesFetcher) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := s.api.GetUpdatesChan(u)
	defer s.api.StopReceivingUpdates()

	for {
		select {
		case <-ctx.Done():
			return
		case update, ok := <-updates:
			if !ok {
				return
			}
			if update.Message == nil {
				continue
			}
			// Only handle messages from the configured chat.
			if update.Message.Chat.ID != s.chatID {
				continue
			}

			switch update.Message.Text {
			case BtnShowActivities, "/activities":
				s.handleShowActivities(fetch)
			}
		}
	}
}

func (s *Sender) handleShowActivities(fetch ActivitiesFetcher) {
	activities, err := fetch()
	if err != nil {
		log.Printf("failed to fetch activities on demand: %v", err)
		if sendErr := s.SendError(err); sendErr != nil {
			log.Printf("failed to send error message: %v", sendErr)
		}
		return
	}

	if err := s.SendAvailableActivities(activities); err != nil {
		log.Printf("failed to send activities list: %v", err)
	}
}
