package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/bricef/taskflow/internal/model"
)

func (s *Server) routes() {
	r := s.router

	// Public endpoints — no auth required.
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	r.Get("/openapi.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(s.openAPISpec)
	})

	// All other routes require auth.
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware(s.svc))

		// === Actors ===
		r.Post("/actors", s.handle(model.RoleAdmin, 201, jsonBody(s.svc.CreateActor)))
		r.Get("/actors", s.handle(model.RoleReadOnly, 200, noInput(s.svc.ListActors)))
		r.Get("/actors/{name}", s.handle(model.RoleReadOnly, 200, pathStr("name", s.svc.GetActor)))
		r.Patch("/actors/{name}", s.handle(model.RoleAdmin, 200, func(ctx context.Context, r *http.Request) (any, error) {
			var p model.UpdateActorParams
			if err := decodeBody(r, &p); err != nil {
				return nil, err
			}
			p.Name = urlParamStr(r, "name")
			return s.svc.UpdateActor(ctx, p)
		}))

		// === Boards ===
		r.Post("/boards", s.handle(model.RoleMember, 201, jsonBody(s.svc.CreateBoard)))
		r.Get("/boards", s.handle(model.RoleReadOnly, 200, func(ctx context.Context, r *http.Request) (any, error) {
			return s.svc.ListBoards(ctx, model.ListBoardsParams{
				IncludeDeleted: queryBool(r, "include_deleted"),
			})
		}))
		r.Get("/boards/{slug}", s.handle(model.RoleReadOnly, 200, pathStr("slug", s.svc.GetBoard)))
		r.Patch("/boards/{slug}", s.handle(model.RoleMember, 200, func(ctx context.Context, r *http.Request) (any, error) {
			var p model.UpdateBoardParams
			if err := decodeBody(r, &p); err != nil {
				return nil, err
			}
			p.Slug = urlParamStr(r, "slug")
			return s.svc.UpdateBoard(ctx, p)
		}))
		r.Delete("/boards/{slug}", s.handle(model.RoleAdmin, 204, func(ctx context.Context, r *http.Request) (any, error) {
			return nil, s.svc.DeleteBoard(ctx, urlParamStr(r, "slug"), ActorFrom(ctx).Name)
		}))
		r.Post("/boards/{slug}/reassign", s.handle(model.RoleAdmin, 200, func(ctx context.Context, r *http.Request) (any, error) {
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
		}))

		// === Workflows ===
		r.Get("/boards/{slug}/workflow", s.handle(model.RoleReadOnly, 200, pathStr("slug", s.svc.GetWorkflow)))
		r.Put("/boards/{slug}/workflow", s.handle(model.RoleMember, 204, func(ctx context.Context, r *http.Request) (any, error) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				return nil, &model.ValidationError{Field: "body", Message: "could not read request body"}
			}
			return nil, s.svc.SetWorkflow(ctx, urlParamStr(r, "slug"), json.RawMessage(body), ActorFrom(ctx).Name)
		}))
		r.Get("/boards/{slug}/workflow/health", s.handle(model.RoleReadOnly, 200, pathStr("slug", s.svc.CheckWorkflowHealth)))

		// === Tasks ===
		r.Post("/boards/{slug}/tasks", s.handle(model.RoleMember, 201, func(ctx context.Context, r *http.Request) (any, error) {
			var p model.CreateTaskParams
			if err := decodeBody(r, &p); err != nil {
				return nil, err
			}
			p.BoardSlug = urlParamStr(r, "slug")
			p.CreatedBy = ActorFrom(ctx).Name
			return s.svc.CreateTask(ctx, p)
		}))
		r.Get("/boards/{slug}/tasks", s.handle(model.RoleReadOnly, 200, func(ctx context.Context, r *http.Request) (any, error) {
			filter := model.TaskFilter{
				BoardSlug:      urlParamStr(r, "slug"),
				State:          queryStr(r, "state"),
				Assignee:       queryStr(r, "assignee"),
				Tag:            queryStr(r, "tag"),
				Query:          queryStr(r, "q"),
				IncludeClosed:  queryBool(r, "include_closed"),
				IncludeDeleted: queryBool(r, "include_deleted"),
			}
			if p := queryStr(r, "priority"); p != nil {
				pv := model.Priority(*p)
				filter.Priority = &pv
			}
			var sort *model.TaskSort
			if field := queryStr(r, "sort"); field != nil {
				sort = &model.TaskSort{
					Field: *field,
					Desc:  r.URL.Query().Get("order") == "desc",
				}
			}
			return s.svc.ListTasks(ctx, filter, sort)
		}))
		r.Get("/boards/{slug}/tasks/{num}", s.handle(model.RoleReadOnly, 200, pathStrInt("slug", "num", s.svc.GetTask)))
		r.Patch("/boards/{slug}/tasks/{num}", s.handle(model.RoleMember, 200, func(ctx context.Context, r *http.Request) (any, error) {
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
			return s.svc.UpdateTask(ctx, p, ActorFrom(ctx).Name)
		}))
		r.Post("/boards/{slug}/tasks/{num}/transition", s.handle(model.RoleMember, 200, func(ctx context.Context, r *http.Request) (any, error) {
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
		}))
		r.Delete("/boards/{slug}/tasks/{num}", s.handle(model.RoleMember, 204, func(ctx context.Context, r *http.Request) (any, error) {
			num, err := urlParamInt(r, "num")
			if err != nil {
				return nil, err
			}
			return nil, s.svc.DeleteTask(ctx, urlParamStr(r, "slug"), num, ActorFrom(ctx).Name)
		}))

		// === Comments ===
		r.Post("/boards/{slug}/tasks/{num}/comments", s.handle(model.RoleMember, 201, func(ctx context.Context, r *http.Request) (any, error) {
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
		}))
		r.Get("/boards/{slug}/tasks/{num}/comments", s.handle(model.RoleReadOnly, 200, pathStrInt("slug", "num", s.svc.ListComments)))
		r.Patch("/comments/{id}", s.handle(model.RoleMember, 200, func(ctx context.Context, r *http.Request) (any, error) {
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
		}))

		// === Dependencies ===
		r.Post("/boards/{slug}/tasks/{num}/dependencies", s.handle(model.RoleMember, 201, func(ctx context.Context, r *http.Request) (any, error) {
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
		}))
		r.Get("/boards/{slug}/tasks/{num}/dependencies", s.handle(model.RoleReadOnly, 200, pathStrInt("slug", "num", s.svc.ListDependencies)))
		r.Delete("/dependencies/{id}", s.handle(model.RoleMember, 204, func(ctx context.Context, r *http.Request) (any, error) {
			id, err := urlParamInt(r, "id")
			if err != nil {
				return nil, err
			}
			return nil, s.svc.DeleteDependency(ctx, id, ActorFrom(ctx).Name)
		}))

		// === Attachments ===
		r.Post("/boards/{slug}/tasks/{num}/attachments", s.handle(model.RoleMember, 201, func(ctx context.Context, r *http.Request) (any, error) {
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
		}))
		r.Get("/boards/{slug}/tasks/{num}/attachments", s.handle(model.RoleReadOnly, 200, pathStrInt("slug", "num", s.svc.ListAttachments)))
		r.Delete("/attachments/{id}", s.handle(model.RoleMember, 204, func(ctx context.Context, r *http.Request) (any, error) {
			id, err := urlParamInt(r, "id")
			if err != nil {
				return nil, err
			}
			return nil, s.svc.DeleteAttachment(ctx, id, ActorFrom(ctx).Name)
		}))

		// === Webhooks ===
		r.Post("/webhooks", s.handle(model.RoleAdmin, 201, func(ctx context.Context, r *http.Request) (any, error) {
			var p model.CreateWebhookParams
			if err := decodeBody(r, &p); err != nil {
				return nil, err
			}
			p.CreatedBy = ActorFrom(ctx).Name
			return s.svc.CreateWebhook(ctx, p)
		}))
		r.Get("/webhooks", s.handle(model.RoleAdmin, 200, noInput(s.svc.ListWebhooks)))
		r.Get("/webhooks/{id}", s.handle(model.RoleAdmin, 200, pathInt("id", s.svc.GetWebhook)))
		r.Patch("/webhooks/{id}", s.handle(model.RoleAdmin, 200, func(ctx context.Context, r *http.Request) (any, error) {
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
		}))
		r.Delete("/webhooks/{id}", s.handle(model.RoleAdmin, 204, func(ctx context.Context, r *http.Request) (any, error) {
			id, err := urlParamInt(r, "id")
			if err != nil {
				return nil, err
			}
			return nil, s.svc.DeleteWebhook(ctx, id)
		}))

		// === Audit ===
		r.Get("/boards/{slug}/tasks/{num}/audit", s.handle(model.RoleReadOnly, 200, pathStrInt("slug", "num", s.svc.QueryAuditByTask)))
		r.Get("/boards/{slug}/audit", s.handle(model.RoleReadOnly, 200, pathStr("slug", s.svc.QueryAuditByBoard)))
	})
}
