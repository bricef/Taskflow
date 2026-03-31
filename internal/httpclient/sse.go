package httpclient

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bricef/taskflow/internal/eventbus"
)

// EventStream delivers domain events from an SSE connection.
// The stream reconnects automatically with exponential backoff.
// Cancel the context passed to Subscribe to stop.
type EventStream struct {
	Events    <-chan eventbus.Event // domain events
	Errors    <-chan StreamError    // connection errors; non-permanent errors are retried automatically
	Connected <-chan struct{}       // sent each time a connection is established
}

// StreamError describes an SSE connection error.
type StreamError struct {
	Err       error
	Permanent bool // if true, the stream will not retry (e.g. 401, 404)
}

// Subscribe opens an SSE event stream for the given board.
// Events are delivered on the returned channels until ctx is cancelled.
func (c *Client) Subscribe(ctx context.Context, boardSlug string) *EventStream {
	events := make(chan eventbus.Event, 64)
	errors := make(chan StreamError, 4)
	connected := make(chan struct{}, 1)

	stream := &EventStream{
		Events:    events,
		Errors:    errors,
		Connected: connected,
	}

	go func() {
		defer close(events)
		defer close(errors)
		defer close(connected)

		backoff := time.Second
		for {
			err := c.streamSSE(ctx, boardSlug, events, connected)
			if ctx.Err() != nil {
				return
			}

			permanent := isPermanent(err)
			select {
			case errors <- StreamError{Err: err, Permanent: permanent}:
			default:
			}
			if permanent {
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

	return stream
}

func (c *Client) streamSSE(ctx context.Context, boardSlug string, events chan<- eventbus.Event, connected chan<- struct{}) error {
	url := c.baseURL + "/boards/" + boardSlug + "/events?token=" + c.apiKey

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("could not create request: %w", err)
	}

	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("could not connect to server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return &sseStatusError{code: resp.StatusCode, boardSlug: boardSlug}
	}

	select {
	case connected <- struct{}{}:
	default:
	}

	scanner := bufio.NewScanner(resp.Body)
	var eventType, data string

	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
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
				select {
				case events <- evt:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			eventType = ""
			data = ""
		}
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return fmt.Errorf("SSE stream ended unexpectedly")
}

type sseStatusError struct {
	code      int
	boardSlug string
}

func (e *sseStatusError) Error() string {
	switch e.code {
	case 401:
		return "authentication failed — check your API key"
	case 404:
		return fmt.Sprintf("board %q not found", e.boardSlug)
	default:
		return fmt.Sprintf("server returned status %d", e.code)
	}
}

func isPermanent(err error) bool {
	if e, ok := err.(*sseStatusError); ok {
		return e.code == 401 || e.code == 404
	}
	return false
}
