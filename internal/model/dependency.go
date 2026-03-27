package model

import "time"

type DependencyType string

const (
	DependencyTypeDependsOn DependencyType = "depends_on"
	DependencyTypeRelatesTo DependencyType = "relates_to"
)

func ValidateDependencyType(d DependencyType) error {
	switch d {
	case DependencyTypeDependsOn, DependencyTypeRelatesTo:
		return nil
	default:
		return &ValidationError{Field: "dep_type", Message: "must be 'depends_on' or 'relates_to'"}
	}
}

type Dependency struct {
	ID             int            `json:"id"`
	BoardSlug      string         `json:"board_slug"` // Board of the task that has the dependency.
	TaskNum        int            `json:"task_num"`
	DependsOnBoard string         `json:"depends_on_board"` // Board of the depended-on task (may differ for cross-board deps).
	DependsOnNum   int            `json:"depends_on_num"`
	DependencyType DependencyType `json:"dep_type"`
	CreatedBy      string         `json:"created_by"`
	CreatedAt      time.Time      `json:"created_at"`
}

type CreateDependencyParams struct {
	BoardSlug      string
	TaskNum        int
	DependsOnBoard string
	DependsOnNum   int
	DependencyType DependencyType
	CreatedBy      string
}

func (p CreateDependencyParams) Validate() error {
	if err := ValidateDependencyType(p.DependencyType); err != nil {
		return err
	}
	if p.BoardSlug == p.DependsOnBoard && p.TaskNum == p.DependsOnNum {
		return &ValidationError{Field: "dependency", Message: "task cannot depend on itself"}
	}
	return nil
}
