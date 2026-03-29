package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/workflow"
)

// Client is a thin HTTP client for the TaskFlow API.
type Client struct {
	baseURL string
	apiKey  string
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{baseURL: baseURL, apiKey: apiKey}
}

func (c *Client) ListBoards() ([]model.Board, error) {
	var boards []model.Board
	return boards, c.get("/boards", &boards)
}

func (c *Client) GetBoard(slug string) (model.Board, error) {
	var board model.Board
	return board, c.get("/boards/"+slug, &board)
}

func (c *Client) ListTasks(boardSlug string) ([]model.Task, error) {
	var tasks []model.Task
	return tasks, c.get("/boards/"+boardSlug+"/tasks?include_closed=true", &tasks)
}

func (c *Client) GetTask(boardSlug string, num int) (model.Task, error) {
	var task model.Task
	return task, c.get(fmt.Sprintf("/boards/%s/tasks/%d", boardSlug, num), &task)
}

func (c *Client) ListComments(boardSlug string, num int) ([]model.Comment, error) {
	var comments []model.Comment
	return comments, c.get(fmt.Sprintf("/boards/%s/tasks/%d/comments", boardSlug, num), &comments)
}

func (c *Client) ListDependencies(boardSlug string, num int) ([]model.Dependency, error) {
	var deps []model.Dependency
	return deps, c.get(fmt.Sprintf("/boards/%s/tasks/%d/dependencies", boardSlug, num), &deps)
}

func (c *Client) ListAttachments(boardSlug string, num int) ([]model.Attachment, error) {
	var atts []model.Attachment
	return atts, c.get(fmt.Sprintf("/boards/%s/tasks/%d/attachments", boardSlug, num), &atts)
}

func (c *Client) GetTaskAudit(boardSlug string, num int) ([]model.AuditEntry, error) {
	var entries []model.AuditEntry
	return entries, c.get(fmt.Sprintf("/boards/%s/tasks/%d/audit", boardSlug, num), &entries)
}

func (c *Client) GetBoardAudit(boardSlug string) ([]model.AuditEntry, error) {
	var entries []model.AuditEntry
	return entries, c.get("/boards/"+boardSlug+"/audit", &entries)
}

func (c *Client) GetWorkflow(boardSlug string) (*workflow.Workflow, error) {
	var wf workflow.Workflow
	return &wf, c.get("/boards/"+boardSlug+"/workflow", &wf)
}

func (c *Client) CreateComment(boardSlug string, num int, body string) (model.Comment, error) {
	var comment model.Comment
	return comment, c.post(fmt.Sprintf("/boards/%s/tasks/%d/comments", boardSlug, num), map[string]string{"body": body}, &comment)
}

func (c *Client) post(path string, payload any, out any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errResp map[string]any
		json.NewDecoder(resp.Body).Decode(&errResp)
		msg := fmt.Sprintf("API error: status %d", resp.StatusCode)
		if m, ok := errResp["message"].(string); ok {
			msg = m
		}
		return fmt.Errorf("%s", msg)
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) get(path string, out any) error {
	req, err := http.NewRequest("GET", c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(out)
}
