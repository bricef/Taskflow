package http

import (
	"encoding/json"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/bricef/taskflow/internal/http/dashboard"
	"github.com/bricef/taskflow/internal/model"
)

// Route is the HTTP-layer representation of an operation, with a handler attached.
type Route struct {
	model.Operation
	Handler handler
}

// allRoutes derives HTTP routes from model.Operations() and attaches handlers.
func (s *Server) allRoutes() []Route {
	ops := model.Operations()
	handlers := s.routeHandlers()

	if len(ops) != len(handlers) {
		panic("operation count does not match handler count")
	}

	routes := make([]Route, len(ops))
	for i, op := range ops {
		if handlers[i] == nil {
			panic("nil handler for " + MethodForAction(op.Action) + " " + op.Path)
		}
		routes[i] = Route{Operation: op, Handler: handlers[i]}
	}
	return routes
}

// routeHandlers returns handlers in the same order as model.Operations().
func (s *Server) routeHandlers() []handler {
	return []handler{
		// Actors
		jsonBody(s.svc.CreateActor),
		noInput(s.svc.ListActors),
		pathStr("name", s.svc.GetActor),
		s.updateActor,

		// Boards
		jsonBody(s.svc.CreateBoard),
		s.listBoards,
		pathStr("slug", s.svc.GetBoard),
		s.updateBoard,
		s.deleteBoard,
		s.reassignTasks,

		// Workflows
		pathStr("slug", s.svc.GetWorkflow),
		s.setWorkflow,
		pathStr("slug", s.svc.CheckWorkflowHealth),

		// Tasks
		s.createTask,
		s.listTasks,
		pathStrInt("slug", "num", s.svc.GetTask),
		s.updateTask,
		s.transitionTask,
		s.deleteTask,

		// Tags
		s.listTags,

		// Comments
		s.createComment,
		pathStrInt("slug", "num", s.svc.ListComments),
		s.updateComment,

		// Dependencies
		s.createDependency,
		pathStrInt("slug", "num", s.svc.ListDependencies),
		s.deleteDependency,

		// Attachments
		s.createAttachment,
		pathStrInt("slug", "num", s.svc.ListAttachments),
		s.deleteAttachment,

		// Webhooks
		s.createWebhook,
		noInput(s.svc.ListWebhooks),
		pathInt("id", s.svc.GetWebhook),
		s.updateWebhook,
		s.deleteWebhook,
		pathInt("id", s.svc.ListWebhookDeliveries),

		// Audit
		pathStrInt("slug", "num", s.svc.QueryAuditByTask),
		pathStr("slug", s.svc.QueryAuditByBoard),
	}
}

// registerRoutes registers all routes with the chi router.
func (s *Server) registerRoutes() {
	r := s.router

	// Dashboard — static HTML, no auth (uses API key from client-side JS).
	dashFS, _ := fs.Sub(dashboard.FS, ".")
	r.Get("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		data, _ := fs.ReadFile(dashFS, "index.html")
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

	// Authenticated routes — derived from operations.
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware(s.svc))

		// Aggregate views and utilities (not domain operations).
		r.Get("/boards/{slug}/detail", s.boardDetailHandler)
		r.Get("/boards/{slug}/events", s.sseHandler)
		r.Get("/admin/stats", s.systemStatsHandler)
		r.Get("/search", s.searchHandler)
		r.Post("/batch", s.batchHandler)

		// Domain operations — derived from model.Operations().
		for _, rt := range s.allRoutes() {
			h := s.handle(rt.MinRole, statusForAction(rt.Action), rt.Handler)
			switch MethodForAction(rt.Action) {
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
