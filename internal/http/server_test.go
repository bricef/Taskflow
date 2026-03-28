package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/service"
	"github.com/bricef/taskflow/internal/sqlite"
	"github.com/bricef/taskflow/internal/workflow"

	taskflowhttp "github.com/bricef/taskflow/internal/http"
)

// testEnv sets up an in-memory store, service, HTTP server, and a seed admin actor.
type testEnv struct {
	server    *httptest.Server
	adminKey  string
	memberKey string
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	store, err := sqlite.New(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	svc := service.New(store)
	srv := taskflowhttp.NewServer(svc)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	// Create an admin actor with a known API key.
	adminKey := "test-admin-key"
	_, err = svc.CreateActor(context.Background(), model.CreateActorParams{
		Name:        "admin",
		DisplayName: "Admin",
		Type:        model.ActorTypeHuman,
		Role:        model.RoleAdmin,
		APIKeyHash:  taskflowhttp.HashAPIKey(adminKey),
	})
	if err != nil {
		t.Fatalf("failed to create admin: %v", err)
	}

	// Create a member actor.
	memberKey := "test-member-key"
	_, err = svc.CreateActor(context.Background(), model.CreateActorParams{
		Name:        "member",
		DisplayName: "Member",
		Type:        model.ActorTypeHuman,
		Role:        model.RoleMember,
		APIKeyHash:  taskflowhttp.HashAPIKey(memberKey),
	})
	if err != nil {
		t.Fatalf("failed to create member: %v", err)
	}

	return &testEnv{server: ts, adminKey: adminKey, memberKey: memberKey}
}

func (e *testEnv) request(t *testing.T, method, path string, body any, apiKey string) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, e.server.URL+path, bodyReader)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

func (e *testEnv) decode(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
}

// --- Health ---

func TestHealth(t *testing.T) {
	env := newTestEnv(t)
	resp := env.request(t, "GET", "/health", nil, "")
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// --- OpenAPI ---

func TestOpenAPISpec(t *testing.T) {
	env := newTestEnv(t)
	resp := env.request(t, "GET", "/openapi.json", nil, "")
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var spec map[string]any
	env.decode(t, resp, &spec)
	if spec["openapi"] != "3.1.0" {
		t.Errorf("expected openapi 3.1.0, got %v", spec["openapi"])
	}
	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatal("expected paths in spec")
	}
	// Verify key endpoints exist.
	for _, path := range []string{"/actors", "/boards", "/boards/{slug}/tasks", "/webhooks"} {
		if _, ok := paths[path]; !ok {
			t.Errorf("expected path %s in spec", path)
		}
	}
	// Verify component schemas exist.
	components := spec["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	for _, name := range []string{"Actor", "Board", "Task", "Comment"} {
		if _, ok := schemas[name]; !ok {
			t.Errorf("expected schema %s in components", name)
		}
	}
}

// --- Auth ---

func TestNoAuth(t *testing.T) {
	env := newTestEnv(t)
	resp := env.request(t, "GET", "/actors", nil, "")
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestInvalidKey(t *testing.T) {
	env := newTestEnv(t)
	resp := env.request(t, "GET", "/actors", nil, "bad-key")
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

// --- RBAC ---

func TestRBACForbidden(t *testing.T) {
	env := newTestEnv(t)
	// Member trying to create an actor (requires admin).
	resp := env.request(t, "POST", "/actors", map[string]any{
		"name": "new", "display_name": "New", "type": "human", "role": "member",
	}, env.memberKey)
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

// --- Actors ---

func TestListActors(t *testing.T) {
	env := newTestEnv(t)
	resp := env.request(t, "GET", "/actors", nil, env.adminKey)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var actors []model.Actor
	env.decode(t, resp, &actors)
	if len(actors) < 2 {
		t.Errorf("expected at least 2 actors, got %d", len(actors))
	}
}

func TestGetActor(t *testing.T) {
	env := newTestEnv(t)
	resp := env.request(t, "GET", "/actors/admin", nil, env.adminKey)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var actor model.Actor
	env.decode(t, resp, &actor)
	if actor.Name != "admin" {
		t.Errorf("expected name admin, got %s", actor.Name)
	}
}

// --- Boards ---

func TestCreateAndGetBoard(t *testing.T) {
	env := newTestEnv(t)

	resp := env.request(t, "POST", "/boards", map[string]any{
		"slug": "test-board", "name": "Test Board",
		"workflow": json.RawMessage(workflow.DefaultWorkflowJSON),
	}, env.memberKey)
	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
	}

	resp = env.request(t, "GET", "/boards/test-board", nil, env.memberKey)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var board model.Board
	env.decode(t, resp, &board)
	if board.Slug != "test-board" {
		t.Errorf("expected slug test-board, got %s", board.Slug)
	}
}

func TestListBoards(t *testing.T) {
	env := newTestEnv(t)

	env.request(t, "POST", "/boards", map[string]any{
		"slug": "b1", "name": "B1", "workflow": json.RawMessage(workflow.DefaultWorkflowJSON),
	}, env.memberKey)

	resp := env.request(t, "GET", "/boards", nil, env.memberKey)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var boards []model.Board
	env.decode(t, resp, &boards)
	if len(boards) != 1 {
		t.Errorf("expected 1 board, got %d", len(boards))
	}
}

// --- Tasks ---

func TestCreateAndGetTask(t *testing.T) {
	env := newTestEnv(t)

	env.request(t, "POST", "/boards", map[string]any{
		"slug": "my-board", "name": "My Board", "workflow": json.RawMessage(workflow.DefaultWorkflowJSON),
	}, env.memberKey)

	resp := env.request(t, "POST", "/boards/my-board/tasks", map[string]any{
		"title": "Fix bug", "priority": "high",
	}, env.memberKey)
	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
	}
	var task model.Task
	env.decode(t, resp, &task)
	if task.Title != "Fix bug" {
		t.Errorf("expected title 'Fix bug', got %s", task.Title)
	}
	if task.State != "backlog" {
		t.Errorf("expected state backlog (from workflow), got %s", task.State)
	}
	if task.CreatedBy != "member" {
		t.Errorf("expected created_by member, got %s", task.CreatedBy)
	}

	resp = env.request(t, "GET", fmt.Sprintf("/boards/my-board/tasks/%d", task.Num), nil, env.memberKey)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestTransitionTask(t *testing.T) {
	env := newTestEnv(t)

	env.request(t, "POST", "/boards", map[string]any{
		"slug": "my-board", "name": "My Board", "workflow": json.RawMessage(workflow.DefaultWorkflowJSON),
	}, env.memberKey)

	resp := env.request(t, "POST", "/boards/my-board/tasks", map[string]any{
		"title": "Task", "priority": "none",
	}, env.memberKey)
	var task model.Task
	env.decode(t, resp, &task)

	resp = env.request(t, "POST", fmt.Sprintf("/boards/my-board/tasks/%d/transition", task.Num), map[string]any{
		"transition": "start", "comment": "Starting work",
	}, env.memberKey)
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	var updated model.Task
	env.decode(t, resp, &updated)
	if updated.State != "in_progress" {
		t.Errorf("expected state in_progress, got %s", updated.State)
	}
}

// --- Error Responses ---

func TestNotFound(t *testing.T) {
	env := newTestEnv(t)
	resp := env.request(t, "GET", "/boards/nonexistent", nil, env.adminKey)
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	var errResp map[string]any
	env.decode(t, resp, &errResp)
	if errResp["error"] != "not_found" {
		t.Errorf("expected error not_found, got %v", errResp["error"])
	}
}

func TestValidationError(t *testing.T) {
	env := newTestEnv(t)
	resp := env.request(t, "POST", "/boards", map[string]any{
		"slug": "X", "name": "bad",
	}, env.memberKey)
	if resp.StatusCode != 400 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, body)
	}
}

func TestConflict(t *testing.T) {
	env := newTestEnv(t)

	env.request(t, "POST", "/boards", map[string]any{
		"slug": "dup", "name": "First", "workflow": json.RawMessage(workflow.DefaultWorkflowJSON),
	}, env.memberKey)

	resp := env.request(t, "POST", "/boards", map[string]any{
		"slug": "dup", "name": "Second", "workflow": json.RawMessage(workflow.DefaultWorkflowJSON),
	}, env.memberKey)
	if resp.StatusCode != 409 {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
}

// --- Delete ---

func TestDeleteBoard(t *testing.T) {
	env := newTestEnv(t)

	env.request(t, "POST", "/boards", map[string]any{
		"slug": "to-delete", "name": "Delete Me", "workflow": json.RawMessage(workflow.DefaultWorkflowJSON),
	}, env.memberKey)

	resp := env.request(t, "DELETE", "/boards/to-delete", nil, env.adminKey)
	if resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 204, got %d: %s", resp.StatusCode, body)
	}

	resp = env.request(t, "GET", "/boards", nil, env.adminKey)
	var boards []model.Board
	env.decode(t, resp, &boards)
	if len(boards) != 0 {
		t.Errorf("expected 0 boards after delete, got %d", len(boards))
	}
}

// --- Comments ---

func TestCreateAndListComments(t *testing.T) {
	env := newTestEnv(t)

	env.request(t, "POST", "/boards", map[string]any{
		"slug": "my-board", "name": "My Board", "workflow": json.RawMessage(workflow.DefaultWorkflowJSON),
	}, env.memberKey)

	resp := env.request(t, "POST", "/boards/my-board/tasks", map[string]any{
		"title": "Task", "priority": "none",
	}, env.memberKey)
	var task model.Task
	env.decode(t, resp, &task)

	resp = env.request(t, "POST", fmt.Sprintf("/boards/my-board/tasks/%d/comments", task.Num), map[string]any{
		"body": "Looks good",
	}, env.memberKey)
	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
	}

	resp = env.request(t, "GET", fmt.Sprintf("/boards/my-board/tasks/%d/comments", task.Num), nil, env.memberKey)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var comments []model.Comment
	env.decode(t, resp, &comments)
	if len(comments) != 1 || comments[0].Body != "Looks good" {
		t.Errorf("unexpected comments: %v", comments)
	}
}

// --- Webhooks ---

func TestWebhookCRUD(t *testing.T) {
	env := newTestEnv(t)

	resp := env.request(t, "POST", "/webhooks", map[string]any{
		"url": "https://example.com/hook", "events": []string{"task.created"}, "secret": "s3cret",
	}, env.adminKey)
	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
	}
	var webhook model.Webhook
	env.decode(t, resp, &webhook)
	if webhook.URL != "https://example.com/hook" {
		t.Errorf("expected URL, got %s", webhook.URL)
	}

	resp = env.request(t, "GET", "/webhooks", nil, env.adminKey)
	var webhooks []model.Webhook
	env.decode(t, resp, &webhooks)
	if len(webhooks) != 1 {
		t.Errorf("expected 1 webhook, got %d", len(webhooks))
	}

	resp = env.request(t, "DELETE", fmt.Sprintf("/webhooks/%d", webhook.ID), nil, env.adminKey)
	if resp.StatusCode != 204 {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
}

// --- Board Detail ---

func TestBoardDetail(t *testing.T) {
	env := newTestEnv(t)

	// Create board with a task and a comment.
	env.request(t, "POST", "/boards", map[string]any{
		"slug": "my-board", "name": "My Board",
		"workflow": json.RawMessage(workflow.DefaultWorkflowJSON),
	}, env.memberKey)

	resp := env.request(t, "POST", "/boards/my-board/tasks", map[string]any{
		"title": "Task 1", "priority": "high",
	}, env.memberKey)
	var task model.Task
	env.decode(t, resp, &task)

	env.request(t, "POST", fmt.Sprintf("/boards/my-board/tasks/%d/comments", task.Num), map[string]any{
		"body": "A comment",
	}, env.memberKey)

	// Fetch board detail.
	resp = env.request(t, "GET", "/boards/my-board/detail", nil, env.memberKey)
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var detail map[string]any
	env.decode(t, resp, &detail)

	// Board present.
	board := detail["board"].(map[string]any)
	if board["slug"] != "my-board" {
		t.Errorf("expected slug my-board, got %v", board["slug"])
	}

	// Workflow present.
	if detail["workflow"] == nil {
		t.Error("expected workflow in detail")
	}

	// Tasks with nested comments.
	tasks := detail["tasks"].([]any)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	taskObj := tasks[0].(map[string]any)
	comments := taskObj["comments"].([]any)
	if len(comments) != 1 {
		t.Errorf("expected 1 comment, got %d", len(comments))
	}
}

// --- System Stats ---

func TestSystemStats(t *testing.T) {
	env := newTestEnv(t)

	// Create a board and task.
	env.request(t, "POST", "/boards", map[string]any{
		"slug": "my-board", "name": "My Board",
		"workflow": json.RawMessage(workflow.DefaultWorkflowJSON),
	}, env.memberKey)
	env.request(t, "POST", "/boards/my-board/tasks", map[string]any{
		"title": "Task 1", "priority": "none",
	}, env.memberKey)

	resp := env.request(t, "GET", "/admin/stats", nil, env.adminKey)
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var stats map[string]any
	env.decode(t, resp, &stats)

	actors := stats["actors"].(map[string]any)
	if actors["total"].(float64) < 2 {
		t.Errorf("expected at least 2 actors, got %v", actors["total"])
	}

	boards := stats["boards"].(map[string]any)
	if boards["active"].(float64) != 1 {
		t.Errorf("expected 1 active board, got %v", boards["active"])
	}

	tasks := stats["tasks"].(map[string]any)
	if tasks["total"].(float64) != 1 {
		t.Errorf("expected 1 total task, got %v", tasks["total"])
	}

	activity := stats["activity"].(map[string]any)
	if activity["total_events"].(float64) < 1 {
		t.Errorf("expected at least 1 event, got %v", activity["total_events"])
	}
}

// --- Cross-board Search ---

func TestSearch(t *testing.T) {
	env := newTestEnv(t)

	env.request(t, "POST", "/boards", map[string]any{
		"slug": "board-a", "name": "A",
		"workflow": json.RawMessage(workflow.DefaultWorkflowJSON),
	}, env.memberKey)
	env.request(t, "POST", "/boards", map[string]any{
		"slug": "board-b", "name": "B",
		"workflow": json.RawMessage(workflow.DefaultWorkflowJSON),
	}, env.memberKey)

	env.request(t, "POST", "/boards/board-a/tasks", map[string]any{
		"title": "Fix authentication bug", "priority": "high",
	}, env.memberKey)
	env.request(t, "POST", "/boards/board-b/tasks", map[string]any{
		"title": "Update authentication docs", "priority": "none",
	}, env.memberKey)
	env.request(t, "POST", "/boards/board-b/tasks", map[string]any{
		"title": "Unrelated task", "priority": "none",
	}, env.memberKey)

	resp := env.request(t, "GET", "/search?q=authentication", nil, env.memberKey)
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	var results []model.Task
	env.decode(t, resp, &results)
	if len(results) != 2 {
		t.Fatalf("expected 2 results across boards, got %d", len(results))
	}
}

func TestSearchRequiresQuery(t *testing.T) {
	env := newTestEnv(t)
	resp := env.request(t, "GET", "/search", nil, env.memberKey)
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400 without q param, got %d", resp.StatusCode)
	}
}

// --- @me alias ---

func TestAtMeAlias(t *testing.T) {
	env := newTestEnv(t)

	env.request(t, "POST", "/boards", map[string]any{
		"slug": "my-board", "name": "My Board",
		"workflow": json.RawMessage(workflow.DefaultWorkflowJSON),
	}, env.memberKey)

	// Create a task assigned to @me.
	resp := env.request(t, "POST", "/boards/my-board/tasks", map[string]any{
		"title": "My task", "priority": "none", "assignee": "@me",
	}, env.memberKey)
	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
	}
	var task model.Task
	env.decode(t, resp, &task)
	if task.Assignee == nil || *task.Assignee != "member" {
		t.Errorf("expected assignee 'member', got %v", task.Assignee)
	}

	// Filter by @me.
	resp = env.request(t, "GET", "/boards/my-board/tasks?assignee=@me", nil, env.memberKey)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var tasks []model.Task
	env.decode(t, resp, &tasks)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task for @me, got %d", len(tasks))
	}
}

// --- Transition error context ---

func TestTransitionErrorContext(t *testing.T) {
	env := newTestEnv(t)

	env.request(t, "POST", "/boards", map[string]any{
		"slug": "my-board", "name": "My Board",
		"workflow": json.RawMessage(workflow.DefaultWorkflowJSON),
	}, env.memberKey)
	resp := env.request(t, "POST", "/boards/my-board/tasks", map[string]any{
		"title": "Task", "priority": "none",
	}, env.memberKey)
	var task model.Task
	env.decode(t, resp, &task)

	// Try an invalid transition.
	resp = env.request(t, "POST", fmt.Sprintf("/boards/my-board/tasks/%d/transition", task.Num), map[string]any{
		"transition": "approve",
	}, env.memberKey)
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	var errResp map[string]any
	env.decode(t, resp, &errResp)

	detail := errResp["detail"].(map[string]any)
	ctx := detail["context"].(map[string]any)
	available := ctx["available"].([]any)
	if len(available) == 0 {
		t.Error("expected available transitions in error context")
	}
	if ctx["current_state"] != "backlog" {
		t.Errorf("expected current_state backlog, got %v", ctx["current_state"])
	}
}
