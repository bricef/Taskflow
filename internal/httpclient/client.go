// Package httpclient provides a domain-aware HTTP client for the TaskFlow API.
// Consumers use the generic functions GetOne, GetMany, Exec, and ExecNoResult
// with model.Resource and model.Operation types — no manual URL building needed.
package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/transport"
	"github.com/bricef/taskflow/internal/version"
)

// PathParams maps path parameter names to values for URL substitution.
type PathParams = map[string]string

// Client makes authenticated JSON HTTP requests to a TaskFlow server.
type Client struct {
	baseURL      string
	apiKey       string
	httpClient   *http.Client
	ctx          context.Context
	versionCheck *sync.Once
}

// New creates a new Client for the given server.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL:      baseURL,
		apiKey:       apiKey,
		httpClient:   http.DefaultClient,
		ctx:          context.Background(),
		versionCheck: &sync.Once{},
	}
}

// WithContext returns a shallow copy of the client with a different default context.
func (c *Client) WithContext(ctx context.Context) *Client {
	c2 := *c
	c2.ctx = ctx
	return &c2
}

// Version returns the client's build version.
func (c *Client) Version() string { return version.Version }

// ServerVersion fetches the server's version from the /health endpoint.
func (c *Client) ServerVersion() (string, error) {
	var health struct {
		Version string `json:"version"`
	}
	if err := c.do("GET", "/health", nil, &health); err != nil {
		return "", err
	}
	return health.Version, nil
}

// WhoAmI returns the authenticated actor for this client's API key.
func (c *Client) WhoAmI() (model.Actor, error) {
	var actor model.Actor
	err := c.do("GET", "/me", nil, &actor)
	return actor, err
}

// resource executes a GET request for a domain resource.
func (c *Client) resource(res model.Resource, params PathParams, filter any, out any) error {
	path := model.SubstitutePath(res.Path, params)
	if qs := model.BuildQueryString(filter); qs != "" {
		path += "?" + qs
	}
	return c.do("GET", path, nil, out)
}

// operation executes a mutation against a domain operation.
func (c *Client) operation(op model.Operation, params PathParams, body any, out any) error {
	path := model.SubstitutePath(op.Path, params)
	method := transport.MethodForAction(op.Action)
	return c.do(method, path, body, out)
}

// do executes an authenticated JSON HTTP request.
func (c *Client) do(method, path string, body any, out any) error {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &ConnectionError{URL: c.baseURL, Err: err}
	}
	defer resp.Body.Close()

	// Check server version once per client instance.
	c.versionCheck.Do(func() {
		if sv := resp.Header.Get("X-TaskFlow-Version"); sv != "" && sv != version.Version {
			fmt.Fprintf(os.Stderr, "Warning: client version (%s) differs from server (%s)\n", version.Version, sv)
		}
	})

	if resp.StatusCode >= 400 {
		return decodeError(resp)
	}

	if resp.StatusCode == 204 || out == nil {
		return nil
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

// --- Generic functions ---
//
// Go does not support generic methods, so these are package-level functions.
// The Client's stored context is used automatically.

// GetOne executes a resource query and returns a single typed result.
func GetOne[T any](c *Client, res model.Resource, params PathParams, filter any) (T, error) {
	var out T
	err := c.resource(res, params, filter, &out)
	return out, err
}

// GetMany executes a resource query and returns a typed slice.
func GetMany[T any](c *Client, res model.Resource, params PathParams, filter any) ([]T, error) {
	var out []T
	err := c.resource(res, params, filter, &out)
	return out, err
}

// Exec executes an operation and returns a typed result.
func Exec[T any](c *Client, op model.Operation, params PathParams, body any) (T, error) {
	var out T
	err := c.operation(op, params, body, &out)
	return out, err
}

// ExecNoResult executes an operation that returns no body (e.g. DELETE → 204).
func ExecNoResult(c *Client, op model.Operation, params PathParams, body any) error {
	return c.operation(op, params, body, nil)
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

// ConnectionError is returned when the HTTP request fails at the transport
// level (server unreachable, DNS failure, timeout, etc.).
type ConnectionError struct {
	URL string
	Err error
}

func (e *ConnectionError) Error() string {
	return fmt.Sprintf(`could not connect to TaskFlow server at %s

%v

Is the server running? Check your configuration:
  --url <url>                                (CLI flag)
  export TASKFLOW_URL=<url>                  (environment variable)
  url: <url> in ~/.config/taskflow/config.yaml  (config file)`, e.URL, e.Err)
}

func (e *ConnectionError) Unwrap() error { return e.Err }

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
