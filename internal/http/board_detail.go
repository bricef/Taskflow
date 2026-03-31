package http

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/bricef/taskflow/internal/model"
)

type boardDetail struct {
	Board    model.Board        `json:"board"`
	Workflow any                `json:"workflow"`
	Tasks    []taskDetail       `json:"tasks"`
	Audit    []model.AuditEntry `json:"audit"`
}

type taskDetail struct {
	model.Task
	Comments     []model.Comment    `json:"comments"`
	Attachments  []model.Attachment `json:"attachments"`
	Dependencies []model.Dependency `json:"dependencies"`
	Audit        []model.AuditEntry `json:"audit"`
}

func (s *Server) boardDetailHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug := chi.URLParam(r, "slug")

	board, err := s.svc.GetBoard(ctx, slug)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wf, err := s.svc.GetWorkflow(ctx, slug)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	tasks, err := s.svc.ListTasks(ctx, model.TaskFilter{
		BoardSlug:      slug,
		IncludeClosed:  true,
		IncludeDeleted: false,
	}, nil)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	boardAudit, err := s.svc.QueryAuditByBoard(ctx, slug)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	// Cap task detail expansion to prevent memory exhaustion.
	if len(tasks) > 500 {
		tasks = tasks[:500]
	}

	var taskDetails []taskDetail
	for _, t := range tasks {
		comments, _ := s.svc.ListComments(ctx, slug, t.Num)
		attachments, _ := s.svc.ListAttachments(ctx, slug, t.Num)
		deps, _ := s.svc.ListDependencies(ctx, slug, t.Num)
		audit, _ := s.svc.QueryAuditByTask(ctx, slug, t.Num)

		taskDetails = append(taskDetails, taskDetail{
			Task:         t,
			Comments:     orEmpty(comments),
			Attachments:  orEmpty(attachments),
			Dependencies: orEmpty(deps),
			Audit:        orEmpty(audit),
		})
	}

	detail := boardDetail{
		Board:    board,
		Workflow: wf,
		Tasks:    taskDetails,
		Audit:    orEmpty(boardAudit),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(detail); err != nil {
		log.Printf("error encoding board detail: %v", err)
	}
}

// orEmpty ensures nil slices serialize as [] rather than null.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
