// taskflow-seed creates a test database populated with realistic content
// across two boards. Run it to regenerate the DB after schema changes.
//
// Usage: taskflow-seed [path]   (default: ./taskflow-test.db)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/bricef/taskflow/internal/eventbus"
	taskflowhttp "github.com/bricef/taskflow/internal/http"
	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/service"
	"github.com/bricef/taskflow/internal/sqlite"
	"github.com/bricef/taskflow/internal/workflow"
)

func main() {
	path := "./taskflow-test.db"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}

	os.Remove(path) // start fresh

	store, err := sqlite.New(path)
	if err != nil {
		log.Fatalf("Failed to create database: %v", err)
	}
	defer store.Close()

	bus := eventbus.New()
	svc := service.New(store, service.WithEventBus(bus))
	ctx := context.Background()

	// --- Actors ---
	adminKey := "seed-admin-key-for-testing"
	actors := []model.CreateActorParams{
		{Name: "brice", DisplayName: "Brice Fernandes", Type: model.ActorTypeHuman, Role: model.RoleAdmin, APIKeyHash: taskflowhttp.HashAPIKey(adminKey)},
		{Name: "claude", DisplayName: "Claude", Type: model.ActorTypeAIAgent, Role: model.RoleMember, APIKeyHash: taskflowhttp.HashAPIKey("claude-key")},
		{Name: "alice", DisplayName: "Alice Chen", Type: model.ActorTypeHuman, Role: model.RoleMember, APIKeyHash: taskflowhttp.HashAPIKey("alice-key")},
		{Name: "reviewer", DisplayName: "Code Reviewer Bot", Type: model.ActorTypeAIAgent, Role: model.RoleReadOnly, APIKeyHash: taskflowhttp.HashAPIKey("reviewer-key")},
	}
	for _, a := range actors {
		must(svc.CreateActor(ctx, a))
		log.Printf("Created actor: %s", a.Name)
	}

	// --- Board 1: "platform" with default workflow ---
	must(svc.CreateBoard(ctx, model.CreateBoardParams{
		Slug: "platform", Name: "Platform Engineering", Workflow: workflow.DefaultWorkflowJSON,
	}))
	log.Println("Created board: platform (default workflow)")

	// Platform tasks
	platformTasks := []struct {
		title       string
		priority    model.Priority
		assignee    string
		tags        []string
		description string
	}{
		{"Set up CI/CD pipeline", model.PriorityCritical, "brice", []string{"infra", "ci"}, "Configure GitHub Actions for automated testing and deployment."},
		{"Add structured logging", model.PriorityHigh, "claude", []string{"observability"}, "Replace fmt.Printf with slog across all services."},
		{"Write API documentation", model.PriorityMedium, "claude", []string{"docs", "api"}, "Generate OpenAPI spec and write usage examples."},
		{"Configure rate limiting", model.PriorityMedium, "alice", []string{"infra", "security"}, "Add rate limiting middleware to protect against abuse."},
		{"Add health check endpoint", model.PriorityLow, "brice", []string{"infra"}, "Already done — verify it works in Docker."},
		{"Set up monitoring dashboards", model.PriorityHigh, "alice", []string{"observability", "infra"}, "Grafana dashboards for request latency, error rates, and DB performance."},
		{"Implement graceful shutdown", model.PriorityMedium, "", []string{"infra"}, "Handle SIGTERM properly, drain connections, close DB."},
		{"Add database migrations CLI", model.PriorityLow, "", []string{"tooling"}, "A command to run migrations manually for debugging."},
		{"Review authentication flow", model.PriorityHigh, "alice", []string{"security"}, "Audit the SHA-256 API key flow for potential weaknesses."},
		{"Benchmark FTS5 search", model.PriorityLow, "claude", []string{"performance"}, "Load test full-text search with 10K tasks."},
	}

	for _, t := range platformTasks {
		var assignee *string
		if t.assignee != "" {
			assignee = &t.assignee
		}
		must(svc.CreateTask(ctx, model.CreateTaskParams{
			BoardSlug: "platform", Title: t.title, Description: t.description,
			Priority: t.priority, Tags: t.tags, Assignee: assignee, CreatedBy: "brice",
		}))
	}
	log.Printf("Created %d tasks on platform", len(platformTasks))

	// Transition some platform tasks through the workflow.
	transitions := []struct {
		num        int
		transition string
		actor      string
		comment    string
	}{
		{1, "start", "brice", "Setting up GitHub Actions"},
		{1, "submit", "brice", "PR ready for review"},
		{1, "approve", "alice", "Looks good, merging"},
		{2, "start", "claude", "Working on structured logging"},
		{2, "submit", "claude", "Replaced all Printf calls with slog"},
		{3, "start", "claude", ""},
		{5, "start", "brice", ""},
		{5, "submit", "brice", "Health check verified in Docker"},
		{5, "approve", "alice", ""},
		{6, "start", "alice", "Setting up Grafana"},
		{9, "start", "alice", "Beginning security audit"},
	}
	for _, tr := range transitions {
		must(svc.TransitionTask(ctx, model.TransitionTaskParams{
			BoardSlug: "platform", Num: tr.num, TransitionName: tr.transition,
			Actor: tr.actor, Comment: tr.comment,
		}))
	}
	log.Println("Applied transitions on platform")

	// Comments on platform tasks.
	comments := []struct {
		num   int
		body  string
		actor string
	}{
		{2, "Should we use zerolog or slog?", "brice"},
		{2, "slog is in stdlib since Go 1.21 — fewer dependencies.", "claude"},
		{2, "Agreed, let's go with slog.", "brice"},
		{4, "What rate limit do we want? 100/min per key?", "alice"},
		{4, "Start with 60/min, we can adjust after monitoring.", "brice"},
		{6, "Which metrics should we track? Request count, latency p50/p95/p99?", "alice"},
		{6, "Yes, plus DB query time and active connections.", "brice"},
		{9, "Found a timing side-channel in the key comparison. Will fix.", "alice"},
	}
	for _, c := range comments {
		must(svc.CreateComment(ctx, model.CreateCommentParams{
			BoardSlug: "platform", TaskNum: c.num, Actor: c.actor, Body: c.body,
		}))
	}
	log.Printf("Added %d comments on platform", len(comments))

	// Dependencies.
	must(svc.CreateDependency(ctx, model.CreateDependencyParams{
		BoardSlug: "platform", TaskNum: 6, DependsOnBoard: "platform", DependsOnNum: 2,
		DependencyType: model.DependencyTypeDependsOn, CreatedBy: "alice",
	}))
	must(svc.CreateDependency(ctx, model.CreateDependencyParams{
		BoardSlug: "platform", TaskNum: 4, DependsOnBoard: "platform", DependsOnNum: 9,
		DependencyType: model.DependencyTypeRelatesTo, CreatedBy: "brice",
	}))
	log.Println("Added dependencies on platform")

	// Attachments.
	must(svc.CreateAttachment(ctx, model.CreateAttachmentParams{
		BoardSlug: "platform", TaskNum: 1, RefType: model.RefTypeGitPR, Reference: "https://github.com/bricef/taskflow/pull/1",
		Label: "CI/CD PR", CreatedBy: "brice",
	}))
	must(svc.CreateAttachment(ctx, model.CreateAttachmentParams{
		BoardSlug: "platform", TaskNum: 3, RefType: model.RefTypeURL, Reference: "https://swagger.io/specification/",
		Label: "OpenAPI Spec", CreatedBy: "claude",
	}))
	log.Println("Added attachments on platform")

	// --- Board 2: "product" with a custom workflow ---
	customWorkflow := json.RawMessage(`{
		"states": ["icebox", "next", "designing", "building", "testing", "shipped", "wontfix"],
		"initial_state": "icebox",
		"terminal_states": ["shipped", "wontfix"],
		"transitions": [
			{"from": "icebox", "to": "next", "name": "prioritize"},
			{"from": "next", "to": "designing", "name": "design"},
			{"from": "designing", "to": "building", "name": "build"},
			{"from": "building", "to": "testing", "name": "test"},
			{"from": "testing", "to": "shipped", "name": "ship"},
			{"from": "testing", "to": "building", "name": "reject"}
		],
		"from_all": [
			{"to": "wontfix", "name": "wontfix"}
		]
	}`)

	must(svc.CreateBoard(ctx, model.CreateBoardParams{
		Slug: "product", Name: "Product Roadmap", Workflow: customWorkflow,
	}))
	log.Println("Created board: product (custom workflow)")

	productTasks := []struct {
		title       string
		priority    model.Priority
		assignee    string
		tags        []string
		description string
	}{
		{"User onboarding flow", model.PriorityCritical, "alice", []string{"ux", "onboarding"}, "Design and implement a smooth first-run experience."},
		{"Dark mode support", model.PriorityMedium, "", []string{"ux", "theme"}, "Add a dark theme option to the TUI."},
		{"Board templates", model.PriorityHigh, "claude", []string{"feature"}, "Allow creating boards from predefined templates."},
		{"Task due date reminders", model.PriorityMedium, "claude", []string{"feature", "notifications"}, "Send a webhook notification when a task due date is approaching."},
		{"Keyboard shortcut help", model.PriorityLow, "", []string{"ux"}, "Show a ? overlay with all keyboard shortcuts."},
		{"Export board to Markdown", model.PriorityLow, "claude", []string{"feature", "export"}, "Generate a Markdown summary of all tasks on a board."},
		{"Mobile-friendly API responses", model.PriorityMedium, "alice", []string{"api"}, "Ensure all responses are usable by mobile clients."},
		{"Bulk task import from CSV", model.PriorityHigh, "", []string{"feature", "import"}, "Import tasks from a CSV file."},
	}

	for _, t := range productTasks {
		var assignee *string
		if t.assignee != "" {
			assignee = &t.assignee
		}
		must(svc.CreateTask(ctx, model.CreateTaskParams{
			BoardSlug: "product", Title: t.title, Description: t.description,
			Priority: t.priority, Tags: t.tags, Assignee: assignee, CreatedBy: "brice",
		}))
	}
	log.Printf("Created %d tasks on product", len(productTasks))

	// Transition some product tasks.
	productTransitions := []struct {
		num        int
		transition string
		actor      string
		comment    string
	}{
		{1, "prioritize", "brice", "Top priority for launch"},
		{1, "design", "alice", "Starting mockups"},
		{1, "build", "alice", "Design approved, implementing"},
		{3, "prioritize", "brice", ""},
		{3, "design", "claude", "Designing template schema"},
		{3, "build", "claude", "Schema done, building the feature"},
		{3, "test", "claude", "Ready for testing"},
		{4, "prioritize", "brice", ""},
		{2, "wontfix", "brice", "Deprioritized — not enough user demand"},
		{7, "prioritize", "brice", ""},
		{7, "design", "alice", ""},
	}
	for _, tr := range productTransitions {
		must(svc.TransitionTask(ctx, model.TransitionTaskParams{
			BoardSlug: "product", Num: tr.num, TransitionName: tr.transition,
			Actor: tr.actor, Comment: tr.comment,
		}))
	}
	log.Println("Applied transitions on product")

	// Delete one task.
	if err := svc.DeleteTask(ctx, "platform", 8, "brice"); err != nil {
		log.Printf("Warning: failed to delete task: %v", err)
	}
	log.Println("Soft-deleted platform task #8")

	fmt.Printf("\nSeed database created: %s\n", path)
	fmt.Printf("Admin API key: %s\n", adminKey)
	fmt.Println("\nTo use:")
	fmt.Printf("  TASKFLOW_DB_PATH=%s TASKFLOW_LISTEN_ADDR=:8374 taskflow-server\n", path)
	fmt.Printf("  TASKFLOW_API_KEY=%s taskflow-tui\n", adminKey)
}

func must[T any](v T, err error) T {
	if err != nil {
		log.Fatalf("Fatal: %v", err)
	}
	return v
}
