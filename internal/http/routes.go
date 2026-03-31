package http

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"

	"github.com/bricef/taskflow/internal/http/dashboard"
	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/transport"
)

// Route is the HTTP-layer representation of a domain resource or operation.
type Route struct {
	Name    string
	Path    string
	Summary string
	MinRole model.Role
	Input   any              // nil for resources
	Output  any
	Params  []model.QueryParam // query params (resources only)
	Method  string           // "GET", "POST", etc.
	Status  int              // 200, 201, 204
	Handler handler
}

// PathParams returns the path parameters inferred from the Path pattern.
func (r Route) PathParams() []model.PathParam {
	return model.InferPathParams(r.Path)
}

// allRoutes derives HTTP routes from model.Resources() and model.Operations(),
// attaching handlers by name.
func (s *Server) allRoutes() []Route {
	resHandlers := s.resourceHandlers()
	opHandlers := s.operationHandlers()

	var routes []Route

	for _, res := range model.Resources() {
		h, ok := resHandlers[res.Name]
		if !ok {
			panic("no handler for resource: " + res.Name)
		}
		routes = append(routes, Route{
			Name:    res.Name,
			Path:    res.Path,
			Summary: res.Summary,
			MinRole: res.MinRole,
			Output:  res.Output,
			Params:  res.Params,
			Method:  "GET",
			Status:  200,
			Handler: h,
		})
	}

	for _, op := range model.Operations() {
		h, ok := opHandlers[op.Name]
		if !ok {
			panic("no handler for operation: " + op.Name)
		}
		routes = append(routes, Route{
			Name:    op.Name,
			Path:    op.Path,
			Summary: op.Summary,
			MinRole: op.MinRole,
			Input:   op.Input,
			Output:  op.Output,
			Method:  transport.MethodForAction(op.Action),
			Status:  transport.StatusForAction(op.Action),
			Handler: h,
		})
	}

	return routes
}

// resourceHandlers returns handlers keyed by resource name.
func (s *Server) resourceHandlers() map[string]handler {
	return map[string]handler{
		// Actors
		"actor_list": noInput(s.svc.ListActors),
		"actor_get":  pathStr("name", s.svc.GetActor),

		// Boards
		"board_list": s.listBoards,
		"board_get":  pathStr("slug", s.svc.GetBoard),

		// Workflows
		"workflow_get": pathStr("slug", s.svc.GetWorkflow),

		// Tasks
		"task_list": s.listTasks,
		"task_get":  pathStrInt("slug", "num", s.svc.GetTask),

		// Tags
		"tag_list": s.listTags,

		// Comments
		"comment_list": pathStrInt("slug", "num", s.svc.ListComments),

		// Dependencies
		"dependency_list": pathStrInt("slug", "num", s.svc.ListDependencies),

		// Attachments
		"attachment_list": pathStrInt("slug", "num", s.svc.ListAttachments),

		// Views
		"board_detail":   pathStr("slug", s.svc.BoardDetail),
		"board_overview": pathStr("slug", s.svc.BoardOverview),
		"admin_stats":    noInput(s.svc.SystemStats),

		// Webhooks
		"webhook_list":  noInput(s.svc.ListWebhooks),
		"webhook_get":   pathInt("id", s.svc.GetWebhook),
		"delivery_list": pathInt("id", s.svc.ListWebhookDeliveries),

	}
}

// operationHandlers returns handlers keyed by operation name.
func (s *Server) operationHandlers() map[string]handler {
	return map[string]handler{
		// Actors
		"actor_create": jsonBody(s.svc.CreateActor),
		"actor_update": s.updateActor,

		// Boards
		"board_create":   jsonBody(s.svc.CreateBoard),
		"board_update":   s.updateBoard,
		"board_delete":   s.deleteBoard,
		"board_reassign": s.reassignTasks,

		// Workflows
		"workflow_set":    s.setWorkflow,
		"workflow_health": pathStr("slug", s.svc.CheckWorkflowHealth),

		// Tasks
		"task_create":     s.createTask,
		"task_update":     s.updateTask,
		"task_transition": s.transitionTask,
		"task_delete":     s.deleteTask,

		// Comments
		"comment_create": s.createComment,
		"comment_update": s.updateComment,

		// Dependencies
		"dependency_create": s.createDependency,
		"dependency_delete": s.deleteDependency,

		// Attachments
		"attachment_create": s.createAttachment,
		"attachment_delete": s.deleteAttachment,

		// Audit
		"task_audit":  pathStrInt("slug", "num", s.svc.QueryAuditByTask),
		"board_audit": pathStr("slug", s.svc.QueryAuditByBoard),

		// Webhooks
		"webhook_create": s.createWebhook,
		"webhook_update": s.updateWebhook,
		"webhook_delete": s.deleteWebhook,
	}
}

// registerRoutes registers all routes with the chi router.
func (s *Server) registerRoutes() {
	r := s.router

	// Rate limit public endpoints by IP (disabled in dev mode).
	if !s.cfg.DevMode {
		r.Use(httprate.Limit(30, time.Minute, httprate.WithKeyFuncs(httprate.KeyByRealIP)))
	}

	// Dashboard — static HTML, no auth (uses API key from client-side JS).
	dashFS, _ := fs.Sub(dashboard.FS, ".")
	r.Get("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		data, _ := fs.ReadFile(dashFS, "index.html")
		w.Write(data)
	})
	r.Get("/dashboard/board/{slug}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		data, _ := fs.ReadFile(dashFS, "board.html")
		w.Write(data)
	})

	// Public endpoints — no auth required.
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	r.Get("/openapi.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(s.openAPISpec)
	})

	// Authenticated routes — derived from resources and operations.
	r.Group(func(r chi.Router) {
		if !s.cfg.DevMode && s.cfg.RateLimitPerSecond > 0 {
			r.Use(httprate.Limit(
				s.cfg.RateLimitPerSecond,
				time.Second,
				httprate.WithKeyFuncs(func(r *http.Request) (string, error) {
					return r.Header.Get("Authorization"), nil
				}),
			))
		}
		r.Use(authMiddleware(s.svc))

		// Convenience endpoints (not yet in model — transport-specific or pending query param work).
		r.Get("/boards/{slug}/events", s.sseHandler)
		r.Get("/events", s.globalSSEHandler)
		r.Get("/tasks", s.globalTasksHandler)
		r.Get("/search", s.searchHandler)
		r.Post("/batch", s.batchHandler)

		// Domain resources and operations.
		for _, rt := range s.allRoutes() {
			h := s.handle(rt.MinRole, rt.Status, rt.Handler)
			switch rt.Method {
			case "GET":
				r.Get(rt.Path, h)
			case "POST":
				r.Post(rt.Path, h)
			case "PUT":
				r.Put(rt.Path, h)
			case "PATCH":
				r.Patch(rt.Path, h)
			case "DELETE":
				r.Delete(rt.Path, h)
			}
		}
	})
}
