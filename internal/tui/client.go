package tui

import (
	"context"
	"fmt"

	"github.com/bricef/taskflow/internal/httpclient"
	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/workflow"
)

// Client is a thin typed wrapper around httpclient for the TUI.
type Client struct {
	http *httpclient.Client
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{http: &httpclient.Client{BaseURL: baseURL, APIKey: apiKey}}
}

func (c *Client) ListBoards(includeArchived bool) ([]model.Board, error) {
	var boards []model.Board
	path := "/boards"
	if includeArchived {
		path += "?include_deleted=true"
	}
	return boards, c.http.Get(context.Background(), path, &boards)
}

func (c *Client) ArchiveBoard(slug string) error {
	return c.http.Delete(context.Background(), "/boards/"+slug)
}

func (c *Client) GetBoard(slug string) (model.Board, error) {
	var board model.Board
	return board, c.http.Get(context.Background(), "/boards/"+slug, &board)
}

func (c *Client) ListTasks(boardSlug string) ([]model.Task, error) {
	var tasks []model.Task
	return tasks, c.http.Get(context.Background(), "/boards/"+boardSlug+"/tasks?include_closed=true", &tasks)
}

func (c *Client) GetTask(boardSlug string, num int) (model.Task, error) {
	var task model.Task
	return task, c.http.Get(context.Background(), fmt.Sprintf("/boards/%s/tasks/%d", boardSlug, num), &task)
}

func (c *Client) ListComments(boardSlug string, num int) ([]model.Comment, error) {
	var comments []model.Comment
	return comments, c.http.Get(context.Background(), fmt.Sprintf("/boards/%s/tasks/%d/comments", boardSlug, num), &comments)
}

func (c *Client) ListDependencies(boardSlug string, num int) ([]model.Dependency, error) {
	var deps []model.Dependency
	return deps, c.http.Get(context.Background(), fmt.Sprintf("/boards/%s/tasks/%d/dependencies", boardSlug, num), &deps)
}

func (c *Client) ListAttachments(boardSlug string, num int) ([]model.Attachment, error) {
	var atts []model.Attachment
	return atts, c.http.Get(context.Background(), fmt.Sprintf("/boards/%s/tasks/%d/attachments", boardSlug, num), &atts)
}

func (c *Client) GetTaskAudit(boardSlug string, num int) ([]model.AuditEntry, error) {
	var entries []model.AuditEntry
	return entries, c.http.Get(context.Background(), fmt.Sprintf("/boards/%s/tasks/%d/audit", boardSlug, num), &entries)
}

func (c *Client) GetBoardAudit(boardSlug string) ([]model.AuditEntry, error) {
	var entries []model.AuditEntry
	return entries, c.http.Get(context.Background(), "/boards/"+boardSlug+"/audit", &entries)
}

func (c *Client) GetWorkflow(boardSlug string) (*workflow.Workflow, error) {
	var wf workflow.Workflow
	return &wf, c.http.Get(context.Background(), "/boards/"+boardSlug+"/workflow", &wf)
}

func (c *Client) ListActors() ([]model.Actor, error) {
	var actors []model.Actor
	return actors, c.http.Get(context.Background(), "/actors", &actors)
}

func (c *Client) AssignTask(boardSlug string, num int, assignee *string) (model.Task, error) {
	var task model.Task
	return task, c.http.Patch(context.Background(), fmt.Sprintf("/boards/%s/tasks/%d", boardSlug, num), map[string]any{"assignee": assignee}, &task)
}

func (c *Client) CreateBoard(slug, name string) (model.Board, error) {
	var board model.Board
	return board, c.http.Post(context.Background(), "/boards", map[string]string{"slug": slug, "name": name}, &board)
}

func (c *Client) CreateComment(boardSlug string, num int, body string) (model.Comment, error) {
	var comment model.Comment
	return comment, c.http.Post(context.Background(), fmt.Sprintf("/boards/%s/tasks/%d/comments", boardSlug, num), map[string]string{"body": body}, &comment)
}

func (c *Client) TransitionTask(boardSlug string, num int, transition string) error {
	return c.http.Post(context.Background(), fmt.Sprintf("/boards/%s/tasks/%d/transition", boardSlug, num), map[string]string{"transition": transition}, nil)
}
