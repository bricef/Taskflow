package model

import (
	"regexp"
	"strings"
)

// Action represents the kind of domain operation.
type Action string

const (
	ActionCreate     Action = "create"
	ActionList       Action = "list"
	ActionGet        Action = "get"
	ActionUpdate     Action = "update"
	ActionDelete     Action = "delete"
	ActionSet        Action = "set"
	ActionTransition Action = "transition"
	ActionReassign   Action = "reassign"
	ActionHealth     Action = "health"
)

// Operation describes a domain operation as runtime metadata — what the system
// can do, its access rules, and its input/output types. This is the canonical
// list consumed by the HTTP layer (which derives routes + handlers), the CLI
// (which derives commands), and the OpenAPI spec generator.
//
// Note: taskflow.TaskFlow defines the same operations as a Go interface for
// compile-time type safety. Both must be kept in sync when operations change.
type Operation struct {
	Action  Action       // the kind of operation
	Path    string       // resource address pattern: /boards/{slug}/tasks/{num}
	Summary string       // human-readable description
	MinRole Role         // minimum role required
	Input   any          // nil, or zero-value of request type (for schema generation)
	Output  any          // nil, or zero-value of response type (for schema generation)
	Params  []QueryParam // query parameters (path params inferred from Path)
}

// QueryParam describes a query string parameter.
type QueryParam struct {
	Name string
	Type string // "string", "integer", "boolean"
	Desc string
}

// PathParams returns the path parameters inferred from the Path pattern.
func (op Operation) PathParams() []PathParam {
	return inferPathParams(op.Path)
}

// PathParam describes a path parameter inferred from the operation's Path.
type PathParam struct {
	Name string
	Type string // "string" or "integer"
}

var pathParamRegex = regexp.MustCompile(`\{(\w+)\}`)
var intParamNames = map[string]bool{"num": true, "id": true}

func inferPathParams(path string) []PathParam {
	matches := pathParamRegex.FindAllStringSubmatch(path, -1)
	var params []PathParam
	for _, m := range matches {
		name := m[1]
		typ := "string"
		if intParamNames[strings.ToLower(name)] {
			typ = "integer"
		}
		params = append(params, PathParam{Name: name, Type: typ})
	}
	return params
}

// ReassignRequest is the input for the reassign operation.
type ReassignRequest struct {
	TargetBoard string   `json:"target_board"`
	States      []string `json:"states,omitempty"`
}

// ReassignResponse is the output of the reassign operation.
type ReassignResponse struct {
	Count int `json:"count"`
}

// --- Builder ---

type opBuilder struct{ op Operation }

func newOp(action Action, path, summary string) *opBuilder {
	return &opBuilder{op: Operation{Action: action, Path: path, Summary: summary, MinRole: defaultRole(action)}}
}

func defaultRole(action Action) Role {
	switch action {
	case ActionList, ActionGet:
		return RoleReadOnly
	default:
		return RoleMember
	}
}

func Create(path, summary string) *opBuilder { return newOp(ActionCreate, path, summary) }
func List(path, summary string) *opBuilder   { return newOp(ActionList, path, summary) }
func GetOne(path, summary string) *opBuilder { return newOp(ActionGet, path, summary) }
func Update(path, summary string) *opBuilder { return newOp(ActionUpdate, path, summary) }
func Remove(path, summary string) *opBuilder { return newOp(ActionDelete, path, summary) }
func SetOp(path, summary string) *opBuilder  { return newOp(ActionSet, path, summary) }
func CustomAction(action Action, path, summary string) *opBuilder {
	return newOp(action, path, summary)
}

func (b *opBuilder) Role(r Role) *opBuilder  { b.op.MinRole = r; return b }
func (b *opBuilder) Input(v any) *opBuilder  { b.op.Input = v; return b }
func (b *opBuilder) Output(v any) *opBuilder { b.op.Output = v; return b }

func (b *opBuilder) QueryParams(params ...QueryParam) *opBuilder {
	b.op.Params = append(b.op.Params, params...)
	return b
}

func Q(name, typ, desc string) QueryParam {
	return QueryParam{Name: name, Type: typ, Desc: desc}
}

func (b *opBuilder) Build() Operation { return b.op }

// Operations returns the canonical list of all domain operations.
func Operations() []Operation {
	return []Operation{
		// Actors
		Create("/actors", "Create an actor").Role(RoleAdmin).
			Input(CreateActorParams{}).Output(Actor{}).Build(),
		List("/actors", "List all actors").
			Output([]Actor{}).Build(),
		GetOne("/actors/{name}", "Get an actor by name").
			Output(Actor{}).Build(),
		Update("/actors/{name}", "Update an actor").Role(RoleAdmin).
			Input(UpdateActorParams{}).Output(Actor{}).Build(),

		// Boards
		Create("/boards", "Create a board").
			Input(CreateBoardParams{}).Output(Board{}).Build(),
		List("/boards", "List boards").
			Output([]Board{}).
			QueryParams(Q("include_deleted", "boolean", "Include soft-deleted boards")).Build(),
		GetOne("/boards/{slug}", "Get a board").
			Output(Board{}).Build(),
		Update("/boards/{slug}", "Update a board").
			Input(UpdateBoardParams{}).Output(Board{}).Build(),
		Remove("/boards/{slug}", "Delete a board (soft-delete)").Role(RoleAdmin).Build(),
		CustomAction(ActionReassign, "/boards/{slug}/reassign", "Reassign tasks to another board").Role(RoleAdmin).
			Input(ReassignRequest{}).Output(ReassignResponse{}).Build(),

		// Workflows
		GetOne("/boards/{slug}/workflow", "Get the board's workflow definition").
			Output(Workflow{}).Build(),
		SetOp("/boards/{slug}/workflow", "Replace the board's workflow").
			Input(Workflow{}).Build(),
		CustomAction(ActionHealth, "/boards/{slug}/workflow/health", "Check workflow health").
			Output([]WorkflowHealthIssue{}).Build(),

		// Tasks
		Create("/boards/{slug}/tasks", "Create a task").
			Input(CreateTaskParams{}).Output(Task{}).Build(),
		List("/boards/{slug}/tasks", "List tasks (with filters and search)").
			Output([]Task{}).
			QueryParams(
				Q("state", "string", "Filter by workflow state"),
				Q("assignee", "string", "Filter by assignee name"),
				Q("priority", "string", "Filter by priority (critical/high/medium/low/none)"),
				Q("tag", "string", "Filter by tag"),
				Q("q", "string", "Full-text search query"),
				Q("include_closed", "boolean", "Include tasks in terminal states"),
				Q("include_deleted", "boolean", "Include soft-deleted tasks"),
				Q("sort", "string", "Sort field (created_at/updated_at/priority/due_date)"),
				Q("order", "string", "Sort order (asc/desc)"),
			).Build(),
		GetOne("/boards/{slug}/tasks/{num}", "Get a task").
			Output(Task{}).Build(),
		Update("/boards/{slug}/tasks/{num}", "Update a task").
			Input(UpdateTaskParams{}).Output(Task{}).Build(),
		CustomAction(ActionTransition, "/boards/{slug}/tasks/{num}/transition", "Transition a task to a new state").
			Input(TransitionTaskParams{}).Output(Task{}).Build(),
		Remove("/boards/{slug}/tasks/{num}", "Delete a task (soft-delete)").Build(),

		// Comments
		Create("/boards/{slug}/tasks/{num}/comments", "Add a comment to a task").
			Input(CreateCommentParams{}).Output(Comment{}).Build(),
		List("/boards/{slug}/tasks/{num}/comments", "List comments on a task").
			Output([]Comment{}).Build(),
		Update("/comments/{id}", "Edit a comment").
			Input(UpdateCommentParams{}).Output(Comment{}).Build(),

		// Dependencies
		Create("/boards/{slug}/tasks/{num}/dependencies", "Add a dependency").
			Input(CreateDependencyParams{}).Output(Dependency{}).Build(),
		List("/boards/{slug}/tasks/{num}/dependencies", "List dependencies for a task").
			Output([]Dependency{}).Build(),
		Remove("/dependencies/{id}", "Remove a dependency").Build(),

		// Attachments
		Create("/boards/{slug}/tasks/{num}/attachments", "Add an attachment").
			Input(CreateAttachmentParams{}).Output(Attachment{}).Build(),
		List("/boards/{slug}/tasks/{num}/attachments", "List attachments on a task").
			Output([]Attachment{}).Build(),
		Remove("/attachments/{id}", "Remove an attachment").Build(),

		// Webhooks
		Create("/webhooks", "Create a webhook").Role(RoleAdmin).
			Input(CreateWebhookParams{}).Output(Webhook{}).Build(),
		List("/webhooks", "List webhooks").Role(RoleAdmin).
			Output([]Webhook{}).Build(),
		GetOne("/webhooks/{id}", "Get a webhook").Role(RoleAdmin).
			Output(Webhook{}).Build(),
		Update("/webhooks/{id}", "Update a webhook").Role(RoleAdmin).
			Input(UpdateWebhookParams{}).Output(Webhook{}).Build(),
		Remove("/webhooks/{id}", "Delete a webhook").Role(RoleAdmin).Build(),

		// Audit
		List("/boards/{slug}/tasks/{num}/audit", "Get audit log for a task").
			Output([]AuditEntry{}).Build(),
		List("/boards/{slug}/audit", "Get audit log for a board").
			Output([]AuditEntry{}).Build(),
	}
}
