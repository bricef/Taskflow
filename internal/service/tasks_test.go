package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/testutil"
)

func TestCreateTask(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")

	task, err := svc.CreateTask(ctx, model.CreateTaskParams{
		BoardSlug: "my-board",
		Title:     "Fix bug",
		State:     "open",
		Priority:  model.PriorityNone,
		CreatedBy: actor.Name,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Num != 1 {
		t.Errorf("expected num 1, got %d", task.Num)
	}
	if task.State != "open" {
		t.Errorf("expected state open, got %s", task.State)
	}
	if task.BoardSlug != "my-board" {
		t.Errorf("expected board_slug my-board, got %s", task.BoardSlug)
	}
}

func TestCreateTaskSequentialNumbers(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")

	t1 := testutil.SeedTask(t, svc, "my-board", "Task 1", actor.Name)
	t2 := testutil.SeedTask(t, svc, "my-board", "Task 2", actor.Name)

	if t1.Num != 1 {
		t.Errorf("expected first task num 1, got %d", t1.Num)
	}
	if t2.Num != 2 {
		t.Errorf("expected second task num 2, got %d", t2.Num)
	}

	// Verify board's next_task_num was updated.
	board, err := svc.GetBoard(ctx, "my-board")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if board.NextTaskNum != 3 {
		t.Errorf("expected next_task_num 3, got %d", board.NextTaskNum)
	}
}

func TestCreateTaskWithAllFields(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")

	assignee := "brice"
	dueDate := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	task, err := svc.CreateTask(ctx, model.CreateTaskParams{
		BoardSlug:   "my-board",
		Title:       "Full task",
		Description: "A detailed description",
		State:       "open",
		Priority:    model.PriorityHigh,
		Tags:        []string{"bug", "urgent"},
		Assignee:    &assignee,
		DueDate:     &dueDate,
		CreatedBy:   actor.Name,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Description != "A detailed description" {
		t.Errorf("unexpected description: %s", task.Description)
	}
	if task.Priority != model.PriorityHigh {
		t.Errorf("expected priority high, got %s", task.Priority)
	}
	if len(task.Tags) != 2 || task.Tags[0] != "bug" || task.Tags[1] != "urgent" {
		t.Errorf("unexpected tags: %v", task.Tags)
	}
	if task.Assignee == nil || *task.Assignee != "brice" {
		t.Errorf("unexpected assignee: %v", task.Assignee)
	}
	if task.DueDate == nil {
		t.Error("expected due_date to be set")
	}
}

func TestCreateTaskInvalidPriority(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")

	_, err := svc.CreateTask(ctx, model.CreateTaskParams{
		BoardSlug: "my-board",
		Title:     "Bad",
		State:     "open",
		Priority:  "urgent",
		CreatedBy: "brice",
	})
	var ve *model.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestCreateTaskNonExistentAssignee(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")

	ghost := "ghost"
	_, err := svc.CreateTask(ctx, model.CreateTaskParams{
		BoardSlug: "my-board",
		Title:     "Bad",
		State:     "open",
		Priority:  model.PriorityNone,
		Assignee:  &ghost,
		CreatedBy: "brice",
	})
	if err == nil {
		t.Fatal("expected error for non-existent assignee")
	}
}

func TestCreateTaskNonExistentBoard(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	testutil.SeedActor(t, svc, "brice", model.RoleAdmin)

	_, err := svc.CreateTask(ctx, model.CreateTaskParams{
		BoardSlug: "no-board",
		Title:     "Bad",
		State:     "open",
		Priority:  model.PriorityNone,
		CreatedBy: "brice",
	})
	var nfe *model.NotFoundError
	if !errors.As(err, &nfe) {
		t.Fatalf("expected NotFoundError, got %v", err)
	}
}

func TestUpdateTask(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")
	task := testutil.SeedTask(t, svc, "my-board", "Original", actor.Name)

	newTitle := "Updated Title"
	updated, err := svc.UpdateTask(ctx, model.UpdateTaskParams{
		BoardSlug: "my-board",
		Num:       task.Num,
		Title:     model.Set(newTitle),
	}, actor.Name)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Title != "Updated Title" {
		t.Errorf("expected title 'Updated Title', got %s", updated.Title)
	}
	if updated.UpdatedAt.Before(task.UpdatedAt) {
		t.Error("expected updated_at to not go backwards")
	}
}

func TestSoftDeleteTask(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")
	task := testutil.SeedTask(t, svc, "my-board", "To Delete", actor.Name)

	err := svc.DeleteTask(ctx, "my-board", task.Num, actor.Name)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Default list should exclude deleted.
	tasks, err := svc.ListTasks(ctx, model.TaskFilter{BoardSlug: "my-board"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}

	// Include deleted should show it.
	tasks, err = svc.ListTasks(ctx, model.TaskFilter{BoardSlug: "my-board", IncludeDeleted: true}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task with include_deleted, got %d", len(tasks))
	}
	if !tasks[0].Deleted {
		t.Error("expected task to be marked deleted")
	}
}

func TestListTasksFilterByState(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")
	testutil.SeedTask(t, svc, "my-board", "Open Task", actor.Name)
	svc.CreateTask(ctx, model.CreateTaskParams{
		BoardSlug: "my-board", Title: "WIP", State: "in_progress",
		Priority: model.PriorityNone, CreatedBy: actor.Name,
	})

	state := "open"
	tasks, err := svc.ListTasks(ctx, model.TaskFilter{BoardSlug: "my-board", State: &state}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 open task, got %d", len(tasks))
	}
	if tasks[0].Title != "Open Task" {
		t.Errorf("expected 'Open Task', got %s", tasks[0].Title)
	}
}

func TestListTasksFilterByAssignee(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	testutil.SeedActor(t, svc, "alice", model.RoleMember)
	testutil.SeedActor(t, svc, "bob", model.RoleMember)
	testutil.SeedBoard(t, svc, "my-board")

	alice := "alice"
	bob := "bob"
	svc.CreateTask(ctx, model.CreateTaskParams{
		BoardSlug: "my-board", Title: "Alice's task", State: "open",
		Priority: model.PriorityNone, Assignee: &alice, CreatedBy: "alice",
	})
	svc.CreateTask(ctx, model.CreateTaskParams{
		BoardSlug: "my-board", Title: "Bob's task", State: "open",
		Priority: model.PriorityNone, Assignee: &bob, CreatedBy: "bob",
	})

	tasks, err := svc.ListTasks(ctx, model.TaskFilter{BoardSlug: "my-board", Assignee: &alice}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Title != "Alice's task" {
		t.Errorf("unexpected tasks: %v", tasks)
	}
}

func TestListTasksFilterByPriority(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")

	svc.CreateTask(ctx, model.CreateTaskParams{
		BoardSlug: "my-board", Title: "High", State: "open",
		Priority: model.PriorityHigh, CreatedBy: actor.Name,
	})
	svc.CreateTask(ctx, model.CreateTaskParams{
		BoardSlug: "my-board", Title: "Low", State: "open",
		Priority: model.PriorityLow, CreatedBy: actor.Name,
	})

	p := model.PriorityHigh
	tasks, err := svc.ListTasks(ctx, model.TaskFilter{BoardSlug: "my-board", Priority: &p}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Title != "High" {
		t.Errorf("unexpected tasks: %v", tasks)
	}
}

func TestListTasksFilterByTag(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")

	svc.CreateTask(ctx, model.CreateTaskParams{
		BoardSlug: "my-board", Title: "Tagged", State: "open",
		Priority: model.PriorityNone, Tags: []string{"bug", "ui"}, CreatedBy: actor.Name,
	})
	svc.CreateTask(ctx, model.CreateTaskParams{
		BoardSlug: "my-board", Title: "Untagged", State: "open",
		Priority: model.PriorityNone, CreatedBy: actor.Name,
	})

	tag := "bug"
	tasks, err := svc.ListTasks(ctx, model.TaskFilter{BoardSlug: "my-board", Tag: &tag}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Title != "Tagged" {
		t.Errorf("unexpected tasks: %v", tasks)
	}
}

func TestListTasksSortByPriority(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")

	svc.CreateTask(ctx, model.CreateTaskParams{
		BoardSlug: "my-board", Title: "Low", State: "open",
		Priority: model.PriorityLow, CreatedBy: actor.Name,
	})
	svc.CreateTask(ctx, model.CreateTaskParams{
		BoardSlug: "my-board", Title: "Critical", State: "open",
		Priority: model.PriorityCritical, CreatedBy: actor.Name,
	})
	svc.CreateTask(ctx, model.CreateTaskParams{
		BoardSlug: "my-board", Title: "High", State: "open",
		Priority: model.PriorityHigh, CreatedBy: actor.Name,
	})

	tasks, err := svc.ListTasks(ctx, model.TaskFilter{BoardSlug: "my-board"}, &model.TaskSort{Field: "priority", Desc: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}
	if tasks[0].Title != "Critical" || tasks[1].Title != "High" || tasks[2].Title != "Low" {
		t.Errorf("unexpected sort order: %s, %s, %s", tasks[0].Title, tasks[1].Title, tasks[2].Title)
	}
}

func TestFTSSearch(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")

	svc.CreateTask(ctx, model.CreateTaskParams{
		BoardSlug: "my-board", Title: "Fix authentication bug", State: "open",
		Priority: model.PriorityNone, CreatedBy: actor.Name,
	})
	svc.CreateTask(ctx, model.CreateTaskParams{
		BoardSlug: "my-board", Title: "Add logging", State: "open",
		Priority: model.PriorityNone, CreatedBy: actor.Name,
	})

	q := "authentication"
	tasks, err := svc.ListTasks(ctx, model.TaskFilter{BoardSlug: "my-board", Query: &q}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Title != "Fix authentication bug" {
		t.Errorf("expected 1 search result, got %d", len(tasks))
	}

	// Non-match
	q2 := "nonexistent"
	tasks2, err := svc.ListTasks(ctx, model.TaskFilter{BoardSlug: "my-board", Query: &q2}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks2) != 0 {
		t.Errorf("expected 0 results, got %d", len(tasks2))
	}
}
