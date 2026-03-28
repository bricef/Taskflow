package http

import (
	"encoding/json"
	"reflect"
	"strings"
	"time"

	"github.com/bricef/taskflow/internal/model"
)

// routeMeta describes an API route for OpenAPI spec generation.
type routeMeta struct {
	Method  string
	Path    string
	Summary string
	MinRole model.Role
	Status  int
	Input   any // nil, or a zero-value instance of the request body type
	Output  any // nil, or a zero-value instance of the response type
	Params  []paramMeta
}

type paramMeta struct {
	Name     string
	In       string // "path" or "query"
	Type     string // "string", "integer", "boolean"
	Required bool
	Desc     string
}

// routeMetadata returns the metadata for all API routes.
// This is the single source of truth that drives both chi registration and OpenAPI generation.
func routeMetadata() []routeMeta {
	return []routeMeta{
		// Actors
		{Method: "POST", Path: "/actors", Summary: "Create an actor", MinRole: model.RoleAdmin, Status: 201,
			Input: model.CreateActorParams{}, Output: model.Actor{}},
		{Method: "GET", Path: "/actors", Summary: "List all actors", MinRole: model.RoleReadOnly, Status: 200,
			Output: []model.Actor{}},
		{Method: "GET", Path: "/actors/{name}", Summary: "Get an actor by name", MinRole: model.RoleReadOnly, Status: 200,
			Output: model.Actor{}, Params: []paramMeta{{Name: "name", In: "path", Type: "string", Required: true}}},
		{Method: "PATCH", Path: "/actors/{name}", Summary: "Update an actor", MinRole: model.RoleAdmin, Status: 200,
			Input: model.UpdateActorParams{}, Output: model.Actor{}, Params: []paramMeta{{Name: "name", In: "path", Type: "string", Required: true}}},

		// Boards
		{Method: "POST", Path: "/boards", Summary: "Create a board", MinRole: model.RoleMember, Status: 201,
			Input: model.CreateBoardParams{}, Output: model.Board{}},
		{Method: "GET", Path: "/boards", Summary: "List boards", MinRole: model.RoleReadOnly, Status: 200,
			Output: []model.Board{}, Params: []paramMeta{
				{Name: "include_deleted", In: "query", Type: "boolean", Desc: "Include soft-deleted boards"},
			}},
		{Method: "GET", Path: "/boards/{slug}", Summary: "Get a board", MinRole: model.RoleReadOnly, Status: 200,
			Output: model.Board{}, Params: []paramMeta{{Name: "slug", In: "path", Type: "string", Required: true}}},
		{Method: "PATCH", Path: "/boards/{slug}", Summary: "Update a board", MinRole: model.RoleMember, Status: 200,
			Input: model.UpdateBoardParams{}, Output: model.Board{}, Params: []paramMeta{{Name: "slug", In: "path", Type: "string", Required: true}}},
		{Method: "DELETE", Path: "/boards/{slug}", Summary: "Delete a board (soft-delete)", MinRole: model.RoleAdmin, Status: 204,
			Params: []paramMeta{{Name: "slug", In: "path", Type: "string", Required: true}}},
		{Method: "POST", Path: "/boards/{slug}/reassign", Summary: "Reassign tasks to another board", MinRole: model.RoleAdmin, Status: 200,
			Params: []paramMeta{{Name: "slug", In: "path", Type: "string", Required: true}}},

		// Workflows
		{Method: "GET", Path: "/boards/{slug}/workflow", Summary: "Get the board's workflow definition", MinRole: model.RoleReadOnly, Status: 200,
			Params: []paramMeta{{Name: "slug", In: "path", Type: "string", Required: true}}},
		{Method: "PUT", Path: "/boards/{slug}/workflow", Summary: "Replace the board's workflow", MinRole: model.RoleMember, Status: 204,
			Params: []paramMeta{{Name: "slug", In: "path", Type: "string", Required: true}}},
		{Method: "GET", Path: "/boards/{slug}/workflow/health", Summary: "Check workflow health", MinRole: model.RoleReadOnly, Status: 200,
			Params: []paramMeta{{Name: "slug", In: "path", Type: "string", Required: true}}},

		// Tasks
		{Method: "POST", Path: "/boards/{slug}/tasks", Summary: "Create a task", MinRole: model.RoleMember, Status: 201,
			Input: model.CreateTaskParams{}, Output: model.Task{}, Params: []paramMeta{{Name: "slug", In: "path", Type: "string", Required: true}}},
		{Method: "GET", Path: "/boards/{slug}/tasks", Summary: "List tasks (with filters and search)", MinRole: model.RoleReadOnly, Status: 200,
			Output: []model.Task{}, Params: []paramMeta{
				{Name: "slug", In: "path", Type: "string", Required: true},
				{Name: "state", In: "query", Type: "string", Desc: "Filter by workflow state"},
				{Name: "assignee", In: "query", Type: "string", Desc: "Filter by assignee name"},
				{Name: "priority", In: "query", Type: "string", Desc: "Filter by priority (critical/high/medium/low/none)"},
				{Name: "tag", In: "query", Type: "string", Desc: "Filter by tag"},
				{Name: "q", In: "query", Type: "string", Desc: "Full-text search query"},
				{Name: "include_closed", In: "query", Type: "boolean", Desc: "Include tasks in terminal states"},
				{Name: "include_deleted", In: "query", Type: "boolean", Desc: "Include soft-deleted tasks"},
				{Name: "sort", In: "query", Type: "string", Desc: "Sort field (created_at/updated_at/priority/due_date)"},
				{Name: "order", In: "query", Type: "string", Desc: "Sort order (asc/desc)"},
			}},
		{Method: "GET", Path: "/boards/{slug}/tasks/{num}", Summary: "Get a task", MinRole: model.RoleReadOnly, Status: 200,
			Output: model.Task{}, Params: []paramMeta{
				{Name: "slug", In: "path", Type: "string", Required: true},
				{Name: "num", In: "path", Type: "integer", Required: true},
			}},
		{Method: "PATCH", Path: "/boards/{slug}/tasks/{num}", Summary: "Update a task", MinRole: model.RoleMember, Status: 200,
			Input: model.UpdateTaskParams{}, Output: model.Task{}, Params: []paramMeta{
				{Name: "slug", In: "path", Type: "string", Required: true},
				{Name: "num", In: "path", Type: "integer", Required: true},
			}},
		{Method: "POST", Path: "/boards/{slug}/tasks/{num}/transition", Summary: "Transition a task to a new state", MinRole: model.RoleMember, Status: 200,
			Input: model.TransitionTaskParams{}, Output: model.Task{}, Params: []paramMeta{
				{Name: "slug", In: "path", Type: "string", Required: true},
				{Name: "num", In: "path", Type: "integer", Required: true},
			}},
		{Method: "DELETE", Path: "/boards/{slug}/tasks/{num}", Summary: "Delete a task (soft-delete)", MinRole: model.RoleMember, Status: 204,
			Params: []paramMeta{
				{Name: "slug", In: "path", Type: "string", Required: true},
				{Name: "num", In: "path", Type: "integer", Required: true},
			}},

		// Comments
		{Method: "POST", Path: "/boards/{slug}/tasks/{num}/comments", Summary: "Add a comment to a task", MinRole: model.RoleMember, Status: 201,
			Input: model.CreateCommentParams{}, Output: model.Comment{}, Params: []paramMeta{
				{Name: "slug", In: "path", Type: "string", Required: true},
				{Name: "num", In: "path", Type: "integer", Required: true},
			}},
		{Method: "GET", Path: "/boards/{slug}/tasks/{num}/comments", Summary: "List comments on a task", MinRole: model.RoleReadOnly, Status: 200,
			Output: []model.Comment{}, Params: []paramMeta{
				{Name: "slug", In: "path", Type: "string", Required: true},
				{Name: "num", In: "path", Type: "integer", Required: true},
			}},
		{Method: "PATCH", Path: "/comments/{id}", Summary: "Edit a comment", MinRole: model.RoleMember, Status: 200,
			Input: model.UpdateCommentParams{}, Output: model.Comment{}, Params: []paramMeta{{Name: "id", In: "path", Type: "integer", Required: true}}},

		// Dependencies
		{Method: "POST", Path: "/boards/{slug}/tasks/{num}/dependencies", Summary: "Add a dependency", MinRole: model.RoleMember, Status: 201,
			Input: model.CreateDependencyParams{}, Output: model.Dependency{}, Params: []paramMeta{
				{Name: "slug", In: "path", Type: "string", Required: true},
				{Name: "num", In: "path", Type: "integer", Required: true},
			}},
		{Method: "GET", Path: "/boards/{slug}/tasks/{num}/dependencies", Summary: "List dependencies for a task", MinRole: model.RoleReadOnly, Status: 200,
			Output: []model.Dependency{}, Params: []paramMeta{
				{Name: "slug", In: "path", Type: "string", Required: true},
				{Name: "num", In: "path", Type: "integer", Required: true},
			}},
		{Method: "DELETE", Path: "/dependencies/{id}", Summary: "Remove a dependency", MinRole: model.RoleMember, Status: 204,
			Params: []paramMeta{{Name: "id", In: "path", Type: "integer", Required: true}}},

		// Attachments
		{Method: "POST", Path: "/boards/{slug}/tasks/{num}/attachments", Summary: "Add an attachment", MinRole: model.RoleMember, Status: 201,
			Input: model.CreateAttachmentParams{}, Output: model.Attachment{}, Params: []paramMeta{
				{Name: "slug", In: "path", Type: "string", Required: true},
				{Name: "num", In: "path", Type: "integer", Required: true},
			}},
		{Method: "GET", Path: "/boards/{slug}/tasks/{num}/attachments", Summary: "List attachments on a task", MinRole: model.RoleReadOnly, Status: 200,
			Output: []model.Attachment{}, Params: []paramMeta{
				{Name: "slug", In: "path", Type: "string", Required: true},
				{Name: "num", In: "path", Type: "integer", Required: true},
			}},
		{Method: "DELETE", Path: "/attachments/{id}", Summary: "Remove an attachment", MinRole: model.RoleMember, Status: 204,
			Params: []paramMeta{{Name: "id", In: "path", Type: "integer", Required: true}}},

		// Webhooks
		{Method: "POST", Path: "/webhooks", Summary: "Create a webhook", MinRole: model.RoleAdmin, Status: 201,
			Input: model.CreateWebhookParams{}, Output: model.Webhook{}},
		{Method: "GET", Path: "/webhooks", Summary: "List webhooks", MinRole: model.RoleAdmin, Status: 200,
			Output: []model.Webhook{}},
		{Method: "GET", Path: "/webhooks/{id}", Summary: "Get a webhook", MinRole: model.RoleAdmin, Status: 200,
			Output: model.Webhook{}, Params: []paramMeta{{Name: "id", In: "path", Type: "integer", Required: true}}},
		{Method: "PATCH", Path: "/webhooks/{id}", Summary: "Update a webhook", MinRole: model.RoleAdmin, Status: 200,
			Input: model.UpdateWebhookParams{}, Output: model.Webhook{}, Params: []paramMeta{{Name: "id", In: "path", Type: "integer", Required: true}}},
		{Method: "DELETE", Path: "/webhooks/{id}", Summary: "Delete a webhook", MinRole: model.RoleAdmin, Status: 204,
			Params: []paramMeta{{Name: "id", In: "path", Type: "integer", Required: true}}},

		// Audit
		{Method: "GET", Path: "/boards/{slug}/tasks/{num}/audit", Summary: "Get audit log for a task", MinRole: model.RoleReadOnly, Status: 200,
			Output: []model.AuditEntry{}, Params: []paramMeta{
				{Name: "slug", In: "path", Type: "string", Required: true},
				{Name: "num", In: "path", Type: "integer", Required: true},
			}},
		{Method: "GET", Path: "/boards/{slug}/audit", Summary: "Get audit log for a board", MinRole: model.RoleReadOnly, Status: 200,
			Output: []model.AuditEntry{}, Params: []paramMeta{{Name: "slug", In: "path", Type: "string", Required: true}}},
	}
}

// generateOpenAPISpec walks the route metadata and generates an OpenAPI 3.1 JSON document.
func generateOpenAPISpec() []byte {
	spec := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "TaskFlow API",
			"version":     "1.0.0",
			"description": "A human/AI collaborative task tracker with kanban boards and workflow state machines.",
		},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"bearerAuth": map[string]any{
					"type":   "http",
					"scheme": "bearer",
				},
			},
			"schemas": map[string]any{
				"Error": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"error":   map[string]any{"type": "string"},
						"message": map[string]any{"type": "string"},
						"detail":  map[string]any{},
					},
				},
			},
		},
	}

	paths := map[string]any{}
	schemas := spec["components"].(map[string]any)["schemas"].(map[string]any)

	for _, rm := range routeMetadata() {
		op := map[string]any{
			"summary":  rm.Summary,
			"security": []any{map[string]any{"bearerAuth": []any{}}},
			"tags":     []string{tagFromPath(rm.Path)},
		}

		// Parameters
		var params []any
		for _, p := range rm.Params {
			param := map[string]any{
				"name":     p.Name,
				"in":       p.In,
				"required": p.Required,
				"schema":   map[string]any{"type": p.Type},
			}
			if p.Desc != "" {
				param["description"] = p.Desc
			}
			params = append(params, param)
		}
		if len(params) > 0 {
			op["parameters"] = params
		}

		// Request body
		if rm.Input != nil {
			schemaName := typeName(rm.Input)
			schemas[schemaName] = typeToSchema(reflect.TypeOf(rm.Input))
			op["requestBody"] = map[string]any{
				"required": true,
				"content": map[string]any{
					"application/json": map[string]any{
						"schema": map[string]any{"$ref": "#/components/schemas/" + schemaName},
					},
				},
			}
		}

		// Responses
		responses := map[string]any{}
		statusStr := statusString(rm.Status)
		if rm.Output != nil {
			outType := reflect.TypeOf(rm.Output)
			var responseSchema map[string]any
			if outType.Kind() == reflect.Slice {
				elemName := typeName(reflect.New(outType.Elem()).Elem().Interface())
				schemas[elemName] = typeToSchema(outType.Elem())
				responseSchema = map[string]any{
					"type":  "array",
					"items": map[string]any{"$ref": "#/components/schemas/" + elemName},
				}
			} else {
				schemaName := typeName(rm.Output)
				schemas[schemaName] = typeToSchema(outType)
				responseSchema = map[string]any{"$ref": "#/components/schemas/" + schemaName}
			}
			responses[statusStr] = map[string]any{
				"description": "Success",
				"content": map[string]any{
					"application/json": map[string]any{"schema": responseSchema},
				},
			}
		} else {
			responses[statusStr] = map[string]any{"description": "Success"}
		}
		responses["4XX"] = map[string]any{
			"description": "Client error",
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{"$ref": "#/components/schemas/Error"},
				},
			},
		}
		op["responses"] = responses

		// Add to paths
		oaPath := chiPathToOpenAPI(rm.Path)
		method := strings.ToLower(rm.Method)
		if _, ok := paths[oaPath]; !ok {
			paths[oaPath] = map[string]any{}
		}
		paths[oaPath].(map[string]any)[method] = op
	}

	spec["paths"] = paths

	b, _ := json.MarshalIndent(spec, "", "  ")
	return b
}

// typeToSchema converts a Go struct type to a JSON Schema object.
func typeToSchema(t reflect.Type) map[string]any {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return map[string]any{"type": goTypeToJSONType(t)}
	}

	properties := map[string]any{}
	var required []string

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get("json")
		if tag == "-" || tag == "" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		omitempty := strings.Contains(tag, "omitempty")

		ft := f.Type
		isOptional := false

		// Unwrap Optional[T]
		if ft.Name() == "Optional" || (ft.Kind() == reflect.Struct && ft.NumField() == 2 && ft.Field(0).Name == "Value" && ft.Field(1).Name == "Set") {
			ft = ft.Field(0).Type
			isOptional = true
		}

		prop := goTypeToProperty(ft)
		properties[name] = prop

		if !isOptional && !omitempty && ft.Kind() != reflect.Ptr {
			required = append(required, name)
		}
	}

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func goTypeToProperty(t reflect.Type) map[string]any {
	if t.Kind() == reflect.Ptr {
		inner := goTypeToProperty(t.Elem())
		inner["nullable"] = true
		return inner
	}
	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Int, reflect.Int64:
		return map[string]any{"type": "integer"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			// json.RawMessage or []byte
			return map[string]any{}
		}
		return map[string]any{"type": "array", "items": goTypeToProperty(t.Elem())}
	default:
		if t == reflect.TypeOf(time.Time{}) {
			return map[string]any{"type": "string", "format": "date-time"}
		}
		if t == reflect.TypeOf(json.RawMessage{}) {
			return map[string]any{"type": "object", "description": "JSON object"}
		}
		return map[string]any{"type": "string"}
	}
}

func goTypeToJSONType(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int64:
		return "integer"
	case reflect.Bool:
		return "boolean"
	default:
		return "string"
	}
}

func typeName(v any) string {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Slice {
		return t.Elem().Name()
	}
	return t.Name()
}

func statusString(code int) string {
	switch code {
	case 200:
		return "200"
	case 201:
		return "201"
	case 204:
		return "204"
	default:
		return "200"
	}
}

// chiPathToOpenAPI converts chi path params {name} to OpenAPI {name} (same format, no change needed).
func chiPathToOpenAPI(path string) string {
	return path
}

// tagFromPath extracts a tag name from the path for grouping in docs.
func tagFromPath(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) == 0 {
		return "other"
	}
	tag := parts[0]
	if tag == "boards" && len(parts) >= 4 {
		// /boards/{slug}/tasks/... → tasks
		switch parts[2] {
		case "tasks":
			if len(parts) >= 5 {
				switch parts[4] {
				case "comments":
					return "comments"
				case "dependencies":
					return "dependencies"
				case "attachments":
					return "attachments"
				case "audit":
					return "audit"
				case "transition":
					return "tasks"
				}
			}
			return "tasks"
		case "workflow":
			return "workflows"
		case "audit":
			return "audit"
		case "reassign":
			return "boards"
		}
	}
	return tag
}
