package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/testutil"
)

func TestCreateDependency(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")
	t1 := testutil.SeedTask(t, svc, "my-board", "Task 1", actor.Name)
	t2 := testutil.SeedTask(t, svc, "my-board", "Task 2", actor.Name)

	dep, err := svc.CreateDependency(ctx, model.CreateDependencyParams{
		BoardSlug:      "my-board",
		TaskNum:        t1.Num,
		DependsOnBoard: "my-board",
		DependsOnNum:   t2.Num,
		DependencyType: model.DependencyTypeDependsOn,
		CreatedBy:      actor.Name,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dep.DependencyType != model.DependencyTypeDependsOn {
		t.Errorf("expected dep_type depends_on, got %s", dep.DependencyType)
	}
}

func TestCreateDependencyCrossBoard(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "web")
	testutil.SeedBoard(t, svc, "infra")
	t1 := testutil.SeedTask(t, svc, "web", "Frontend", actor.Name)
	t2 := testutil.SeedTask(t, svc, "infra", "Backend", actor.Name)

	dep, err := svc.CreateDependency(ctx, model.CreateDependencyParams{
		BoardSlug: "web", TaskNum: t1.Num,
		DependsOnBoard: "infra", DependsOnNum: t2.Num,
		DependencyType: model.DependencyTypeDependsOn, CreatedBy: actor.Name,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dep.DependsOnBoard != "infra" {
		t.Errorf("expected depends_on_board infra, got %s", dep.DependsOnBoard)
	}
}

func TestCreateRelationship(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")
	t1 := testutil.SeedTask(t, svc, "my-board", "Task 1", actor.Name)
	t2 := testutil.SeedTask(t, svc, "my-board", "Task 2", actor.Name)

	dep, err := svc.CreateDependency(ctx, model.CreateDependencyParams{
		BoardSlug: "my-board", TaskNum: t1.Num,
		DependsOnBoard: "my-board", DependsOnNum: t2.Num,
		DependencyType: model.DependencyTypeRelatesTo, CreatedBy: actor.Name,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dep.DependencyType != model.DependencyTypeRelatesTo {
		t.Errorf("expected relates_to, got %s", dep.DependencyType)
	}
}

func TestListDependenciesBothDirections(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")
	t1 := testutil.SeedTask(t, svc, "my-board", "Task 1", actor.Name)
	t2 := testutil.SeedTask(t, svc, "my-board", "Task 2", actor.Name)
	t3 := testutil.SeedTask(t, svc, "my-board", "Task 3", actor.Name)

	// t1 depends on t2, t3 depends on t1
	svc.CreateDependency(ctx, model.CreateDependencyParams{
		BoardSlug: "my-board", TaskNum: t1.Num,
		DependsOnBoard: "my-board", DependsOnNum: t2.Num,
		DependencyType: model.DependencyTypeDependsOn, CreatedBy: actor.Name,
	})
	svc.CreateDependency(ctx, model.CreateDependencyParams{
		BoardSlug: "my-board", TaskNum: t3.Num,
		DependsOnBoard: "my-board", DependsOnNum: t1.Num,
		DependencyType: model.DependencyTypeDependsOn, CreatedBy: actor.Name,
	})

	// Listing deps for t1 should show both.
	deps, err := svc.ListDependencies(ctx, "my-board", t1.Num)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deps) != 2 {
		t.Fatalf("expected 2 dependencies, got %d", len(deps))
	}
}

func TestDeleteDependency(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")
	t1 := testutil.SeedTask(t, svc, "my-board", "Task 1", actor.Name)
	t2 := testutil.SeedTask(t, svc, "my-board", "Task 2", actor.Name)

	dep, _ := svc.CreateDependency(ctx, model.CreateDependencyParams{
		BoardSlug: "my-board", TaskNum: t1.Num,
		DependsOnBoard: "my-board", DependsOnNum: t2.Num,
		DependencyType: model.DependencyTypeDependsOn, CreatedBy: actor.Name,
	})

	err := svc.DeleteDependency(ctx, dep.ID, actor.Name)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	deps, _ := svc.ListDependencies(ctx, "my-board", t1.Num)
	if len(deps) != 0 {
		t.Errorf("expected 0 dependencies after delete, got %d", len(deps))
	}
}

func TestDuplicateDependency(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")
	t1 := testutil.SeedTask(t, svc, "my-board", "Task 1", actor.Name)
	t2 := testutil.SeedTask(t, svc, "my-board", "Task 2", actor.Name)

	params := model.CreateDependencyParams{
		BoardSlug: "my-board", TaskNum: t1.Num,
		DependsOnBoard: "my-board", DependsOnNum: t2.Num,
		DependencyType: model.DependencyTypeDependsOn, CreatedBy: actor.Name,
	}
	svc.CreateDependency(ctx, params)

	_, err := svc.CreateDependency(ctx, params)
	var ce *model.ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("expected ConflictError, got %v", err)
	}
}

func TestSelfDependency(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor := testutil.SeedActor(t, svc, "brice", model.RoleAdmin)
	testutil.SeedBoard(t, svc, "my-board")
	task := testutil.SeedTask(t, svc, "my-board", "Task", actor.Name)

	_, err := svc.CreateDependency(ctx, model.CreateDependencyParams{
		BoardSlug: "my-board", TaskNum: task.Num,
		DependsOnBoard: "my-board", DependsOnNum: task.Num,
		DependencyType: model.DependencyTypeDependsOn, CreatedBy: actor.Name,
	})
	var ve *model.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}
