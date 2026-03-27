package service_test

import (
	"context"
	"testing"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/testutil"
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
