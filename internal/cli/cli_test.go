package cli_test

import (
	"bytes"
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bricef/taskflow/internal/cli"
	taskflowhttp "github.com/bricef/taskflow/internal/http"
	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/service"
	"github.com/bricef/taskflow/internal/sqlite"
	"github.com/bricef/taskflow/internal/workflow"
)

type testEnv struct {
	cfg cli.Config
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

	// Seed an admin actor.
	adminKey := "test-admin-key"
	svc.CreateActor(context.Background(), model.CreateActorParams{
		Name: "admin", DisplayName: "Admin", Type: model.ActorTypeHuman,
		Role: model.RoleAdmin, APIKeyHash: taskflowhttp.HashAPIKey(adminKey),
	})

	// Seed a board with the default workflow.
	svc.CreateBoard(context.Background(), model.CreateBoardParams{
		Slug: "test-board", Name: "Test Board", Workflow: workflow.DefaultWorkflowJSON,
	})

	return &testEnv{cfg: cli.Config{ServerURL: ts.URL, APIKey: adminKey}}
}

// run executes a CLI command and returns stdout and the error.
func (e *testEnv) run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := cli.BuildCLI(&e.cfg)
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

// --- Actor commands ---

func TestCLIActorList(t *testing.T) {
	env := newTestEnv(t)
	out, err := env.run(t, "actor", "list")
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "admin") {
		t.Errorf("expected output to contain 'admin', got: %s", out)
	}
}

func TestCLIActorCreate(t *testing.T) {
	env := newTestEnv(t)
	out, err := env.run(t, "actor", "create",
		"--name", "bot",
		"--display_name", "Bot",
		"--type", "ai_agent",
		"--role", "member")
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "bot") {
		t.Errorf("expected output to contain 'bot', got: %s", out)
	}
}

func TestCLIActorGet(t *testing.T) {
	env := newTestEnv(t)
	out, err := env.run(t, "actor", "get", "admin")
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "admin") {
		t.Errorf("expected output to contain 'admin', got: %s", out)
	}
}

// --- Board commands ---

func TestCLIBoardList(t *testing.T) {
	env := newTestEnv(t)
	out, err := env.run(t, "board", "list")
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "test-board") {
		t.Errorf("expected output to contain 'test-board', got: %s", out)
	}
}

func TestCLIBoardGet(t *testing.T) {
	env := newTestEnv(t)
	out, err := env.run(t, "board", "get", "test-board")
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "test-board") {
		t.Errorf("expected output to contain 'test-board', got: %s", out)
	}
}

func TestCLIBoardCreate(t *testing.T) {
	env := newTestEnv(t)
	// Board creation requires a workflow JSON — use --json flag for raw input.
	// For now, test that the command exists and gives a useful error without required fields.
	_, err := env.run(t, "board", "create", "--name", "New Board", "--slug", "new-board")
	// This will fail because workflow is missing, but the command should exist and make the HTTP call.
	if err == nil {
		t.Fatal("expected error due to missing workflow, got success")
	}
	if !strings.Contains(err.Error(), "400") && !strings.Contains(err.Error(), "workflow") {
		t.Errorf("expected validation error about workflow, got: %v", err)
	}
}

// --- Task commands ---

func TestCLITaskCreate(t *testing.T) {
	env := newTestEnv(t)
	out, err := env.run(t, "task", "create", "test-board",
		"--title", "Fix bug",
		"--priority", "high")
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "Fix bug") {
		t.Errorf("expected output to contain 'Fix bug', got: %s", out)
	}
}

func TestCLITaskList(t *testing.T) {
	env := newTestEnv(t)
	// Create a task first.
	env.run(t, "task", "create", "test-board", "--title", "Task 1", "--priority", "none")

	out, err := env.run(t, "task", "list", "test-board")
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "Task 1") {
		t.Errorf("expected output to contain 'Task 1', got: %s", out)
	}
}

func TestCLITaskGet(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, "task", "create", "test-board", "--title", "My Task", "--priority", "none")

	out, err := env.run(t, "task", "get", "test-board", "1")
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "My Task") {
		t.Errorf("expected output to contain 'My Task', got: %s", out)
	}
}

func TestCLITaskTransition(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, "task", "create", "test-board", "--title", "To Start", "--priority", "none")

	out, err := env.run(t, "task", "transition", "test-board", "1",
		"--transition", "start",
		"--comment", "Starting now")
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "in_progress") {
		t.Errorf("expected output to contain 'in_progress', got: %s", out)
	}
}

func TestCLITaskDelete(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, "task", "create", "test-board", "--title", "To Delete", "--priority", "none")

	_, err := env.run(t, "task", "delete", "test-board", "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- JSON output ---

func TestCLIJSONOutput(t *testing.T) {
	env := newTestEnv(t)
	out, err := env.run(t, "actor", "get", "admin", "--json")
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, `"name"`) {
		t.Errorf("expected JSON output with 'name' field, got: %s", out)
	}
}

// --- Error handling ---

func TestCLINotFound(t *testing.T) {
	env := newTestEnv(t)
	_, err := env.run(t, "board", "get", "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent board")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected 404 error, got: %v", err)
	}
}

func TestCLINoArgs(t *testing.T) {
	env := newTestEnv(t)
	_, err := env.run(t, "task", "get")
	if err == nil {
		t.Fatal("expected error for missing arguments")
	}
	if !strings.Contains(err.Error(), "argument") {
		t.Errorf("expected argument error, got: %v", err)
	}
}

func TestCLIMissingAPIKey(t *testing.T) {
	root := cli.BuildCLI(&cli.Config{ServerURL: "http://localhost:8374"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs([]string{"actor", "list"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
	if !strings.Contains(err.Error(), "API key") {
		t.Errorf("expected helpful API key message, got: %v", err)
	}
}

func TestCLIServerNotReachable(t *testing.T) {
	root := cli.BuildCLI(&cli.Config{
		ServerURL: "http://localhost:19999",
		APIKey:    "some-key",
	})
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs([]string{"actor", "list"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
	if !strings.Contains(err.Error(), "could not connect") {
		t.Errorf("expected connection error with help text, got: %v", err)
	}
}
