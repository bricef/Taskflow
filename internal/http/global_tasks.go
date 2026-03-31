package http

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/bricef/taskflow/internal/model"
)

const maxQueryResults = 1000

// globalTasksHandler lists tasks across all boards with optional filters.
// GET /tasks?assignee=@me&state=in_progress&priority=high&tag=bug&include_closed=true
func (s *Server) globalTasksHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	boards, err := s.svc.ListBoards(ctx, model.ListBoardsParams{})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	assignee := queryStr(r, "assignee")
	resolveAtMe(ctx, assignee)

	var results []model.Task
	for _, b := range boards {
		filter := model.TaskFilter{
			BoardSlug:     b.Slug,
			Assignee:      assignee,
			State:         queryStr(r, "state"),
			Tag:           queryStr(r, "tag"),
			IncludeClosed: queryBool(r, "include_closed"),
		}
		if q := queryStr(r, "q"); q != nil {
			filter.Query = q
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
		log.Printf("error encoding global tasks: %v", err)
	}
}
