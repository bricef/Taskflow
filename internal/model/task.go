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
	BoardSlug   string
	Title       string
	Description string
	State       string // Initial state from the board's workflow.
	Priority    Priority
	Tags        []string
	Assignee    *string
	DueDate     *time.Time
	CreatedBy   string
}

func (p CreateTaskParams) Validate() error {
	if p.BoardSlug == "" {
		return &ValidationError{Field: "board_slug", Message: "must not be empty"}
	}
	if p.Title == "" {
		return &ValidationError{Field: "title", Message: "must not be empty"}
	}
	if p.State == "" {
		return &ValidationError{Field: "state", Message: "must not be empty"}
	}
	if err := ValidatePriority(p.Priority); err != nil {
		return err
	}
	if p.CreatedBy == "" {
		return &ValidationError{Field: "created_by", Message: "must not be empty"}
	}
	return nil
}

type UpdateTaskParams struct {
	BoardSlug   string
	Num         int
	Title       Optional[string]
	Description Optional[string]
	State       Optional[string]
	Priority    Optional[Priority]
	Tags        Optional[[]string]
	Assignee    Optional[*string]    // Set with nil value clears the assignee.
	DueDate     Optional[*time.Time] // Set with nil value clears the due date.
}

type TaskFilter struct {
	BoardSlug      string
	State          *string
	Assignee       *string
	Priority       *Priority
	Tag            *string
	Query          *string // Full-text search query.
	IncludeClosed  bool
	IncludeDeleted bool
}

type TaskSort struct {
	Field string // "created_at", "updated_at", "priority", "due_date"
	Desc  bool
}
