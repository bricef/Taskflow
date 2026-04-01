package http

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/bricef/taskflow/internal/model"
)

// --- Actors ---

func (s *Server) updateActor(ctx context.Context, r *http.Request) (any, error) {
	var p model.UpdateActorParams
	if err := decodeBody(r, &p); err != nil {
		return nil, err
	}
	p.Name = urlParamStr(r, "name")
	return s.svc.UpdateActor(ctx, p)
}

// --- Actors ---

func (s *Server) createActor(ctx context.Context, r *http.Request) (any, error) {
	var p model.CreateActorParams
	if err := decodeBody(r, &p); err != nil {
		return nil, err
	}

	// Generate an API key for the new actor.
	apiKey, err := generateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("generating API key: %w", err)
	}
	p.APIKeyHash = HashAPIKey(apiKey)

	actor, err := s.svc.CreateActor(ctx, p)
	if err != nil {
		return nil, err
	}

	// Return the actor with the plaintext API key (shown once).
	type actorWithKey struct {
		model.Actor
		APIKey string `json:"api_key"`
	}
	return actorWithKey{Actor: actor, APIKey: apiKey}, nil
}

func generateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// --- Boards ---

func (s *Server) listBoards(ctx context.Context, r *http.Request) (any, error) {
	return s.svc.ListBoards(ctx, model.ListBoardsParams{
		IncludeDeleted: queryBool(r, "include_deleted"),
	})
}

func (s *Server) updateBoard(ctx context.Context, r *http.Request) (any, error) {
	var p model.UpdateBoardParams
	if err := decodeBody(r, &p); err != nil {
		return nil, err
	}
	p.Slug = urlParamStr(r, "slug")
	return s.svc.UpdateBoard(ctx, p)
}

func (s *Server) deleteBoard(ctx context.Context, r *http.Request) (any, error) {
	return nil, s.svc.DeleteBoard(ctx, urlParamStr(r, "slug"), ActorFrom(ctx).Name)
}

func (s *Server) reassignTasks(ctx context.Context, r *http.Request) (any, error) {
	var body struct {
		TargetBoard string   `json:"target_board"`
		States      []string `json:"states"`
	}
	if err := decodeBody(r, &body); err != nil {
		return nil, err
	}
	count, err := s.svc.ReassignTasks(ctx, urlParamStr(r, "slug"), body.TargetBoard, ActorFrom(ctx).Name, body.States)
	if err != nil {
		return nil, err
	}
	return map[string]int{"count": count}, nil
}

// --- Workflows ---

func (s *Server) setWorkflow(ctx context.Context, r *http.Request) (any, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, &model.ValidationError{Field: "body", Message: "could not read request body"}
	}
	return nil, s.svc.SetWorkflow(ctx, urlParamStr(r, "slug"), json.RawMessage(body), ActorFrom(ctx).Name)
}

// --- Tasks ---

func (s *Server) createTask(ctx context.Context, r *http.Request) (any, error) {
	var p model.CreateTaskParams
	if err := decodeBody(r, &p); err != nil {
		return nil, err
	}
	p.BoardSlug = urlParamStr(r, "slug")
	p.CreatedBy = ActorFrom(ctx).Name
	resolveAtMe(ctx, p.Assignee)
	return s.svc.CreateTask(ctx, p)
}

func parseTaskFilter(ctx context.Context, r *http.Request) model.TaskFilter {
	assignee := queryStr(r, "assignee")
	resolveAtMe(ctx, assignee)
	filter := model.TaskFilter{
		State:          queryStr(r, "state"),
		Assignee:       assignee,
		Tag:            queryStr(r, "tag"),
		Query:          queryStr(r, "q"),
		IncludeClosed:  queryBool(r, "include_closed"),
		IncludeDeleted: queryBool(r, "include_deleted"),
	}
	if p := queryStr(r, "priority"); p != nil {
		pv := model.Priority(*p)
		filter.Priority = &pv
	}
	return filter
}

func parseTaskSort(r *http.Request) (*model.TaskSort, error) {
	field := queryStr(r, "sort")
	if field == nil {
		return nil, nil
	}
	validSortFields := map[string]bool{
		"created_at": true, "updated_at": true, "due_date": true, "priority": true,
	}
	if !validSortFields[*field] {
		return nil, &model.ValidationError{Field: "sort", Message: "must be one of: created_at, updated_at, due_date, priority"}
	}
	return &model.TaskSort{
		Field: *field,
		Desc:  r.URL.Query().Get("order") == "desc",
	}, nil
}

func (s *Server) listTasks(ctx context.Context, r *http.Request) (any, error) {
	filter := parseTaskFilter(ctx, r)
	filter.BoardSlug = urlParamStr(r, "slug")
	sort, err := parseTaskSort(r)
	if err != nil {
		return nil, err
	}
	return s.svc.ListTasks(ctx, filter, sort)
}

func (s *Server) searchTasks(ctx context.Context, r *http.Request) (any, error) {
	filter := parseTaskFilter(ctx, r)
	return s.svc.SearchTasks(ctx, filter)
}

func (s *Server) updateTask(ctx context.Context, r *http.Request) (any, error) {
	var p model.UpdateTaskParams
	if err := decodeBody(r, &p); err != nil {
		return nil, err
	}
	p.BoardSlug = urlParamStr(r, "slug")
	var err error
	p.Num, err = urlParamInt(r, "num")
	if err != nil {
		return nil, err
	}
	if p.Assignee.Set {
		resolveAtMe(ctx, p.Assignee.Value)
	}
	return s.svc.UpdateTask(ctx, p, ActorFrom(ctx).Name)
}

func (s *Server) transitionTask(ctx context.Context, r *http.Request) (any, error) {
	var p model.TransitionTaskParams
	if err := decodeBody(r, &p); err != nil {
		return nil, err
	}
	p.BoardSlug = urlParamStr(r, "slug")
	var err error
	p.Num, err = urlParamInt(r, "num")
	if err != nil {
		return nil, err
	}
	p.Actor = ActorFrom(ctx).Name
	return s.svc.TransitionTask(ctx, p)
}

func (s *Server) deleteTask(ctx context.Context, r *http.Request) (any, error) {
	num, err := urlParamInt(r, "num")
	if err != nil {
		return nil, err
	}
	return nil, s.svc.DeleteTask(ctx, urlParamStr(r, "slug"), num, ActorFrom(ctx).Name)
}

// --- Comments ---

func (s *Server) createComment(ctx context.Context, r *http.Request) (any, error) {
	var p model.CreateCommentParams
	if err := decodeBody(r, &p); err != nil {
		return nil, err
	}
	p.BoardSlug = urlParamStr(r, "slug")
	var err error
	p.TaskNum, err = urlParamInt(r, "num")
	if err != nil {
		return nil, err
	}
	p.Actor = ActorFrom(ctx).Name
	return s.svc.CreateComment(ctx, p)
}

func (s *Server) updateComment(ctx context.Context, r *http.Request) (any, error) {
	var p model.UpdateCommentParams
	if err := decodeBody(r, &p); err != nil {
		return nil, err
	}
	var err error
	p.ID, err = urlParamInt(r, "id")
	if err != nil {
		return nil, err
	}
	return s.svc.UpdateComment(ctx, p, ActorFrom(ctx).Name)
}

// --- Dependencies ---

func (s *Server) createDependency(ctx context.Context, r *http.Request) (any, error) {
	var p model.CreateDependencyParams
	if err := decodeBody(r, &p); err != nil {
		return nil, err
	}
	p.BoardSlug = urlParamStr(r, "slug")
	var err error
	p.TaskNum, err = urlParamInt(r, "num")
	if err != nil {
		return nil, err
	}
	p.CreatedBy = ActorFrom(ctx).Name
	return s.svc.CreateDependency(ctx, p)
}

func (s *Server) deleteDependency(ctx context.Context, r *http.Request) (any, error) {
	id, err := urlParamInt(r, "id")
	if err != nil {
		return nil, err
	}
	return nil, s.svc.DeleteDependency(ctx, id, ActorFrom(ctx).Name)
}

// --- Attachments ---

func (s *Server) createAttachment(ctx context.Context, r *http.Request) (any, error) {
	var p model.CreateAttachmentParams
	if err := decodeBody(r, &p); err != nil {
		return nil, err
	}
	p.BoardSlug = urlParamStr(r, "slug")
	var err error
	p.TaskNum, err = urlParamInt(r, "num")
	if err != nil {
		return nil, err
	}
	p.CreatedBy = ActorFrom(ctx).Name
	return s.svc.CreateAttachment(ctx, p)
}

func (s *Server) deleteAttachment(ctx context.Context, r *http.Request) (any, error) {
	id, err := urlParamInt(r, "id")
	if err != nil {
		return nil, err
	}
	return nil, s.svc.DeleteAttachment(ctx, id, ActorFrom(ctx).Name)
}

// --- Webhooks ---

func (s *Server) createWebhook(ctx context.Context, r *http.Request) (any, error) {
	var p model.CreateWebhookParams
	if err := decodeBody(r, &p); err != nil {
		return nil, err
	}
	p.CreatedBy = ActorFrom(ctx).Name
	return s.svc.CreateWebhook(ctx, p)
}

func (s *Server) updateWebhook(ctx context.Context, r *http.Request) (any, error) {
	var p model.UpdateWebhookParams
	if err := decodeBody(r, &p); err != nil {
		return nil, err
	}
	var err error
	p.ID, err = urlParamInt(r, "id")
	if err != nil {
		return nil, err
	}
	return s.svc.UpdateWebhook(ctx, p)
}

func (s *Server) deleteWebhook(ctx context.Context, r *http.Request) (any, error) {
	id, err := urlParamInt(r, "id")
	if err != nil {
		return nil, err
	}
	return nil, s.svc.DeleteWebhook(ctx, id)
}

// --- Tags ---

func (s *Server) listTags(ctx context.Context, r *http.Request) (any, error) {
	return s.svc.ListTags(ctx, urlParamStr(r, "slug"))
}
