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

// --- Test helpers ---

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

// countingHandler returns an HTTP handler that counts calls and responds with the given status.
func countingHandler(counter *int32, status int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(counter, 1)
		w.WriteHeader(status)
	}
}

// capturingHandler returns an HTTP handler that captures the request body and
// headers, and a channel that is sent to after the capture is complete.
// The test must receive from the channel before reading body/headers.
func capturingHandler() (http.HandlerFunc, *[]byte, *http.Header, <-chan struct{}) {
	var body []byte
	var headers http.Header
	done := make(chan struct{}, 1)
	handler := func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		headers = r.Header.Clone()
		w.WriteHeader(200)
		select {
		case done <- struct{}{}:
		default:
		}
	}
	return handler, &body, &headers, done
}

// failThenSucceedHandler returns 500 for the first n calls, then 200.
func failThenSucceedHandler(failCount int) (http.HandlerFunc, *int32) {
	var calls int32
	return func(w http.ResponseWriter, r *http.Request) {
		n := int(atomic.AddInt32(&calls, 1))
		if n <= failCount {
			w.WriteHeader(500)
		} else {
			w.WriteHeader(200)
		}
	}, &calls
}

// newTestDispatcher creates a bus, dispatcher with the given webhooks, and returns
// both for test use. The dispatcher uses zero retry delays.
func newTestDispatcher(t *testing.T, webhooks []model.Webhook, logger DeliveryLogger) (*eventbus.EventBus, *Dispatcher) {
	t.Helper()
	bus := eventbus.New()
	lister := &mockLister{webhooks: webhooks}
	d := NewDispatcher(bus, lister, logger, WithRetryDelays([]time.Duration{0, 0, 0}))
	t.Cleanup(d.Stop)
	return bus, d
}

const eventWait = 300 * time.Millisecond
const retryWait = 500 * time.Millisecond

// --- Delivery ---

func TestDeliversMatchingEvent(t *testing.T) {
	// Arrange
	handler, body, _, done := capturingHandler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	logger := &mockLogger{}
	bus, _ := newTestDispatcher(t, []model.Webhook{testWebhook(srv.URL, "secret", []string{"task.created"})}, logger)

	// Act
	bus.Publish(testEvent("task.created"))
	<-done

	// Assert — payload is valid JSON with correct event type.
	if len(*body) == 0 {
		t.Fatal("expected delivery, got none")
	}
	var evt eventbus.Event
	if err := json.Unmarshal(*body, &evt); err != nil {
		t.Fatalf("invalid JSON payload: %v", err)
	}
	if evt.Type != "task.created" {
		t.Errorf("expected event type task.created, got %s", evt.Type)
	}
}

func TestDeliveryHeaders(t *testing.T) {
	// Arrange
	handler, _, headers, done := capturingHandler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	bus, _ := newTestDispatcher(t, []model.Webhook{testWebhook(srv.URL, "secret", []string{"task.created"})}, nil)

	// Act
	bus.Publish(testEvent("task.created"))
	<-done

	// Assert
	if ct := headers.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
	if eh := headers.Get(eventHeader); eh != "task.created" {
		t.Errorf("expected %s header task.created, got %s", eventHeader, eh)
	}
	if sig := headers.Get(signatureHeader); sig == "" {
		t.Error("expected non-empty signature header")
	}
}

func TestDeliverySignature(t *testing.T) {
	// Arrange
	secret := "test-secret"
	handler, body, headers, done := capturingHandler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	bus, _ := newTestDispatcher(t, []model.Webhook{testWebhook(srv.URL, secret, []string{"task.created"})}, nil)

	// Act
	bus.Publish(testEvent("task.created"))
	<-done

	// Assert — receiver can verify the HMAC.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(*body)
	expected := fmt.Sprintf("sha256=%s", hex.EncodeToString(mac.Sum(nil)))
	if got := headers.Get(signatureHeader); got != expected {
		t.Errorf("signature mismatch:\n  got:  %s\n  want: %s", got, expected)
	}
}

func TestDeliveryLogRecord(t *testing.T) {
	// Arrange
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
	}))
	defer srv.Close()

	logger := &mockLogger{}
	wh := testWebhook(srv.URL, "s", []string{"task.created"})
	wh.ID = 42
	bus, _ := newTestDispatcher(t, []model.Webhook{wh}, logger)

	// Act
	bus.Publish(testEvent("task.created"))
	time.Sleep(eventWait)

	// Assert
	deliveries := logger.getDeliveries()
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 logged delivery, got %d", len(deliveries))
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
	if dl.StatusCode == nil || *dl.StatusCode != 201 {
		t.Errorf("expected status 201, got %v", dl.StatusCode)
	}
	if dl.Error != nil {
		t.Errorf("expected no error, got %s", *dl.Error)
	}
	if dl.DurationMs == nil || *dl.DurationMs < 0 {
		t.Errorf("expected non-negative duration, got %v", dl.DurationMs)
	}
	if dl.RequestBody == "" {
		t.Error("expected non-empty request_body")
	}
}

func TestNilLoggerDoesNotPanic(t *testing.T) {
	// Arrange
	srv := httptest.NewServer(countingHandler(new(int32), 200))
	defer srv.Close()

	bus, _ := newTestDispatcher(t, []model.Webhook{testWebhook(srv.URL, "s", []string{"task.created"})}, nil)

	// Act + Assert (no panic)
	bus.Publish(testEvent("task.created"))
	time.Sleep(eventWait)
}

// --- Filtering ---

func TestSkipsNonMatchingEventType(t *testing.T) {
	// Arrange
	var calls int32
	srv := httptest.NewServer(countingHandler(&calls, 200))
	defer srv.Close()

	bus, _ := newTestDispatcher(t, []model.Webhook{testWebhook(srv.URL, "s", []string{"task.transitioned"})}, nil)

	// Act
	bus.Publish(testEvent("task.created"))
	time.Sleep(eventWait)

	// Assert
	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Errorf("expected 0 deliveries for non-matching event, got %d", n)
	}
}

func TestSkipsWrongBoard(t *testing.T) {
	// Arrange
	var calls int32
	srv := httptest.NewServer(countingHandler(&calls, 200))
	defer srv.Close()

	boardSlug := "my-board"
	wh := testWebhook(srv.URL, "s", []string{"task.created"})
	wh.BoardSlug = &boardSlug
	bus, _ := newTestDispatcher(t, []model.Webhook{wh}, nil)

	// Act — event is from "test-board", webhook scoped to "my-board"
	bus.Publish(testEvent("task.created"))
	time.Sleep(eventWait)

	// Assert
	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Errorf("expected 0 deliveries for wrong board, got %d", n)
	}
}

func TestDeliversToMatchingBoard(t *testing.T) {
	// Arrange
	var calls int32
	srv := httptest.NewServer(countingHandler(&calls, 200))
	defer srv.Close()

	boardSlug := "my-board"
	wh := testWebhook(srv.URL, "s", []string{"task.created"})
	wh.BoardSlug = &boardSlug
	bus, _ := newTestDispatcher(t, []model.Webhook{wh}, nil)

	// Act
	evt := testEvent("task.created")
	evt.Board.Slug = "my-board"
	bus.Publish(evt)
	time.Sleep(eventWait)

	// Assert
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("expected 1 delivery for matching board, got %d", n)
	}
}

func TestSkipsInactiveWebhook(t *testing.T) {
	// Arrange
	var calls int32
	srv := httptest.NewServer(countingHandler(&calls, 200))
	defer srv.Close()

	wh := testWebhook(srv.URL, "s", []string{"task.created"})
	wh.Active = false
	bus, _ := newTestDispatcher(t, []model.Webhook{wh}, nil)

	// Act
	bus.Publish(testEvent("task.created"))
	time.Sleep(eventWait)

	// Assert
	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Errorf("expected 0 deliveries for inactive webhook, got %d", n)
	}
}

func TestMultipleWebhooksReceiveSameEvent(t *testing.T) {
	// Arrange
	var calls1, calls2 int32
	srv1 := httptest.NewServer(countingHandler(&calls1, 200))
	defer srv1.Close()
	srv2 := httptest.NewServer(countingHandler(&calls2, 200))
	defer srv2.Close()

	wh1 := testWebhook(srv1.URL, "s1", []string{"task.created"})
	wh1.ID = 1
	wh2 := testWebhook(srv2.URL, "s2", []string{"task.created"})
	wh2.ID = 2
	bus, _ := newTestDispatcher(t, []model.Webhook{wh1, wh2}, nil)

	// Act
	bus.Publish(testEvent("task.created"))
	time.Sleep(eventWait)

	// Assert
	if n := atomic.LoadInt32(&calls1); n != 1 {
		t.Errorf("expected 1 delivery to webhook 1, got %d", n)
	}
	if n := atomic.LoadInt32(&calls2); n != 1 {
		t.Errorf("expected 1 delivery to webhook 2, got %d", n)
	}
}

// --- Retry ---

func TestRetriesOnServerError(t *testing.T) {
	// Arrange — fail twice, succeed on third attempt.
	handler, calls := failThenSucceedHandler(2)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	logger := &mockLogger{}
	bus, _ := newTestDispatcher(t, []model.Webhook{testWebhook(srv.URL, "s", []string{"task.created"})}, logger)

	// Act
	bus.Publish(testEvent("task.created"))
	time.Sleep(retryWait)

	// Assert
	if n := atomic.LoadInt32(calls); n != 3 {
		t.Fatalf("expected 3 attempts, got %d", n)
	}
	deliveries := logger.getDeliveries()
	if len(deliveries) != 3 {
		t.Fatalf("expected 3 logged deliveries, got %d", len(deliveries))
	}
	// First two: error with status 500; third: success with status 200.
	for i := 0; i < 2; i++ {
		if deliveries[i].Error == nil {
			t.Errorf("attempt %d: expected error", i+1)
		}
		if deliveries[i].StatusCode == nil || *deliveries[i].StatusCode != 500 {
			t.Errorf("attempt %d: expected status 500, got %v", i+1, deliveries[i].StatusCode)
		}
	}
	if deliveries[2].Error != nil {
		t.Errorf("attempt 3: expected success, got error %s", *deliveries[2].Error)
	}
}

func TestSuccessStopsRetries(t *testing.T) {
	// Arrange
	var calls int32
	srv := httptest.NewServer(countingHandler(&calls, 200))
	defer srv.Close()

	bus, _ := newTestDispatcher(t, []model.Webhook{testWebhook(srv.URL, "s", []string{"task.created"})}, nil)

	// Act
	bus.Publish(testEvent("task.created"))
	time.Sleep(eventWait)

	// Assert — only 1 attempt, no retries.
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("expected 1 attempt, got %d", n)
	}
}

func TestAllRetriesExhausted(t *testing.T) {
	// Arrange — always fail.
	var calls int32
	srv := httptest.NewServer(countingHandler(&calls, 500))
	defer srv.Close()

	logger := &mockLogger{}
	bus, _ := newTestDispatcher(t, []model.Webhook{testWebhook(srv.URL, "s", []string{"task.created"})}, logger)

	// Act
	bus.Publish(testEvent("task.created"))
	time.Sleep(retryWait)

	// Assert
	if n := atomic.LoadInt32(&calls); n != 3 {
		t.Errorf("expected 3 attempts, got %d", n)
	}
	for _, dl := range logger.getDeliveries() {
		if dl.Error == nil {
			t.Error("expected error on every attempt")
		}
	}
}

func TestRetriesOnConnectionError(t *testing.T) {
	// Arrange — server is already closed.
	srv := httptest.NewServer(countingHandler(new(int32), 200))
	badURL := srv.URL
	srv.Close()

	logger := &mockLogger{}
	bus, _ := newTestDispatcher(t, []model.Webhook{testWebhook(badURL, "s", []string{"task.created"})}, logger)

	// Act
	bus.Publish(testEvent("task.created"))
	time.Sleep(retryWait)

	// Assert
	deliveries := logger.getDeliveries()
	if len(deliveries) != 3 {
		t.Fatalf("expected 3 attempts on connection error, got %d", len(deliveries))
	}
	for _, dl := range deliveries {
		if dl.Error == nil {
			t.Error("expected error on connection failure")
		}
		if dl.StatusCode != nil {
			t.Errorf("expected nil status on connection error, got %d", *dl.StatusCode)
		}
	}
}

// --- Timeout ---

func TestDeliveryTimeout(t *testing.T) {
	// Arrange — endpoint sleeps longer than the client timeout.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(15 * time.Second)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	logger := &mockLogger{}
	bus, d := newTestDispatcher(t, []model.Webhook{testWebhook(srv.URL, "s", []string{"task.created"})}, logger)
	d.client = &http.Client{Timeout: 100 * time.Millisecond}

	// Act
	bus.Publish(testEvent("task.created"))
	time.Sleep(1 * time.Second)

	// Assert
	deliveries := logger.getDeliveries()
	if len(deliveries) != 3 {
		t.Fatalf("expected 3 attempts on timeout, got %d", len(deliveries))
	}
	for _, dl := range deliveries {
		if dl.Error == nil {
			t.Error("expected error on timeout")
		}
		if dl.DurationMs == nil {
			t.Error("expected duration recorded even on timeout")
		}
	}
}

// --- Signature ---

func TestSignatureComputation(t *testing.T) {
	// Arrange
	payload := []byte(`{"event":"task.created"}`)
	secret := "my-secret"

	// Act
	sig := sign(payload, secret)

	// Assert
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := fmt.Sprintf("sha256=%s", hex.EncodeToString(mac.Sum(nil)))
	if sig != expected {
		t.Errorf("signature mismatch:\n  got:  %s\n  want: %s", sig, expected)
	}
}

// --- Lifecycle ---

func TestStopPreventsDelivery(t *testing.T) {
	// Arrange
	var calls int32
	srv := httptest.NewServer(countingHandler(&calls, 200))
	defer srv.Close()

	bus := eventbus.New()
	lister := &mockLister{webhooks: []model.Webhook{testWebhook(srv.URL, "s", []string{"task.created"})}}
	d := NewDispatcher(bus, lister, nil, WithRetryDelays([]time.Duration{0, 0, 0}))

	// Act — stop before publishing.
	d.Stop()
	bus.Publish(testEvent("task.created"))
	time.Sleep(eventWait)

	// Assert
	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Errorf("expected 0 deliveries after stop, got %d", n)
	}
}
