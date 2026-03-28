// Package eventbus provides an in-process pub/sub system for domain events.
// Events are emitted by the service layer after successful mutations and
// consumed by SSE streams, webhook dispatch, and MCP notifications.
package eventbus

import "time"

// Event type constants matching the PRD §7.1 webhook event types.
const (
	EventTaskCreated       = "task.created"
	EventTaskUpdated       = "task.updated"
	EventTaskTransitioned  = "task.transitioned"
	EventTaskDeleted       = "task.deleted"
	EventTaskAssigned      = "task.assigned"
	EventTaskCommented     = "task.commented"
	EventDependencyAdded   = "dependency.added"
	EventDependencyRemoved = "dependency.removed"
	EventAttachmentAdded   = "attachment.added"
	EventAttachmentRemoved = "attachment.removed"
)

// Event represents a domain event emitted after a successful mutation.
type Event struct {
	Type      string    `json:"event"`
	Timestamp time.Time `json:"timestamp"`
	Actor     ActorRef  `json:"actor"`
	Board     BoardRef  `json:"board"`
	Task      *TaskRef  `json:"task,omitempty"`
	Detail    any       `json:"detail,omitempty"`
}

// ActorRef identifies the actor who caused the event.
type ActorRef struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// BoardRef identifies the board the event belongs to.
type BoardRef struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// TaskRef identifies the task affected by the event.
type TaskRef struct {
	Ref           string `json:"ref"`
	Title         string `json:"title"`
	State         string `json:"state"`
	PreviousState string `json:"previous_state,omitempty"`
}
