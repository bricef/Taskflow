package tui

import (
	"context"
	"fmt"

	"github.com/bricef/taskflow/internal/httpclient"
	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/workflow"
)

// resource and operation definitions, looked up once.
var (
	resActorList   = mustResource("actor_list")
	resBoardList   = mustResource("board_list")
	resBoardGet    = mustResource("board_get")
	resTaskList    = mustResource("task_list")
	resTaskGet     = mustResource("task_get")
	resCommentList = mustResource("comment_list")
	resDepList     = mustResource("dependency_list")
	resAttachList  = mustResource("attachment_list")
	resTaskAudit   = mustOperation("task_audit")
	resBoardAudit  = mustOperation("board_audit")
	resWorkflowGet = mustResource("workflow_get")

	opBoardCreate    = mustOperation("board_create")
	opBoardDelete    = mustOperation("board_delete")
	opTaskUpdate     = mustOperation("task_update")
	opTaskTransition = mustOperation("task_transition")
	opCommentCreate  = mustOperation("comment_create")
)

func mustResource(name string) model.Resource {
	r, ok := model.LookupResource(name)
	if !ok {
		panic("unknown resource: " + name)
	}
	return r
}

func mustOperation(name string) model.Operation {
	op, ok := model.LookupOperation(name)
	if !ok {
		panic("unknown operation: " + name)
	}
	return op
}

// Client is a typed wrapper around httpclient for the TUI.
type Client struct {
	http *httpclient.Client
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{http: &httpclient.Client{BaseURL: baseURL, APIKey: apiKey}}
}

func (c *Client) ListActors() ([]model.Actor, error) {
	var actors []model.Actor
	return actors, c.http.Resource(ctx(), resActorList, nil, nil, &actors)
}

func (c *Client) ListBoards(includeArchived bool) ([]model.Board, error) {
	var boards []model.Board
	var filter any
	if includeArchived {
		filter = model.ListBoardsParams{IncludeDeleted: true}
	}
	return boards, c.http.Resource(ctx(), resBoardList, nil, filter, &boards)
}

func (c *Client) GetBoard(slug string) (model.Board, error) {
	var board model.Board
	return board, c.http.Resource(ctx(), resBoardGet, p("slug", slug), nil, &board)
}

func (c *Client) ListTasks(boardSlug string) ([]model.Task, error) {
	var tasks []model.Task
	filter := model.TaskFilter{IncludeClosed: true}
	return tasks, c.http.Resource(ctx(), resTaskList, p("slug", boardSlug), filter, &tasks)
}

func (c *Client) GetTask(boardSlug string, num int) (model.Task, error) {
	var task model.Task
	return task, c.http.Resource(ctx(), resTaskGet, tp(boardSlug, num), nil, &task)
}

func (c *Client) ListComments(boardSlug string, num int) ([]model.Comment, error) {
	var comments []model.Comment
	return comments, c.http.Resource(ctx(), resCommentList, tp(boardSlug, num), nil, &comments)
}

func (c *Client) ListDependencies(boardSlug string, num int) ([]model.Dependency, error) {
	var deps []model.Dependency
	return deps, c.http.Resource(ctx(), resDepList, tp(boardSlug, num), nil, &deps)
}

func (c *Client) ListAttachments(boardSlug string, num int) ([]model.Attachment, error) {
	var atts []model.Attachment
	return atts, c.http.Resource(ctx(), resAttachList, tp(boardSlug, num), nil, &atts)
}

func (c *Client) GetTaskAudit(boardSlug string, num int) ([]model.AuditEntry, error) {
	var entries []model.AuditEntry
	return entries, c.http.Operation(ctx(), resTaskAudit, tp(boardSlug, num), nil, &entries)
}

func (c *Client) GetBoardAudit(boardSlug string) ([]model.AuditEntry, error) {
	var entries []model.AuditEntry
	return entries, c.http.Operation(ctx(), resBoardAudit, p("slug", boardSlug), nil, &entries)
}

func (c *Client) GetWorkflow(boardSlug string) (*workflow.Workflow, error) {
	var wf workflow.Workflow
	return &wf, c.http.Resource(ctx(), resWorkflowGet, p("slug", boardSlug), nil, &wf)
}

func (c *Client) CreateBoard(slug, name string) (model.Board, error) {
	var board model.Board
	return board, c.http.Operation(ctx(), opBoardCreate, nil, map[string]string{"slug": slug, "name": name}, &board)
}

func (c *Client) ArchiveBoard(slug string) error {
	return c.http.Operation(ctx(), opBoardDelete, p("slug", slug), nil, nil)
}

func (c *Client) AssignTask(boardSlug string, num int, assignee *string) (model.Task, error) {
	var task model.Task
	return task, c.http.Operation(ctx(), opTaskUpdate, tp(boardSlug, num), map[string]any{"assignee": assignee}, &task)
}

func (c *Client) TransitionTask(boardSlug string, num int, transition string) error {
	return c.http.Operation(ctx(), opTaskTransition, tp(boardSlug, num), map[string]string{"transition": transition}, nil)
}

func (c *Client) CreateComment(boardSlug string, num int, body string) (model.Comment, error) {
	var comment model.Comment
	return comment, c.http.Operation(ctx(), opCommentCreate, tp(boardSlug, num), map[string]string{"body": body}, &comment)
}

// helpers

func ctx() context.Context { return context.Background() }

func p(key, val string) httpclient.PathParams {
	return httpclient.PathParams{key: val}
}

func tp(slug string, num int) httpclient.PathParams {
	return httpclient.PathParams{"slug": slug, "num": fmt.Sprint(num)}
}
