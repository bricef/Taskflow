package model

import "time"

type Comment struct {
	ID        int        `json:"id"`
	BoardSlug string     `json:"board_slug"`
	TaskNum   int        `json:"task_num"`
	Actor     string     `json:"actor"` // The actor who wrote the comment (not necessarily the task creator).
	Body      string     `json:"body"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
}

type CreateCommentParams struct {
	BoardSlug string `json:"-"`
	TaskNum   int    `json:"-"`
	Actor     string `json:"-"`
	Body      string `json:"body"`
}

func (p CreateCommentParams) Validate() error {
	if p.BoardSlug == "" {
		return &ValidationError{Field: "board_slug", Message: "must not be empty"}
	}
	if p.TaskNum <= 0 {
		return &ValidationError{Field: "task_num", Message: "must be positive"}
	}
	if p.Actor == "" {
		return &ValidationError{Field: "actor", Message: "must not be empty"}
	}
	if p.Body == "" {
		return &ValidationError{Field: "body", Message: "must not be empty"}
	}
	return nil
}

// UpdateCommentParams always requires a new body — there is no partial
// update for comments, so Body is a plain value rather than Optional.
type UpdateCommentParams struct {
	ID   int    `json:"-"`
	Body string `json:"body"`
}
