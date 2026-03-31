# Extract shared HTTP client

**Date:** 2026-03-31
**Status:** Active
**Prerequisite:** Model Resource/Operation split (must land first)

## Problem

TaskFlow has four independent HTTP client implementations that all do the same thing: make authenticated JSON requests to the TaskFlow server.

- **TUI** (`internal/tui/client.go`): `Client` struct with `doJSON()`, `get()`, `post()`, `patch()` and ~15 typed methods
- **CLI** (`internal/cli/cli.go`): inline HTTP call in `makeRunFunc()` (~40 lines)
- **Simulator** (`cmd/taskflow-sim/main.go`): `doRequest()` method (~25 lines)
- **MCP** (not yet built): will need the same client

Each reimplements Bearer auth, JSON marshaling, error decoding, and Content-Type handling with minor inconsistencies (some use context, some don't; some check `!= 200`, others check `>= 400`; some handle 204, some don't).

## Goal

Extract a single `internal/httpclient` package that all consumers share. This:
- Eliminates duplicated HTTP plumbing
- Provides a consistent error handling and auth pattern
- Gives the future MCP server a ready-made HTTP client
- Makes it easy to add features like request tracing or retry in one place

## Design

### The shared client

```go
// internal/httpclient/client.go
package httpclient

type Client struct {
    BaseURL string
    APIKey  string
    HTTP    *http.Client  // defaults to http.DefaultClient if nil
}

// Do executes an authenticated JSON HTTP request.
// - Sets Authorization: Bearer header if APIKey is non-empty
// - Sets Content-Type: application/json if body is non-nil
// - Decodes JSON error responses (extracts "message" field)
// - Handles 204 No Content (skips decode)
// - Skips response decode if out is nil
func (c *Client) Do(ctx context.Context, method, path string, body any, out any) error

// Convenience methods
func (c *Client) Get(ctx context.Context, path string, out any) error
func (c *Client) Post(ctx context.Context, path string, body any, out any) error
func (c *Client) Patch(ctx context.Context, path string, body any, out any) error
func (c *Client) Put(ctx context.Context, path string, body any, out any) error
func (c *Client) Delete(ctx context.Context, path string) error
```

Features unified from all four implementations:
- Bearer auth (set if non-empty, matching CLI's conditional pattern)
- Context support on all calls (from MCP's pattern — the best one)
- Content-Type only when payload is non-nil (CLI/MCP pattern)
- Error decoding: try `message` field, fall back to status code
- Handle 204 No Content
- Handle nil `out`

### Migration approach

Each consumer keeps its own API — we only replace the HTTP plumbing underneath.

**TUI**: `internal/tui/client.go` keeps its typed methods (`ListBoards()`, `GetTask()`, etc.) but replaces the private `doJSON()`/`get()`/`post()`/`patch()` with calls to `httpclient.Client`. The TUI client embeds or holds an `httpclient.Client`.

**CLI**: `internal/cli/cli.go` replaces the inline 40-line HTTP block in `makeRunFunc()` with a call to `httpclient.Client.Do()`. The path substitution, query building, and flag parsing stay as-is — only the actual HTTP call changes.

**Simulator**: `cmd/taskflow-sim/main.go` replaces `doRequest()` with `httpclient.Client.Do()`.

**MCP** (future): will use `httpclient.Client` directly — no need for a separate `HTTPBackend`.

## Steps

### Step 1: Create `internal/httpclient/client.go`
- `Client` struct with `Do()` and convenience methods
- Unit test with httptest server

### Step 2: Migrate TUI client
- Add `httpclient.Client` field to `tui.Client`
- Replace `doJSON()`, `get()`, `post()`, `patch()` with calls to `c.http.Do()`
- Remove the private HTTP methods
- All typed methods stay unchanged
- Run tests

### Step 3: Migrate CLI
- Add `httpclient.Client` to CLI config
- Replace the inline HTTP block in `makeRunFunc()` with `client.Do()`
- Keep path substitution, query building, flag parsing, output formatting
- `MethodForAction` stays in `internal/http` (server-side concern)
- The CLI needs its own method mapping (same logic, different location) or imports from `internal/http`
- Run tests

### Step 4: Migrate simulator
- Replace `doRequest()` with `httpclient.Client.Do()`
- Simplify the simulator struct
- Build and manual test

### Step 5: Clean up
- Remove dead code from TUI client (private HTTP methods)
- Verify no other packages have inline HTTP clients

## What stays unchanged

- TUI typed method signatures (public API)
- CLI operation-driven command derivation (flags, path substitution, output formatting)
- `MethodForAction` location in `internal/http`
- Server-side code (routes, handlers, middleware)
- `model.Operations()` — no changes needed for this refactor

## Files

| File | Action |
|------|--------|
| `internal/httpclient/client.go` | Create |
| `internal/httpclient/client_test.go` | Create |
| `internal/tui/client.go` | Modify — use httpclient internally |
| `internal/cli/cli.go` | Modify — use httpclient for HTTP calls |
| `cmd/taskflow-sim/main.go` | Modify — use httpclient |

## Verification

1. `go build ./...` — all packages compile
2. `go test ./...` — all tests pass
3. Manual: run server + TUI + simulator, verify live updates
4. Manual: `taskflow board list` via CLI
5. Manual: verify simulator creates/transitions/assigns tasks
