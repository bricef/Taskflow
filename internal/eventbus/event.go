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
	EventCommentEdited     = "comment.edited"
	EventDependencyAdded   = "dependency.added"
	EventDependencyRemoved = "dependency.removed"
	EventAttachmentAdded   = "attachment.added"
	EventAttachmentRemoved = "attachment.removed"
)

// Event represents a domain event emitted after a successful mutation.
//
// For task events, Before and After carry snapshots of the task state
// before and after the mutation:
//   - Create:     Before=nil,  After=snapshot
//   - Update:     Before=snap, After=snapshot
//   - Transition: Before=snap, After=snapshot
//   - Delete:     Before=snap, After=nil
//   - Comment/Dep/Attachment: Before=nil, After=snapshot (current state)
type Event struct {
	Type      string        `json:"event"`
	Timestamp time.Time     `json:"timestamp"`
	Actor     ActorRef      `json:"actor"`
	Board     BoardRef      `json:"board"`
	Before    *TaskSnapshot `json:"before,omitempty"`
	After     *TaskSnapshot `json:"after,omitempty"`
	Detail    any           `json:"detail,omitempty"`
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

// TaskSnapshot is a point-in-time snapshot of a task's display-relevant fields.
type TaskSnapshot struct {
	Ref      string  `json:"ref"`
	Num      int     `json:"num"`
	Title    string  `json:"title"`
	State    string  `json:"state"`
	Priority string  `json:"priority,omitempty"`
	Assignee *string `json:"assignee,omitempty"`
}
