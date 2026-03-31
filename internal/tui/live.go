package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bricef/taskflow/internal/eventbus"
	"github.com/bricef/taskflow/internal/httpclient"
)

// LiveEvent is a Bubble Tea message carrying a domain event.
type LiveEvent struct {
	Event eventbus.Event
}

// LiveEventError is a Bubble Tea message indicating a connection error.
type LiveEventError struct {
	Err       error
	Permanent bool
}

// LiveEventConnected is sent when the event stream is established.
type LiveEventConnected struct{}

// startLiveEvents connects to the event stream and forwards domain events
// into the Bubble Tea program. Returns a cancel function that stops the stream.
func startLiveEvents(p *tea.Program, client *httpclient.Client, boardSlug string) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	stream := client.Subscribe(ctx, boardSlug)

	go func() {
		for {
			select {
			case evt, ok := <-stream.Events:
				if !ok {
					return
				}
				p.Send(LiveEvent{Event: evt})
			case err, ok := <-stream.Errors:
				if !ok {
					return
				}
				p.Send(LiveEventError{Err: err.Err, Permanent: err.Permanent})
				if err.Permanent {
					return
				}
			case <-stream.Connected:
				p.Send(LiveEventConnected{})
			case <-ctx.Done():
				return
			}
		}
	}()

	return cancel
}
