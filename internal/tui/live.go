package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bricef/taskflow/internal/httpclient"
)

// liveConnected is a Bubble Tea message sent when the event stream connects.
type liveConnected struct{}

// startLiveEvents connects to the global event stream (all boards) and
// forwards domain events into the Bubble Tea program.
// Returns a cancel function that stops the stream.
func startLiveEvents(p *tea.Program, client *httpclient.Client) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	stream := client.Subscribe(ctx, httpclient.SubscribeOptions{})

	go func() {
		for {
			select {
			case evt, ok := <-stream.Events:
				if !ok {
					return
				}
				p.Send(evt)
			case err, ok := <-stream.Errors:
				if !ok {
					return
				}
				p.Send(err)
				if err.Permanent {
					return
				}
			case <-stream.Connected:
				p.Send(liveConnected{})
			case <-ctx.Done():
				return
			}
		}
	}()

	return cancel
}
