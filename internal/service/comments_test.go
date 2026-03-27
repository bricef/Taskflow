package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/testutil"
)

func TestCreateComment(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")
	task := testutil.SeedTask(t, svc, "my-board", "Task", actor.Name)

	comment, err := svc.CreateComment(ctx, model.CreateCommentParams{
		BoardSlug: "my-board",
		TaskNum:   task.Num,
		Actor:     actor.Name,
		Body:      "This is a comment",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if comment.Body != "This is a comment" {
		t.Errorf("unexpected body: %s", comment.Body)
	}
	if comment.Actor != "brice" {
		t.Errorf("unexpected actor: %s", comment.Actor)
	}
}

func TestListCommentsChronological(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")
	task := testutil.SeedTask(t, svc, "my-board", "Task", actor.Name)

	svc.CreateComment(ctx, model.CreateCommentParams{
		BoardSlug: "my-board", TaskNum: task.Num, Actor: actor.Name, Body: "First",
	})
	svc.CreateComment(ctx, model.CreateCommentParams{
		BoardSlug: "my-board", TaskNum: task.Num, Actor: actor.Name, Body: "Second",
	})

	comments, err := svc.ListComments(ctx, "my-board", task.Num)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(comments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(comments))
	}
	if comments[0].Body != "First" || comments[1].Body != "Second" {
		t.Errorf("comments not in chronological order: %s, %s", comments[0].Body, comments[1].Body)
	}
}

func TestEditComment(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")
	task := testutil.SeedTask(t, svc, "my-board", "Task", actor.Name)

	comment, _ := svc.CreateComment(ctx, model.CreateCommentParams{
		BoardSlug: "my-board", TaskNum: task.Num, Actor: actor.Name, Body: "Original",
	})

	updated, err := svc.UpdateComment(ctx, model.UpdateCommentParams{
		ID:   comment.ID,
		Body: "Edited",
	}, actor.Name)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Body != "Edited" {
		t.Errorf("expected body 'Edited', got %s", updated.Body)
	}
	if updated.UpdatedAt == nil {
		t.Error("expected updated_at to be set")
	}
}

func TestCreateCommentNonExistentTask(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")

	_, err := svc.CreateComment(ctx, model.CreateCommentParams{
		BoardSlug: "my-board",
		TaskNum:   999,
		Actor:     "brice",
		Body:      "Orphan",
	})
	var nfe *model.NotFoundError
	if !errors.As(err, &nfe) {
		t.Fatalf("expected NotFoundError, got %v", err)
	}
}
