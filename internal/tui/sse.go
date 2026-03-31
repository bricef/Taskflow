package tui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bricef/taskflow/internal/eventbus"
)

// SSEEvent is a Bubble Tea message carrying a domain event from the SSE stream.
type SSEEvent struct {
	Event eventbus.Event
}

// SSEError is a Bubble Tea message indicating an SSE connection error.
type SSEError struct {
	Err       error
	Permanent bool // if true, don't retry (e.g., board not found, auth failed)
}

// SSEConnected is sent when the SSE connection is established.
type SSEConnected struct{}

// startSSE connects to the SSE endpoint and streams events into the
// Bubble Tea program. It reconnects on error with backoff. Returns a
// cancel function that stops the goroutine and closes the connection.
type sseResult struct {
	err       error
	permanent bool
}

func startSSE(p *tea.Program, serverURL, boardSlug, apiKey string) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		backoff := time.Second
		for {
			result := streamSSE(ctx, p, serverURL, boardSlug, apiKey)
			if ctx.Err() != nil {
				return // cancelled, don't send error or retry
			}
			p.Send(SSEError{Err: result.err, Permanent: result.permanent})
			if result.permanent {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
		}
	}()
	return cancel
}

func streamSSE(ctx context.Context, p *tea.Program, serverURL, boardSlug, apiKey string) sseResult {
	url := serverURL + "/boards/" + boardSlug + "/events?token=" + apiKey

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return sseResult{err: fmt.Errorf("could not create request: %w", err)}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return sseResult{err: ctx.Err()}
		}
		return sseResult{err: fmt.Errorf("could not connect to server at %s: %w", serverURL, err)}
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case 200:
		// OK, proceed.
	case 401:
		return sseResult{err: fmt.Errorf("authentication failed — check your API key"), permanent: true}
	case 404:
		return sseResult{err: fmt.Errorf("board %q not found — create it first with: taskflow board create --slug %s --name \"...\" --workflow '...'", boardSlug, boardSlug), permanent: true}
	default:
		return sseResult{err: fmt.Errorf("server returned status %d", resp.StatusCode)}
	}

	p.Send(SSEConnected{})

	scanner := bufio.NewScanner(resp.Body)
	var eventType, data string

	for scanner.Scan() {
		if ctx.Err() != nil {
			return sseResult{err: ctx.Err()}
		}
		line := scanner.Text()

		switch {
		case strings.HasPrefix(line, "event: "):
			eventType = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		case line == "" && data != "":
			var evt eventbus.Event
			if err := json.Unmarshal([]byte(data), &evt); err == nil {
				if eventType != "" {
					evt.Type = eventType
				}
				p.Send(SSEEvent{Event: evt})
			}
			eventType = ""
			data = ""
		}
	}

	if ctx.Err() != nil {
		return sseResult{err: ctx.Err()}
	}
	if err := scanner.Err(); err != nil {
		return sseResult{err: err}
	}
	return sseResult{err: fmt.Errorf("SSE stream ended unexpectedly")}
}
