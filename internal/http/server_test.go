package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bricef/taskflow/internal/eventbus"
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

// --- Idempotency ---

func TestIdempotencyKey(t *testing.T) {
	env := newTestEnv(t)

	env.request(t, "POST", "/boards", map[string]any{
		"slug": "my-board", "name": "My Board",
		"workflow": json.RawMessage(workflow.DefaultWorkflowJSON),
	}, env.memberKey)

	// First request with idempotency key.
	body := map[string]any{"title": "Idempotent Task", "priority": "none"}
	b, _ := json.Marshal(body)

	req1, _ := http.NewRequest("POST", env.server.URL+"/boards/my-board/tasks", bytes.NewReader(b))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Authorization", "Bearer "+env.memberKey)
	req1.Header.Set("Idempotency-Key", "test-key-123")
	resp1, err := http.DefaultClient.Do(req1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()
	if resp1.StatusCode != 201 {
		t.Fatalf("expected 201, got %d: %s", resp1.StatusCode, body1)
	}

	// Second request with same key — should get cached response, not create a second task.
	req2, _ := http.NewRequest("POST", env.server.URL+"/boards/my-board/tasks", bytes.NewReader(b))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+env.memberKey)
	req2.Header.Set("Idempotency-Key", "test-key-123")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if resp2.StatusCode != 201 {
		t.Fatalf("expected cached 201, got %d", resp2.StatusCode)
	}
	if string(body1) != string(body2) {
		t.Error("expected identical response bodies for idempotent request")
	}

	// Verify only one task was created.
	resp := env.request(t, "GET", "/boards/my-board/tasks", nil, env.memberKey)
	var tasks []model.Task
	env.decode(t, resp, &tasks)
	if len(tasks) != 1 {
		t.Errorf("expected 1 task (idempotent), got %d", len(tasks))
	}
}

func TestIdempotencyKeyNotUsedForGET(t *testing.T) {
	env := newTestEnv(t)

	// GET requests should not be cached even with an idempotency key.
	req, _ := http.NewRequest("GET", env.server.URL+"/actors", nil)
	req.Header.Set("Authorization", "Bearer "+env.adminKey)
	req.Header.Set("Idempotency-Key", "get-key")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()
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
	// Verify key endpoints exist (domain operations + convenience).
	for _, path := range []string{
		"/actors", "/boards", "/boards/{slug}/tasks", "/webhooks",
		"/boards/{slug}/detail", "/admin/stats", "/search", "/batch",
	} {
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

func TestCreateBoardWithDefaultWorkflow(t *testing.T) {
	env := newTestEnv(t)

	// Create board without a workflow — should use default.
	resp := env.request(t, "POST", "/boards", map[string]any{
		"slug": "default-wf", "name": "Default Workflow Board",
	}, env.memberKey)
	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
	}

	// Verify the board has a workflow with the default initial state.
	resp = env.request(t, "GET", "/boards/default-wf", nil, env.memberKey)
	var board model.Board
	env.decode(t, resp, &board)
	if len(board.Workflow) == 0 {
		t.Fatal("expected board to have a workflow")
	}

	// Create a task — should start in "backlog" (default workflow initial state).
	resp = env.request(t, "POST", "/boards/default-wf/tasks", map[string]any{
		"title": "Test Task", "priority": "none",
	}, env.memberKey)
	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
	}
	var task model.Task
	env.decode(t, resp, &task)
	if task.State != "backlog" {
		t.Errorf("expected state 'backlog' from default workflow, got %s", task.State)
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

	// Create.
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
	if !webhook.Active {
		t.Error("expected webhook to be active by default")
	}

	// List.
	resp = env.request(t, "GET", "/webhooks", nil, env.adminKey)
	var webhooks []model.Webhook
	env.decode(t, resp, &webhooks)
	if len(webhooks) != 1 {
		t.Errorf("expected 1 webhook, got %d", len(webhooks))
	}

	// Get.
	resp = env.request(t, "GET", fmt.Sprintf("/webhooks/%d", webhook.ID), nil, env.adminKey)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 on get, got %d", resp.StatusCode)
	}
	var got model.Webhook
	env.decode(t, resp, &got)
	if got.URL != webhook.URL {
		t.Errorf("expected URL %s, got %s", webhook.URL, got.URL)
	}

	// Update.
	resp = env.request(t, "PATCH", fmt.Sprintf("/webhooks/%d", webhook.ID), map[string]any{
		"url": "https://example.com/updated", "active": false,
	}, env.adminKey)
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 on update, got %d: %s", resp.StatusCode, body)
	}
	env.decode(t, resp, &got)
	if got.URL != "https://example.com/updated" {
		t.Errorf("expected updated URL, got %s", got.URL)
	}
	if got.Active {
		t.Error("expected webhook to be inactive after update")
	}

	// Delete.
	resp = env.request(t, "DELETE", fmt.Sprintf("/webhooks/%d", webhook.ID), nil, env.adminKey)
	if resp.StatusCode != 204 {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	// Get after delete — should 404.
	resp = env.request(t, "GET", fmt.Sprintf("/webhooks/%d", webhook.ID), nil, env.adminKey)
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404 after delete, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestWebhookValidation(t *testing.T) {
	env := newTestEnv(t)

	tests := []struct {
		name string
		body map[string]any
	}{
		{"missing url", map[string]any{"events": []string{"task.created"}, "secret": "s"}},
		{"missing events", map[string]any{"url": "https://x.com", "secret": "s"}},
		{"empty events", map[string]any{"url": "https://x.com", "events": []string{}, "secret": "s"}},
		{"missing secret", map[string]any{"url": "https://x.com", "events": []string{"task.created"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := env.request(t, "POST", "/webhooks", tt.body, env.adminKey)
			if resp.StatusCode != 400 {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("expected 400, got %d: %s", resp.StatusCode, body)
			}
			resp.Body.Close()
		})
	}
}

func TestWebhookRequiresAdmin(t *testing.T) {
	env := newTestEnv(t)

	// Member should not be able to create webhooks.
	resp := env.request(t, "POST", "/webhooks", map[string]any{
		"url": "https://example.com/hook", "events": []string{"task.created"}, "secret": "s",
	}, env.memberKey)
	if resp.StatusCode != 403 {
		t.Errorf("expected 403 for member creating webhook, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Member should not be able to list webhooks.
	resp = env.request(t, "GET", "/webhooks", nil, env.memberKey)
	if resp.StatusCode != 403 {
		t.Errorf("expected 403 for member listing webhooks, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestWebhookDeliveriesEndpoint(t *testing.T) {
	env := newTestEnv(t)

	// Create a webhook.
	resp := env.request(t, "POST", "/webhooks", map[string]any{
		"url": "https://example.com/hook", "events": []string{"task.created"}, "secret": "s",
	}, env.adminKey)
	var webhook model.Webhook
	env.decode(t, resp, &webhook)

	// List deliveries — should be empty initially.
	resp = env.request(t, "GET", fmt.Sprintf("/webhooks/%d/deliveries", webhook.ID), nil, env.adminKey)
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	var deliveries []model.WebhookDelivery
	env.decode(t, resp, &deliveries)
	if len(deliveries) != 0 {
		t.Errorf("expected 0 deliveries, got %d", len(deliveries))
	}

	// Deliveries for non-existent webhook — should 404.
	resp = env.request(t, "GET", "/webhooks/9999/deliveries", nil, env.adminKey)
	if resp.StatusCode != 404 {
		t.Errorf("expected 404 for non-existent webhook, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Member should not access deliveries.
	resp = env.request(t, "GET", fmt.Sprintf("/webhooks/%d/deliveries", webhook.ID), nil, env.memberKey)
	if resp.StatusCode != 403 {
		t.Errorf("expected 403 for member, got %d", resp.StatusCode)
	}
	resp.Body.Close()
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

// --- Batch ---

func TestBatch(t *testing.T) {
	env := newTestEnv(t)

	env.request(t, "POST", "/boards", map[string]any{
		"slug": "my-board", "name": "My Board",
		"workflow": json.RawMessage(workflow.DefaultWorkflowJSON),
	}, env.memberKey)

	resp := env.request(t, "POST", "/batch", map[string]any{
		"operations": []map[string]any{
			{"method": "POST", "path": "/boards/my-board/tasks", "body": map[string]any{"title": "Task 1", "priority": "high"}},
			{"method": "POST", "path": "/boards/my-board/tasks", "body": map[string]any{"title": "Task 2", "priority": "none"}},
			{"method": "GET", "path": "/boards/my-board/tasks"},
		},
	}, env.memberKey)
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var batch map[string]any
	env.decode(t, resp, &batch)
	results := batch["results"].([]any)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// First two should be 201 (create).
	r0 := results[0].(map[string]any)
	if r0["status"].(float64) != 201 {
		t.Errorf("expected result 0 status 201, got %v", r0["status"])
	}
	r1 := results[1].(map[string]any)
	if r1["status"].(float64) != 201 {
		t.Errorf("expected result 1 status 201, got %v", r1["status"])
	}

	// Third should be 200 (list) with 2 tasks.
	r2 := results[2].(map[string]any)
	if r2["status"].(float64) != 200 {
		t.Errorf("expected result 2 status 200, got %v", r2["status"])
	}
	tasks := r2["body"].([]any)
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks in list result, got %d", len(tasks))
	}
}

func TestBatchPartialFailure(t *testing.T) {
	env := newTestEnv(t)

	env.request(t, "POST", "/boards", map[string]any{
		"slug": "my-board", "name": "My Board",
		"workflow": json.RawMessage(workflow.DefaultWorkflowJSON),
	}, env.memberKey)

	resp := env.request(t, "POST", "/batch", map[string]any{
		"operations": []map[string]any{
			{"method": "POST", "path": "/boards/my-board/tasks", "body": map[string]any{"title": "Good task", "priority": "none"}},
			{"method": "GET", "path": "/boards/nonexistent"},
			{"method": "POST", "path": "/boards/my-board/tasks", "body": map[string]any{"title": "Also good", "priority": "none"}},
		},
	}, env.memberKey)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var batch map[string]any
	env.decode(t, resp, &batch)
	results := batch["results"].([]any)

	r0 := results[0].(map[string]any)
	if r0["status"].(float64) != 201 {
		t.Errorf("expected result 0 status 201, got %v", r0["status"])
	}
	r1 := results[1].(map[string]any)
	if r1["status"].(float64) != 404 {
		t.Errorf("expected result 1 status 404, got %v", r1["status"])
	}
	r2 := results[2].(map[string]any)
	if r2["status"].(float64) != 201 {
		t.Errorf("expected result 2 status 201 (continued after failure), got %v", r2["status"])
	}
}

func TestBatchTooManyOperations(t *testing.T) {
	env := newTestEnv(t)

	ops := make([]map[string]any, 51)
	for i := range ops {
		ops[i] = map[string]any{"method": "GET", "path": "/actors"}
	}
	resp := env.request(t, "POST", "/batch", map[string]any{"operations": ops}, env.memberKey)
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// --- SSE ---

func TestSSEReceivesEvents(t *testing.T) {
	// Create a test env with an event bus.
	store, err := sqlite.New(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	bus := eventbus.New()
	svc := service.New(store, service.WithEventBus(bus))

	srv := taskflowhttp.NewServer(svc, taskflowhttp.ServerConfig{EventBus: bus})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	adminKey := "test-admin-key"
	svc.CreateActor(context.Background(), model.CreateActorParams{
		Name: "admin", DisplayName: "Admin", Type: model.ActorTypeHuman,
		Role: model.RoleAdmin, APIKeyHash: taskflowhttp.HashAPIKey(adminKey),
	})
	svc.CreateBoard(context.Background(), model.CreateBoardParams{
		Slug: "my-board", Name: "My Board", Workflow: workflow.DefaultWorkflowJSON,
	})

	// Open SSE connection using ?token= auth.
	resp, err := http.Get(ts.URL + "/boards/my-board/events?token=" + adminKey)
	if err != nil {
		t.Fatalf("failed to connect SSE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %s", ct)
	}

	// Create a task — should trigger an event.
	reqBody, _ := json.Marshal(map[string]any{"title": "SSE Test", "priority": "none"})
	req, _ := http.NewRequest("POST", ts.URL+"/boards/my-board/tasks", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminKey)
	http.DefaultClient.Do(req)

	// Read the SSE event with a timeout.
	ch := make(chan string, 1)
	go func() {
		buf := make([]byte, 4096)
		n, _ := resp.Body.Read(buf)
		ch <- string(buf[:n])
	}()

	select {
	case output := <-ch:
		if !strings.Contains(output, "event: task.created") {
			t.Errorf("expected SSE event 'task.created', got: %s", output)
		}
		if !strings.Contains(output, "SSE Test") {
			t.Errorf("expected task title in event data, got: %s", output)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for SSE event")
	}
}

// --- Global Tasks ---

func TestGlobalTasks(t *testing.T) {
	env := newTestEnv(t)

	// Arrange — two boards with tasks.
	env.request(t, "POST", "/boards", map[string]any{
		"slug": "board-a", "name": "Board A",
		"workflow": json.RawMessage(workflow.DefaultWorkflowJSON),
	}, env.adminKey)
	env.request(t, "POST", "/boards", map[string]any{
		"slug": "board-b", "name": "Board B",
		"workflow": json.RawMessage(workflow.DefaultWorkflowJSON),
	}, env.adminKey)

	env.request(t, "POST", "/boards/board-a/tasks", map[string]any{
		"title": "Task A1", "assignee": "member",
	}, env.memberKey)
	env.request(t, "POST", "/boards/board-b/tasks", map[string]any{
		"title": "Task B1", "assignee": "member",
	}, env.memberKey)
	env.request(t, "POST", "/boards/board-a/tasks", map[string]any{
		"title": "Task A2", "assignee": "admin",
	}, env.adminKey)

	// Act — list all tasks.
	resp := env.request(t, "GET", "/tasks", nil, env.memberKey)
	var all []model.Task
	env.decode(t, resp, &all)
	if len(all) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(all))
	}

	// Act — filter by assignee=@me (member).
	resp = env.request(t, "GET", "/tasks?assignee=@me", nil, env.memberKey)
	var mine []model.Task
	env.decode(t, resp, &mine)
	if len(mine) != 2 {
		t.Fatalf("expected 2 tasks for @me, got %d", len(mine))
	}
	for _, task := range mine {
		if task.Assignee == nil || *task.Assignee != "member" {
			t.Errorf("expected assignee member, got %v", task.Assignee)
		}
	}
}

// --- Board Overview ---

func TestBoardOverview(t *testing.T) {
	env := newTestEnv(t)

	// Arrange
	env.request(t, "POST", "/boards", map[string]any{
		"slug": "overview-test", "name": "Overview Test",
		"workflow": json.RawMessage(workflow.DefaultWorkflowJSON),
	}, env.adminKey)
	env.request(t, "POST", "/boards/overview-test/tasks", map[string]any{"title": "T1"}, env.memberKey)
	env.request(t, "POST", "/boards/overview-test/tasks", map[string]any{"title": "T2"}, env.memberKey)
	env.request(t, "POST", "/boards/overview-test/tasks/1/transition", map[string]any{"transition": "start"}, env.memberKey)

	// Act
	resp := env.request(t, "GET", "/boards/overview-test/overview", nil, env.memberKey)
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	var overview struct {
		Slug       string         `json:"slug"`
		Name       string         `json:"name"`
		TaskCounts map[string]int `json:"task_counts"`
		TotalTasks int            `json:"total_tasks"`
	}
	env.decode(t, resp, &overview)

	// Assert
	if overview.Slug != "overview-test" {
		t.Errorf("expected slug overview-test, got %s", overview.Slug)
	}
	if overview.TotalTasks != 2 {
		t.Errorf("expected 2 total tasks, got %d", overview.TotalTasks)
	}
	if overview.TaskCounts["backlog"] != 1 {
		t.Errorf("expected 1 task in backlog, got %d", overview.TaskCounts["backlog"])
	}
	if overview.TaskCounts["in_progress"] != 1 {
		t.Errorf("expected 1 task in in_progress, got %d", overview.TaskCounts["in_progress"])
	}
}

func TestBoardOverviewNotFound(t *testing.T) {
	env := newTestEnv(t)
	resp := env.request(t, "GET", "/boards/nonexistent/overview", nil, env.adminKey)
	if resp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}
