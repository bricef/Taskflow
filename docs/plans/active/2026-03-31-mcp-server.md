# MCP Server Implementation

**Date:** 2026-03-31
**Status:** Planned (not started)
**Ref:** PRD §8.3, Phase 3 plan (docs/03-Phase-3.md)

## Context

The Phase 3 plan was written before the model refactor. Most prerequisites are now complete:

| Prerequisite | Status |
|---|---|
| Resource/Operation split with Names | Done |
| Shared httpclient with `GetOne/GetMany/Exec/ExecNoResult` | Done |
| Global SSE endpoint (`GET /events`) | Done |
| `httpclient.Subscribe` with reconnect/backoff | Done |
| Cross-board task search (`task_search` Resource at `/tasks`) | Done |
| Board overview with task counts (`board_overview` Resource) | Done |
| Query param derivation from struct tags | Done |
| Named resource/operation registry (`model.Res*`, `model.Op*`) | Done |

The original plan's `Backend` interface and `httpbackend` abstraction are **no longer needed**. The `httpclient` already provides the domain-aware transport layer. The MCP handlers call `httpclient.GetOne/GetMany/Exec/ExecNoResult` directly — the same pattern as the TUI.

## Revised design

### Single transport: stdio binary

Start with the stdio binary only. The HTTP transport (embedded `/mcp` endpoint) adds complexity (multi-tenant auth, direct service calls) for marginal benefit. An agent launches `taskflow-mcp` with its API key — the binary connects to the server over HTTP like any other client.

The HTTP transport can be added later if needed.

### Auto-derivation from the model

MCP resources and tools are derived directly from `model.Resources()` and `model.Operations()`:

- Each `Resource` → MCP resource with URI `taskflow://{path}` and read handler calling `httpclient.GetOne` or `httpclient.GetMany`
- Each `Operation` → MCP tool with input schema from `Input` struct's json tags and handler calling `httpclient.Exec` or `httpclient.ExecNoResult`
- Names map directly: `model.ResTaskList` → MCP resource name `task_list`, `model.OpTaskCreate` → MCP tool name `task_create`

No manual tool registration — iterate the model, derive everything.

### Notifications

The MCP server subscribes via `httpclient.Subscribe` in the background. Events from other actors (filtered by comparing `evt.Actor.Name` to the configured actor identity) are queued. Two delivery mechanisms:

1. **MCP `notifications/resources/list_changed`** — sent when events arrive, so the agent can re-read affected resources
2. **Piggyback `_notifications`** — pending events included in tool response metadata for clients that don't support MCP notifications

Ring buffer capped at 50 entries. Older entries summarised.

## Implementation steps

### Step 1: Scaffold + capability negotiation

- `cmd/taskflow-mcp/main.go` — stdio entry point
- Use official Go MCP SDK (`github.com/modelcontextprotocol/go-sdk`)
- Config from env vars: `TASKFLOW_URL`, `TASKFLOW_API_KEY`
- Create `httpclient.Client`, register capabilities (tools + resources)
- Verify: `echo '{"jsonrpc":"2.0","method":"initialize","params":{},"id":1}' | taskflow-mcp`

### Step 2: Resources

Derive MCP resources from `model.Resources()`:
- URI: `taskflow://` + resource path (e.g. `taskflow://boards/{slug}/tasks`)
- Read handler: call `httpclient.GetOne` or `httpclient.GetMany` depending on output type (slice = list, otherwise = single)
- Return JSON text content
- Filter/sort: derive input schema from `Filter`/`Sort` struct query tags

### Step 3: Tools

Derive MCP tools from `model.Operations()`:
- Name: operation Name (e.g. `task_create`)
- Input schema: derive from `Input` struct json tags + path params
- Handler: call `httpclient.Exec` or `httpclient.ExecNoResult`
- Error mapping: `httpclient.APIError` → MCP tool error, preserving status code and message

### Step 4: Notifications

- Start `httpclient.Subscribe` in background goroutine at startup
- Filter events: skip events where `evt.Actor.Name` matches the configured agent identity
- Ring buffer of up to 50 pending notifications
- On tool call: attach pending notifications as `_notifications` in response metadata, clear buffer
- Send `notifications/resources/list_changed` when events arrive

### Step 5: Documentation + testing

- Agent setup guide (Claude Code config, Aider, Cursor)
- Example MCP config JSON
- Update README Phase 3 status
- Update SKILL.md with MCP notes
- Integration test: launch `taskflow-mcp` against test server, exercise resources and tools

## Files

| File | Action |
|------|--------|
| `cmd/taskflow-mcp/main.go` | Create — stdio entry point |
| `internal/mcp/server.go` | Create — MCP server setup, resource/tool registration |
| `internal/mcp/resources.go` | Create — resource handlers derived from model |
| `internal/mcp/tools.go` | Create — tool handlers derived from model |
| `internal/mcp/notifications.go` | Create — event subscription and notification queue |
| `go.mod` | Add `github.com/modelcontextprotocol/go-sdk` dependency |

## Out of scope

- HTTP transport (embedded `/mcp` endpoint) — add later if needed
- Reporting tools (CFD, cycle time, throughput) — requires new service methods
- File upload/download — attachments are references, not file storage
- Batch tool — can be added after the core tools work

## Verification

1. `go build ./...` + `go test ./...`
2. Manual: launch `taskflow-mcp` against test server, exercise with Claude Code
3. QA smoke test: extend `scripts/qa-test.sh` with MCP resource and tool checks
4. Verify notification delivery by running simulator alongside agent
