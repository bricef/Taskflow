// Package repo defines the repository interfaces for data persistence.
// These interfaces contain no business logic — they are pure CRUD operations.
// The service layer owns validation, audit recording, and orchestration.
//
// Method names are prefixed with the entity name (e.g., ActorInsert, BoardGet)
// because a single Store implementation satisfies all interfaces.
package repo

import (
	"context"
	"encoding/json"
	"io"

	"github.com/bricef/taskflow/internal/model"
)

// Tx is an opaque transaction handle. The service layer passes it through
// to repo methods without knowing its concrete type.
type Tx interface{}

// Transactor creates atomic units of work.
type Transactor interface {
	InTransaction(ctx context.Context, fn func(tx Tx) error) error
}

type ActorRepo interface {
	ActorInsert(ctx context.Context, tx Tx, actor model.Actor) (model.Actor, error)
	ActorGet(ctx context.Context, name string) (model.Actor, error)
	ActorGetByAPIKeyHash(ctx context.Context, hash string) (model.Actor, error)
	ActorList(ctx context.Context) ([]model.Actor, error)
	ActorUpdate(ctx context.Context, tx Tx, params model.UpdateActorParams) (model.Actor, error)
}

type BoardRepo interface {
	BoardInsert(ctx context.Context, tx Tx, board model.Board) (model.Board, error)
	BoardGet(ctx context.Context, slug string) (model.Board, error)
	BoardList(ctx context.Context, params model.ListBoardsParams) ([]model.Board, error)
	BoardUpdate(ctx context.Context, tx Tx, params model.UpdateBoardParams) (model.Board, error)
	BoardSetWorkflow(ctx context.Context, tx Tx, slug string, workflow json.RawMessage) error
	BoardSetDeleted(ctx context.Context, tx Tx, slug string) error
	BoardAllocateTaskNum(ctx context.Context, tx Tx, slug string) (int, error)
	BoardUpdateNextTaskNum(ctx context.Context, tx Tx, slug string, nextNum int) error
}

type TaskRepo interface {
	TaskInsert(ctx context.Context, tx Tx, task model.Task) (model.Task, error)
	TaskGet(ctx context.Context, boardSlug string, num int) (model.Task, error)
	TaskList(ctx context.Context, filter model.TaskFilter, sort *model.TaskSort) ([]model.Task, error)
	TaskUpdate(ctx context.Context, tx Tx, params model.UpdateTaskParams) (model.Task, error)
	TaskSetDeleted(ctx context.Context, tx Tx, boardSlug string, num int) error
	TaskDeleteByBoardAndNums(ctx context.Context, tx Tx, boardSlug string, nums []int) error
}

type CommentRepo interface {
	CommentInsert(ctx context.Context, tx Tx, comment model.Comment) (model.Comment, error)
	CommentGet(ctx context.Context, id int) (model.Comment, error)
	CommentList(ctx context.Context, boardSlug string, taskNum int) ([]model.Comment, error)
	CommentUpdateBody(ctx context.Context, tx Tx, id int, body string) (model.Comment, error)
	CommentUpdateTaskRef(ctx context.Context, tx Tx, oldBoard string, oldNum int, newBoard string, newNum int) error
}

type DependencyRepo interface {
	DependencyInsert(ctx context.Context, tx Tx, dep model.Dependency) (model.Dependency, error)
	DependencyGet(ctx context.Context, id int) (model.Dependency, error)
	DependencyList(ctx context.Context, boardSlug string, taskNum int) ([]model.Dependency, error)
	DependencyDelete(ctx context.Context, tx Tx, id int) error
	DependencyUpdateTaskRefs(ctx context.Context, tx Tx, oldBoard string, oldNum int, newBoard string, newNum int) error
}

type AttachmentRepo interface {
	AttachmentInsert(ctx context.Context, tx Tx, att model.Attachment) (model.Attachment, error)
	AttachmentGet(ctx context.Context, id int) (model.Attachment, error)
	AttachmentList(ctx context.Context, boardSlug string, taskNum int) ([]model.Attachment, error)
	AttachmentDelete(ctx context.Context, tx Tx, id int) error
	AttachmentUpdateTaskRef(ctx context.Context, tx Tx, oldBoard string, oldNum int, newBoard string, newNum int) error
}

type WebhookRepo interface {
	WebhookInsert(ctx context.Context, tx Tx, webhook model.Webhook) (model.Webhook, error)
	WebhookGet(ctx context.Context, id int) (model.Webhook, error)
	WebhookList(ctx context.Context) ([]model.Webhook, error)
	WebhookUpdate(ctx context.Context, tx Tx, params model.UpdateWebhookParams) (model.Webhook, error)
	WebhookDelete(ctx context.Context, tx Tx, id int) error
}

type AuditRepo interface {
	AuditInsert(ctx context.Context, tx Tx, entry model.AuditEntry) error
	AuditQueryByTask(ctx context.Context, boardSlug string, taskNum int) ([]model.AuditEntry, error)
	AuditQueryByBoard(ctx context.Context, boardSlug string) ([]model.AuditEntry, error)
	AuditUpdateTaskRef(ctx context.Context, tx Tx, oldBoard string, oldNum int, newBoard string, newNum int) error
}

// Store groups all repositories and the transactor into a single dependency.
type Store interface {
	Transactor
	ActorRepo
	BoardRepo
	TaskRepo
	CommentRepo
	DependencyRepo
	AttachmentRepo
	WebhookRepo
	AuditRepo
	io.Closer
}
