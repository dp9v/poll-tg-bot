// Package notifier watches the local PostgreSQL cache for activity-availability
// changes and posts notifications to Telegram. It is fully decoupled from the
// loader: it never talks to the Alteg API directly. The loader's job is to
// keep the cache fresh; the notifier's job is to react to changes in that
// cache.
//
// The notifier maintains a small in-memory baseline of the last observed
// near-window state. Each "check" reads the current state from storage,
// diffs it against the baseline, sends Telegram messages and updates the
// baseline.
//
// Two trigger sources are supported:
//
//   - an event channel (typically wired to Loader.NearLoaded()) that fires
//     immediately after a successful near-window save;
//   - a periodic ticker that acts as a safety net so the notifier never
//     gets stuck waiting on a silent loader.
package notifier

import (
	"log"
	"time"

	"go-notification-tg-bot/internal/alteg"
	"go-notification-tg-bot/internal/bot"
	"go-notification-tg-bot/internal/storage"
	"go-notification-tg-bot/internal/timewindow"
)

// Notifier diffs the near-window cache and sends Telegram notifications.
type Notifier struct {
	storage  *storage.Storage
	sender   *bot.Sender
	trigger  <-chan struct{}
	interval time.Duration // safety-net polling interval; <=0 disables it

	previous []alteg.Activity // last observed state (in-memory baseline)
	seeded   bool
}

// New creates a Notifier.
//
// trigger may be nil — in that case the notifier relies entirely on the
// safety-net ticker (and the user must pass a positive interval).
//
// interval may be zero — in that case the notifier reacts only to trigger
// events.
func New(
	store *storage.Storage,
	sender *bot.Sender,
	trigger <-chan struct{},
	interval time.Duration,
) *Notifier {
	return &Notifier{
		storage:  store,
		sender:   sender,
		trigger:  trigger,
		interval: interval,
	}
}

// Run blocks forever, calling Check whenever a trigger event arrives or the
// safety-net ticker fires.
//
// On the very first call it seeds the in-memory baseline from the database
// so that a process restart does not flood the chat with "new" notifications
// for activities the user has already been informed about.
func (n *Notifier) Run() {
	n.seedBaseline()

	var tickerC <-chan time.Time
	if n.interval > 0 {
		t := time.NewTicker(n.interval)
		defer t.Stop()
		tickerC = t.C
	}

	for {
		select {
		case <-n.trigger:
			n.Check()
		case <-tickerC:
			n.Check()
		}
	}
}

// ListenCommands attaches a Telegram command/button handler that answers
// "what's available right now?" requests by reading directly from the cache.
// Call it in a separate goroutine alongside Run.
func (n *Notifier) ListenCommands() {
	n.sender.ListenCommands(func() ([]alteg.Activity, error) {
		from, till := timewindow.Near(time.Now())
		return n.storage.LoadBetween(from, till)
	})
}

// Check loads the current near-window state from the cache, computes the diff
// against the in-memory baseline, sends Telegram notifications about any
// changes and finally updates the baseline. Exposed primarily for tests.
func (n *Notifier) Check() {
	from, till := timewindow.Near(time.Now())
	current, err := n.storage.LoadBetween(from, till)
	if err != nil {
		log.Printf("notifier: could not load activities from storage: %v", err)
		return
	}

	added, removed := calculateDiff(n.previous, current)
	log.Printf("notifier: activities changed: +%d added, -%d removed", len(added), len(removed))

	if len(added) > 0 {
		if err := n.sender.SendNewActivities(added); err != nil {
			log.Printf("notifier: failed to send new activities notification: %v", err)
		}
	}
	if len(removed) > 0 {
		if err := n.sender.SendRemovedActivities(removed); err != nil {
			log.Printf("notifier: failed to send removed activities notification: %v", err)
		}
	}

	n.previous = current
	n.seeded = true
}

// seedBaseline preloads the in-memory baseline from storage on startup so
// the first check after a restart doesn't fire stale notifications.
func (n *Notifier) seedBaseline() {
	if n.seeded {
		return
	}
	from, till := timewindow.Near(time.Now())
	saved, err := n.storage.LoadBetween(from, till)
	if err != nil {
		log.Printf("notifier: warning — could not seed baseline from storage: %v", err)
		return
	}
	n.previous = saved
	n.seeded = true
	log.Printf("notifier: seeded baseline with %d activities", len(saved))
}

// calculateDiff compares the previous and current activity slices and returns
// two slices: added (newly available or got more places) and removed (lost
// availability — either dropped from the cache while having free spots, or
// became fully booked).
func calculateDiff(old, current []alteg.Activity) (added, removed []alteg.Activity) {
	oldByID := make(map[int]alteg.Activity, len(old))
	newByID := make(map[int]alteg.Activity, len(current))
	allIDs := make(map[int]struct{}, len(old)+len(current))

	for _, a := range old {
		oldByID[a.ID] = a
		allIDs[a.ID] = struct{}{}
	}
	for _, a := range current {
		newByID[a.ID] = a
		allIDs[a.ID] = struct{}{}
	}

	for id := range allIDs {
		previous, inOld := oldByID[id]
		actual, inNew := newByID[id]

		switch {
		case !inOld && inNew:
			// Brand-new activity — notify only if it has free spots.
			if actual.AvailablePlaces() > 0 {
				added = append(added, actual)
			}
		case inOld && !inNew:
			// Activity disappeared from the cache — notify if it previously had free spots.
			if previous.AvailablePlaces() > 0 {
				previous.RecordsCount = previous.Capacity
				removed = append(removed, previous)
			}
		default:
			oldPlaces := previous.AvailablePlaces()
			newPlaces := actual.AvailablePlaces()
			if newPlaces > oldPlaces {
				added = append(added, actual)
			} else if newPlaces < oldPlaces && newPlaces == 0 {
				removed = append(removed, actual)
			}
		}
	}

	return added, removed
}

