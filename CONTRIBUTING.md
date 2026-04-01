# Contributing to TaskFlow

## Getting set up

**Prerequisites:** Go 1.25+, [just](https://github.com/casey/just), and optionally Docker.

```bash
git clone https://github.com/bricef/taskflow.git
cd taskflow
just build        # build all binaries
just test-unit    # run unit + integration tests
just test         # full suite including QA smoke test
```

## Development workflow

1. Create a branch from `main`
2. Make your changes
3. Run `just check` — this runs formatting, vet, tests with race detector, and the 45-check QA smoke test
4. Commit with a clear message describing the **why**, not the what
5. Open a PR against `main`

### Useful commands

```
just check          # full CI suite (fmt + vet + test + QA smoke)
just test-unit      # fast feedback (no server startup)
just fmt            # format all Go files
just seed && just run-test   # run server with test data
just tui-test       # run TUI against test server
```

## Code organisation

The codebase follows a strict dependency flow — see [ARCHITECTURE.md](ARCHITECTURE.md) for the full picture. The key principle: **define once, derive everywhere**.

- Add a new endpoint? Define it in `model.Resources()` or `model.Operations()` in `internal/model/operations.go`. The HTTP route, CLI command, MCP tool, and OpenAPI spec are all derived automatically.
- Add a filter parameter? Add a `query` tag to the filter struct. It propagates to OpenAPI, CLI flags, and the httpclient.
- Add a service method? Define it in `internal/taskflow/taskflow.go` (interface), implement in `internal/service/`.

### Where things live

| What you're changing | Where to look |
|---------------------|---------------|
| Domain types (Task, Board, etc.) | `internal/model/` |
| Business logic | `internal/service/` |
| HTTP handlers | `internal/http/handlers.go` |
| Route registration | `internal/http/routes.go` |
| CLI command derivation | `internal/cli/cli.go` |
| MCP tool/resource derivation | `internal/mcp/server.go` |
| Shared HTTP client | `internal/httpclient/` |
| TUI views | `internal/tui/` |
| Database queries | `internal/sqlite/` |

## Tests

The test suite has four layers:

1. **Unit tests** — pure logic (path params, method mapping, query derivation)
2. **Golden tests** — snapshot the full OpenAPI spec and CLI command tree; any drift fails the test
3. **Integration tests** — HTTP server and CLI against in-memory SQLite
4. **QA smoke test** — `scripts/qa-test.sh` starts a real server and runs 45 endpoint checks

If you add a new resource or operation, the golden tests will fail. Regenerate with:

```bash
go test ./internal/http/ -update
go test ./internal/cli/ -update
```

Review the diff to confirm the change is intentional, then commit the updated golden files.

## Conventions

- **Formatting**: `gofmt`. CI enforces it.
- **Naming**: resources use `<resource>_<action>` (e.g. `task_create`, `board_list`). This convention drives CLI command grouping and MCP tool names.
- **Errors**: return domain error types (`ValidationError`, `NotFoundError`, `ConflictError`) from the service layer. The HTTP layer maps them to status codes.
- **Query params**: declare on filter structs with `query:"name,description"` tags, not manually.
- **No unnecessary abstraction**: three similar lines are better than a premature helper.

## Commit messages

Write messages that describe the purpose, not the mechanics:

```
Add task detail endpoint for composite task view      # good
Update operations.go and routes.go                    # bad
```

## Releasing

Releases are handled by maintainers:

```bash
just release v0.1.2
```

This tags, pushes, and triggers CI to build cross-platform binaries, create a GitHub Release, and push Docker images. See the [README](README.md#releasing) for the full flow.
