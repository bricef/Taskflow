package model

import (
	"regexp"
	"strings"
)

// Action represents the kind of domain operation (mutation).
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

// Resource describes a read-only domain resource as runtime metadata.
// Resources are always served via GET and return 200.
type Resource struct {
	Name    string // canonical identifier, e.g. "board_list"
	Path    string // resource address pattern: /boards/{slug}
	Summary string // human-readable description
	MinRole Role   // minimum role required (defaults to RoleReadOnly)
	Output  any    // zero-value of response type (for schema generation)
	Filter  any    // nil, or zero-value of filter struct (query params derived from `query` tags)
	Sort    any    // nil, or zero-value of sort struct (query params derived from `query` tags)
}

// PathParams returns the path parameters inferred from the Path pattern.
func (r Resource) PathParams() []PathParam {
	return InferPathParams(r.Path)
}

// QueryParams returns query parameters derived from the Filter and Sort structs.
func (r Resource) QueryParams() []QueryParam {
	params := QueryParamsFrom(r.Filter)
	params = append(params, QueryParamsFrom(r.Sort)...)
	return params
}

// Operation describes a domain mutation as runtime metadata.
// Operations change state (create, update, delete, transition, etc).
type Operation struct {
	Name    string // canonical identifier, e.g. "task_create"
	Action  Action // the kind of mutation
	Path    string // resource address pattern: /boards/{slug}/tasks/{num}
	Summary string // human-readable description
	MinRole Role   // minimum role required
	Input   any    // nil, or zero-value of request type (for schema generation)
	Output  any    // nil, or zero-value of response type (for schema generation)
}

// PathParams returns the path parameters inferred from the Path pattern.
func (op Operation) PathParams() []PathParam {
	return InferPathParams(op.Path)
}

// PathParam describes a path parameter inferred from an operation's Path.
type PathParam struct {
	Name string
	Type string // "string" or "integer"
}

var pathParamRegex = regexp.MustCompile(`\{(\w+)\}`)
var intParamNames = map[string]bool{"num": true, "id": true}

func InferPathParams(path string) []PathParam {
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

// --- Resource builder ---

type resBuilder struct{ res Resource }

func newRes(path, summary string) *resBuilder {
	return &resBuilder{res: Resource{Path: path, Summary: summary, MinRole: RoleReadOnly}}
}

func ListRes(path, summary string) *resBuilder  { return newRes(path, summary) }
func GetRes(path, summary string) *resBuilder    { return newRes(path, summary) }

func (b *resBuilder) Name(n string) *resBuilder   { b.res.Name = n; return b }
func (b *resBuilder) Role(r Role) *resBuilder      { b.res.MinRole = r; return b }
func (b *resBuilder) Output(v any) *resBuilder     { b.res.Output = v; return b }
func (b *resBuilder) FilterType(v any) *resBuilder { b.res.Filter = v; return b }
func (b *resBuilder) SortType(v any) *resBuilder   { b.res.Sort = v; return b }
func (b *resBuilder) Build() Resource              { return b.res }

// --- Operation builder ---

type opBuilder struct{ op Operation }

func newOp(action Action, path, summary string) *opBuilder {
	return &opBuilder{op: Operation{Action: action, Path: path, Summary: summary, MinRole: defaultRole(action)}}
}

func defaultRole(action Action) Role {
	return RoleMember
}

func Create(path, summary string) *opBuilder                        { return newOp(ActionCreate, path, summary) }
func Update(path, summary string) *opBuilder                        { return newOp(ActionUpdate, path, summary) }
func Remove(path, summary string) *opBuilder                        { return newOp(ActionDelete, path, summary) }
func SetOp(path, summary string) *opBuilder                         { return newOp(ActionSet, path, summary) }
func CustomAction(action Action, path, summary string) *opBuilder   { return newOp(action, path, summary) }

func (b *opBuilder) Name(n string) *opBuilder  { b.op.Name = n; return b }
func (b *opBuilder) Role(r Role) *opBuilder     { b.op.MinRole = r; return b }
func (b *opBuilder) Input(v any) *opBuilder     { b.op.Input = v; return b }
func (b *opBuilder) Output(v any) *opBuilder    { b.op.Output = v; return b }
func (b *opBuilder) Build() Operation           { return b.op }


// LookupResource returns the Resource with the given name, or false if not found.
func LookupResource(name string) (Resource, bool) {
	for _, r := range Resources() {
		if r.Name == name {
			return r, true
		}
	}
	return Resource{}, false
}

// LookupOperation returns the Operation with the given name, or false if not found.
func LookupOperation(name string) (Operation, bool) {
	for _, op := range Operations() {
		if op.Name == name {
			return op, true
		}
	}
	return Operation{}, false
}

// Resources returns the canonical list of all read-only domain resources.
func Resources() []Resource {
	return []Resource{
		// Actors
		ListRes("/actors", "List all actors").Name("actor_list").
			Output([]Actor{}).Build(),
		GetRes("/actors/{name}", "Get an actor by name").Name("actor_get").
			Output(Actor{}).Build(),

		// Boards
		ListRes("/boards", "List boards").Name("board_list").
			Output([]Board{}).FilterType(ListBoardsParams{}).Build(),
		GetRes("/boards/{slug}", "Get a board").Name("board_get").
			Output(Board{}).Build(),

		// Workflows
		GetRes("/boards/{slug}/workflow", "Get the board's workflow definition").Name("workflow_get").
			Output(Workflow{}).Build(),

		// Tasks
		ListRes("/boards/{slug}/tasks", "List tasks (with filters and search)").Name("task_list").
			Output([]Task{}).FilterType(TaskFilter{}).SortType(TaskSort{}).Build(),
		GetRes("/boards/{slug}/tasks/{num}", "Get a task").Name("task_get").
			Output(Task{}).Build(),

		// Tags
		ListRes("/boards/{slug}/tags", "List tags in use on a board with counts").Name("tag_list").
			Output([]TagCount{}).Build(),

		// Comments
		ListRes("/boards/{slug}/tasks/{num}/comments", "List comments on a task").Name("comment_list").
			Output([]Comment{}).Build(),

		// Dependencies
		ListRes("/boards/{slug}/tasks/{num}/dependencies", "List dependencies for a task").Name("dependency_list").
			Output([]Dependency{}).Build(),

		// Attachments
		ListRes("/boards/{slug}/tasks/{num}/attachments", "List attachments on a task").Name("attachment_list").
			Output([]Attachment{}).Build(),

		// Cross-board
		ListRes("/tasks", "Search tasks across all boards").Name("task_search").
			Output([]Task{}).FilterType(TaskFilter{}).SortType(TaskSort{}).Build(),

		// Views
		GetRes("/boards/{slug}/detail", "Get complete board with all tasks, comments, attachments, dependencies, and audit").Name("board_detail").
			Output(BoardDetail{}).Build(),
		GetRes("/boards/{slug}/overview", "Board with task counts by state").Name("board_overview").
			Output(BoardOverview{}).Build(),
		GetRes("/admin/stats", "System-wide statistics").Name("admin_stats").Role(RoleAdmin).
			Output(SystemStats{}).Build(),

		// Webhooks
		ListRes("/webhooks", "List webhooks").Name("webhook_list").Role(RoleAdmin).
			Output([]Webhook{}).Build(),
		GetRes("/webhooks/{id}", "Get a webhook").Name("webhook_get").Role(RoleAdmin).
			Output(Webhook{}).Build(),
		ListRes("/webhooks/{id}/deliveries", "List webhook delivery attempts").Name("delivery_list").Role(RoleAdmin).
			Output([]WebhookDelivery{}).Build(),

	}
}

// Operations returns the canonical list of all domain mutations.
func Operations() []Operation {
	return []Operation{
		// Actors
		Create("/actors", "Create an actor").Name("actor_create").Role(RoleAdmin).
			Input(CreateActorParams{}).Output(Actor{}).Build(),
		Update("/actors/{name}", "Update an actor").Name("actor_update").Role(RoleAdmin).
			Input(UpdateActorParams{}).Output(Actor{}).Build(),

		// Boards
		Create("/boards", "Create a board").Name("board_create").
			Input(CreateBoardParams{}).Output(Board{}).Build(),
		Update("/boards/{slug}", "Update a board").Name("board_update").
			Input(UpdateBoardParams{}).Output(Board{}).Build(),
		Remove("/boards/{slug}", "Delete a board (soft-delete)").Name("board_delete").Role(RoleAdmin).Build(),
		CustomAction(ActionReassign, "/boards/{slug}/reassign", "Reassign tasks to another board").Name("board_reassign").Role(RoleAdmin).
			Input(ReassignRequest{}).Output(ReassignResponse{}).Build(),

		// Workflows
		SetOp("/boards/{slug}/workflow", "Replace the board's workflow").Name("workflow_set").
			Input(Workflow{}).Build(),
		CustomAction(ActionHealth, "/boards/{slug}/workflow/health", "Check workflow health").Name("workflow_health").
			Output([]WorkflowHealthIssue{}).Build(),

		// Tasks
		Create("/boards/{slug}/tasks", "Create a task").Name("task_create").
			Input(CreateTaskParams{}).Output(Task{}).Build(),
		Update("/boards/{slug}/tasks/{num}", "Update a task").Name("task_update").
			Input(UpdateTaskParams{}).Output(Task{}).Build(),
		CustomAction(ActionTransition, "/boards/{slug}/tasks/{num}/transition", "Transition a task to a new state").Name("task_transition").
			Input(TransitionTaskParams{}).Output(Task{}).Build(),
		Remove("/boards/{slug}/tasks/{num}", "Delete a task (soft-delete)").Name("task_delete").Build(),

		// Comments
		Create("/boards/{slug}/tasks/{num}/comments", "Add a comment to a task").Name("comment_create").
			Input(CreateCommentParams{}).Output(Comment{}).Build(),
		Update("/comments/{id}", "Edit a comment").Name("comment_update").
			Input(UpdateCommentParams{}).Output(Comment{}).Build(),

		// Dependencies
		Create("/boards/{slug}/tasks/{num}/dependencies", "Add a dependency").Name("dependency_create").
			Input(CreateDependencyParams{}).Output(Dependency{}).Build(),
		Remove("/dependencies/{id}", "Remove a dependency").Name("dependency_delete").Build(),

		// Attachments
		Create("/boards/{slug}/tasks/{num}/attachments", "Add an attachment").Name("attachment_create").
			Input(CreateAttachmentParams{}).Output(Attachment{}).Build(),
		Remove("/attachments/{id}", "Remove an attachment").Name("attachment_delete").Build(),

		// Audit
		CustomAction(ActionList, "/boards/{slug}/tasks/{num}/audit", "Get audit log for a task").Name("task_audit").Role(RoleReadOnly).
			Output([]AuditEntry{}).Build(),
		CustomAction(ActionList, "/boards/{slug}/audit", "Get audit log for a board").Name("board_audit").Role(RoleReadOnly).
			Output([]AuditEntry{}).Build(),

		// Webhooks
		Create("/webhooks", "Create a webhook").Name("webhook_create").Role(RoleAdmin).
			Input(CreateWebhookParams{}).Output(Webhook{}).Build(),
		Update("/webhooks/{id}", "Update a webhook").Name("webhook_update").Role(RoleAdmin).
			Input(UpdateWebhookParams{}).Output(Webhook{}).Build(),
		Remove("/webhooks/{id}", "Delete a webhook").Name("webhook_delete").Role(RoleAdmin).Build(),
	}
}
