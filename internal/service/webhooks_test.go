package service_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/testutil"
	"github.com/bricef/taskflow/internal/webhook"
)

func TestCreateWebhook(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	testutil.SeedActor(t, svc, "brice", model.RoleAdmin)

	w, err := svc.CreateWebhook(ctx, model.CreateWebhookParams{
		URL:       "https://example.com/hook",
		Events:    []string{"task.created", "task.transitioned"},
		Secret:    "secret123",
		CreatedBy: "brice",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.URL != "https://example.com/hook" {
		t.Errorf("expected URL, got %s", w.URL)
	}
	if len(w.Events) != 2 {
		t.Errorf("expected 2 events, got %d", len(w.Events))
	}
	if w.BoardSlug != nil {
		t.Error("expected nil board_slug for global webhook")
	}
	if !w.Active {
		t.Error("expected webhook to be active by default")
	}
}

func TestCreateWebhookWithBoardScope(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")

	slug := "my-board"
	w, err := svc.CreateWebhook(ctx, model.CreateWebhookParams{
		URL:       "https://example.com/hook",
		Events:    []string{"task.created"},
		BoardSlug: &slug,
		Secret:    "secret",
		CreatedBy: "brice",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.BoardSlug == nil || *w.BoardSlug != "my-board" {
		t.Errorf("expected board_slug my-board, got %v", w.BoardSlug)
	}
}

func TestListWebhooks(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	testutil.SeedActor(t, svc, "brice", model.RoleAdmin)

	svc.CreateWebhook(ctx, model.CreateWebhookParams{
		URL: "https://a.com", Events: []string{"e1"}, Secret: "s1", CreatedBy: "brice",
	})
	svc.CreateWebhook(ctx, model.CreateWebhookParams{
		URL: "https://b.com", Events: []string{"e2"}, Secret: "s2", CreatedBy: "brice",
	})

	webhooks, err := svc.ListWebhooks(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(webhooks) != 2 {
		t.Fatalf("expected 2 webhooks, got %d", len(webhooks))
	}
}

func TestUpdateWebhookURL(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	testutil.SeedActor(t, svc, "brice", model.RoleAdmin)

	w, _ := svc.CreateWebhook(ctx, model.CreateWebhookParams{
		URL: "https://old.com", Events: []string{"e1"}, Secret: "s1", CreatedBy: "brice",
	})

	newURL := "https://new.com"
	updated, err := svc.UpdateWebhook(ctx, model.UpdateWebhookParams{ID: w.ID, URL: model.Set(newURL)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.URL != "https://new.com" {
		t.Errorf("expected new URL, got %s", updated.URL)
	}
}

func TestUpdateWebhookEvents(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	testutil.SeedActor(t, svc, "brice", model.RoleAdmin)

	w, _ := svc.CreateWebhook(ctx, model.CreateWebhookParams{
		URL: "https://x.com", Events: []string{"e1"}, Secret: "s1", CreatedBy: "brice",
	})

	newEvents := []string{"e2", "e3"}
	updated, err := svc.UpdateWebhook(ctx, model.UpdateWebhookParams{ID: w.ID, Events: model.Set(newEvents)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updated.Events) != 2 || updated.Events[0] != "e2" {
		t.Errorf("unexpected events: %v", updated.Events)
	}
}

func TestUpdateWebhookActive(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	testutil.SeedActor(t, svc, "brice", model.RoleAdmin)

	w, _ := svc.CreateWebhook(ctx, model.CreateWebhookParams{
		URL: "https://x.com", Events: []string{"e1"}, Secret: "s1", CreatedBy: "brice",
	})

	inactive := false
	updated, err := svc.UpdateWebhook(ctx, model.UpdateWebhookParams{ID: w.ID, Active: model.Set(inactive)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Active {
		t.Error("expected webhook to be inactive")
	}
}

func TestDeleteWebhook(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	testutil.SeedActor(t, svc, "brice", model.RoleAdmin)

	w, _ := svc.CreateWebhook(ctx, model.CreateWebhookParams{
		URL: "https://x.com", Events: []string{"e1"}, Secret: "s1", CreatedBy: "brice",
	})

	err := svc.DeleteWebhook(ctx, w.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	webhooks, _ := svc.ListWebhooks(ctx)
	if len(webhooks) != 0 {
		t.Errorf("expected 0 webhooks, got %d", len(webhooks))
	}
}

func TestListWebhookDeliveries(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	testutil.SeedActor(t, svc, "brice", model.RoleAdmin)

	w, _ := svc.CreateWebhook(ctx, model.CreateWebhookParams{
		URL: "https://x.com", Events: []string{"e1"}, Secret: "s1", CreatedBy: "brice",
	})

	// Empty initially.
	deliveries, err := svc.ListWebhookDeliveries(ctx, w.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deliveries) != 0 {
		t.Errorf("expected 0 deliveries, got %d", len(deliveries))
	}

	// Non-existent webhook should 404.
	_, err = svc.ListWebhookDeliveries(ctx, 9999)
	if err == nil {
		t.Error("expected error for non-existent webhook")
	}
}

// TestWebhookIntegration tests the full flow: event published → dispatcher
// delivers to endpoint → delivery logged in DB → queryable via service.
func TestWebhookIntegration(t *testing.T) {
	svc, store, bus := testutil.NewTestServiceWithBus(t)
	ctx := context.Background()

	testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "test-board")

	// Set up a test HTTP endpoint.
	var deliveries int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&deliveries, 1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	// Create a webhook pointing to the test endpoint.
	wh, err := svc.CreateWebhook(ctx, model.CreateWebhookParams{
		URL:       srv.URL,
		Events:    []string{"task.created"},
		Secret:    "integration-secret",
		CreatedBy: "brice",
	})
	if err != nil {
		t.Fatalf("failed to create webhook: %v", err)
	}

	// Start the dispatcher with no retry delays.
	dispatcher := webhook.NewDispatcher(bus, svc, store,
		webhook.WithRetryDelays([]time.Duration{0, 0, 0}))
	defer dispatcher.Stop()

	// Create a task — this publishes an event to the bus.
	_, err = svc.CreateTask(ctx, model.CreateTaskParams{
		BoardSlug: "test-board",
		Title:     "Integration test task",
		CreatedBy: "brice",
	})
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Wait for async delivery.
	time.Sleep(500 * time.Millisecond)

	// Verify the endpoint was called.
	if n := atomic.LoadInt32(&deliveries); n != 1 {
		t.Fatalf("expected 1 delivery to endpoint, got %d", n)
	}

	// Verify the delivery was logged in the database.
	logged, err := svc.ListWebhookDeliveries(ctx, wh.ID)
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
	if dl.Attempt != 1 {
		t.Errorf("expected attempt 1, got %d", dl.Attempt)
	}
	if dl.StatusCode == nil || *dl.StatusCode != 200 {
		t.Errorf("expected status 200, got %v", dl.StatusCode)
	}
	if dl.Error != nil {
		t.Errorf("expected no error, got %s", *dl.Error)
	}
}

// TestWebhookIntegrationRetry tests that failed deliveries are retried
// and all attempts are logged.
func TestWebhookIntegrationRetry(t *testing.T) {
	svc, store, bus := testutil.NewTestServiceWithBus(t)
	ctx := context.Background()

	testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "test-board")

	// Endpoint fails twice, succeeds on third attempt.
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			w.WriteHeader(502)
		} else {
			w.WriteHeader(200)
		}
	}))
	defer srv.Close()

	wh, _ := svc.CreateWebhook(ctx, model.CreateWebhookParams{
		URL:       srv.URL,
		Events:    []string{"task.created"},
		Secret:    "retry-secret",
		CreatedBy: "brice",
	})

	dispatcher := webhook.NewDispatcher(bus, svc, store,
		webhook.WithRetryDelays([]time.Duration{0, 0, 0}))
	defer dispatcher.Stop()

	_, _ = svc.CreateTask(ctx, model.CreateTaskParams{
		BoardSlug: "test-board",
		Title:     "Retry test task",
		CreatedBy: "brice",
	})

	time.Sleep(500 * time.Millisecond)

	if n := atomic.LoadInt32(&attempts); n != 3 {
		t.Fatalf("expected 3 attempts, got %d", n)
	}

	logged, _ := svc.ListWebhookDeliveries(ctx, wh.ID)
	if len(logged) != 3 {
		t.Fatalf("expected 3 logged deliveries, got %d", len(logged))
	}

	// Verify all 3 attempt numbers are present.
	attemptsSeen := map[int]bool{}
	var successCount, failCount int
	for _, dl := range logged {
		attemptsSeen[dl.Attempt] = true
		if dl.Error == nil {
			successCount++
		} else {
			failCount++
		}
	}
	for _, a := range []int{1, 2, 3} {
		if !attemptsSeen[a] {
			t.Errorf("missing attempt %d in delivery log", a)
		}
	}
	if successCount != 1 {
		t.Errorf("expected 1 success, got %d", successCount)
	}
	if failCount != 2 {
		t.Errorf("expected 2 failures, got %d", failCount)
	}
}
