// Package httpclient provides a shared HTTP client for making authenticated
// JSON requests to the TaskFlow server. Used by the CLI, TUI, and simulator.
package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/transport"
)

// Client makes authenticated JSON HTTP requests to a TaskFlow server.
type Client struct {
	BaseURL string       // e.g. "http://localhost:8374"
	APIKey  string       // bearer token; omitted from requests if empty
	HTTP    *http.Client // defaults to http.DefaultClient if nil
}

// Do executes an authenticated JSON HTTP request.
//
//   - Sets Authorization: Bearer header if APIKey is non-empty
//   - Sets Content-Type: application/json if body is non-nil
//   - Decodes JSON error responses (extracts "message" field)
//   - Handles 204 No Content (skips response decode)
//   - Skips response decode if out is nil
func (c *Client) Do(ctx context.Context, method, path string, body any, out any) error {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reqBody)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return decodeError(resp)
	}

	if resp.StatusCode == 204 || out == nil {
		return nil
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

// Get is shorthand for Do with GET method and no body.
func (c *Client) Get(ctx context.Context, path string, out any) error {
	return c.Do(ctx, "GET", path, nil, out)
}

// Post is shorthand for Do with POST method.
func (c *Client) Post(ctx context.Context, path string, body any, out any) error {
	return c.Do(ctx, "POST", path, body, out)
}

// Patch is shorthand for Do with PATCH method.
func (c *Client) Patch(ctx context.Context, path string, body any, out any) error {
	return c.Do(ctx, "PATCH", path, body, out)
}

// Put is shorthand for Do with PUT method.
func (c *Client) Put(ctx context.Context, path string, body any, out any) error {
	return c.Do(ctx, "PUT", path, body, out)
}

// Delete is shorthand for Do with DELETE method and no body/output.
func (c *Client) Delete(ctx context.Context, path string) error {
	return c.Do(ctx, "DELETE", path, nil, nil)
}

// PathParams maps path parameter names to values for URL substitution.
type PathParams = map[string]string

// Resource executes a GET request for a domain resource.
// params substitutes {param} placeholders in the resource path.
// filter is an optional struct with `query` tags (e.g. model.TaskFilter) —
// non-zero fields become query parameters. Pass nil for no filtering.
// out receives the decoded JSON response.
func (c *Client) Resource(ctx context.Context, res model.Resource, params PathParams, filter any, out any) error {
	path := model.SubstitutePath(res.Path, params)
	if qs := model.BuildQueryString(filter); qs != "" {
		path += "?" + qs
	}
	return c.Get(ctx, path, out)
}

// Operation executes a mutation against a domain operation.
// params substitutes {param} placeholders in the operation path.
// body is the request payload (nil for deletes).
// out receives the decoded JSON response (nil for 204 responses).
func (c *Client) Operation(ctx context.Context, op model.Operation, params PathParams, body any, out any) error {
	path := model.SubstitutePath(op.Path, params)
	method := transport.MethodForAction(op.Action)
	return c.Do(ctx, method, path, body, out)
}

// APIError is returned when the server responds with a 4xx or 5xx status.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("error %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("error %d", e.StatusCode)
}

func decodeError(resp *http.Response) error {
	apiErr := &APIError{StatusCode: resp.StatusCode}
	var errResp map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil {
		if msg, ok := errResp["message"].(string); ok {
			apiErr.Message = msg
		}
	}
	return apiErr
}
