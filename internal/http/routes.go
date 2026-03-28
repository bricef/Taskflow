package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/bricef/taskflow/internal/model"
)

// route is the single source of truth for each API endpoint.
// Both chi registration and OpenAPI spec generation consume this data.
type route struct {
	Method  string
	Path    string
	Summary string
	MinRole model.Role
	Status  int
	Input   any         // nil, or a zero-value of the request body type (for OpenAPI schema)
	Output  any         // nil, or a zero-value of the response type (for OpenAPI schema)
	Params  []paramMeta // path and query parameters (for OpenAPI schema)
	Handler handler     // the actual handler function
}

type paramMeta struct {
	Name     string
	In       string // "path" or "query"
	Type     string // "string", "integer", "boolean"
	Required bool
	Desc     string
}

// allRoutes returns every API route with its metadata and handler.
func (s *Server) allRoutes() []route {
	return []route{
		// Actors
		Post("/actors", "Create an actor").
			Role(model.RoleAdmin).
			Input(model.CreateActorParams{}).
			Output(model.Actor{}).
			Handle(jsonBody(s.svc.CreateActor)),

		Get("/actors", "List all actors").
			Output([]model.Actor{}).
			Handle(noInput(s.svc.ListActors)),

		Get("/actors/{name}", "Get an actor by name").
			Output(model.Actor{}).
			Handle(pathStr("name", s.svc.GetActor)),

		Patch("/actors/{name}", "Update an actor").
			Role(model.RoleAdmin).
			Input(model.UpdateActorParams{}).
			Output(model.Actor{}).
			Handle(s.updateActor),

		// Boards
		Post("/boards", "Create a board").
			Input(model.CreateBoardParams{}).
			Output(model.Board{}).
			Handle(jsonBody(s.svc.CreateBoard)),

		Get("/boards", "List boards").
			Output([]model.Board{}).
			QueryParams(Query("include_deleted", "boolean", "Include soft-deleted boards")).
			Handle(s.listBoards),

		Get("/boards/{slug}", "Get a board").
			Output(model.Board{}).
			Handle(pathStr("slug", s.svc.GetBoard)),

		Patch("/boards/{slug}", "Update a board").
			Input(model.UpdateBoardParams{}).
			Output(model.Board{}).
			Handle(s.updateBoard),

		Delete("/boards/{slug}", "Delete a board (soft-delete)").
			Role(model.RoleAdmin).
			Handle(s.deleteBoard),

		Post("/boards/{slug}/reassign", "Reassign tasks to another board").
			Role(model.RoleAdmin).
			Status(200).
			Handle(s.reassignTasks),

		// Workflows
		Get("/boards/{slug}/workflow", "Get the board's workflow definition").
			Handle(pathStr("slug", s.svc.GetWorkflow)),

		Put("/boards/{slug}/workflow", "Replace the board's workflow").
			Handle(s.setWorkflow),

		Get("/boards/{slug}/workflow/health", "Check workflow health").
			Handle(pathStr("slug", s.svc.CheckWorkflowHealth)),

		// Tasks
		Post("/boards/{slug}/tasks", "Create a task").
			Input(model.CreateTaskParams{}).
			Output(model.Task{}).
			Handle(s.createTask),

		Get("/boards/{slug}/tasks", "List tasks (with filters and search)").
			Output([]model.Task{}).
			QueryParams(
				Query("state", "string", "Filter by workflow state"),
				Query("assignee", "string", "Filter by assignee name"),
				Query("priority", "string", "Filter by priority (critical/high/medium/low/none)"),
				Query("tag", "string", "Filter by tag"),
				Query("q", "string", "Full-text search query"),
				Query("include_closed", "boolean", "Include tasks in terminal states"),
				Query("include_deleted", "boolean", "Include soft-deleted tasks"),
				Query("sort", "string", "Sort field (created_at/updated_at/priority/due_date)"),
				Query("order", "string", "Sort order (asc/desc)"),
			).
			Handle(s.listTasks),

		Get("/boards/{slug}/tasks/{num}", "Get a task").
			Output(model.Task{}).
			Handle(pathStrInt("slug", "num", s.svc.GetTask)),

		Patch("/boards/{slug}/tasks/{num}", "Update a task").
			Input(model.UpdateTaskParams{}).
			Output(model.Task{}).
			Handle(s.updateTask),

		Post("/boards/{slug}/tasks/{num}/transition", "Transition a task to a new state").
			Status(200).
			Input(model.TransitionTaskParams{}).
			Output(model.Task{}).
			Handle(s.transitionTask),

		Delete("/boards/{slug}/tasks/{num}", "Delete a task (soft-delete)").
			Handle(s.deleteTask),

		// Comments
		Post("/boards/{slug}/tasks/{num}/comments", "Add a comment to a task").
			Input(model.CreateCommentParams{}).
			Output(model.Comment{}).
			Handle(s.createComment),

		Get("/boards/{slug}/tasks/{num}/comments", "List comments on a task").
			Output([]model.Comment{}).
			Handle(pathStrInt("slug", "num", s.svc.ListComments)),

		Patch("/comments/{id}", "Edit a comment").
			Input(model.UpdateCommentParams{}).
			Output(model.Comment{}).
			Handle(s.updateComment),

		// Dependencies
		Post("/boards/{slug}/tasks/{num}/dependencies", "Add a dependency").
			Input(model.CreateDependencyParams{}).
			Output(model.Dependency{}).
			Handle(s.createDependency),

		Get("/boards/{slug}/tasks/{num}/dependencies", "List dependencies for a task").
			Output([]model.Dependency{}).
			Handle(pathStrInt("slug", "num", s.svc.ListDependencies)),

		Delete("/dependencies/{id}", "Remove a dependency").
			Handle(s.deleteDependency),

		// Attachments
		Post("/boards/{slug}/tasks/{num}/attachments", "Add an attachment").
			Input(model.CreateAttachmentParams{}).
			Output(model.Attachment{}).
			Handle(s.createAttachment),

		Get("/boards/{slug}/tasks/{num}/attachments", "List attachments on a task").
			Output([]model.Attachment{}).
			Handle(pathStrInt("slug", "num", s.svc.ListAttachments)),

		Delete("/attachments/{id}", "Remove an attachment").
			Handle(s.deleteAttachment),

		// Webhooks
		Post("/webhooks", "Create a webhook").
			Role(model.RoleAdmin).
			Input(model.CreateWebhookParams{}).
			Output(model.Webhook{}).
			Handle(s.createWebhook),

		Get("/webhooks", "List webhooks").
			Role(model.RoleAdmin).
			Output([]model.Webhook{}).
			Handle(noInput(s.svc.ListWebhooks)),

		Get("/webhooks/{id}", "Get a webhook").
			Role(model.RoleAdmin).
			Output(model.Webhook{}).
			Handle(pathInt("id", s.svc.GetWebhook)),

		Patch("/webhooks/{id}", "Update a webhook").
			Role(model.RoleAdmin).
			Input(model.UpdateWebhookParams{}).
			Output(model.Webhook{}).
			Handle(s.updateWebhook),

		Delete("/webhooks/{id}", "Delete a webhook").
			Role(model.RoleAdmin).
			Handle(s.deleteWebhook),

		// Audit
		Get("/boards/{slug}/tasks/{num}/audit", "Get audit log for a task").
			Output([]model.AuditEntry{}).
			Handle(pathStrInt("slug", "num", s.svc.QueryAuditByTask)),

		Get("/boards/{slug}/audit", "Get audit log for a board").
			Output([]model.AuditEntry{}).
			Handle(pathStr("slug", s.svc.QueryAuditByBoard)),
	}
}

// registerRoutes registers all routes from allRoutes() with the chi router.
func (s *Server) registerRoutes() {
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

	// Authenticated routes — derived from allRoutes().
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware(s.svc))

		for _, rt := range s.allRoutes() {
			h := s.handle(rt.MinRole, rt.Status, rt.Handler)
			switch strings.ToUpper(rt.Method) {
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
