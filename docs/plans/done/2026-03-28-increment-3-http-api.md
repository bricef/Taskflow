# Increment 3: HTTP API Layer

## Context

We have 38 service operations on the `TaskFlow` interface. We need an HTTP API that exposes them as REST endpoints. Rather than a custom framework, we derive handlers from the service methods using generic adapters for common patterns and explicit closures for complex ones. Middleware handles auth, RBAC, and error mapping.

## Architecture

```
Request → Auth Middleware → RBAC Middleware → Handler → Error Middleware → JSON Response
                 │                │                │
          Resolve actor    Check MinRole    Call TaskFlow method
          from API key     against route    via adapter or closure
```

### Package: `internal/http`

```
internal/http/
├── server.go       Server struct, New(), ListenAndServe, route table
├── routes.go       All route registrations (one line per simple, ~5 lines per complex)
├── adapters.go     Generic adapter functions (jsonBody, pathStr, noInput, etc.)
├── middleware.go   Auth, RBAC, error mapping, JSON response formatting
└── context.go      ActorFromContext helper
```

Dependencies: `chi` for routing. API keys hashed with SHA-256 (fast, sufficient for random keys; bcrypt unnecessary since keys aren't user-chosen passwords).

## Generic Adapters

Each adapter converts an HTTP request into a service method call:

```go
// Handler is the inner function that adapters produce.
type Handler func(ctx context.Context, r *http.Request) (any, error)

// jsonBody: JSON request body → params struct → service method
func jsonBody[P any, R any](fn func(context.Context, P) (R, error)) Handler

// noInput: no request params → service method
func noInput[R any](fn func(context.Context) (R, error)) Handler

// pathStr: single string path param → service method
func pathStr[R any](param string, fn func(context.Context, string) (R, error)) Handler

// pathInt: single int path param → service method
func pathInt[R any](param string, fn func(context.Context, int) (R, error)) Handler

// pathStrInt: two path params (string + int) → service method
func pathStrInt[R any](p1, p2 string, fn func(context.Context, string, int) (R, error)) Handler
```

### Coverage

These 5 adapters handle 16 of 35 HTTP routes (the simple ones):

| Adapter | Routes |
|---------|--------|
| jsonBody | CreateActor, CreateBoard |
| noInput | ListActors, ListWebhooks |
| pathStr | GetActor, GetBoard, GetWorkflow, CheckWorkflowHealth, QueryAuditByBoard |
| pathInt | GetWebhook |
| pathStrInt | GetTask, ListComments, ListDependencies, ListAttachments, QueryAuditByTask |

The remaining 19 routes need path/body/actor injection — these are explicit closures in `routes.go`, each 5-10 lines. No magic, fully readable.

## Middleware

### Auth Middleware
- Extracts `Authorization: Bearer <key>` header
- Hashes the key with SHA-256, calls `GetActorByAPIKeyHash` to resolve the actor
- Injects the actor into the request context
- Returns 401 if missing/invalid, 403 if deactivated
- Skips auth for `GET /health`

### RBAC Middleware
- Each route declares a `MinRole` (admin/member/read_only)
- Compares the actor's role from context against the route's MinRole
- Returns 403 with role details if insufficient

### Error Middleware
- Wraps the handler, catches errors, maps to HTTP status:
  - `ValidationError` → 400
  - `NotFoundError` → 404
  - `ConflictError` → 409
  - Unknown → 500
- Formats consistent JSON error response: `{"error": "...", "message": "...", "detail": {...}}`

### JSON Response Formatting
- Writes `Content-Type: application/json`
- 204 No Content for nil results (deletes)
- Marshals result to JSON for everything else

## Route Table

```go
// routes.go — declarative, one line per simple route, closure for complex ones
func (s *Server) routes() {
    r := s.router

    // Health (no auth)
    r.Get("/health", s.health)

    // Actors
    r.Post("/actors", s.handle(RoleAdmin, 201, jsonBody(s.svc.CreateActor)))
    r.Get("/actors", s.handle(RoleReadOnly, 200, noInput(s.svc.ListActors)))
    r.Get("/actors/{name}", s.handle(RoleReadOnly, 200, pathStr("name", s.svc.GetActor)))
    r.Patch("/actors/{name}", s.handle(RoleAdmin, 200, func(ctx context.Context, r *http.Request) (any, error) {
        var p model.UpdateActorParams
        if err := json.NewDecoder(r.Body).Decode(&p); err != nil { ... }
        p.Name = chi.URLParam(r, "name")
        return s.svc.UpdateActor(ctx, p)
    }))

    // Tasks (complex — needs path + body + actor)
    r.Post("/boards/{slug}/tasks", s.handle(RoleMember, 201, func(ctx context.Context, r *http.Request) (any, error) {
        var p model.CreateTaskParams
        if err := json.NewDecoder(r.Body).Decode(&p); err != nil { ... }
        p.BoardSlug = chi.URLParam(r, "slug")
        p.CreatedBy = ActorFrom(ctx)
        return s.svc.CreateTask(ctx, p)
    }))
    // ... etc
}
```

## Prerequisites: Model Changes

### 1. `Optional[T]` needs `json.Unmarshaler`

For PATCH operations, the JSON decoder must distinguish "field absent" (don't update) from "field present with null" (clear the field). Add:

```go
func (o *Optional[T]) UnmarshalJSON(data []byte) error {
    o.Set = true
    return json.Unmarshal(data, &o.Value)
}
```

When a JSON field is absent, `UnmarshalJSON` is never called, so `Set` stays `false`.

### 2. Add `json` tags to params structs

Update params need `json` tags so the decoder maps JSON field names to struct fields. The identifier fields (BoardSlug, Num, Name, ID) that come from path params get `json:"-"` to exclude them from the body.

### 3. Add `json` tags to Create params

Fields injected from path/auth context (BoardSlug, CreatedBy, Actor) get `json:"-"`.

## Server Bootstrap

```go
// cmd/taskflow-server/main.go
func main() {
    store, _ := sqlite.New(os.Getenv("TASKFLOW_DB_PATH"))
    svc := service.New(store)
    srv := http.NewServer(svc, http.Config{
        Addr: os.Getenv("TASKFLOW_LISTEN_ADDR"),
    })
    srv.ListenAndServe()
}
```

Seed admin bootstrap: on first start, if `TASKFLOW_SEED_ADMIN_NAME` is set and no actors exist, create an admin actor and write the API key to a file.

## Implementation Steps

1. Add `json` tags to all params structs, implement `Optional.UnmarshalJSON`
2. `go get chi`
3. Write `internal/http/context.go` — actor context helpers
4. Write `internal/http/middleware.go` — auth, RBAC, error mapping
5. Write `internal/http/adapters.go` — generic handler adapters
6. Write `internal/http/routes.go` — all route registrations
7. Write `internal/http/server.go` — server struct, config, startup
8. Write `cmd/taskflow-server/main.go` — minimal server binary
9. Write integration tests using `httptest`

## Verification

```bash
# Unit tests for middleware and adapters
go test ./internal/http/...

# Integration tests: start server, hit endpoints, verify responses
go test ./internal/http/... -run TestIntegration

# Full suite
just check
```

## Files to Create/Modify

| File | Action |
|------|--------|
| `internal/model/optional.go` | Add UnmarshalJSON |
| `internal/model/task.go` | Add json tags to CreateTaskParams, UpdateTaskParams, TransitionTaskParams |
| `internal/model/actor.go` | Add json tags to CreateActorParams, UpdateActorParams |
| `internal/model/board.go` | Add json tags to CreateBoardParams, UpdateBoardParams |
| `internal/model/comment.go` | Add json tags to CreateCommentParams, UpdateCommentParams |
| `internal/model/dependency.go` | Add json tags to CreateDependencyParams |
| `internal/model/attachment.go` | Add json tags to CreateAttachmentParams |
| `internal/model/webhook.go` | Add json tags to CreateWebhookParams, UpdateWebhookParams |
| `internal/http/server.go` | NEW |
| `internal/http/routes.go` | NEW |
| `internal/http/adapters.go` | NEW |
| `internal/http/middleware.go` | NEW |
| `internal/http/context.go` | NEW |
| `cmd/taskflow-server/main.go` | NEW |
| `go.mod` | Add chi |
