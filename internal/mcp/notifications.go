package mcp

import (
	"context"
	"fmt"
	"sync"

	"github.com/bricef/taskflow/internal/eventbus"
	"github.com/bricef/taskflow/internal/httpclient"
)

const maxPendingNotifications = 50

// Notification is a summary of a domain event from another actor.
type Notification struct {
	Event     string `json:"event"`
	Board     string `json:"board"`
	Task      string `json:"task,omitempty"`
	Actor     string `json:"actor"`
	Summary   string `json:"summary"`
	Timestamp string `json:"timestamp"`
}

// Notifier subscribes to the global event stream and buffers notifications
// from other actors. Pending notifications are retrieved and cleared on each
// tool call via Drain().
type Notifier struct {
	mu       sync.Mutex
	pending  []Notification
	actorName string
}

// NewNotifier starts a background event subscription and buffers notifications
// from actors other than selfName. Cancel the context to stop.
func NewNotifier(ctx context.Context, client *httpclient.Client, selfName string) *Notifier {
	n := &Notifier{actorName: selfName}

	stream := client.Subscribe(ctx, httpclient.SubscribeOptions{})
	go func() {
		for {
			select {
			case evt, ok := <-stream.Events:
				if !ok {
					return
				}
				if evt.Actor.Name == n.actorName {
					continue // skip self-events
				}
				n.add(evt)
			case <-stream.Errors:
				// Reconnection is handled by httpclient.Subscribe.
				// We just ignore errors here.
			case <-ctx.Done():
				return
			}
		}
	}()

	return n
}

func (n *Notifier) add(evt eventbus.Event) {
	n.mu.Lock()
	defer n.mu.Unlock()

	task := ""
	snap := evt.After
	if snap == nil {
		snap = evt.Before
	}
	if snap != nil {
		task = snap.Ref
	}

	notif := Notification{
		Event:     evt.Type,
		Board:     evt.Board.Slug,
		Task:      task,
		Actor:     evt.Actor.Name,
		Summary:   formatNotification(evt),
		Timestamp: evt.Timestamp.Format("2006-01-02T15:04:05Z"),
	}

	n.pending = append(n.pending, notif)
	if len(n.pending) > maxPendingNotifications {
		n.pending = n.pending[len(n.pending)-maxPendingNotifications:]
	}
}

// Drain returns all pending notifications and clears the buffer.
func (n *Notifier) Drain() []Notification {
	n.mu.Lock()
	defer n.mu.Unlock()

	if len(n.pending) == 0 {
		return nil
	}
	result := n.pending
	n.pending = nil
	return result
}

func formatNotification(evt eventbus.Event) string {
	actor := evt.Actor.Name
	if actor == "" {
		actor = "system"
	}

	snap := evt.After
	if snap == nil {
		snap = evt.Before
	}

	switch evt.Type {
	case eventbus.EventTaskCreated:
		if snap != nil {
			return fmt.Sprintf("%s created %s %q", actor, snap.Ref, snap.Title)
		}
	case eventbus.EventTaskTransitioned:
		if evt.Before != nil && snap != nil {
			return fmt.Sprintf("%s moved %s from %s to %s", actor, snap.Ref, evt.Before.State, snap.State)
		}
	case eventbus.EventTaskAssigned:
		if snap != nil && snap.Assignee != nil {
			return fmt.Sprintf("%s assigned %s to %s", actor, snap.Ref, *snap.Assignee)
		}
	case eventbus.EventTaskCommented:
		if snap != nil {
			return fmt.Sprintf("%s commented on %s", actor, snap.Ref)
		}
	case eventbus.EventTaskDeleted:
		if evt.Before != nil {
			return fmt.Sprintf("%s deleted %s", actor, evt.Before.Ref)
		}
	}

	if snap != nil {
		return fmt.Sprintf("%s: %s on %s", actor, evt.Type, snap.Ref)
	}
	return fmt.Sprintf("%s: %s", actor, evt.Type)
}
