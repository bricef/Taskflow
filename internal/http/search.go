package http

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/bricef/taskflow/internal/model"
)

// searchHandler searches tasks across all boards via full-text search.
// GET /search?q=<query>&state=<state>&assignee=<name>&priority=<p>
func (s *Server) searchHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "query parameter 'q' is required", nil)
		return
	}
	if len(q) > 500 {
		writeError(w, http.StatusBadRequest, "validation_error", "query too long (max 500 characters)", nil)
		return
	}

	boards, err := s.svc.ListBoards(ctx, model.ListBoardsParams{})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	assignee := queryStr(r, "assignee")
	resolveAtMe(ctx, assignee)

	var results []model.Task
	for _, b := range boards {
		if len(results) >= maxQueryResults {
			break
		}
		filter := model.TaskFilter{
			BoardSlug: b.Slug,
			Query:     &q,
			Assignee:  assignee,
			State:     queryStr(r, "state"),
		}
		if p := queryStr(r, "priority"); p != nil {
			pv := model.Priority(*p)
			filter.Priority = &pv
		}
		tasks, err := s.svc.ListTasks(ctx, filter, nil)
		if err != nil {
			continue
		}
		results = append(results, tasks...)
	}

	if len(results) > maxQueryResults {
		results = results[:maxQueryResults]
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(orEmpty(results)); err != nil {
		log.Printf("error encoding search results: %v", err)
	}
}
