package model

// BoardDetail is a complete board snapshot with all nested data.
type BoardDetail struct {
	Board    Board        `json:"board"`
	Workflow any          `json:"workflow"`
	Tasks    []TaskDetail `json:"tasks"`
	Audit    []AuditEntry `json:"audit"`
}

// TaskDetail is a task with its comments, attachments, dependencies, and audit.
type TaskDetail struct {
	Task
	Comments     []Comment    `json:"comments"`
	Attachments  []Attachment `json:"attachments"`
	Dependencies []Dependency `json:"dependencies"`
	Audit        []AuditEntry `json:"audit"`
}

// BoardOverview is a lightweight board summary with task counts by state.
type BoardOverview struct {
	Board
	TaskCounts map[string]int `json:"task_counts"`
	TotalTasks int            `json:"total_tasks"`
}

// SystemStats contains system-wide statistics.
type SystemStats struct {
	Actors   ActorStats       `json:"actors"`
	Boards   BoardStats       `json:"boards"`
	Tasks    TaskStatsSummary `json:"tasks"`
	Activity ActivityStats    `json:"activity"`
}

// ActorStats summarises actor counts.
type ActorStats struct {
	Total  int            `json:"total"`
	Active int            `json:"active"`
	ByRole map[string]int `json:"by_role"`
}

// BoardStats summarises board counts.
type BoardStats struct {
	Total  int `json:"total"`
	Active int `json:"active"`
}

// TaskStatsSummary summarises task counts across the system.
type TaskStatsSummary struct {
	Total           int            `json:"total"`
	ByState         map[string]int `json:"by_state"`
	CreatedLast7d   int            `json:"created_last_7d"`
	CompletedLast7d int            `json:"completed_last_7d"`
}

// ActivityStats summarises audit event activity.
type ActivityStats struct {
	TotalEvents int             `json:"total_events"`
	Last7d      int             `json:"last_7d"`
	ByActor     []ActorActivity `json:"by_actor"`
}

// ActorActivity is a per-actor event count.
type ActorActivity struct {
	Name         string `json:"name"`
	EventsLast7d int    `json:"events_last_7d"`
}
