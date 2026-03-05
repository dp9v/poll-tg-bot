package bot

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go-notification-tg-bot/internal/alteg"
)

// ── Telegram stub ─────────────────────────────────────────────────────────────

type commandsTGStub struct {
	mu       sync.Mutex
	messages []string          // texts sent via sendMessage
	updates  []tgbotapi.Update // updates to deliver via getUpdates
	updateID int
	srv      *httptest.Server
}

func newCommandsTGStub(t *testing.T) *commandsTGStub {
	t.Helper()
	stub := &commandsTGStub{}
	mux := http.NewServeMux()

	mux.HandleFunc("/botTEST_TOKEN/getMe", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"result": map[string]any{
				"id": 1, "is_bot": true,
				"first_name": "TestBot", "username": "test_bot",
			},
		})
	})

	mux.HandleFunc("/botTEST_TOKEN/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		stub.mu.Lock()
		stub.messages = append(stub.messages, r.FormValue("text"))
		stub.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"result": map[string]any{"message_id": 1},
		})
	})

	mux.HandleFunc("/botTEST_TOKEN/getUpdates", func(w http.ResponseWriter, r *http.Request) {
		stub.mu.Lock()
		pending := stub.updates
		stub.updates = nil
		stub.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"result": pending,
		})
	})

	stub.srv = httptest.NewServer(mux)
	t.Cleanup(stub.srv.Close)
	return stub
}

// push enqueues a text message from the configured chat to be delivered on the next getUpdates poll.
func (s *commandsTGStub) push(chatID int64, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateID++
	s.updates = append(s.updates, tgbotapi.Update{
		UpdateID: s.updateID,
		Message: &tgbotapi.Message{
			MessageID: s.updateID,
			Chat:      &tgbotapi.Chat{ID: chatID},
			Text:      text,
		},
	})
}

func (s *commandsTGStub) captured() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.messages))
	copy(out, s.messages)
	return out
}

// ── helpers ───────────────────────────────────────────────────────────────────

const testChatID int64 = 12345

func setupSender(t *testing.T) (*Sender, *commandsTGStub) {
	t.Helper()
	stub := newCommandsTGStub(t)
	api, err := tgbotapi.NewBotAPIWithAPIEndpoint("TEST_TOKEN", stub.srv.URL+"/bot%s/%s")
	require.NoError(t, err)
	return NewSender(api, testChatID), stub
}

// runListener starts ListenCommands in a goroutine and returns a cancel func
// that stops it and waits for it to exit.
func runListener(sender *Sender, fetch ActivitiesFetcher) (cancel func()) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		sender.listenCommandsCtx(ctx, fetch)
	}()
	return func() {
		cancelCtx()
		<-done
	}
}

// ── tests ─────────────────────────────────────────────────────────────────────

// TestListenCommands_ButtonShowActivities verifies that pressing the button
// triggers a fetch and sends the available-activities message.
func TestListenCommands_ButtonShowActivities(t *testing.T) {
	sender, stub := setupSender(t)

	acts := []alteg.Activity{
		{ID: 1, Date: "2026-04-01 10:00:00", Capacity: 10, RecordsCount: 3,
			Staff: alteg.Staff{Name: "Coach"}, Service: alteg.Service{Title: "Yoga"}},
	}
	fetch := func() ([]alteg.Activity, error) { return acts, nil }

	stub.push(testChatID, BtnShowActivities)

	cancel := runListener(sender, fetch)
	defer cancel()

	require.Eventually(t, func() bool {
		return len(stub.captured()) >= 1
	}, 3*time.Second, 50*time.Millisecond)

	msgs := stub.captured()
	assert.Contains(t, msgs[0], "Available activities")
}

// TestListenCommands_SlashActivities verifies that /activities command works the same as the button.
func TestListenCommands_SlashActivities(t *testing.T) {
	sender, stub := setupSender(t)

	acts := []alteg.Activity{
		{ID: 2, Date: "2026-04-02 12:00:00", Capacity: 5, RecordsCount: 1,
			Staff: alteg.Staff{Name: "Bob"}, Service: alteg.Service{Title: "Pilates"}},
	}
	fetch := func() ([]alteg.Activity, error) { return acts, nil }

	stub.push(testChatID, "/activities")

	cancel := runListener(sender, fetch)
	defer cancel()

	require.Eventually(t, func() bool {
		return len(stub.captured()) >= 1
	}, 3*time.Second, 50*time.Millisecond)

	assert.Contains(t, stub.captured()[0], "Available activities")
}

// TestListenCommands_NoAvailablePlaces verifies that when all activities are fully booked
// the response still sends a message (with "No available" text).
func TestListenCommands_NoAvailablePlaces(t *testing.T) {
	sender, stub := setupSender(t)

	acts := []alteg.Activity{
		{ID: 3, Date: "2026-04-03 09:00:00", Capacity: 5, RecordsCount: 5,
			Staff: alteg.Staff{Name: "Alice"}, Service: alteg.Service{Title: "Boxing"}},
	}
	fetch := func() ([]alteg.Activity, error) { return acts, nil }

	stub.push(testChatID, BtnShowActivities)

	cancel := runListener(sender, fetch)
	defer cancel()

	require.Eventually(t, func() bool {
		return len(stub.captured()) >= 1
	}, 3*time.Second, 50*time.Millisecond)

	assert.Contains(t, stub.captured()[0], "No available")
}

// TestListenCommands_FetchError verifies that a fetch error sends an error message to the user.
func TestListenCommands_FetchError(t *testing.T) {
	sender, stub := setupSender(t)

	fetchErr := errors.New("api is down")
	fetch := func() ([]alteg.Activity, error) { return nil, fetchErr }

	stub.push(testChatID, BtnShowActivities)

	cancel := runListener(sender, fetch)
	defer cancel()

	require.Eventually(t, func() bool {
		return len(stub.captured()) >= 1
	}, 3*time.Second, 50*time.Millisecond)

	assert.Contains(t, stub.captured()[0], "Error")
}

// TestListenCommands_UnknownMessage verifies that unknown messages are silently ignored.
func TestListenCommands_UnknownMessage(t *testing.T) {
	sender, stub := setupSender(t)

	fetch := func() ([]alteg.Activity, error) { return nil, nil }

	stub.push(testChatID, "hello bot")
	stub.push(testChatID, "/start")
	stub.push(testChatID, "some random text")

	cancel := runListener(sender, fetch)
	defer cancel()

	// Give the listener time to process the messages.
	time.Sleep(300 * time.Millisecond)

	assert.Empty(t, stub.captured(), "unknown messages should not produce any reply")
}

// TestListenCommands_WrongChat verifies that messages from other chats are ignored.
func TestListenCommands_WrongChat(t *testing.T) {
	sender, stub := setupSender(t)

	fetch := func() ([]alteg.Activity, error) { return nil, nil }

	const otherChatID int64 = 99999
	stub.push(otherChatID, BtnShowActivities)

	cancel := runListener(sender, fetch)
	defer cancel()

	time.Sleep(300 * time.Millisecond)

	assert.Empty(t, stub.captured(), "messages from other chats should be ignored")
}

// TestListenCommands_MultipleCommands verifies that multiple sequential button presses
// each trigger a separate response.
func TestListenCommands_MultipleCommands(t *testing.T) {
	sender, stub := setupSender(t)

	acts := []alteg.Activity{
		{ID: 1, Date: "2026-04-01 10:00:00", Capacity: 10, RecordsCount: 2,
			Staff: alteg.Staff{Name: "Coach"}, Service: alteg.Service{Title: "Yoga"}},
	}
	fetch := func() ([]alteg.Activity, error) { return acts, nil }

	stub.push(testChatID, BtnShowActivities)
	stub.push(testChatID, BtnShowActivities)
	stub.push(testChatID, BtnShowActivities)

	cancel := runListener(sender, fetch)
	defer cancel()

	require.Eventually(t, func() bool {
		return len(stub.captured()) >= 3
	}, 3*time.Second, 50*time.Millisecond)

	assert.Len(t, stub.captured(), 3)
}
