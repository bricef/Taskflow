package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bricef/taskflow/internal/eventbus"
)

// globalSSEHandler streams events across all boards with optional filtering.
// GET /events?boards=platform,product&assignee=alice
// Supports @me for assignee.
func (s *Server) globalSSEHandler(w http.ResponseWriter, r *http.Request) {
	if s.bus == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "event streaming not enabled", nil)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal", "streaming not supported", nil)
		return
	}

	// Parse filters.
	boardFilter := map[string]bool{}
	if boards := r.URL.Query().Get("boards"); boards != "" {
		for _, slug := range strings.Split(boards, ",") {
			slug = strings.TrimSpace(slug)
			if slug != "" {
				boardFilter[slug] = true
			}
		}
	}

	var assigneeFilter string
	if a := r.URL.Query().Get("assignee"); a != "" {
		if a == "@me" {
			assigneeFilter = ActorFrom(r.Context()).Name
		} else {
			assigneeFilter = a
		}
	}

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	sub := s.bus.Subscribe()
	defer sub.Cancel()

	heartbeat := time.NewTicker(sseHeartbeatInterval)
	defer heartbeat.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-sub.C:
			if !ok {
				return
			}
			// Board filter.
			if len(boardFilter) > 0 && !boardFilter[evt.Board.Slug] {
				continue
			}
			// Assignee filter: only send events related to tasks assigned to this user.
			if assigneeFilter != "" && !eventMatchesAssignee(evt, assigneeFilter) {
				continue
			}
			data, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Type, data)
			flusher.Flush()
			heartbeat.Reset(sseHeartbeatInterval)
		case <-heartbeat.C:
			fmt.Fprint(w, ":ping\n\n")
			flusher.Flush()
		}
	}
}

// eventMatchesAssignee checks if the event relates to a task assigned to the given user.
// Uses the After snapshot (or Before for deletes) to check assignee.
func eventMatchesAssignee(evt eventbus.Event, assignee string) bool {
	snap := evt.After
	if snap == nil {
		snap = evt.Before
	}
	if snap == nil {
		return false
	}
	return snap.Assignee != nil && *snap.Assignee == assignee
}
