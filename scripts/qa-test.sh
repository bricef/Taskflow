#!/usr/bin/env bash
#
# QA smoke test for TaskFlow. Builds, seeds, starts a server, and runs checks
# against every resource and operation endpoint plus CLI commands.
#
# Usage: ./scripts/qa-test.sh
#
set -euo pipefail

BASE_URL="http://localhost:18374"
API_KEY="seed-admin-key-for-testing"
DB_PATH="./taskflow-qa-test.db"
PASS=0
FAIL=0
SERVER_PID=""

# --- Helpers ---

cleanup() {
  if [ -n "$SERVER_PID" ]; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  rm -f "$DB_PATH"
}
trap cleanup EXIT

auth() {
  echo "Authorization: Bearer $API_KEY"
}

# check NAME STATUS_CODE CURL_ARGS...
# Verifies the HTTP status code matches expectations.
check_status() {
  local name="$1" expect="$2"; shift 2
  local status body
  body=$(curl -s -o /dev/null -w '%{http_code}' -H "$(auth)" "$@")
  if [ "$body" = "$expect" ]; then
    echo "  PASS  $name (HTTP $body)"
    PASS=$((PASS + 1))
  else
    echo "  FAIL  $name (expected $expect, got $body)"
    FAIL=$((FAIL + 1))
  fi
}

# check_json NAME CURL_ARGS...
# Verifies the response is valid JSON with HTTP 200.
check_json() {
  local name="$1"; shift
  local body status
  body=$(curl -s -w '\n%{http_code}' -H "$(auth)" "$@")
  status=$(echo "$body" | tail -1)
  body=$(echo "$body" | sed '$d')
  if [ "$status" != "200" ]; then
    echo "  FAIL  $name (expected 200, got $status)"
    FAIL=$((FAIL + 1))
    return
  fi
  if echo "$body" | jq empty 2>/dev/null; then
    echo "  PASS  $name"
    PASS=$((PASS + 1))
  else
    echo "  FAIL  $name (response is not valid JSON)"
    FAIL=$((FAIL + 1))
  fi
}

# check_json_field NAME JQ_FILTER CURL_ARGS...
# Verifies a jq filter produces non-null output from a 200 response.
check_json_field() {
  local name="$1" filter="$2"; shift 2
  local body status value
  body=$(curl -s -w '\n%{http_code}' -H "$(auth)" "$@")
  status=$(echo "$body" | tail -1)
  body=$(echo "$body" | sed '$d')
  if [ "$status" != "200" ]; then
    echo "  FAIL  $name (expected 200, got $status)"
    FAIL=$((FAIL + 1))
    return
  fi
  value=$(echo "$body" | jq -r "$filter" 2>/dev/null)
  if [ -n "$value" ] && [ "$value" != "null" ]; then
    echo "  PASS  $name ($filter = $value)"
    PASS=$((PASS + 1))
  else
    echo "  FAIL  $name ($filter was null or empty)"
    FAIL=$((FAIL + 1))
  fi
}

# check_cli NAME ARGS...
# Runs a CLI command and verifies it exits 0 with non-empty output.
check_cli() {
  local name="$1"; shift
  local output
  if output=$(./taskflow --url "$BASE_URL" --api-key "$API_KEY" "$@" 2>&1) && [ -n "$output" ]; then
    echo "  PASS  $name"
    PASS=$((PASS + 1))
  else
    echo "  FAIL  $name (exit=$?, output=${output:-empty})"
    FAIL=$((FAIL + 1))
  fi
}

# --- Setup ---

echo "Building..."
go build -o taskflow-server ./cmd/taskflow-server
go build -o taskflow ./cmd/taskflow

echo "Seeding test database..."
go run ./cmd/taskflow-seed "$DB_PATH" > /dev/null 2>&1

echo "Starting server on port 18374 (dev mode)..."
TASKFLOW_DB_PATH="$DB_PATH" TASKFLOW_LISTEN_ADDR=":18374" TASKFLOW_DEV_MODE=true ./taskflow-server &
SERVER_PID=$!

# Wait for server to be ready.
for i in $(seq 1 30); do
  if curl -s "$BASE_URL/health" > /dev/null 2>&1; then
    break
  fi
  sleep 0.1
done

if ! curl -s "$BASE_URL/health" > /dev/null 2>&1; then
  echo "FAIL: server did not start"
  exit 1
fi
echo "Server ready."
echo

# --- 1. Resource endpoints (GET, read-only) ---

echo "=== Resource endpoints ==="
check_json_field "actor_list"      '.[0].name'    "$BASE_URL/actors"
check_json_field "actor_get"       '.name'        "$BASE_URL/actors/brice"
check_json_field "board_list"      '.[0].slug'    "$BASE_URL/boards"
check_json_field "board_get"       '.slug'        "$BASE_URL/boards/platform"
check_json_field "workflow_get"    '.states'      "$BASE_URL/boards/platform/workflow"
check_json_field "task_list"       '.[0].title'   "$BASE_URL/boards/platform/tasks"
check_json_field "task_get"        '.title'       "$BASE_URL/boards/platform/tasks/1"
check_json       "tag_list"                       "$BASE_URL/boards/platform/tags"
check_json       "comment_list"                   "$BASE_URL/boards/platform/tasks/1/comments"
check_json       "dependency_list"                "$BASE_URL/boards/platform/tasks/1/dependencies"
check_json       "attachment_list"                "$BASE_URL/boards/platform/tasks/1/attachments"
check_json       "webhook_list"                   "$BASE_URL/webhooks"
check_json       "delivery_list"                  "$BASE_URL/webhooks"
echo

# --- 2. Audit endpoints ---

echo "=== Audit endpoints ==="
check_json "board_audit" "$BASE_URL/boards/platform/audit"
check_json "task_audit"  "$BASE_URL/boards/platform/tasks/1/audit"
echo

# --- 3. Mutation endpoints ---

echo "=== Mutation endpoints ==="

# Create a board
check_status "board_create" "201" \
  -X POST -H "Content-Type: application/json" \
  -d '{"slug":"qa-tmp","name":"QA Temp Board"}' \
  "$BASE_URL/boards"

# Update the board
check_status "board_update" "200" \
  -X PATCH -H "Content-Type: application/json" \
  -d '{"name":"QA Temp Board Updated"}' \
  "$BASE_URL/boards/qa-tmp"

# Create a task on it
check_status "task_create" "201" \
  -X POST -H "Content-Type: application/json" \
  -d '{"title":"QA test task","priority":"medium"}' \
  "$BASE_URL/boards/platform/tasks"

# Get latest task number for transition test.
TASK_NUM=$(curl -s -H "$(auth)" "$BASE_URL/boards/platform/tasks?sort=created_at&order=desc" | jq '.[0].num')

# Update a task
check_status "task_update" "200" \
  -X PATCH -H "Content-Type: application/json" \
  -d '{"title":"QA test task updated"}' \
  "$BASE_URL/boards/platform/tasks/$TASK_NUM"

# Transition a task
check_status "task_transition" "200" \
  -X POST -H "Content-Type: application/json" \
  -d '{"transition":"start"}' \
  "$BASE_URL/boards/platform/tasks/$TASK_NUM/transition"

# Add a comment
check_status "comment_create" "201" \
  -X POST -H "Content-Type: application/json" \
  -d '{"body":"QA test comment"}' \
  "$BASE_URL/boards/platform/tasks/$TASK_NUM/comments"

# Add a dependency
check_status "dependency_create" "201" \
  -X POST -H "Content-Type: application/json" \
  -d "{\"depends_on_board\":\"platform\",\"depends_on_num\":1,\"dep_type\":\"relates_to\"}" \
  "$BASE_URL/boards/platform/tasks/$TASK_NUM/dependencies"

# Add an attachment
check_status "attachment_create" "201" \
  -X POST -H "Content-Type: application/json" \
  -d '{"ref_type":"url","reference":"https://example.com","label":"test ref"}' \
  "$BASE_URL/boards/platform/tasks/$TASK_NUM/attachments"

# Workflow health
check_json "workflow_health" \
  -X POST "$BASE_URL/boards/platform/workflow/health"

# Delete a task
check_status "task_delete" "204" \
  -X DELETE "$BASE_URL/boards/platform/tasks/$TASK_NUM"

# Delete the temp board
check_status "board_delete" "204" \
  -X DELETE "$BASE_URL/boards/qa-tmp"
echo

# --- 4. Convenience endpoints ---

echo "=== Convenience endpoints ==="
check_json_field "board_detail"  '.board.slug'      "$BASE_URL/boards/platform/detail"
check_json_field "board_overview" '.slug'            "$BASE_URL/boards/platform/overview"
check_json_field "admin_stats"   '.actors.total'    "$BASE_URL/admin/stats"
check_json       "global_tasks"                     "$BASE_URL/tasks"
check_json       "task_search"                      "$BASE_URL/tasks?q=pipeline"
echo

# --- 5. OpenAPI spec ---

echo "=== OpenAPI spec ==="
check_json_field "openapi_version"  '.openapi'                              "$BASE_URL/openapi.json"
check_json_field "operationId"      '.paths["/actors"].get.operationId'     "$BASE_URL/openapi.json"
echo

# --- 6. Dashboard ---

echo "=== Dashboard ==="
check_status "dashboard_index" "200" "$BASE_URL/dashboard"
check_status "dashboard_board" "200" "$BASE_URL/dashboard/board/platform"
echo

# --- 7. CLI commands ---

echo "=== CLI commands ==="
check_cli "cli board list"          board list
check_cli "cli board get"           board get platform
check_cli "cli board audit"         board audit platform
check_cli "cli task list"           task list platform
check_cli "cli task get"            task get platform 1
check_cli "cli task audit"          task audit platform 1
check_cli "cli actor list"          actor list
check_cli "cli actor get"           actor get brice
check_cli "cli workflow get"        workflow get platform
check_cli "cli webhook list"        webhook list
echo

# --- Summary ---

TOTAL=$((PASS + FAIL))
echo "=== Results: $PASS/$TOTAL passed ==="
if [ "$FAIL" -gt 0 ]; then
  echo "FAILED ($FAIL failures)"
  exit 1
else
  echo "ALL PASSED"
fi
