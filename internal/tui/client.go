package tui

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/bricef/taskflow/internal/model"
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
