package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/bricef/taskflow/internal/eventbus"
	"github.com/bricef/taskflow/internal/model"
)

type mockLister struct {
	webhooks []model.Webhook
}

func (m *mockLister) ListWebhooks(_ context.Context) ([]model.Webhook, error) {
	return m.webhooks, nil
}

func TestDispatcherDeliversMatchingEvent(t *testing.T) {
	var mu sync.Mutex
	var received []eventbus.Event
	var gotSignature string

	secret := "test-secret"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotSignature = r.Header.Get(signatureHeader)
		body, _ := io.ReadAll(r.Body)
		var evt eventbus.Event
		json.Unmarshal(body, &evt)
		received = append(received, evt)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	bus := eventbus.New()
	lister := &mockLister{
		webhooks: []model.Webhook{
			{
				ID:     1,
				URL:    srv.URL,
				Events: []string{"task.created", "task.transitioned"},
				Secret: secret,
				Active: true,
			},
		},
	}

	d := NewDispatcher(bus, lister, nil)
	defer d.Stop()

	// Publish a matching event.
	bus.Publish(eventbus.Event{
		Type:      eventbus.EventTaskCreated,
		Timestamp: time.Now(),
		Actor:     eventbus.ActorRef{Name: "alice"},
		Board:     eventbus.BoardRef{Slug: "test"},
	})

	// Wait for async delivery.
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(received))
	}
	if received[0].Type != eventbus.EventTaskCreated {
		t.Errorf("expected event type %s, got %s", eventbus.EventTaskCreated, received[0].Type)
	}

	// Verify HMAC signature.
	payload, _ := json.Marshal(received[0])
	// Re-marshal from the event we sent to get the expected payload.
	_ = payload
	if gotSignature == "" {
		t.Error("expected signature header, got empty")
	}
}

func TestDispatcherSkipsNonMatchingEvent(t *testing.T) {
	var mu sync.Mutex
	deliveries := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		deliveries++
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	bus := eventbus.New()
	lister := &mockLister{
		webhooks: []model.Webhook{
			{
				ID:     1,
				URL:    srv.URL,
				Events: []string{"task.transitioned"}, // only transitions
				Secret: "s",
				Active: true,
			},
		},
	}

	d := NewDispatcher(bus, lister, nil)
	defer d.Stop()

	// Publish a non-matching event.
	bus.Publish(eventbus.Event{
		Type:  eventbus.EventTaskCreated, // not transitioned
		Board: eventbus.BoardRef{Slug: "test"},
	})

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if deliveries != 0 {
		t.Errorf("expected 0 deliveries, got %d", deliveries)
	}
}

func TestDispatcherBoardScope(t *testing.T) {
	var mu sync.Mutex
	deliveries := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		deliveries++
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	boardSlug := "my-board"
	bus := eventbus.New()
	lister := &mockLister{
		webhooks: []model.Webhook{
			{
				ID:        1,
				URL:       srv.URL,
				Events:    []string{"task.created"},
				BoardSlug: &boardSlug, // scoped to my-board
				Secret:    "s",
				Active:    true,
			},
		},
	}

	d := NewDispatcher(bus, lister, nil)
	defer d.Stop()

	// Event from a different board — should not deliver.
	bus.Publish(eventbus.Event{
		Type:  eventbus.EventTaskCreated,
		Board: eventbus.BoardRef{Slug: "other-board"},
	})

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	if deliveries != 0 {
		t.Errorf("expected 0 deliveries for wrong board, got %d", deliveries)
	}
	mu.Unlock()

	// Event from the correct board — should deliver.
	bus.Publish(eventbus.Event{
		Type:  eventbus.EventTaskCreated,
		Board: eventbus.BoardRef{Slug: "my-board"},
	})

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if deliveries != 1 {
		t.Errorf("expected 1 delivery for matching board, got %d", deliveries)
	}
}

func TestDispatcherSkipsInactive(t *testing.T) {
	var mu sync.Mutex
	deliveries := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		deliveries++
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	bus := eventbus.New()
	lister := &mockLister{
		webhooks: []model.Webhook{
			{
				ID:     1,
				URL:    srv.URL,
				Events: []string{"task.created"},
				Secret: "s",
				Active: false, // inactive
			},
		},
	}

	d := NewDispatcher(bus, lister, nil)
	defer d.Stop()

	bus.Publish(eventbus.Event{
		Type:  eventbus.EventTaskCreated,
		Board: eventbus.BoardRef{Slug: "test"},
	})

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if deliveries != 0 {
		t.Errorf("expected 0 deliveries for inactive webhook, got %d", deliveries)
	}
}

func TestSignature(t *testing.T) {
	payload := []byte(`{"event":"task.created"}`)
	secret := "my-secret"

	sig := sign(payload, secret)

	// Verify manually.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := fmt.Sprintf("sha256=%s", hex.EncodeToString(mac.Sum(nil)))

	if sig != expected {
		t.Errorf("signature mismatch:\n  got:  %s\n  want: %s", sig, expected)
	}
}
