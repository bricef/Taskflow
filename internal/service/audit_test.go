package service_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/testutil"
)

func TestCreateTaskAuditEntry(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")
	task := testutil.SeedTask(t, svc, "my-board", "Audited Task", actor.Name)

	entries, err := svc.QueryAuditByTask(ctx, "my-board", task.Num)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(entries))
	}
	if entries[0].Action != model.AuditActionCreated {
		t.Errorf("expected action 'created', got %s", entries[0].Action)
	}
	if entries[0].Actor != "brice" {
		t.Errorf("expected actor 'brice', got %s", entries[0].Actor)
	}
}

func TestUpdateTaskAuditEntry(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")
	task := testutil.SeedTask(t, svc, "my-board", "Task", actor.Name)

	newTitle := "Updated"
	svc.UpdateTask(ctx, model.UpdateTaskParams{
		BoardSlug: "my-board", Num: task.Num, Title: model.Set(newTitle),
	}, actor.Name)

	entries, err := svc.QueryAuditByTask(ctx, "my-board", task.Num)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 audit entries, got %d", len(entries))
	}
	if entries[1].Action != model.AuditActionUpdated {
		t.Errorf("expected action 'updated', got %s", entries[1].Action)
	}

	// Verify detail contains changed fields.
	var detail map[string]any
	json.Unmarshal(entries[1].Detail, &detail)
	fields, ok := detail["fields"].(map[string]any)
	if !ok {
		t.Fatal("expected fields in detail")
	}
	titleChange, ok := fields["title"].(map[string]any)
	if !ok {
		t.Fatal("expected title in fields")
	}
	if titleChange["old"] != "Task" || titleChange["new"] != "Updated" {
		t.Errorf("unexpected title change: %v", titleChange)
	}
}

func TestAuditAppendOnly(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")
	testutil.SeedTask(t, svc, "my-board", "Task 1", actor.Name)
	testutil.SeedTask(t, svc, "my-board", "Task 2", actor.Name)

	// The Service interface has no update/delete methods for audit entries.
	// Verify entries accumulate.
	entries, err := svc.QueryAuditByBoard(ctx, "my-board")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) < 2 {
		t.Errorf("expected at least 2 audit entries, got %d", len(entries))
	}
}

func TestQueryAuditByBoard(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "board-a")
	testutil.SeedBoard(t, svc, "board-b")
	testutil.SeedTask(t, svc, "board-a", "A1", actor.Name)
	testutil.SeedTask(t, svc, "board-a", "A2", actor.Name)
	testutil.SeedTask(t, svc, "board-b", "B1", actor.Name)

	entriesA, err := svc.QueryAuditByBoard(ctx, "board-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entriesA) != 2 {
		t.Errorf("expected 2 entries for board-a, got %d", len(entriesA))
	}

	entriesB, err := svc.QueryAuditByBoard(ctx, "board-b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entriesB) != 1 {
		t.Errorf("expected 1 entry for board-b, got %d", len(entriesB))
	}
}

func TestAuditChronologicalOrder(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")
	task := testutil.SeedTask(t, svc, "my-board", "Task", actor.Name)

	newTitle := "Updated"
	svc.UpdateTask(ctx, model.UpdateTaskParams{
		BoardSlug: "my-board", Num: task.Num, Title: model.Set(newTitle),
	}, actor.Name)

	entries, _ := svc.QueryAuditByTask(ctx, "my-board", task.Num)
	if len(entries) < 2 {
		t.Fatal("expected at least 2 entries")
	}
	for i := 1; i < len(entries); i++ {
		if entries[i].CreatedAt.Before(entries[i-1].CreatedAt) {
			t.Error("audit entries not in chronological order")
		}
	}
}
