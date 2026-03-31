package model

import "fmt"

// ValidationError indicates invalid input for a specific field.
// Detail optionally carries structured context (e.g., available transitions).
type ValidationError struct {
	Field   string
	Message string
	Detail  any
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error: %s: %s", e.Field, e.Message)
}

// NotFoundError indicates a resource was not found.
type NotFoundError struct {
	Resource string
	ID       string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s not found: %s", e.Resource, e.ID)
}

// ArchivedError indicates an operation was attempted on an archived (soft-deleted) board.
// Comments are still allowed on archived boards; mutations are not.
type ArchivedError struct {
	BoardSlug string
}

func (e *ArchivedError) Error() string {
	return fmt.Sprintf("board %q is archived; only comments are allowed", e.BoardSlug)
}

// ConflictError indicates a uniqueness or referential integrity violation.
type ConflictError struct {
	Resource string
	Field    string
	Value    string
	Message  string
}

func (e *ConflictError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("conflict: %s", e.Message)
	}
	return fmt.Sprintf("conflict: %s with %s %q already exists", e.Resource, e.Field, e.Value)
}
