package service_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/taskflow"
	"github.com/bricef/taskflow/internal/testutil"
	"github.com/bricef/taskflow/internal/webhook"
)

// seedWebhookEnv creates a service with an admin actor and returns
// a helper to create webhooks with minimal boilerplate.
func seedWebhookEnv(t *testing.T) (taskflow.TaskFlow, func(url string, events []string) model.Webhook) {
	t.Helper()
	svc := testutil.NewTestService(t)
	testutil.SeedActor(t, svc, "brice", model.RoleAdmin)

	create := func(url string, events []string) model.Webhook {
		t.Helper()
		wh, err := svc.CreateWebhook(context.Background(), model.CreateWebhookParams{
			URL: url, Events: events, Secret: "test-secret", CreatedBy: "brice",
		})
		if err != nil {
			t.Fatalf("failed to create webhook: %v", err)
		}
		return wh
	}
	return svc, create
}

// --- CRUD ---

func TestCreateWebhook(t *testing.T) {
	// Arrange
	svc, _ := seedWebhookEnv(t)

	// Act
	wh, err := svc.CreateWebhook(context.Background(), model.CreateWebhookParams{
		URL:       "https://example.com/hook",
		Events:    []string{"task.created", "task.transitioned"},
		Secret:    "secret123",
		CreatedBy: "brice",
	})

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wh.URL != "https://example.com/hook" {
		t.Errorf("expected URL, got %s", wh.URL)
	}
	if len(wh.Events) != 2 {
		t.Errorf("expected 2 events, got %d", len(wh.Events))
	}
	if wh.BoardSlug != nil {
		t.Error("expected nil board_slug for global webhook")
	}
	if !wh.Active {
		t.Error("expected webhook to be active by default")
	}
}

func TestCreateWebhookWithBoardScope(t *testing.T) {
	// Arrange
	svc, _ := seedWebhookEnv(t)
	testutil.SeedBoard(t, svc, "my-board")
	slug := "my-board"

	// Act
	wh, err := svc.CreateWebhook(context.Background(), model.CreateWebhookParams{
		URL: "https://example.com/hook", Events: []string{"task.created"},
		BoardSlug: &slug, Secret: "secret", CreatedBy: "brice",
	})

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wh.BoardSlug == nil || *wh.BoardSlug != "my-board" {
		t.Errorf("expected board_slug my-board, got %v", wh.BoardSlug)
	}
}

func TestListWebhooks(t *testing.T) {
	// Arrange
	svc, create := seedWebhookEnv(t)
	create("https://a.com", []string{"e1"})
	create("https://b.com", []string{"e2"})

	// Act
	webhooks, err := svc.ListWebhooks(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(webhooks) != 2 {
		t.Fatalf("expected 2 webhooks, got %d", len(webhooks))
	}
}

func TestUpdateWebhookURL(t *testing.T) {
	// Arrange
	svc, create := seedWebhookEnv(t)
	wh := create("https://old.com", []string{"e1"})

	// Act
	updated, err := svc.UpdateWebhook(context.Background(), model.UpdateWebhookParams{
		ID: wh.ID, URL: model.Set("https://new.com"),
	})

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.URL != "https://new.com" {
		t.Errorf("expected new URL, got %s", updated.URL)
	}
}

func TestUpdateWebhookEvents(t *testing.T) {
	// Arrange
	svc, create := seedWebhookEnv(t)
	wh := create("https://x.com", []string{"e1"})

	// Act
	updated, err := svc.UpdateWebhook(context.Background(), model.UpdateWebhookParams{
		ID: wh.ID, Events: model.Set([]string{"e2", "e3"}),
	})

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updated.Events) != 2 || updated.Events[0] != "e2" {
		t.Errorf("unexpected events: %v", updated.Events)
	}
}

func TestUpdateWebhookActive(t *testing.T) {
	// Arrange
	svc, create := seedWebhookEnv(t)
	wh := create("https://x.com", []string{"e1"})

	// Act
	updated, err := svc.UpdateWebhook(context.Background(), model.UpdateWebhookParams{
		ID: wh.ID, Active: model.Set(false),
	})

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Active {
		t.Error("expected webhook to be inactive")
	}
}

func TestDeleteWebhook(t *testing.T) {
	// Arrange
	svc, create := seedWebhookEnv(t)
	wh := create("https://x.com", []string{"e1"})

	// Act
	err := svc.DeleteWebhook(context.Background(), wh.ID)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	webhooks, _ := svc.ListWebhooks(context.Background())
	if len(webhooks) != 0 {
		t.Errorf("expected 0 webhooks after delete, got %d", len(webhooks))
	}
}

// --- Delivery listing ---

func TestListWebhookDeliveries(t *testing.T) {
	// Arrange
	svc, create := seedWebhookEnv(t)
	wh := create("https://x.com", []string{"e1"})

	// Act + Assert — empty initially.
	deliveries, err := svc.ListWebhookDeliveries(context.Background(), wh.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deliveries) != 0 {
		t.Errorf("expected 0 deliveries, got %d", len(deliveries))
	}
}

func TestListWebhookDeliveriesNotFound(t *testing.T) {
	// Arrange
	svc, _ := seedWebhookEnv(t)

	// Act
	_, err := svc.ListWebhookDeliveries(context.Background(), 9999)

	// Assert
	if err == nil {
		t.Error("expected error for non-existent webhook")
	}
}

// --- Integration: event → dispatch → delivery log ---

func TestWebhookIntegration(t *testing.T) {
	// Arrange
	svc, store, bus := testutil.NewTestServiceWithBus(t)
	testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "test-board")

	var endpointCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&endpointCalls, 1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	wh, _ := svc.CreateWebhook(context.Background(), model.CreateWebhookParams{
		URL: srv.URL, Events: []string{"task.created"}, Secret: "s", CreatedBy: "brice",
	})
	dispatcher := webhook.NewDispatcher(bus, svc, store,
		webhook.WithRetryDelays([]time.Duration{0, 0, 0}))
	defer dispatcher.Stop()

	// Act — create a task, which publishes an event.
	_, err := svc.CreateTask(context.Background(), model.CreateTaskParams{
		BoardSlug: "test-board", Title: "Integration test", CreatedBy: "brice",
	})
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	// Assert — endpoint was called.
	if n := atomic.LoadInt32(&endpointCalls); n != 1 {
		t.Fatalf("expected 1 delivery, got %d", n)
	}

	// Assert — delivery was logged in the database.
	logged, err := svc.ListWebhookDeliveries(context.Background(), wh.ID)
	if err != nil {
		t.Fatalf("failed to list deliveries: %v", err)
	}
	if len(logged) != 1 {
		t.Fatalf("expected 1 logged delivery, got %d", len(logged))
	}
	dl := logged[0]
	if dl.WebhookID != wh.ID {
		t.Errorf("expected webhook_id %d, got %d", wh.ID, dl.WebhookID)
	}
	if dl.EventType != "task.created" {
		t.Errorf("expected event_type task.created, got %s", dl.EventType)
	}
	if dl.StatusCode == nil || *dl.StatusCode != 200 {
		t.Errorf("expected status 200, got %v", dl.StatusCode)
	}
	if dl.Error != nil {
		t.Errorf("expected no error, got %s", *dl.Error)
	}
}

func TestWebhookIntegrationRetry(t *testing.T) {
	// Arrange
	svc, store, bus := testutil.NewTestServiceWithBus(t)
	testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "test-board")

	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n := atomic.AddInt32(&attempts, 1); n < 3 {
			w.WriteHeader(502)
		} else {
			w.WriteHeader(200)
		}
	}))
	defer srv.Close()

	wh, _ := svc.CreateWebhook(context.Background(), model.CreateWebhookParams{
		URL: srv.URL, Events: []string{"task.created"}, Secret: "s", CreatedBy: "brice",
	})
	dispatcher := webhook.NewDispatcher(bus, svc, store,
		webhook.WithRetryDelays([]time.Duration{0, 0, 0}))
	defer dispatcher.Stop()

	// Act
	_, _ = svc.CreateTask(context.Background(), model.CreateTaskParams{
		BoardSlug: "test-board", Title: "Retry test", CreatedBy: "brice",
	})
	time.Sleep(500 * time.Millisecond)

	// Assert — 3 attempts total.
	if n := atomic.LoadInt32(&attempts); n != 3 {
		t.Fatalf("expected 3 attempts, got %d", n)
	}

	// Assert — all 3 logged with correct success/fail counts.
	logged, _ := svc.ListWebhookDeliveries(context.Background(), wh.ID)
	if len(logged) != 3 {
		t.Fatalf("expected 3 logged deliveries, got %d", len(logged))
	}
	attemptsSeen := map[int]bool{}
	var successes, failures int
	for _, dl := range logged {
		attemptsSeen[dl.Attempt] = true
		if dl.Error == nil {
			successes++
		} else {
			failures++
		}
	}
	for _, a := range []int{1, 2, 3} {
		if !attemptsSeen[a] {
			t.Errorf("missing attempt %d in delivery log", a)
		}
	}
	if successes != 1 {
		t.Errorf("expected 1 success, got %d", successes)
	}
	if failures != 2 {
		t.Errorf("expected 2 failures, got %d", failures)
	}
}
