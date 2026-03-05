package notifier

import (
	"errors"
	"go-notification-tg-bot/internal/alteg"
	"go-notification-tg-bot/internal/bot"
	"go-notification-tg-bot/internal/storage"
	"log"
	"time"
)

type Notifier struct {
	client   *alteg.Client
	sender   *bot.Sender
	storage  *storage.Storage
	interval time.Duration

	lastKey      string // last observed set of available activities
	lastWasError bool   // whether the previous poll ended in an error
}

// New creates a new Notifier.
func New(client *alteg.Client, sender *bot.Sender, store *storage.Storage, interval time.Duration) *Notifier {
	return &Notifier{
		client:   client,
		sender:   sender,
		storage:  store,
		interval: interval,
	}
}

// Run starts the polling loop. It polls once immediately, then on every interval tick.
// It blocks until the process is terminated.
func (n *Notifier) Run() {
	if err := n.sender.SendStartup(n.interval); err != nil {
		log.Printf("failed to send startup notification: %v", err)
	}

	n.poll()

	ticker := time.NewTicker(n.interval)
	defer ticker.Stop()

	for range ticker.C {
		n.poll()
	}
}

// poll fetches activities and sends a notification if anything has changed.
func (n *Notifier) poll() {
	from, till := searchDateRange()

	activities, err := n.client.FetchActivities(from, till)
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

	var oldActivities []alteg.Activity
	if saved, err := n.storage.LoadBetween(from, till); err != nil {
		log.Printf("warning: could not load saved activities from storage: %v", err)
		oldActivities = activities
	} else {
		oldActivities = saved
	}

	added, removed := calculateDiff(oldActivities, activities)
	log.Printf("activities changed: +%d added, -%d removed", len(added), len(removed))

	if len(added) > 0 {
		if err := n.sender.SendNewActivities(added); err != nil {
			log.Printf("failed to send new activities notification: %v", err)
		}
	}
	if len(removed) > 0 {
		if err := n.sender.SendRemovedActivities(removed); err != nil {
			log.Printf("failed to send removed activities notification: %v", err)
		}
	}

	// Persist the latest known activities so they can be restored on the next startup.
	if err := n.storage.Save(activities); err != nil {
		log.Printf("warning: could not save activities to storage: %v", err)
	}
}

// ListenCommands starts the Telegram command/button handler in the current goroutine.
// Call it in a separate goroutine alongside Run.
func (n *Notifier) ListenCommands() {
	n.sender.ListenCommands(func() ([]alteg.Activity, error) {
		from, till := searchDateRange()
		return n.client.FetchActivities(from, till)
	})
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

// searchDateRange returns the [from, till] window used for each API poll.
// from — today's date.
// till — the first day of next month shifted two months forward (i.e. roughly 3 months ahead).
func searchDateRange() (from, till time.Time) {
	now := time.Now()
	from = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	till = time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location()).AddDate(0, 2, 0)
	return
}

func calculateDiff(old, new []alteg.Activity) (added, removed []alteg.Activity) {
	oldByID := make(map[int]alteg.Activity, len(old))
	newByID := make(map[int]alteg.Activity, len(new))
	allIDs := make(map[int]struct{}, len(old)+len(new))

	for _, a := range old {
		oldByID[a.ID] = a
		allIDs[a.ID] = struct{}{}
	}
	for _, a := range new {
		newByID[a.ID] = a
		allIDs[a.ID] = struct{}{}
	}

	for id := range allIDs {
		previous, inOld := oldByID[id]
		current, inNew := newByID[id]

		switch {
		case !inOld && inNew:
			// Brand-new activity — notify only if it has free spots.
			if current.AvailablePlaces() > 0 {
				added = append(added, current)
			}
		case inOld && !inNew:
			// Activity disappeared from API entirely — notify if it previously had free spots.
			if previous.AvailablePlaces() > 0 {
				removed = append(removed, previous)
			}
		default:
			oldPlaces := previous.AvailablePlaces()
			newPlaces := current.AvailablePlaces()
			if newPlaces > oldPlaces {
				// More spots opened up — notify.
				added = append(added, current)
			} else if newPlaces < oldPlaces && newPlaces == 0 {
				// All spots are taken — notify as removed.
				removed = append(removed, current)
			}
		}
	}

	return added, removed
}
