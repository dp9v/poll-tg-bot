package notifier

import (
	"errors"
	"fmt"
	"go-notification-tg-bot/internal/alteg"
	"go-notification-tg-bot/internal/bot"
	"log"
	"strings"
	"time"
)

// Notifier polls the Alteg API and sends Telegram notifications when availability changes.
type Notifier struct {
	client   *alteg.Client
	sender   *bot.Sender
	interval time.Duration

	lastKey      string // last observed set of available activities
	lastWasError bool   // whether the previous poll ended in an error
}

// New creates a new Notifier.
func New(client *alteg.Client, sender *bot.Sender, interval time.Duration) *Notifier {
	return &Notifier{
		client:   client,
		sender:   sender,
		interval: interval,
	}
}

// Run starts the polling loop. It polls once immediately, then on every interval tick.
// It blocks until the process is terminated.
func (n *Notifier) Run() {
	n.poll()

	ticker := time.NewTicker(n.interval)
	defer ticker.Stop()

	for range ticker.C {
		n.poll()
	}
}

// poll fetches activities and sends a notification if anything has changed.
func (n *Notifier) poll() {
	activities, err := n.client.FetchAvailableActivities()
	if err != nil {
		log.Printf("error fetching activities: %v", err)

		// Token expired — ask the user for a new one via Telegram.
		if errors.Is(err, alteg.ErrUnauthorized) {
			n.renewToken()
			return
		}

		if !n.lastWasError {
			// Send error notification only on the first failure in a row.
			if sendErr := n.sender.SendError(err); sendErr != nil {
				log.Printf("failed to send error notification: %v", sendErr)
			}
			n.lastWasError = true
		}
		return
	}

	// Error streak is resolved.
	n.lastWasError = false

	currentKey := activitiesKey(activities)
	if currentKey == n.lastKey {
		log.Println("no changes in available activities, skipping notification")
		return
	}

	log.Printf("activities changed, sending notification (%d available)", len(activities))
	if err := n.sender.SendActivities(activities); err != nil {
		log.Printf("failed to send activities notification: %v", err)
	}
	n.lastKey = currentKey
}

// activitiesKey builds a comparable string key from a list of activities.
// It encodes each activity's ID + available places so we can detect any change.
func activitiesKey(activities []alteg.Activity) string {
	parts := make([]string, 0, len(activities))
	for _, a := range activities {
		parts = append(parts, fmt.Sprintf("%d:%d", a.ID, a.AvailablePlaces()))
	}
	return strings.Join(parts, ",")
}

// renewToken blocks until the user sends a new bearer token via Telegram,
// then updates the API client with the new token.
func (n *Notifier) renewToken() {
	log.Println("bearer token expired, waiting for new token from Telegram...")
	newToken := n.sender.TokenDialog()
	if newToken == "" {
		log.Println("no token received, will retry on the next poll")
		return
	}
	n.client.UpdateToken(newToken)
	n.lastWasError = false
	log.Println("bearer token updated successfully")
}
