package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/testutil"
)

func TestCreateBoard(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	board, err := svc.CreateBoard(ctx, model.CreateBoardParams{
		Slug:     "my-board",
		Name:     "My Board",
		Workflow: testutil.DefaultTestWorkflow(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if board.Slug != "my-board" {
		t.Errorf("expected slug my-board, got %s", board.Slug)
	}
	if board.Name != "My Board" {
		t.Errorf("expected name 'My Board', got %s", board.Name)
	}
	if board.NextTaskNum != 1 {
		t.Errorf("expected next_task_num 1, got %d", board.NextTaskNum)
	}
	if board.Deleted {
		t.Error("expected board not to be deleted")
	}
}

func TestCreateBoardDuplicateSlug(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	testutil.SeedBoard(t, svc, "my-board")

	_, err := svc.CreateBoard(ctx, model.CreateBoardParams{
		Slug:     "my-board",
		Name:     "Another",
		Workflow: testutil.DefaultTestWorkflow(),
	})
	var ce *model.ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("expected ConflictError, got %v", err)
	}
}

func TestCreateBoardInvalidSlug(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	cases := []struct {
		name string
		slug string
	}{
		{"uppercase", "MyBoard"},
		{"spaces", "my board"},
		{"special chars", "my@board"},
		{"too short", "a"},
		{"too long", "abcdefghijklmnopqrstuvwxyz1234567"},
		{"leading hyphen", "-board"},
		{"trailing hyphen", "board-"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreateBoard(ctx, model.CreateBoardParams{
				Slug:     tc.slug,
				Name:     "Test",
				Workflow: testutil.DefaultTestWorkflow(),
			})
			var ve *model.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("expected ValidationError for slug %q, got %v", tc.slug, err)
			}
		})
	}
}

func TestUpdateBoardName(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	testutil.SeedBoard(t, svc, "my-board")

	newName := "Updated Name"
	board, err := svc.UpdateBoard(ctx, model.UpdateBoardParams{
		Slug: "my-board",
		Name: model.Set(newName),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if board.Name != "Updated Name" {
		t.Errorf("expected name 'Updated Name', got %s", board.Name)
	}
	if board.Slug != "my-board" {
		t.Error("slug should not change")
	}
}

func TestSoftDeleteBoard(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	testutil.SeedActor(t, svc, "admin", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")

	err := svc.DeleteBoard(ctx, "my-board", "admin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	board, err := svc.GetBoard(ctx, "my-board")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !board.Deleted {
		t.Error("expected board to be soft-deleted")
	}
}

func TestListBoardsExcludesDeleted(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	testutil.SeedActor(t, svc, "admin", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "active-board")
	testutil.SeedBoard(t, svc, "deleted-board")
	svc.DeleteBoard(ctx, "deleted-board", "admin")

	boards, err := svc.ListBoards(ctx, model.ListBoardsParams{IncludeDeleted: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(boards) != 1 {
		t.Fatalf("expected 1 board, got %d", len(boards))
	}
	if boards[0].Slug != "active-board" {
		t.Errorf("expected active-board, got %s", boards[0].Slug)
	}
}

func TestListBoardsIncludeDeleted(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	testutil.SeedActor(t, svc, "admin", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "active-board")
	testutil.SeedBoard(t, svc, "deleted-board")
	svc.DeleteBoard(ctx, "deleted-board", "admin")

	boards, err := svc.ListBoards(ctx, model.ListBoardsParams{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(boards) != 2 {
		t.Fatalf("expected 2 boards, got %d", len(boards))
	}
}

func TestReassignTasks(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "board-a")
	testutil.SeedBoard(t, svc, "board-b")

	// Create tasks on board-a.
	testutil.SeedTask(t, svc, "board-a", "Task 1", actor.Name)
	testutil.SeedTask(t, svc, "board-a", "Task 2", actor.Name)

	count, err := svc.ReassignTasks(ctx, "board-a", "board-b", actor.Name, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 tasks reassigned, got %d", count)
	}

	// board-b should now have 2 tasks.
	tasks, err := svc.ListTasks(ctx, model.TaskFilter{BoardSlug: "board-b"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks on board-b, got %d", len(tasks))
	}

	// board-a should have no tasks.
	tasksA, err := svc.ListTasks(ctx, model.TaskFilter{BoardSlug: "board-a"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasksA) != 0 {
		t.Errorf("expected 0 tasks on board-a, got %d", len(tasksA))
	}
}

func TestReassignTasksWithStateFilter(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "board-a")
	testutil.SeedBoard(t, svc, "board-b")

	testutil.SeedTask(t, svc, "board-a", "Open Task", actor.Name)
	// Create a task and move it to in_progress.
	task2, err := svc.CreateTask(ctx, model.CreateTaskParams{
		BoardSlug: "board-a",
		Title:     "In Progress Task",
		Priority:  model.PriorityNone, CreatedBy: actor.Name,
	})
	if err != nil {
		t.Fatalf("unexpected error creating second task: %v", err)
	}
	_, err = svc.TransitionTask(ctx, model.TransitionTaskParams{
		BoardSlug:      "board-a",
		Num:            task2.Num,
		TransitionName: "start",
		Actor:          actor.Name,
	})
	if err != nil {
		t.Fatalf("unexpected error transitioning task: %v", err)
	}

	// Only reassign "open" tasks.
	count, err := svc.ReassignTasks(ctx, "board-a", "board-b", actor.Name, []string{"open"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 task reassigned, got %d", count)
	}

	// board-a should still have the in_progress task.
	tasksA, err := svc.ListTasks(ctx, model.TaskFilter{BoardSlug: "board-a"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasksA) != 1 {
		t.Fatalf("expected 1 task on board-a, got %d", len(tasksA))
	}
	if tasksA[0].State != "in_progress" {
		t.Errorf("expected remaining task to be in_progress, got %s", tasksA[0].State)
	}
}
