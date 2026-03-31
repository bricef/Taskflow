package model

import "time"

type Priority string

const (
	PriorityCritical Priority = "critical"
	PriorityHigh     Priority = "high"
	PriorityMedium   Priority = "medium"
	PriorityLow      Priority = "low"
	PriorityNone     Priority = "none"
)

func ValidatePriority(p Priority) error {
	switch p {
	case PriorityCritical, PriorityHigh, PriorityMedium, PriorityLow, PriorityNone:
		return nil
	default:
		return &ValidationError{Field: "priority", Message: "must be 'critical', 'high', 'medium', 'low', or 'none'"}
	}
}

type Task struct {
	BoardSlug   string     `json:"board_slug"`
	Num         int        `json:"num"` // Board-scoped sequential identifier (e.g., my-board/42).
	Title       string     `json:"title"`
	Description string     `json:"description"`
	State       string     `json:"state"` // Workflow-defined state; valid values depend on the board's workflow.
	Priority    Priority   `json:"priority"`
	Tags        []string   `json:"tags"`
	Assignee    *string    `json:"assignee"`
	DueDate     *time.Time `json:"due_date"`
	CreatedBy   string     `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	Deleted     bool       `json:"deleted"`
}

type CreateTaskParams struct {
	BoardSlug   string     `json:"-"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Priority    Priority   `json:"priority,omitempty"`
	Tags        []string   `json:"tags,omitempty"`
	Assignee    *string    `json:"assignee"`
	DueDate     *time.Time `json:"due_date"`
	CreatedBy   string     `json:"-"`
}

func (p CreateTaskParams) Validate() error {
	if p.BoardSlug == "" {
		return &ValidationError{Field: "board_slug", Message: "must not be empty"}
	}
	if p.Title == "" {
		return &ValidationError{Field: "title", Message: "must not be empty"}
	}
	if p.Priority != "" {
		if err := ValidatePriority(p.Priority); err != nil {
			return err
		}
	}
	if p.CreatedBy == "" {
		return &ValidationError{Field: "created_by", Message: "must not be empty"}
	}
	return nil
}

// TransitionTaskParams describes a task state transition.
type TransitionTaskParams struct {
	BoardSlug      string `json:"-"`
	Num            int    `json:"-"`
	TransitionName string `json:"transition"`
	Comment        string `json:"comment,omitempty"`
	Actor          string `json:"-"`
}

type UpdateTaskParams struct {
	BoardSlug   string               `json:"-"`
	Num         int                  `json:"-"`
	Title       Optional[string]     `json:"title"`
	Description Optional[string]     `json:"description"`
	State       Optional[string]     `json:"state"`
	Priority    Optional[Priority]   `json:"priority"`
	Tags        Optional[[]string]   `json:"tags"`
	Assignee    Optional[*string]    `json:"assignee"` // Set with nil value clears the assignee.
	DueDate     Optional[*time.Time] `json:"due_date"` // Set with nil value clears the due date.
}

// TaskFilter describes query parameters for filtering task lists.
// Fields with a `query` tag are exposed as query parameters on list endpoints.
type TaskFilter struct {
	BoardSlug      string    // path param, not a query param
	State          *string   `query:"state,Filter by workflow state"`
	Assignee       *string   `query:"assignee,Filter by assignee name"`
	Priority       *Priority `query:"priority,Filter by priority (critical/high/medium/low/none)"`
	Tag            *string   `query:"tag,Filter by tag"`
	Query          *string   `query:"q,Full-text search query"`
	IncludeClosed  bool      `query:"include_closed,Include tasks in terminal states"`
	IncludeDeleted bool      `query:"include_deleted,Include soft-deleted tasks"`
}

// TaskSort describes query parameters for sorting task lists.
type TaskSort struct {
	Field string `query:"sort,Sort field (created_at/updated_at/priority/due_date)"`
	Desc  bool   `query:"order,string,Sort order (asc/desc)"`
}
