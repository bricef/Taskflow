package http

import (
	"context"

	"github.com/bricef/taskflow/internal/model"
)

type contextKey string

const actorContextKey contextKey = "actor"

func withActor(ctx context.Context, actor model.Actor) context.Context {
	return context.WithValue(ctx, actorContextKey, actor)
}

// ActorFrom returns the authenticated actor from the request context.
// Panics if called without auth middleware (programming error, not a runtime condition).
func ActorFrom(ctx context.Context) model.Actor {
	return ctx.Value(actorContextKey).(model.Actor)
}
