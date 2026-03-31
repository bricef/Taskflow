# Manual Testing

This document covers manual QA checks that supplement the automated test suite. Run these after any refactor that changes routing, command derivation, or API surface.

## Prerequisites

```bash
just seed        # generate test database
just run-test    # start server with test data (localhost:8374)
```

The seed admin key is `seed-admin-key-for-testing`. For CLI commands, set:

```bash
export TASKFLOW_URL=http://localhost:8374
export TASKFLOW_API_KEY=seed-admin-key-for-testing
```

## 1. Routing correctness

Verify that each resource group returns the correct shape of data. A handler mismatch would return wrong fields or a 500.

| # | Endpoint | Expected |
|---|----------|----------|
| 1 | `GET /actors` | Array of objects with `name`, `role` fields |
| 2 | `GET /boards` | Array of objects with `slug`, `name` fields |
| 3 | `GET /boards/{slug}/tasks` | Array of task objects |
| 4 | `GET /boards/{slug}/workflow` | Object with `states` array |
| 5 | `GET /webhooks` | Array (possibly empty) |
| 6 | `GET /boards/{slug}/tags` | Array of tag count objects |
| 7 | `GET /boards/{slug}/tasks/1/dependencies` | Array (possibly empty) |
| 8 | `GET /boards/{slug}/tasks/1/attachments` | Array (possibly empty) |
| 9 | `GET /webhooks` (as admin) | Array (possibly empty) |

```bash
# Quick smoke test — all should return 200 with valid JSON:
for path in /actors /boards /boards/test-board/tasks /boards/test-board/workflow \
            /boards/test-board/tags /boards/test-board/tasks/1/dependencies \
            /boards/test-board/tasks/1/attachments /webhooks; do
  STATUS=$(curl -s -o /dev/null -w '%{http_code}' \
    -H "Authorization: Bearer seed-admin-key-for-testing" \
    "http://localhost:8374$path")
  echo "$STATUS $path"
done
```

All should print `200`. Any `404`, `405`, or `500` indicates a handler mismatch.

## 2. Audit endpoints

Audit is a domain operation on boards and tasks (not a standalone resource).

| # | Endpoint | Expected |
|---|----------|----------|
| 10 | `GET /boards/{slug}/audit` | Array of audit entries |
| 11 | `GET /boards/{slug}/tasks/1/audit` | Array of audit entries for the task |

```bash
curl -s -H "Authorization: Bearer seed-admin-key-for-testing" \
  http://localhost:8374/boards/test-board/audit | jq length

curl -s -H "Authorization: Bearer seed-admin-key-for-testing" \
  http://localhost:8374/boards/test-board/tasks/1/audit | jq length
```

Both should return a number (0 or more).

## 3. Mutation endpoints

Verify a write operation in each category still works.

| # | Test | Command |
|---|------|---------|
| 12 | Create + get board | `POST /boards` then `GET /boards/{slug}` |
| 13 | Create + list tasks | `POST /boards/{slug}/tasks` then `GET /boards/{slug}/tasks` |
| 14 | Transition task | `POST /boards/{slug}/tasks/{num}/transition` |
| 15 | Workflow health | `POST /boards/{slug}/workflow/health` |

```bash
# Create a board
curl -s -X POST -H "Authorization: Bearer seed-admin-key-for-testing" \
  -H "Content-Type: application/json" \
  -d '{"slug":"qa-test","name":"QA Test"}' \
  http://localhost:8374/boards | jq .slug

# Workflow health check
curl -s -X POST -H "Authorization: Bearer seed-admin-key-for-testing" \
  http://localhost:8374/boards/test-board/workflow/health | jq .
```

## 4. CLI commands

Verify key commands work end-to-end, especially the renamed audit commands.

| # | Command | Expected |
|---|---------|----------|
| 16 | `taskflow board list` | Table of boards |
| 17 | `taskflow board audit test-board` | Audit entries |
| 18 | `taskflow task audit test-board 1` | Task audit entries |
| 19 | `taskflow task list test-board` | Table of tasks |
| 20 | `taskflow task transition test-board 1 --state done` | Transitions task |

## 5. OpenAPI spec

| # | Test | How |
|---|------|-----|
| 21 | operationIds present | `curl -s localhost:8374/openapi.json \| jq '.paths["/actors"].get.operationId'` should return `"actor_list"` |
| 22 | Spec is valid | Paste `http://localhost:8374/openapi.json` into [Swagger Editor](https://editor.swagger.io) — no validation errors |

## 6. Dashboard and TUI (smoke test)

| # | Test | How |
|---|------|-----|
| 23 | Dashboard loads | Open `http://localhost:8374/dashboard` — board list renders |
| 24 | Board detail page | Click a board — tasks render |
| 25 | TUI connects | `just tui-test` — shows boards and tasks |

## Pass criteria

All 25 checks pass. Any unexpected status code, wrong response shape, or panic indicates a regression.
