package model

import (
	"encoding/json"
	"time"
)

type AuditAction string

const (
	AuditActionCreated           AuditAction = "created"
	AuditActionTransitioned      AuditAction = "transitioned"
	AuditActionUpdated           AuditAction = "updated"
	AuditActionCommented         AuditAction = "commented"
	AuditActionCommentEdited     AuditAction = "comment_edited"
	AuditActionAssigned          AuditAction = "assigned"
	AuditActionDeleted           AuditAction = "deleted"
	AuditActionDependencyAdded   AuditAction = "dependency_added"
	AuditActionDependencyRemoved AuditAction = "dependency_removed"
	AuditActionFileUploaded      AuditAction = "file_uploaded"
	AuditActionFileDeleted       AuditAction = "file_deleted"
	AuditActionAttachmentAdded   AuditAction = "attachment_added"
	AuditActionAttachmentRemoved AuditAction = "attachment_removed"
	AuditActionWorkflowChanged   AuditAction = "workflow_changed"
	AuditActionBoardDeleted      AuditAction = "board_deleted"
	AuditActionTasksReassigned   AuditAction = "tasks_reassigned"
)

type AuditEntry struct {
	ID        int             `json:"id"`
	BoardSlug string          `json:"board_slug"`
	TaskNum   *int            `json:"task_num"` // Nil for board-level events.
	Actor     string          `json:"actor"`    // The actor who performed the action.
	Action    AuditAction     `json:"action"`
	Detail    json.RawMessage `json:"detail"` // Action-specific payload; shape varies by Action.
	CreatedAt time.Time       `json:"created_at"`
}
