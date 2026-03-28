package model

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"
)

var slugRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`)

func ValidateSlug(s string) error {
	if len(s) < 2 || len(s) > 32 {
		return &ValidationError{Field: "slug", Message: "must be between 2 and 32 characters"}
	}
	if !slugRegex.MatchString(s) {
		return &ValidationError{Field: "slug", Message: fmt.Sprintf("must match pattern %s", slugRegex.String())}
	}
	return nil
}

type Board struct {
	Slug        string          `json:"slug"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Workflow    json.RawMessage `json:"workflow"`      // JSON workflow definition; see Workflow struct for schema.
	NextTaskNum int             `json:"next_task_num"` // Auto-increment counter for task numbering on this board.
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	Deleted     bool            `json:"deleted"`
}

type CreateBoardParams struct {
	Slug        string          `json:"slug"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Workflow    json.RawMessage `json:"workflow"`
}

func (p CreateBoardParams) Validate() error {
	if err := ValidateSlug(p.Slug); err != nil {
		return err
	}
	if p.Name == "" {
		return &ValidationError{Field: "name", Message: "must not be empty"}
	}
	if len(p.Workflow) == 0 {
		return &ValidationError{Field: "workflow", Message: "must not be empty"}
	}
	if !json.Valid(p.Workflow) {
		return &ValidationError{Field: "workflow", Message: "must be valid JSON"}
	}
	return nil
}

type UpdateBoardParams struct {
	Slug        string           `json:"-"`
	Name        Optional[string] `json:"name"`
	Description Optional[string] `json:"description"`
}

type ListBoardsParams struct {
	IncludeDeleted bool
}
