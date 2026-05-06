package notifier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"go-notification-tg-bot/internal/alteg"
	"go-notification-tg-bot/internal/bot"
	"go-notification-tg-bot/internal/storage"
)

// ── Telegram stub ────────────────────────────────────────────────────────────

// tgStub is a minimal Telegram Bot API stub that records sendMessage calls.
type tgStub struct {
	mu       sync.Mutex
	messages []string // collected message texts
	srv      *httptest.Server
}

func newTGStub(t *testing.T) *tgStub {
	t.Helper()
	stub := &tgStub{}
	mux := http.NewServeMux()

	// getMe — required during tgbotapi.NewBotAPI
	mux.HandleFunc("/botTEST_TOKEN/getMe", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"result": map[string]any{
				"id":         1,
				"is_bot":     true,
				"first_name": "TestBot",
				"username":   "test_bot",
			},
		})
	})

	// sendMessage — capture the text
	mux.HandleFunc("/botTEST_TOKEN/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		text := r.FormValue("text")
		stub.mu.Lock()
		stub.messages = append(stub.messages, text)
		stub.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"result": map[string]any{"message_id": 1},
		})
	})

	stub.srv = httptest.NewServer(mux)
	t.Cleanup(stub.srv.Close)
	return stub
}

// captured returns a copy of all collected messages.
func (s *tgStub) captured() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.messages))
	copy(out, s.messages)
	return out
}

// ── Alteg API stub ───────────────────────────────────────────────────────────

type altegStub struct {
	mu         sync.Mutex
	activities []alteg.Activity
	statusCode int
	srv        *httptest.Server
}

func newAltegStub(t *testing.T) *altegStub {
	t.Helper()
	stub := &altegStub{statusCode: http.StatusOK}
	stub.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stub.mu.Lock()
		code := stub.statusCode
		acts := stub.activities
		stub.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		if code == http.StatusOK {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data":    acts,
			})
		}
	}))
	t.Cleanup(stub.srv.Close)
	return stub
}

func (s *altegStub) set(activities []alteg.Activity) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activities = activities
}

func (s *altegStub) setStatus(code int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statusCode = code
}

// ── helpers ──────────────────────────────────────────────────────────────────

func makeActivity(id, capacity, records int) alteg.Activity {
	return alteg.Activity{
		ID:           id,
		Date:         "2026-06-01 10:00:00",
		Capacity:     capacity,
		RecordsCount: records,
		Staff:        alteg.Staff{ID: 1, Name: "Coach"},
		Service:      alteg.Service{ID: 1, Title: "Yoga"},
	}
}

// setupNotifier wires everything together and returns the notifier and the two stubs.
func setupNotifier(t *testing.T) (*Notifier, *altegStub, *tgStub) {
	t.Helper()

	// Telegram stub
	tg := newTGStub(t)
	api, err := tgbotapi.NewBotAPIWithAPIEndpoint("TEST_TOKEN", tg.srv.URL+"/bot%s/%s")
	require.NoError(t, err)
	sender := bot.NewSender(api, 12345)

	// Alteg stub
	altegS := newAltegStub(t)
	client := alteg.NewClient("token").WithBaseURL(altegS.srv.URL)

	// PostgreSQL via testcontainers (fresh database per test)
	dsn := startPostgres(t)
	store, err := storage.New(dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	n := New(client, sender, store, time.Hour /* interval not used in tests */)
	return n, altegS, tg
}

// startPostgres spins up a throwaway Postgres container and returns its DSN.
// The container is automatically terminated at the end of the test.
func startPostgres(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = container.Terminate(context.Background())
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	return dsn
}

// ── tests ────────────────────────────────────────────────────────────────────

// TestPoll_FirstRun_NewActivityWithPlaces verifies that on the very first poll
// a new activity with free places is reported as "added".
func TestPoll_FirstRun_NewActivityWithPlaces(t *testing.T) {
	n, altegS, tg := setupNotifier(t)

	altegS.set([]alteg.Activity{makeActivity(1, 10, 3)}) // 7 free
	n.poll()

	msgs := tg.captured()
	require.Len(t, msgs, 1, "expected exactly one Telegram message")
	assert.True(t, strings.Contains(msgs[0], "New"), "expected a 'new' notification")
}

// TestPoll_FirstRun_ActivityFullyBooked verifies that a fully-booked activity
// on the very first poll does NOT produce any notification.
func TestPoll_FirstRun_ActivityFullyBooked(t *testing.T) {
	n, altegS, tg := setupNotifier(t)

	altegS.set([]alteg.Activity{makeActivity(1, 10, 10)}) // 0 free
	n.poll()

	assert.Empty(t, tg.captured(), "fully-booked activity should produce no notification")
}

// TestPoll_PlacesOpenUp verifies that when an activity gains free spots
// a "new" notification is sent.
func TestPoll_PlacesOpenUp(t *testing.T) {
	n, altegS, tg := setupNotifier(t)

	// First poll: activity is fully booked → no notification
	altegS.set([]alteg.Activity{makeActivity(1, 10, 10)})
	n.poll()
	require.Empty(t, tg.captured())

	// Second poll: a cancellation freed a spot
	altegS.set([]alteg.Activity{makeActivity(1, 10, 9)})
	n.poll()

	msgs := tg.captured()
	require.Len(t, msgs, 1)
	assert.True(t, strings.Contains(msgs[0], "New"), "expected a 'new' notification when places open up")
}

// TestPoll_PlacesTaken verifies that when the last free spot is taken
// a "removed" notification is sent.
func TestPoll_PlacesTaken(t *testing.T) {
	n, altegS, tg := setupNotifier(t)

	// First poll: one free spot
	altegS.set([]alteg.Activity{makeActivity(1, 10, 9)})
	n.poll()
	require.Len(t, tg.captured(), 1) // "new" notification

	// Second poll: last spot taken
	altegS.set([]alteg.Activity{makeActivity(1, 10, 10)})
	n.poll()

	msgs := tg.captured()
	require.Len(t, msgs, 2)
	assert.True(t, strings.Contains(msgs[1], "taken"), "expected a 'removed' notification when all spots are taken")
}

// TestPoll_ActivityDisappears verifies that when an activity disappears from
// the API while it still had free places, a "removed" notification is sent.
func TestPoll_ActivityDisappears(t *testing.T) {
	n, altegS, tg := setupNotifier(t)

	// First poll: activity has free spots
	altegS.set([]alteg.Activity{makeActivity(1, 10, 5)})
	n.poll()
	require.Len(t, tg.captured(), 1)

	// Second poll: activity is gone from API
	altegS.set([]alteg.Activity{})
	n.poll()

	msgs := tg.captured()
	require.Len(t, msgs, 2)
	assert.True(t, strings.Contains(msgs[1], "taken") || strings.Contains(msgs[1], "removed"),
		"expected a 'removed' notification when activity disappears")
}

// TestPoll_ActivityDisappears_NotificationOnlyOnce verifies that when an activity
// stored in the DB with free places stops coming from the API, the "removed"
// notification is sent exactly once, not on every subsequent poll.
func TestPoll_ActivityDisappears_NotificationOnlyOnce(t *testing.T) {
	n, altegS, tg := setupNotifier(t)

	// First poll: activity has free spots → "new" notification
	altegS.set([]alteg.Activity{makeActivity(1, 10, 5)})
	n.poll()
	require.Len(t, tg.captured(), 1)

	// Second poll: activity is gone → "removed" notification
	altegS.set([]alteg.Activity{})
	n.poll()
	require.Len(t, tg.captured(), 2)

	// Third and fourth polls: activity still absent → no additional notifications
	n.poll()
	n.poll()

	assert.Len(t, tg.captured(), 2, "removed notification must be sent only once when activity stays gone")
}

// TestPoll_ActivityDisappears_WasFullyBooked verifies that when a fully-booked
// activity disappears, no notification is sent (nothing was available anyway).
func TestPoll_ActivityDisappears_WasFullyBooked(t *testing.T) {
	n, altegS, tg := setupNotifier(t)

	altegS.set([]alteg.Activity{makeActivity(1, 10, 10)})
	n.poll()
	require.Empty(t, tg.captured())

	altegS.set([]alteg.Activity{})
	n.poll()

	assert.Empty(t, tg.captured(), "no notification expected when a fully-booked activity disappears")
}

// TestPoll_NoChanges verifies that repeated polls with identical data
// produce no additional notifications.
func TestPoll_NoChanges(t *testing.T) {
	n, altegS, tg := setupNotifier(t)

	acts := []alteg.Activity{makeActivity(1, 10, 3)}
	altegS.set(acts)
	n.poll()
	require.Len(t, tg.captured(), 1) // initial "new" notification

	// Poll again with same data
	n.poll()
	n.poll()

	assert.Len(t, tg.captured(), 1, "no new notifications expected when nothing changed")
}

// TestPoll_APIError verifies that an API error sends an error notification
// and does not update storage.
func TestPoll_APIError(t *testing.T) {
	n, altegS, tg := setupNotifier(t)

	altegS.setStatus(http.StatusInternalServerError)
	n.poll()

	msgs := tg.captured()
	require.Len(t, msgs, 1)
	assert.True(t, strings.Contains(msgs[0], "Error"), "expected an error notification")
}

// TestPoll_APIError_OnlyOnce verifies that consecutive errors produce
// only one error notification (not repeated on every poll).
func TestPoll_APIError_OnlyOnce(t *testing.T) {
	n, altegS, tg := setupNotifier(t)

	altegS.setStatus(http.StatusInternalServerError)
	n.poll()
	n.poll()
	n.poll()

	assert.Len(t, tg.captured(), 1, "error notification should be sent only once per error streak")
}

// TestPoll_RecoveryAfterError verifies that after a successful poll following
// an error, normal notifications resume.
func TestPoll_RecoveryAfterError(t *testing.T) {
	n, altegS, tg := setupNotifier(t)

	// First poll: error
	altegS.setStatus(http.StatusInternalServerError)
	n.poll()
	require.Len(t, tg.captured(), 1)

	// Second poll: recovered, new activity available
	altegS.setStatus(http.StatusOK)
	altegS.set([]alteg.Activity{makeActivity(1, 10, 5)})
	n.poll()

	msgs := tg.captured()
	require.Len(t, msgs, 2)
	assert.True(t, strings.Contains(msgs[1], "New"))
}

// TestPoll_MultipleActivities verifies that all relevant activities are
// included in a single notification message.
func TestPoll_MultipleActivities(t *testing.T) {
	n, altegS, tg := setupNotifier(t)

	altegS.set([]alteg.Activity{
		makeActivity(1, 10, 3),
		makeActivity(2, 5, 2),
		makeActivity(3, 8, 8), // fully booked — should NOT appear
	})
	n.poll()

	msgs := tg.captured()
	require.Len(t, msgs, 1)
	// Both free-spot activities should be mentioned
	assert.True(t, strings.Contains(msgs[0], "New"))
}

// ── calculateDiff unit tests ─────────────────────────────────────────────────

func TestCalculateDiff_NewActivityWithPlaces(t *testing.T) {
	added, removed := calculateDiff(nil, []alteg.Activity{makeActivity(1, 10, 3)})
	assert.Len(t, added, 1)
	assert.Empty(t, removed)
}

func TestCalculateDiff_NewActivityNoPlaces(t *testing.T) {
	added, removed := calculateDiff(nil, []alteg.Activity{makeActivity(1, 10, 10)})
	assert.Empty(t, added)
	assert.Empty(t, removed)
}

func TestCalculateDiff_PlacesIncrease(t *testing.T) {
	old := []alteg.Activity{makeActivity(1, 10, 9)}
	cur := []alteg.Activity{makeActivity(1, 10, 8)}
	added, removed := calculateDiff(old, cur)
	assert.Len(t, added, 1)
	assert.Empty(t, removed)
}

func TestCalculateDiff_PlacesDropToZero(t *testing.T) {
	old := []alteg.Activity{makeActivity(1, 10, 9)}
	cur := []alteg.Activity{makeActivity(1, 10, 10)}
	added, removed := calculateDiff(old, cur)
	assert.Empty(t, added)
	assert.Len(t, removed, 1)
}

func TestCalculateDiff_PlacesDecreaseButNotZero(t *testing.T) {
	old := []alteg.Activity{makeActivity(1, 10, 3)}
	cur := []alteg.Activity{makeActivity(1, 10, 5)}
	added, removed := calculateDiff(old, cur)
	assert.Empty(t, added)
	assert.Empty(t, removed)
}

func TestCalculateDiff_ActivityDisappearsWithPlaces(t *testing.T) {
	old := []alteg.Activity{makeActivity(1, 10, 5)}
	added, removed := calculateDiff(old, nil)
	assert.Empty(t, added)
	assert.Len(t, removed, 1)
}

func TestCalculateDiff_ActivityDisappearsFullyBooked(t *testing.T) {
	old := []alteg.Activity{makeActivity(1, 10, 10)}
	added, removed := calculateDiff(old, nil)
	assert.Empty(t, added)
	assert.Empty(t, removed)
}

func TestCalculateDiff_NoChange(t *testing.T) {
	acts := []alteg.Activity{makeActivity(1, 10, 5)}
	added, removed := calculateDiff(acts, acts)
	assert.Empty(t, added)
	assert.Empty(t, removed)
}
