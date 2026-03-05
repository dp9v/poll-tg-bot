package bot

import (
	"fmt"
	"go-notification-tg-bot/internal/alteg"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Sender wraps the Telegram bot and provides messaging helpers.
type Sender struct {
	api    *tgbotapi.BotAPI
	chatID int64
}

// NewSender creates a new Sender for the given bot and chat.
func NewSender(api *tgbotapi.BotAPI, chatID int64) *Sender {
	return &Sender{api: api, chatID: chatID}
}

// SendNewActivities formats and sends newly appeared or grown activities.
func (s *Sender) SendNewActivities(activities []alteg.Activity) error {
	return s.send(formatActivitiesMessage("🆕 *New / more places available:*", activities))
}

// SendRemovedActivities formats and sends activities that lost spots or disappeared.
func (s *Sender) SendRemovedActivities(activities []alteg.Activity) error {
	return s.send(formatActivitiesMessage("❌ *Places taken / activity removed:*", activities))
}

// SendError sends an error notification message.
func (s *Sender) SendError(err error) error {
	return s.send(fmt.Sprintf("⚠️ Error fetching activities: %s", err.Error()))
}

// send delivers a Markdown-formatted message to the configured chat.
func (s *Sender) send(text string) error {
	msg := tgbotapi.NewMessage(s.chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	msg.DisableWebPagePreview = true
	_, err := s.api.Send(msg)
	return err
}

// formatActivitiesMessage builds a Telegram message from a list of activities with the given header.
func formatActivitiesMessage(header string, activities []alteg.Activity) string {
	if len(activities) == 0 {
		return "❌ No available activities found."
	}

	var sb strings.Builder
	sb.WriteString(header + "\n\n")

	for _, a := range activities {
		// Parse date to display it nicely.
		t, err := time.Parse("2006-01-02 15:04:05", a.Date)
		dateStr := a.Date
		timeStr := ""
		if err == nil {
			dateStr = t.Format("2006-01-02 (Mon)")
			timeStr = t.Format("15:04")
		}

		sb.WriteString(fmt.Sprintf(
			"📅 *Date:* %s\n"+
				"⏰ *Time:* %s\n"+
				"🏷 *Title:* %s\n"+
				"👤 *Coach:* %s\n"+
				"🪑 *Available places:* %d\n"+
				"🔗 [Book now](%s)\n\n",
			dateStr,
			timeStr,
			escapeMarkdown(a.Service.Title),
			escapeMarkdown(a.Staff.Name),
			a.AvailablePlaces(),
			alteg.ActivityURL(a.ID),
		))
	}

	return sb.String()
}

// escapeMarkdown escapes Telegram MarkdownV1 special characters.
func escapeMarkdown(s string) string {
	replacer := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"`", "\\`",
		"[", "\\[",
	)
	return replacer.Replace(s)
}
