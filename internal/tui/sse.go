package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bricef/taskflow/internal/eventbus"
	"github.com/bricef/taskflow/internal/httpclient"
)

// SSEEvent is a Bubble Tea message carrying a domain event from the SSE stream.
type SSEEvent struct {
	Event eventbus.Event
}

// SSEError is a Bubble Tea message indicating an SSE connection error.
type SSEError struct {
	Err       error
	Permanent bool
}

// SSEConnected is sent when the SSE connection is established.
type SSEConnected struct{}

// startSSE connects to the SSE endpoint and forwards events into the
// Bubble Tea program. Returns a cancel function that stops the stream.
func startSSE(p *tea.Program, client *httpclient.Client, boardSlug string) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	stream := client.Subscribe(ctx, boardSlug)

	go func() {
		for {
			select {
			case evt, ok := <-stream.Events:
				if !ok {
					return
				}
				p.Send(SSEEvent{Event: evt})
			case err, ok := <-stream.Errors:
				if !ok {
					return
				}
				p.Send(SSEError{Err: err.Err, Permanent: err.Permanent})
				if err.Permanent {
					return
				}
			case <-stream.Connected:
				p.Send(SSEConnected{})
			case <-ctx.Done():
				return
			}
		}
	}()

	return cancel
}
