package testutil

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bricef/taskflow/internal/eventbus"
	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/repo"
	"github.com/bricef/taskflow/internal/service"
	"github.com/bricef/taskflow/internal/sqlite"
	"github.com/bricef/taskflow/internal/taskflow"
)

// NewTestService creates a fresh in-memory SQLite store and service for testing.
// Enables development mode (allows private webhook URLs).
// Returns the taskflow.TaskFlow interface, not the concrete type.
func NewTestService(t *testing.T) taskflow.TaskFlow {
	model.AllowPrivateWebhookURLs = true
	t.Cleanup(func() { model.AllowPrivateWebhookURLs = false })
	t.Helper()
	store, err := sqlite.New(":memory:")
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return service.New(store)
}

// NewTestServiceWithBus creates a service with an event bus and returns
// the service, store, and bus for integration testing.
func NewTestServiceWithBus(t *testing.T) (taskflow.TaskFlow, repo.Store, *eventbus.EventBus) {
	t.Helper()
	model.AllowPrivateWebhookURLs = true
	t.Cleanup(func() { model.AllowPrivateWebhookURLs = false })
	store, err := sqlite.New(":memory:")
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	bus := eventbus.New()
	svc := service.New(store, service.WithEventBus(bus))
	return svc, store, bus
}

// SeedActor creates a test actor with sensible defaults.
func SeedActor(t *testing.T, svc taskflow.TaskFlow, name string, role model.Role) model.Actor {
	t.Helper()
	actor, err := svc.CreateActor(context.Background(), model.CreateActorParams{
		Name:        name,
		DisplayName: name,
		Type:        model.ActorTypeHuman,
		Role:        role,
		APIKeyHash:  "hash_" + name,
	})
	if err != nil {
		t.Fatalf("failed to seed actor %s: %v", name, err)
	}
	return actor
}

// DefaultTestWorkflow returns a minimal valid workflow JSON for testing.
func DefaultTestWorkflow() json.RawMessage {
	return json.RawMessage(`{
		"states": ["open", "in_progress", "done", "cancelled"],
		"initial_state": "open",
		"terminal_states": ["done", "cancelled"],
		"transitions": [
			{"from": "open", "to": "in_progress", "name": "start"},
			{"from": "in_progress", "to": "done", "name": "finish"},
			{"from": "open", "to": "cancelled", "name": "cancel"},
			{"from": "in_progress", "to": "cancelled", "name": "cancel_wip"}
		]
	}`)
}

// SeedBoard creates a test board with the default test workflow.
func SeedBoard(t *testing.T, svc taskflow.TaskFlow, slug string) model.Board {
	t.Helper()
	board, err := svc.CreateBoard(context.Background(), model.CreateBoardParams{
		Slug:     slug,
		Name:     slug,
		Workflow: DefaultTestWorkflow(),
	})
	if err != nil {
		t.Fatalf("failed to seed board %s: %v", slug, err)
	}
	return board
}

// SeedTask creates a test task on the given board.
func SeedTask(t *testing.T, svc taskflow.TaskFlow, boardSlug, title, createdBy string) model.Task {
	t.Helper()
	task, err := svc.CreateTask(context.Background(), model.CreateTaskParams{
		BoardSlug: boardSlug,
		Title:     title,
		Priority:  model.PriorityNone,
		CreatedBy: createdBy,
	})
	if err != nil {
		t.Fatalf("failed to seed task %s on %s: %v", title, boardSlug, err)
	}
	return task
}
