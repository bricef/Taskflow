package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/testutil"
)

func TestAttachURL(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")
	task := testutil.SeedTask(t, svc, "my-board", "Task", actor.Name)

	att, err := svc.CreateAttachment(ctx, model.CreateAttachmentParams{
		BoardSlug: "my-board", TaskNum: task.Num,
		RefType: model.RefTypeURL, Reference: "https://example.com",
		Label: "Link", CreatedBy: "brice",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if att.RefType != model.RefTypeURL {
		t.Errorf("expected ref_type url, got %s", att.RefType)
	}
	if att.Reference != "https://example.com" {
		t.Errorf("expected reference https://example.com, got %s", att.Reference)
	}
}

func TestAttachGitBranch(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")
	task := testutil.SeedTask(t, svc, "my-board", "Task", actor.Name)

	att, err := svc.CreateAttachment(ctx, model.CreateAttachmentParams{
		BoardSlug: "my-board", TaskNum: task.Num,
		RefType: model.RefTypeGitBranch, Reference: "feat/auth",
		Label: "Branch", CreatedBy: "brice",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if att.RefType != model.RefTypeGitBranch {
		t.Errorf("expected ref_type git_branch, got %s", att.RefType)
	}
	if att.Reference != "feat/auth" {
		t.Errorf("expected reference feat/auth, got %s", att.Reference)
	}
}

func TestAttachFile(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")
	task := testutil.SeedTask(t, svc, "my-board", "Task", actor.Name)

	att, err := svc.CreateAttachment(ctx, model.CreateAttachmentParams{
		BoardSlug: "my-board", TaskNum: task.Num,
		RefType: model.RefTypeFile, Reference: "/files/doc.pdf",
		Label: "Document", CreatedBy: "brice",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if att.RefType != model.RefTypeFile {
		t.Errorf("expected ref_type file, got %s", att.RefType)
	}
}

func TestAttachInvalidRefType(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")
	testutil.SeedTask(t, svc, "my-board", "Task", actor.Name)

	_, err := svc.CreateAttachment(ctx, model.CreateAttachmentParams{
		BoardSlug: "my-board", TaskNum: 1,
		RefType: "invalid", Reference: "something",
		Label: "Bad", CreatedBy: "brice",
	})
	var ve *model.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestAttachEmptyReference(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")
	testutil.SeedTask(t, svc, "my-board", "Task", actor.Name)

	_, err := svc.CreateAttachment(ctx, model.CreateAttachmentParams{
		BoardSlug: "my-board", TaskNum: 1,
		RefType: model.RefTypeURL, Reference: "",
		Label: "Empty", CreatedBy: "brice",
	})
	var ve *model.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestListAttachments(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")
	task := testutil.SeedTask(t, svc, "my-board", "Task", actor.Name)

	svc.CreateAttachment(ctx, model.CreateAttachmentParams{
		BoardSlug: "my-board", TaskNum: task.Num,
		RefType: model.RefTypeURL, Reference: "https://example.com",
		Label: "Link", CreatedBy: "brice",
	})
	svc.CreateAttachment(ctx, model.CreateAttachmentParams{
		BoardSlug: "my-board", TaskNum: task.Num,
		RefType: model.RefTypeGitBranch, Reference: "feat/x",
		Label: "Branch", CreatedBy: "brice",
	})

	atts, err := svc.ListAttachments(ctx, "my-board", task.Num)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(atts) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(atts))
	}
}

func TestRemoveAttachment(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")
	task := testutil.SeedTask(t, svc, "my-board", "Task", actor.Name)

	att, _ := svc.CreateAttachment(ctx, model.CreateAttachmentParams{
		BoardSlug: "my-board", TaskNum: task.Num,
		RefType: model.RefTypeURL, Reference: "https://example.com",
		Label: "Link", CreatedBy: "brice",
	})

	err := svc.DeleteAttachment(ctx, att.ID, "brice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	atts, _ := svc.ListAttachments(ctx, "my-board", task.Num)
	if len(atts) != 0 {
		t.Errorf("expected 0 attachments, got %d", len(atts))
	}
}
