// Package loader is responsible for keeping the local PostgreSQL cache in
// sync with the upstream Alteg API. It does not send any activity-availability
// notifications — that is the notifier's job. The loader only:
//
//   - runs two periodic schedulers (near + long-term) that fetch and persist
//     activities, services, categories and trainers;
//   - reports operational problems to Telegram (startup banner, fetch errors);
//   - drives the token-renewal dialog when the API replies with HTTP 401;
//   - emits a non-blocking event after every successful near-window save so
//     the notifier can react immediately instead of polling on a timer.
package loader

import (
	"errors"
	"log"
	"sync"
	"time"

	"go-notification-tg-bot/internal/alteg"
	"go-notification-tg-bot/internal/bot"
	"go-notification-tg-bot/internal/storage"
	"go-notification-tg-bot/internal/timewindow"
)

// Loader runs the two API-poll schedulers.
type Loader struct {
	client           *alteg.Client
	storage          *storage.Storage
	sender           *bot.Sender
	nearInterval     time.Duration
	longTermInterval time.Duration

	mu       sync.Mutex
	nearErr  bool // last near poll ended with an error?
	renewing bool // serializes token renewal across both schedulers
}

// New constructs a Loader.
func New(
	client *alteg.Client,
	store *storage.Storage,
	sender *bot.Sender,
	nearInterval, longTermInterval time.Duration,
) *Loader {
	return &Loader{
		client:           client,
		storage:          store,
		sender:           sender,
		nearInterval:     nearInterval,
		longTermInterval: longTermInterval,
	}
}

// Run starts both schedulers and blocks until the process is terminated.
// Each scheduler polls once immediately, then every interval tick.
func (l *Loader) Run() {
	if err := l.sender.SendStartup(l.nearInterval); err != nil {
		log.Printf("failed to send startup notification: %v", err)
	}

	// Long-term scheduler runs in the background.
	go l.runLoop("long-term", l.longTermInterval, l.LoadLongTerm)

	// Near scheduler runs on the calling goroutine and blocks forever.
	l.runLoop("near", l.nearInterval, l.LoadNear)
}

// LoadNear fetches the near window (current + 2 next weeks) and persists it.
// Exposed for tests.
func (l *Loader) LoadNear() {
	from, till := timewindow.Near(time.Now())
	if l.loadWindow("near", from, till) {
		log.Println("[near] near window saved successfully")
	}
}

// LoadLongTerm fetches the long-term window (the rest of the current month
// plus the next two months, excluding the near window) and persists it.
// Exposed for tests.
func (l *Loader) LoadLongTerm() {
	from, till := timewindow.LongTerm(time.Now())
	if !till.After(from) {
		// Edge case: the near range already swallowed the long-term horizon.
		return
	}
	l.loadWindow("long-term", from, till)
}

// runLoop drives a single ticker-based scheduler. fn is invoked once
// immediately on entry and then on every tick.
func (l *Loader) runLoop(label string, interval time.Duration, fn func()) {
	log.Printf("[%s] scheduler starting (interval=%s)", label, interval)
	fn()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		fn()
	}
}

// loadWindow performs one fetch + persist cycle for a single window.
// Returns true on success, false on any error (including 401).
func (l *Loader) loadWindow(label string, from, till time.Time) bool {
	activities, err := l.client.FetchActivities(from, till)
	if err != nil {
		log.Printf("[%s] error fetching activities: %v", label, err)

		if errors.Is(err, alteg.ErrUnauthorized) {
			l.renewToken()
			return false
		}

		// Send the error message only on the first failure of a streak,
		// and only from the user-facing "near" scheduler.
		if label == "near" && !l.markNearError() {
			if sendErr := l.sender.SendError(err); sendErr != nil {
				log.Printf("[%s] failed to send error notification: %v", label, sendErr)
			}
		}
		return false
	}

	if label == "near" {
		l.clearNearError()
	}

	if err := l.storage.SaveWindow(activities, from, till); err != nil {
		log.Printf("[%s] warning: could not save activities to storage: %v", label, err)
		return false
	}

	log.Printf("[%s] cached %d activities (%s — %s)",
		label, len(activities), from.Format("2006-01-02"), till.Format("2006-01-02"))
	return true
}

// markNearError flips the near-streak flag and returns its previous value.
func (l *Loader) markNearError() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	prev := l.nearErr
	l.nearErr = true
	return prev
}

// clearNearError resets the near-streak flag.
func (l *Loader) clearNearError() {
	l.mu.Lock()
	l.nearErr = false
	l.mu.Unlock()
}

// renewToken blocks until the user sends a new bearer token via Telegram,
// then updates the API client. Concurrent calls (one from each scheduler)
// are coalesced — only the first invocation actually waits.
func (l *Loader) renewToken() {
	l.mu.Lock()
	if l.renewing {
		l.mu.Unlock()
		return
	}
	l.renewing = true
	l.mu.Unlock()

	defer func() {
		l.mu.Lock()
		l.renewing = false
		l.mu.Unlock()
	}()

	log.Println("bearer token expired, waiting for new token from Telegram...")
	newToken := l.sender.TokenDialog()
	if newToken == "" {
		log.Println("no token received, will retry on the next poll")
		return
	}
	l.client.UpdateToken(newToken)
	l.clearNearError()
	log.Println("bearer token updated successfully")
}
