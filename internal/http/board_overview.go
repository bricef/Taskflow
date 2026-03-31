package http

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/bricef/taskflow/internal/model"
)

type boardOverview struct {
	model.Board
	TaskCounts map[string]int `json:"task_counts"` // state → count
	TotalTasks int            `json:"total_tasks"`
}

// boardOverviewHandler returns board metadata with task counts by state.
// GET /boards/{slug}/overview
func (s *Server) boardOverviewHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug := chi.URLParam(r, "slug")

	board, err := s.svc.GetBoard(ctx, slug)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	tasks, err := s.svc.ListTasks(ctx, model.TaskFilter{
		BoardSlug:     slug,
		IncludeClosed: true,
	}, nil)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	counts := map[string]int{}
	for _, t := range tasks {
		counts[t.State]++
	}

	overview := boardOverview{
		Board:      board,
		TaskCounts: counts,
		TotalTasks: len(tasks),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(overview); err != nil {
		log.Printf("error encoding board overview: %v", err)
	}
}
