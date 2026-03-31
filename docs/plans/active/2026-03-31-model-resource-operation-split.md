# Refactor: Split model.Operations() into Resources and Operations

**Date:** 2026-03-31
**Status:** Planned (not started)
**Prerequisite:** None — this lands first; the httpclient extraction depends on this.
**Depends on:** None of the existing functionality needs this to work — this is a design improvement that enables better auto-derivation for new clients (MCP, future gRPC, etc.)

## Problem

`model.Operations()` conflates two fundamentally different things:

1. **Resources** — reading data (list boards, get task, get workflow, list audit)
2. **Operations** — mutations that change state (create task, transition, delete, add comment)

This makes it harder for new clients (MCP, future gRPC) to auto-derive their surface:
- MCP needs to register resources and tools separately
- The distinction between read and write is important for caching, permissions, and UI
- Currently, the action type (list/get vs create/update/delete) implicitly encodes this, but it's not explicit

Additionally:
- Operation names are derived by munging the path, which is fragile
- `HTTPMethod` mapping is done in the HTTP server package, but every client needs it
- Convenience endpoints (board detail, stats, search, global tasks) aren't in `Operations()` at all

## Design

### Explicit naming

Add a `Name` field to the builder, set explicitly:

```go
Create("/boards/{slug}/tasks", "Create a task").Name("create_task").
    Input(CreateTaskParams{}).Output(Task{}).Build()
```

This replaces path-based name derivation. The name is used by:
- CLI: command name (`taskflow task create`)
- MCP: tool name (`create_task`) or resource URI
- Logging, metrics, error messages

### Resource vs Operation distinction

**Decision: Separate types (Option A).**

```go
type Resource struct {
    Name    string
    Path    string
    Summary string
    MinRole Role
    Output  any       // query params derived from this type automatically
}

type Operation struct {
    Name    string
    Action  Action
    Path    string
    Summary string
    MinRole Role
    Input   any
    Output  any
}
```

Separate types are stronger and more readable. Derivation functions (HTTP routes, CLI commands, MCP registration) can loop over two arrays or a mixed array without adding much complexity.

### Auto-derivation from Resources

A `Resource` definition carries enough information to automatically derive both read endpoints:

- **GetOne**: `GET /path/{id}` — return a single item
- **List**: `GET /path` — return a collection

The transport layer generates both from the resource definition. The model only needs to explicitly declare mutations as `Operation`s — read access falls out of the resource definitions.

Query parameters should also be derived automatically from the domain model structs (e.g. via struct tags or reflection on the `Output` type's filter fields) rather than requiring manual `QueryParam` declarations. This eliminates a class of boilerplate and keeps the query surface in sync with the domain model by construction.

### HTTPMethod stays in transport

**Decision: `MethodForAction` lives in `internal/transport/http.go`.**

The model says "this is a create action" — the transport layer decides POST. `MethodForAction` lives in `internal/transport/http.go` (not `internal/http`, which is the server). Both the HTTP server and the HTTP client import from `internal/transport` for method mapping. This avoids the server package becoming a dependency of the client.

### Naming convention

**Decision: Names use `<resource>_<action>` format.**

All operations and resources get explicit `Name` fields using the pattern `<resource>_<action>`, e.g.:
- `task_create`, `task_delete`, `task_transition`
- `board_list`, `board_detail`, `board_overview`
- `comment_add`, `attachment_upload`
- `admin_stats`, `search`

The CLI derives its command tree directly from these names. The name is the canonical identifier used across all clients (CLI, MCP, logging, metrics).

### Convenience endpoints — deferred

**Decision: Deferred pending analysis.**

Before adding convenience endpoints to the model, we need to determine which are transport-agnostic (and belong in the model) vs transport-specific (and should stay in the HTTP layer). All five current convenience endpoints have separate handler methods (not closures):

| Endpoint | Handler | File |
|----------|---------|------|
| `GET /boards/{slug}/detail` | `boardDetailHandler` | `internal/http/board_detail.go` |
| `GET /boards/{slug}/overview` | `boardOverviewHandler` | `internal/http/board_overview.go` |
| `GET /admin/stats` | `systemStatsHandler` | `internal/http/system_stats.go` |
| `GET /tasks` | `globalTasksHandler` | `internal/http/global_tasks.go` |
| `GET /search` | `searchHandler` | `internal/http/search.go` |

Additionally, SSE endpoints (`/boards/{slug}/events`, `/events`) and the batch endpoint (`/batch`) are transport-specific by nature. The analysis should determine if any of the five above are similarly HTTP-specific or if they map cleanly to model-level resources.

## Phase 0: Pre-refactor test baseline

Before touching any production code, add tests that lock down current behaviour. These become the regression safety net — the refactor must produce identical output from all of them.

### 0.1 Unit tests for derivation functions

**`internal/model/operations_test.go`** — new file:
- `TestInferPathParams`: exercise `inferPathParams()` with representative paths (`/boards/{slug}`, `/boards/{slug}/tasks/{num}`, `/boards/{slug}/tasks/{num}/transition`, paths with no params). Assert correct names, types (string vs int via `intParamNames`), and ordering.
- `TestOperationsNoDuplicatePaths`: verify no two operations share the same (Action, Path) pair.
- `TestOperationsFieldsPopulated`: every operation has non-empty Action, Path, Summary, and a non-zero MinRole.

**`internal/http/adapters_test.go`** — new file:
- `TestMethodForAction`: table-driven test covering every `Action` constant → expected HTTP method.
- `TestStatusForAction`: table-driven test covering every `Action` constant → expected status code.

### 0.2 Golden tests (snapshot baselines)

**`internal/http/openapi_golden_test.go`** — new file:
- Generate the full OpenAPI spec via `generateOpenAPISpec()`, marshal to JSON.
- Compare against a checked-in golden file (`internal/http/testdata/openapi.golden.json`).
- On mismatch, fail with a diff and a hint to run `go test -update` to regenerate.
- This catches any change to paths, methods, parameters, schemas, or status codes.

**`internal/cli/cli_golden_test.go`** — new file:
- Build the CLI via `BuildCLI()`, walk the command tree, and emit a sorted list of all commands with their usage lines (e.g. `actor create <name>`, `task transition <slug> <num>`).
- Compare against a checked-in golden file (`internal/cli/testdata/commands.golden.txt`).
- This catches any change to command names, grouping, or argument structure.

### 0.3 Implementation details to resolve after Phase 0

Once the test baseline is green, fill in these details before starting the refactor:

**Handler matching strategy.** Change `routeHandlers()` from returning `[]handler` to `map[string]handler` keyed by operation `Name`. `allRoutes()` looks up each operation by name instead of by positional index. Panic message becomes `"no handler for operation: " + op.Name`. This eliminates the fragile positional coupling.

**CLI name parsing.** `strings.SplitN(op.Name, "_", 2)` splits `<resource>_<action>` into group + subcommand. `deriveCommandName()` and `singularize()` are deleted.

**Resource default roles.** Resources are read-only by definition, so `MinRole` defaults to `RoleReadOnly` unless overridden in the builder. No `Action` field needed — the transport layer knows resources are always GET.

## Scope of changes

### Phase 0

| File | Change |
|------|--------|
| `internal/model/operations_test.go` | Create — unit tests for `inferPathParams`, operation invariants |
| `internal/http/adapters_test.go` | Create — unit tests for `MethodForAction`, `statusForAction` |
| `internal/http/openapi_golden_test.go` | Create — golden test for full OpenAPI spec |
| `internal/http/testdata/openapi.golden.json` | Create — checked-in golden file |
| `internal/cli/cli_golden_test.go` | Create — golden test for CLI command tree |
| `internal/cli/testdata/commands.golden.txt` | Create — checked-in golden file |

### Phase 1 (after Phase 0 is green)

| File | Change |
|------|--------|
| `internal/model/operations.go` | Split into separate `Resource` and `Operation` types, add `Name` field, update builder |
| `internal/transport/http.go` | Create — `MethodForAction` moves here |
| `internal/http/routes.go` | Change `routeHandlers()` to `map[string]handler`, match by Name |
| `internal/http/adapters.go` | Remove `MethodForAction` (moved to transport), update imports |
| `internal/http/openapi.go` | Use Name for operation IDs |
| `internal/cli/cli.go` | Use Name for command derivation; delete `deriveCommandName()`, `singularize()` |
| All tests | Golden tests must produce identical output; update golden files only if behaviour intentionally changes |

## Risks

- Large changeset touching many files — mitigated by Phase 0 golden tests providing a regression baseline
- HTTP server handler matching currently relies on positional ordering — mitigated by switching to `map[string]handler` keyed by Name
- ~~CLI command tree derivation from names vs paths needs careful design~~ — resolved: `strings.SplitN(name, "_", 2)` with `<resource>_<action>` convention
- ~~Convenience endpoint handlers are currently closures in `registerRoutes()`~~ — not true, they are separate handler methods; convenience endpoint inclusion deferred pending transport analysis

## Verification

1. `go build ./...` + `go test ./...` — golden tests catch any behavioural drift
2. Manual: verify dashboard and TUI still work
3. Count operations before/after — should increase (convenience endpoints added)
