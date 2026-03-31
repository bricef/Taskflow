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
	"sync/atomic"
	"testing"
	"time"

	"github.com/bricef/taskflow/internal/eventbus"
	"github.com/bricef/taskflow/internal/model"
)

// --- Mock helpers ---

type mockLister struct {
	webhooks []model.Webhook
}

func (m *mockLister) ListWebhooks(_ context.Context) ([]model.Webhook, error) {
	return m.webhooks, nil
}

type mockLogger struct {
	mu         sync.Mutex
	deliveries []model.WebhookDelivery
}

func (m *mockLogger) WebhookDeliveryInsert(_ context.Context, d model.WebhookDelivery) (model.WebhookDelivery, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d.ID = len(m.deliveries) + 1
	m.deliveries = append(m.deliveries, d)
	return d, nil
}

func (m *mockLogger) getDeliveries() []model.WebhookDelivery {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]model.WebhookDelivery, len(m.deliveries))
	copy(result, m.deliveries)
	return result
}

func noRetryDelays() Option {
	return WithRetryDelays([]time.Duration{0, 0, 0})
}

func testEvent(eventType string) eventbus.Event {
	return eventbus.Event{
		Type:      eventType,
		Timestamp: time.Now(),
		Actor:     eventbus.ActorRef{Name: "alice", Type: "human"},
		Board:     eventbus.BoardRef{Slug: "test-board"},
		After:     &eventbus.TaskSnapshot{Ref: "test-board/1", Num: 1, Title: "Test", State: "backlog"},
	}
}

func testWebhook(url, secret string, events []string) model.Webhook {
	return model.Webhook{
		ID:     1,
		URL:    url,
		Events: events,
		Secret: secret,
		Active: true,
	}
}

// --- Delivery tests ---

func TestDeliverMatchingEvent(t *testing.T) {
	var mu sync.Mutex
	var gotBody []byte
	var gotHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotBody, _ = io.ReadAll(r.Body)
		gotHeaders = r.Header.Clone()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	bus := eventbus.New()
	logger := &mockLogger{}
	lister := &mockLister{webhooks: []model.Webhook{testWebhook(srv.URL, "secret", []string{"task.created"})}}

	d := NewDispatcher(bus, lister, logger, noRetryDelays())
	defer d.Stop()

	bus.Publish(testEvent("task.created"))
	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Verify body is valid JSON event.
	if len(gotBody) == 0 {
		t.Fatal("expected delivery, got none")
	}
	var evt eventbus.Event
	if err := json.Unmarshal(gotBody, &evt); err != nil {
		t.Fatalf("invalid JSON payload: %v", err)
	}
	if evt.Type != "task.created" {
		t.Errorf("expected event type task.created, got %s", evt.Type)
	}

	// Verify headers.
	if ct := gotHeaders.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
	if eh := gotHeaders.Get(eventHeader); eh != "task.created" {
		t.Errorf("expected %s header task.created, got %s", eventHeader, eh)
	}

	// Verify HMAC signature.
	sig := gotHeaders.Get(signatureHeader)
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write(gotBody)
	expected := fmt.Sprintf("sha256=%s", hex.EncodeToString(mac.Sum(nil)))
	if sig != expected {
		t.Errorf("signature mismatch:\n  got:  %s\n  want: %s", sig, expected)
	}

	// Verify delivery was logged.
	deliveries := logger.getDeliveries()
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 logged delivery, got %d", len(deliveries))
	}
	dl := deliveries[0]
	if dl.Attempt != 1 {
		t.Errorf("expected attempt 1, got %d", dl.Attempt)
	}
	if dl.StatusCode == nil || *dl.StatusCode != 200 {
		t.Errorf("expected status 200, got %v", dl.StatusCode)
	}
	if dl.Error != nil {
		t.Errorf("expected no error, got %s", *dl.Error)
	}
	if dl.DurationMs == nil || *dl.DurationMs < 0 {
		t.Errorf("expected positive duration, got %v", dl.DurationMs)
	}
	if dl.EventType != "task.created" {
		t.Errorf("expected event type task.created, got %s", dl.EventType)
	}
}

func TestSkipNonMatchingEvent(t *testing.T) {
	var deliveries int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&deliveries, 1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	bus := eventbus.New()
	lister := &mockLister{webhooks: []model.Webhook{testWebhook(srv.URL, "s", []string{"task.transitioned"})}}

	d := NewDispatcher(bus, lister, nil, noRetryDelays())
	defer d.Stop()

	bus.Publish(testEvent("task.created")) // doesn't match
	time.Sleep(200 * time.Millisecond)

	if n := atomic.LoadInt32(&deliveries); n != 0 {
		t.Errorf("expected 0 deliveries, got %d", n)
	}
}

func TestBoardScopeFiltering(t *testing.T) {
	var deliveries int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&deliveries, 1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	boardSlug := "my-board"
	wh := testWebhook(srv.URL, "s", []string{"task.created"})
	wh.BoardSlug = &boardSlug

	bus := eventbus.New()
	lister := &mockLister{webhooks: []model.Webhook{wh}}

	d := NewDispatcher(bus, lister, nil, noRetryDelays())
	defer d.Stop()

	// Wrong board — should not deliver.
	bus.Publish(testEvent("task.created")) // board is "test-board"
	time.Sleep(200 * time.Millisecond)
	if n := atomic.LoadInt32(&deliveries); n != 0 {
		t.Errorf("expected 0 deliveries for wrong board, got %d", n)
	}

	// Correct board — should deliver.
	evt := testEvent("task.created")
	evt.Board.Slug = "my-board"
	bus.Publish(evt)
	time.Sleep(200 * time.Millisecond)
	if n := atomic.LoadInt32(&deliveries); n != 1 {
		t.Errorf("expected 1 delivery for matching board, got %d", n)
	}
}

func TestSkipInactiveWebhook(t *testing.T) {
	var deliveries int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&deliveries, 1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	wh := testWebhook(srv.URL, "s", []string{"task.created"})
	wh.Active = false

	bus := eventbus.New()
	lister := &mockLister{webhooks: []model.Webhook{wh}}

	d := NewDispatcher(bus, lister, nil, noRetryDelays())
	defer d.Stop()

	bus.Publish(testEvent("task.created"))
	time.Sleep(200 * time.Millisecond)

	if n := atomic.LoadInt32(&deliveries); n != 0 {
		t.Errorf("expected 0 deliveries for inactive webhook, got %d", n)
	}
}

// --- Retry tests ---

func TestRetryOn5xx(t *testing.T) {
	var mu sync.Mutex
	var attempts int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts++
		a := attempts
		mu.Unlock()
		if a < 3 {
			w.WriteHeader(500)
		} else {
			w.WriteHeader(200)
		}
	}))
	defer srv.Close()

	bus := eventbus.New()
	logger := &mockLogger{}
	lister := &mockLister{webhooks: []model.Webhook{testWebhook(srv.URL, "s", []string{"task.created"})}}

	d := NewDispatcher(bus, lister, logger, noRetryDelays())
	defer d.Stop()

	bus.Publish(testEvent("task.created"))
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}

	// Verify delivery log has all 3 attempts.
	deliveries := logger.getDeliveries()
	if len(deliveries) != 3 {
		t.Fatalf("expected 3 logged deliveries, got %d", len(deliveries))
	}

	// First two should be failures.
	for i := 0; i < 2; i++ {
		if deliveries[i].Error == nil {
			t.Errorf("attempt %d: expected error, got nil", i+1)
		}
		if deliveries[i].StatusCode == nil || *deliveries[i].StatusCode != 500 {
			t.Errorf("attempt %d: expected status 500, got %v", i+1, deliveries[i].StatusCode)
		}
		if deliveries[i].Attempt != i+1 {
			t.Errorf("expected attempt %d, got %d", i+1, deliveries[i].Attempt)
		}
	}

	// Third should succeed.
	if deliveries[2].Error != nil {
		t.Errorf("attempt 3: expected no error, got %s", *deliveries[2].Error)
	}
	if deliveries[2].StatusCode == nil || *deliveries[2].StatusCode != 200 {
		t.Errorf("attempt 3: expected status 200, got %v", deliveries[2].StatusCode)
	}
}

func TestRetryStopsOnSuccess(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	bus := eventbus.New()
	lister := &mockLister{webhooks: []model.Webhook{testWebhook(srv.URL, "s", []string{"task.created"})}}

	d := NewDispatcher(bus, lister, nil, noRetryDelays())
	defer d.Stop()

	bus.Publish(testEvent("task.created"))
	time.Sleep(300 * time.Millisecond)

	if n := atomic.LoadInt32(&attempts); n != 1 {
		t.Errorf("expected 1 attempt (success, no retry), got %d", n)
	}
}

func TestAllRetriesExhausted(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(500)
	}))
	defer srv.Close()

	bus := eventbus.New()
	logger := &mockLogger{}
	lister := &mockLister{webhooks: []model.Webhook{testWebhook(srv.URL, "s", []string{"task.created"})}}

	d := NewDispatcher(bus, lister, logger, noRetryDelays())
	defer d.Stop()

	bus.Publish(testEvent("task.created"))
	time.Sleep(500 * time.Millisecond)

	if n := atomic.LoadInt32(&attempts); n != 3 {
		t.Errorf("expected 3 attempts (all exhausted), got %d", n)
	}

	deliveries := logger.getDeliveries()
	if len(deliveries) != 3 {
		t.Fatalf("expected 3 logged deliveries, got %d", len(deliveries))
	}
	for _, dl := range deliveries {
		if dl.Error == nil {
			t.Error("expected error on every attempt")
		}
	}
}

func TestRetryOnConnectionError(t *testing.T) {
	// Use a server that immediately closes.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	badURL := srv.URL
	srv.Close() // close immediately so connection fails

	bus := eventbus.New()
	logger := &mockLogger{}
	lister := &mockLister{webhooks: []model.Webhook{testWebhook(badURL, "s", []string{"task.created"})}}

	d := NewDispatcher(bus, lister, logger, noRetryDelays())
	defer d.Stop()

	bus.Publish(testEvent("task.created"))
	time.Sleep(500 * time.Millisecond)

	deliveries := logger.getDeliveries()
	if len(deliveries) != 3 {
		t.Fatalf("expected 3 delivery attempts on connection error, got %d", len(deliveries))
	}
	for _, dl := range deliveries {
		if dl.Error == nil {
			t.Error("expected error on connection failure")
		}
		if dl.StatusCode != nil {
			t.Errorf("expected nil status code on connection error, got %d", *dl.StatusCode)
		}
	}
}

// --- Delivery logging tests ---

func TestDeliveryLogFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
	}))
	defer srv.Close()

	bus := eventbus.New()
	logger := &mockLogger{}
	wh := testWebhook(srv.URL, "s", []string{"task.created"})
	wh.ID = 42
	lister := &mockLister{webhooks: []model.Webhook{wh}}

	d := NewDispatcher(bus, lister, logger, noRetryDelays())
	defer d.Stop()

	bus.Publish(testEvent("task.created"))
	time.Sleep(300 * time.Millisecond)

	deliveries := logger.getDeliveries()
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}

	dl := deliveries[0]
	if dl.WebhookID != 42 {
		t.Errorf("expected webhook_id 42, got %d", dl.WebhookID)
	}
	if dl.EventType != "task.created" {
		t.Errorf("expected event_type task.created, got %s", dl.EventType)
	}
	if dl.EventID == "" {
		t.Error("expected non-empty event_id")
	}
	if dl.Attempt != 1 {
		t.Errorf("expected attempt 1, got %d", dl.Attempt)
	}
	if dl.RequestBody == "" {
		t.Error("expected non-empty request_body")
	}

	// Verify request body is valid JSON.
	var evt eventbus.Event
	if err := json.Unmarshal([]byte(dl.RequestBody), &evt); err != nil {
		t.Errorf("request_body is not valid JSON: %v", err)
	}
}

func TestDeliveryLoggingWithNilLogger(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	bus := eventbus.New()
	lister := &mockLister{webhooks: []model.Webhook{testWebhook(srv.URL, "s", []string{"task.created"})}}

	// nil logger should not panic.
	d := NewDispatcher(bus, lister, nil, noRetryDelays())
	defer d.Stop()

	bus.Publish(testEvent("task.created"))
	time.Sleep(200 * time.Millisecond)
	// No panic = pass.
}

// --- Multiple webhooks ---

func TestMultipleWebhooksReceiveSameEvent(t *testing.T) {
	var mu sync.Mutex
	received := map[string]int{}

	handler := func(name string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			received[name]++
			mu.Unlock()
			w.WriteHeader(200)
		}
	}

	srv1 := httptest.NewServer(handler("wh1"))
	defer srv1.Close()
	srv2 := httptest.NewServer(handler("wh2"))
	defer srv2.Close()

	wh1 := testWebhook(srv1.URL, "s1", []string{"task.created"})
	wh1.ID = 1
	wh2 := testWebhook(srv2.URL, "s2", []string{"task.created"})
	wh2.ID = 2

	bus := eventbus.New()
	logger := &mockLogger{}
	lister := &mockLister{webhooks: []model.Webhook{wh1, wh2}}

	d := NewDispatcher(bus, lister, logger, noRetryDelays())
	defer d.Stop()

	bus.Publish(testEvent("task.created"))
	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if received["wh1"] != 1 {
		t.Errorf("expected 1 delivery to wh1, got %d", received["wh1"])
	}
	if received["wh2"] != 1 {
		t.Errorf("expected 1 delivery to wh2, got %d", received["wh2"])
	}

	deliveries := logger.getDeliveries()
	if len(deliveries) != 2 {
		t.Errorf("expected 2 logged deliveries, got %d", len(deliveries))
	}
}

// --- Timeout test ---

func TestDeliveryTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(15 * time.Second) // longer than deliveryTimeout
		w.WriteHeader(200)
	}))
	defer srv.Close()

	bus := eventbus.New()
	logger := &mockLogger{}
	lister := &mockLister{webhooks: []model.Webhook{testWebhook(srv.URL, "s", []string{"task.created"})}}

	// Use a dispatcher with a short timeout for testing.
	d := NewDispatcher(bus, lister, logger, noRetryDelays())
	d.client = &http.Client{Timeout: 100 * time.Millisecond}
	defer d.Stop()

	bus.Publish(testEvent("task.created"))
	time.Sleep(1 * time.Second)

	deliveries := logger.getDeliveries()
	if len(deliveries) != 3 {
		t.Fatalf("expected 3 delivery attempts on timeout, got %d", len(deliveries))
	}
	for _, dl := range deliveries {
		if dl.Error == nil {
			t.Error("expected error on timeout")
		}
		if dl.DurationMs == nil {
			t.Error("expected duration to be recorded even on timeout")
		}
	}
}

// --- Signature tests ---

func TestSignatureComputation(t *testing.T) {
	payload := []byte(`{"event":"task.created","timestamp":"2026-01-01T00:00:00Z"}`)
	secret := "my-secret"

	sig := sign(payload, secret)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := fmt.Sprintf("sha256=%s", hex.EncodeToString(mac.Sum(nil)))

	if sig != expected {
		t.Errorf("signature mismatch:\n  got:  %s\n  want: %s", sig, expected)
	}
}

func TestSignatureVerification(t *testing.T) {
	// Simulate a receiver verifying the signature.
	var gotSig string
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get(signatureHeader)
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	secret := "verification-test-secret"
	bus := eventbus.New()
	lister := &mockLister{webhooks: []model.Webhook{testWebhook(srv.URL, secret, []string{"task.created"})}}

	d := NewDispatcher(bus, lister, nil, noRetryDelays())
	defer d.Stop()

	bus.Publish(testEvent("task.created"))
	time.Sleep(300 * time.Millisecond)

	// Receiver verifies: compute HMAC of body with shared secret.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(gotBody)
	expected := fmt.Sprintf("sha256=%s", hex.EncodeToString(mac.Sum(nil)))

	if gotSig != expected {
		t.Errorf("receiver could not verify signature:\n  got:  %s\n  want: %s", gotSig, expected)
	}
}

// --- Dispatcher lifecycle ---

func TestStopPreventsDelivery(t *testing.T) {
	var deliveries int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&deliveries, 1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	bus := eventbus.New()
	lister := &mockLister{webhooks: []model.Webhook{testWebhook(srv.URL, "s", []string{"task.created"})}}

	d := NewDispatcher(bus, lister, nil, noRetryDelays())
	d.Stop()

	bus.Publish(testEvent("task.created"))
	time.Sleep(200 * time.Millisecond)

	if n := atomic.LoadInt32(&deliveries); n != 0 {
		t.Errorf("expected 0 deliveries after stop, got %d", n)
	}
}
