// Package service contains the business logic layer. It orchestrates
// validation, audit recording, and transactional operations by calling
// the repo interfaces. The service layer is storage-agnostic.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bricef/taskflow/internal/eventbus"
	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/repo"
	"github.com/bricef/taskflow/internal/taskflow"
)

type Service struct {
	store repo.Store
	bus   *eventbus.EventBus
}

// Option configures the Service.
type Option func(*Service)

// WithEventBus injects an event bus. If not provided, events are silently discarded.
func WithEventBus(bus *eventbus.EventBus) Option {
	return func(s *Service) { s.bus = bus }
}

// New creates a new Service backed by the given store.
func New(store repo.Store, opts ...Option) taskflow.TaskFlow {
	s := &Service{store: store}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Service) audit(ctx context.Context, tx repo.Tx, boardSlug string, taskNum *int, actor string, action model.AuditAction, detail any) error {
	var detailJSON json.RawMessage
	if detail != nil {
		b, err := json.Marshal(detail)
		if err != nil {
			return err
		}
		detailJSON = b
	}
	return s.store.AuditInsert(ctx, tx, model.AuditEntry{
		BoardSlug: boardSlug,
		TaskNum:   taskNum,
		Actor:     actor,
		Action:    action,
		Detail:    detailJSON,
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	})
}

// emit publishes an event to the bus. No-op if the bus is nil.
func (s *Service) emit(evt eventbus.Event) {
	if s.bus == nil {
		return
	}
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now().UTC()
	}
	s.bus.Publish(evt)
}

// taskRef builds a TaskRef from a task.
func taskRef(t model.Task) *eventbus.TaskRef {
	return &eventbus.TaskRef{
		Ref:   fmt.Sprintf("%s/%d", t.BoardSlug, t.Num),
		Title: t.Title,
		State: t.State,
	}
}

// boardRef builds a BoardRef from a slug. Name is left empty — callers
// can enrich it if they have the board loaded.
func boardRef(slug string) eventbus.BoardRef {
	return eventbus.BoardRef{Slug: slug}
}

// actorRef builds an ActorRef from the actor name.
func actorRef(name string) eventbus.ActorRef {
	return eventbus.ActorRef{Name: name}
}
