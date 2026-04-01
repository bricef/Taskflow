# Pagination

**Date:** 2026-04-01
**Status:** Planned (not started)

## Problem

All list endpoints return full result sets. A board with thousands of tasks will produce large responses and high latency. Cross-board search (`/tasks`) caps at 1000 results without pagination controls.

## Design

Add `limit` and `offset` query parameters to all list Resources. Return pagination metadata in the response.

### Query parameters

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `limit` | integer | 100 | Maximum results to return (max 1000) |
| `offset` | integer | 0 | Number of results to skip |

### Response envelope

List responses gain a wrapper with pagination metadata:

```json
{
  "items": [...],
  "total": 342,
  "limit": 100,
  "offset": 0
}
```

### Scope

- Add `limit`/`offset` to the `TaskFilter` struct with `query` tags
- Add `ListBoardsParams` limit/offset
- Update all list handlers to pass through to SQL queries
- Update the service's `SearchTasks` to respect limits
- Consider cursor-based pagination for SSE-heavy workflows (offset can skip items during concurrent writes)

### Breaking change

Current list endpoints return bare arrays. Wrapping in `{"items": [...]}` is a breaking change for all clients. Needs a migration plan (version header, or just ship it and update clients).
