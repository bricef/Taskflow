package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

const sseHeartbeatInterval = 30 * time.Second

func (s *Server) sseHandler(w http.ResponseWriter, r *http.Request) {
	if s.bus == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "event streaming not enabled", nil)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal", "streaming not supported", nil)
		return
	}

	slug := chi.URLParam(r, "slug")

	// Validate board exists.
	if _, err := s.svc.GetBoard(r.Context(), slug); err != nil {
		writeServiceError(w, err)
		return
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
			// Filter to requested board.
			if evt.Board.Slug != slug {
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
