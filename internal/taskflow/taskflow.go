// Package taskflow defines the Go interface for all TaskFlow business
// operations. The service package provides the concrete implementation;
// consumers (HTTP handlers, CLI, tests) should depend on this interface
// rather than the concrete type.
//
// Note: model.Operations() describes the same operations as runtime metadata
// (paths, roles, input/output types) for the HTTP, CLI, and OpenAPI layers.
// Both must be kept in sync when operations are added or changed.
package taskflow

import (
	"context"
	"encoding/json"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/workflow"
)

type TaskFlow interface {
	// Actors
	CreateActor(ctx context.Context, params model.CreateActorParams) (model.Actor, error)
	GetActor(ctx context.Context, name string) (model.Actor, error)
	GetActorByAPIKeyHash(ctx context.Context, hash string) (model.Actor, error)
	ListActors(ctx context.Context) ([]model.Actor, error)
	UpdateActor(ctx context.Context, params model.UpdateActorParams) (model.Actor, error)

	// Boards
	CreateBoard(ctx context.Context, params model.CreateBoardParams) (model.Board, error)
	GetBoard(ctx context.Context, slug string) (model.Board, error)
	ListBoards(ctx context.Context, params model.ListBoardsParams) ([]model.Board, error)
	UpdateBoard(ctx context.Context, params model.UpdateBoardParams) (model.Board, error)
	DeleteBoard(ctx context.Context, slug, actor string) error
	ReassignTasks(ctx context.Context, fromSlug, toSlug, actor string, states []string) (int, error) // Returns count of tasks reassigned.

	// Workflows
	GetWorkflow(ctx context.Context, boardSlug string) (*workflow.Workflow, error)
	SetWorkflow(ctx context.Context, boardSlug string, workflowJSON json.RawMessage, actor string) error
	CheckWorkflowHealth(ctx context.Context, boardSlug string) ([]workflow.HealthIssue, error)

	// Tasks
	CreateTask(ctx context.Context, params model.CreateTaskParams) (model.Task, error)
	GetTask(ctx context.Context, boardSlug string, num int) (model.Task, error)
	ListTasks(ctx context.Context, filter model.TaskFilter, sort *model.TaskSort) ([]model.Task, error)
	UpdateTask(ctx context.Context, params model.UpdateTaskParams, actor string) (model.Task, error)
	TransitionTask(ctx context.Context, params model.TransitionTaskParams) (model.Task, error)
	DeleteTask(ctx context.Context, boardSlug string, num int, actor string) error
	ListTags(ctx context.Context, boardSlug string) ([]model.TagCount, error)

	// Comments
	CreateComment(ctx context.Context, params model.CreateCommentParams) (model.Comment, error)
	ListComments(ctx context.Context, boardSlug string, taskNum int) ([]model.Comment, error)
	UpdateComment(ctx context.Context, params model.UpdateCommentParams, actor string) (model.Comment, error)

	// Dependencies
	CreateDependency(ctx context.Context, params model.CreateDependencyParams) (model.Dependency, error)
	ListDependencies(ctx context.Context, boardSlug string, taskNum int) ([]model.Dependency, error)
	DeleteDependency(ctx context.Context, id int, actor string) error

	// Attachments
	CreateAttachment(ctx context.Context, params model.CreateAttachmentParams) (model.Attachment, error)
	ListAttachments(ctx context.Context, boardSlug string, taskNum int) ([]model.Attachment, error)
	DeleteAttachment(ctx context.Context, id int, actor string) error

	// Webhooks
	CreateWebhook(ctx context.Context, params model.CreateWebhookParams) (model.Webhook, error)
	GetWebhook(ctx context.Context, id int) (model.Webhook, error)
	ListWebhooks(ctx context.Context) ([]model.Webhook, error)
	UpdateWebhook(ctx context.Context, params model.UpdateWebhookParams) (model.Webhook, error)
	DeleteWebhook(ctx context.Context, id int) error
	ListWebhookDeliveries(ctx context.Context, webhookID int) ([]model.WebhookDelivery, error)

	// Audit
	QueryAuditByTask(ctx context.Context, boardSlug string, taskNum int) ([]model.AuditEntry, error)
	QueryAuditByBoard(ctx context.Context, boardSlug string) ([]model.AuditEntry, error)
}
