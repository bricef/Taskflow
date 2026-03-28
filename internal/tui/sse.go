package tui

import (
	"bufio"
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

// startSSE returns a tea.Cmd that connects to the SSE endpoint and streams
// events into the Bubble Tea program. It reconnects on error with backoff.
type sseResult struct {
	err       error
	permanent bool
}

func startSSE(p *tea.Program, serverURL, boardSlug, apiKey string) {
	go func() {
		backoff := time.Second
		for {
			result := streamSSE(p, serverURL, boardSlug, apiKey)
			p.Send(SSEError{Err: result.err, Permanent: result.permanent})
			if result.permanent {
				return
			}
			time.Sleep(backoff)
			if backoff < 30*time.Second {
				backoff *= 2
			}
		}
	}()
}

func streamSSE(p *tea.Program, serverURL, boardSlug, apiKey string) sseResult {
	url := serverURL + "/boards/" + boardSlug + "/events?token=" + apiKey

	resp, err := http.Get(url)
	if err != nil {
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

	if err := scanner.Err(); err != nil {
		return sseResult{err: err}
	}
	return sseResult{err: fmt.Errorf("SSE stream ended unexpectedly")}
}
