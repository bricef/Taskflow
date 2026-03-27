package model

import "time"

type RefType string

const (
	RefTypeURL       RefType = "url"
	RefTypeFile      RefType = "file"
	RefTypeGitCommit RefType = "git_commit"
	RefTypeGitBranch RefType = "git_branch"
	RefTypeGitPR     RefType = "git_pr"
)

func ValidateRefType(r RefType) error {
	switch r {
	case RefTypeURL, RefTypeFile, RefTypeGitCommit, RefTypeGitBranch, RefTypeGitPR:
		return nil
	default:
		return &ValidationError{Field: "ref_type", Message: "must be 'url', 'file', 'git_commit', 'git_branch', or 'git_pr'"}
	}
}

// Attachment links a task to an external reference (URL, file, git branch, etc.).
type Attachment struct {
	ID        int       `json:"id"`
	BoardSlug string    `json:"board_slug"`
	TaskNum   int       `json:"task_num"`
	RefType   RefType   `json:"ref_type"`
	Reference string    `json:"reference"`
	Label     string    `json:"label"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateAttachmentParams struct {
	BoardSlug string
	TaskNum   int
	RefType   RefType
	Reference string
	Label     string
	CreatedBy string
}

func (p CreateAttachmentParams) Validate() error {
	if err := ValidateRefType(p.RefType); err != nil {
		return err
	}
	if p.Reference == "" {
		return &ValidationError{Field: "reference", Message: "must not be empty"}
	}
	if p.Label == "" {
		return &ValidationError{Field: "label", Message: "must not be empty"}
	}
	return nil
}
