package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/testutil"
)

func TestCreateActor(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor, err := svc.CreateActor(ctx, model.CreateActorParams{
		Name:        "brice",
		DisplayName: "Brice",
		Type:        model.ActorTypeHuman,
		Role:        model.RoleAdmin,
		APIKeyHash:  "somehash",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if actor.Name != "brice" {
		t.Errorf("expected name brice, got %s", actor.Name)
	}
	if actor.DisplayName != "Brice" {
		t.Errorf("expected display_name Brice, got %s", actor.DisplayName)
	}
	if actor.Type != model.ActorTypeHuman {
		t.Errorf("expected type human, got %s", actor.Type)
	}
	if actor.Role != model.RoleAdmin {
		t.Errorf("expected role admin, got %s", actor.Role)
	}
	if !actor.Active {
		t.Error("expected actor to be active")
	}
}

func TestCreateActorRoles(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	roles := []model.Role{model.RoleAdmin, model.RoleMember, model.RoleReadOnly}
	for _, role := range roles {
		t.Run(string(role), func(t *testing.T) {
			actor, err := svc.CreateActor(ctx, model.CreateActorParams{
				Name:        "actor-" + string(role),
				DisplayName: string(role),
				Type:        model.ActorTypeHuman,
				Role:        role,
				APIKeyHash:  "hash_" + string(role),
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if actor.Role != role {
				t.Errorf("expected role %s, got %s", role, actor.Role)
			}
		})
	}
}

func TestCreateActorInvalidRole(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	_, err := svc.CreateActor(ctx, model.CreateActorParams{
		Name:        "bad",
		DisplayName: "Bad",
		Type:        model.ActorTypeHuman,
		Role:        "superadmin",
		APIKeyHash:  "hash",
	})
	var ve *model.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestCreateActorDuplicate(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	testutil.SeedActor(t, svc, "brice", model.RoleAdmin)

	_, err := svc.CreateActor(ctx, model.CreateActorParams{
		Name:        "brice",
		DisplayName: "Brice2",
		Type:        model.ActorTypeHuman,
		Role:        model.RoleMember,
		APIKeyHash:  "hash2",
	})
	var ce *model.ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("expected ConflictError, got %v", err)
	}
}

func TestCreateActorEmptyName(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	_, err := svc.CreateActor(ctx, model.CreateActorParams{
		Name:        "",
		DisplayName: "X",
		Type:        model.ActorTypeHuman,
		Role:        model.RoleAdmin,
		APIKeyHash:  "hash",
	})
	var ve *model.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestCreateActorInvalidType(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	_, err := svc.CreateActor(ctx, model.CreateActorParams{
		Name:        "bot",
		DisplayName: "Bot",
		Type:        "robot",
		Role:        model.RoleAdmin,
		APIKeyHash:  "hash",
	})
	var ve *model.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestCreateActorAIAgent(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	actor, err := svc.CreateActor(ctx, model.CreateActorParams{
		Name:        "claude",
		DisplayName: "Claude",
		Type:        model.ActorTypeAIAgent,
		Role:        model.RoleMember,
		APIKeyHash:  "hash",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if actor.Type != model.ActorTypeAIAgent {
		t.Errorf("expected type ai_agent, got %s", actor.Type)
	}
}

func TestListActors(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	testutil.SeedActor(t, svc, "alice", model.RoleAdmin)
	testutil.SeedActor(t, svc, "bob", model.RoleMember)
	testutil.SeedActor(t, svc, "carol", model.RoleReadOnly)

	actors, err := svc.ListActors(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actors) != 3 {
		t.Fatalf("expected 3 actors, got %d", len(actors))
	}
	// Should be ordered by name.
	if actors[0].Name != "alice" || actors[1].Name != "bob" || actors[2].Name != "carol" {
		t.Errorf("actors not in expected order: %v", actors)
	}
}

func TestUpdateActorRole(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	testutil.SeedActor(t, svc, "brice", model.RoleMember)

	newRole := model.RoleAdmin
	actor, err := svc.UpdateActor(ctx, model.UpdateActorParams{
		Name: "brice",
		Role: model.Set(newRole),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if actor.Role != model.RoleAdmin {
		t.Errorf("expected role admin, got %s", actor.Role)
	}
}

func TestUpdateActorDisplayName(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	testutil.SeedActor(t, svc, "brice", model.RoleAdmin)

	newName := "Brice Updated"
	actor, err := svc.UpdateActor(ctx, model.UpdateActorParams{
		Name:        "brice",
		DisplayName: model.Set(newName),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if actor.DisplayName != "Brice Updated" {
		t.Errorf("expected display_name 'Brice Updated', got %s", actor.DisplayName)
	}
}

func TestDeactivateActor(t *testing.T) {
	svc := testutil.NewTestService(t)
	ctx := context.Background()

	testutil.SeedActor(t, svc, "brice", model.RoleAdmin)

	active := false
	actor, err := svc.UpdateActor(ctx, model.UpdateActorParams{
		Name:   "brice",
		Active: model.Set(active),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if actor.Active {
		t.Error("expected actor to be inactive")
	}

	// Should still be queryable.
	got, err := svc.GetActor(ctx, "brice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Active {
		t.Error("expected deactivated actor to remain inactive")
	}
}
