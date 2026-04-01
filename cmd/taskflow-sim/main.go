// taskflow-sim simulates realistic board activity via the HTTP API,
// useful for testing SSE live updates in the TUI.
//
// It connects as multiple actors and performs a mix of actions:
// creating tasks, transitioning them through the workflow, assigning,
// and commenting — at randomised intervals between 2–8 seconds.
//
// Usage:
//
//	taskflow-sim [--url URL] [--board SLUG]
//
// Requires a running server with the seed database.
package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/bricef/taskflow/internal/httpclient"
	"github.com/bricef/taskflow/internal/model"
)

type actor struct {
	name   string
	apiKey string
}

var actors = []actor{
	{"brice", "seed-admin-key-for-testing"},
	{"claude", "claude-key"},
	{"alice", "alice-key"},
}

var taskTitles = []string{
	"Add request tracing",
	"Fix pagination on task list",
	"Write migration guide",
	"Investigate memory leak in SSE handler",
	"Add retry logic for webhook delivery",
	"Update Go to 1.23",
	"Refactor SQL queries to use CTEs",
	"Add board archival feature",
	"Improve error messages in CLI",
	"Set up nightly benchmarks",
	"Add task search endpoint",
	"Document webhook payload format",
	"Fix race condition in event bus",
	"Add CSV export for boards",
	"Implement task labels",
}

var commentBodies = []string{
	"I'll pick this up today.",
	"Looks like this is blocked on the migration.",
	"Can someone review this when they have a moment?",
	"Done — pushed a fix, ready for review.",
	"Not sure about the approach here. Thoughts?",
	"This is more complex than expected. Splitting into subtasks.",
	"Tested locally, works well. Merging.",
	"Found an edge case we need to handle.",
	"Updated the docs to reflect this change.",
	"Closing as duplicate of #3.",
	"Good catch. I'll add a test for this.",
	"Let's revisit this after the release.",
}

var priorities = []string{"critical", "high", "medium", "low", ""}

func main() {
	url := flag.String("url", envOr("TASKFLOW_URL", "http://localhost:8374"), "server URL")
	board := flag.String("board", "platform", "board slug to simulate activity on")
	flag.Parse()

	log.Printf("Simulating activity on %s (board: %s)", *url, *board)
	log.Printf("Press Ctrl+C to stop")

	clients := map[string]*httpclient.Client{}
	for _, a := range actors {
		clients[a.name] = httpclient.New(*url, a.apiKey)
	}

	sim := &simulator{
		boardSlug: *board,
		nextTitle: 0,
		clients:   clients,
	}

	// Ensure the board exists, create if not.
	sim.ensureBoard()

	// Discover current tasks so we can act on them.
	sim.refreshTasks()

	for {
		delay := time.Duration(2+rand.Intn(7)) * time.Second
		time.Sleep(delay)

		action := sim.pickAction()
		a := actors[rand.Intn(len(actors))]

		switch action {
		case "create":
			sim.createTask(a)
		case "transition":
			sim.transitionTask(a)
		case "assign":
			sim.assignTask(a)
		case "comment":
			sim.commentOnTask(a)
		}
	}
}

type taskInfo struct {
	Num   int    `json:"num"`
	State string `json:"state"`
	Title string `json:"title"`
}

type simulator struct {
	boardSlug string
	tasks     []taskInfo
	nextTitle int
	clients   map[string]*httpclient.Client // keyed by actor name
}

func (s *simulator) client(a actor) *httpclient.Client {
	return s.clients[a.name]
}

func (s *simulator) ensureBoard() {
	c := s.client(actors[0])
	p := httpclient.PathParams{"slug": s.boardSlug}
	_, err := httpclient.GetOne[model.Board](c, model.ResBoardGet, p, nil)
	if err == nil {
		log.Printf("Board %q exists", s.boardSlug)
		return
	}
	log.Printf("Board %q not found, creating...", s.boardSlug)
	_, err = httpclient.Exec[model.Board](c, model.OpBoardCreate, nil, map[string]string{"slug": s.boardSlug, "name": s.boardSlug})
	if err != nil {
		log.Fatalf("Failed to create board: %v", err)
	}
	log.Printf("Created board %q", s.boardSlug)
}

// pickAction returns a weighted random action.
const wipLimitPerState = 20

// pickAction returns a weighted random action.
// Skips "create" if any non-terminal state has 20+ tasks.
func (s *simulator) pickAction() string {
	if len(s.tasks) == 0 {
		return "create"
	}
	r := rand.Intn(100)
	switch {
	case r < 20:
		if s.anyStateOverWIP() {
			return "transition"
		}
		return "create"
	case r < 55:
		return "transition"
	case r < 75:
		return "assign"
	default:
		return "comment"
	}
}

func (s *simulator) anyStateOverWIP() bool {
	counts := map[string]int{}
	terminal := map[string]bool{"done": true, "cancelled": true, "shipped": true, "wontfix": true}
	for _, t := range s.tasks {
		if !terminal[t.State] {
			counts[t.State]++
		}
	}
	for _, c := range counts {
		if c >= wipLimitPerState {
			return true
		}
	}
	return false
}

func (s *simulator) boardParams() httpclient.PathParams {
	return httpclient.PathParams{"slug": s.boardSlug}
}

func (s *simulator) taskParams(num int) httpclient.PathParams {
	return httpclient.PathParams{"slug": s.boardSlug, "num": fmt.Sprint(num)}
}

func (s *simulator) refreshTasks() {
	tasks, err := httpclient.GetMany[taskInfo](s.client(actors[0]), model.ResTaskList, s.boardParams(), nil)
	if err != nil {
		log.Printf("  warning: failed to refresh tasks: %v", err)
		return
	}
	s.tasks = tasks
}

func (s *simulator) createTask(a actor) {
	title := taskTitles[s.nextTitle%len(taskTitles)]
	s.nextTitle++
	priority := priorities[rand.Intn(len(priorities))]

	result, err := httpclient.Exec[taskInfo](s.client(a), model.OpTaskCreate, s.boardParams(), map[string]any{"title": title, "priority": priority})
	if err != nil {
		log.Printf("  [%s] create failed: %v", a.name, err)
		return
	}
	log.Printf("  [%s] created #%d %q", a.name, result.Num, title)
	s.tasks = append(s.tasks, result)
}

func (s *simulator) transitionTask(a actor) {
	candidates := s.nonTerminalTasks()
	if len(candidates) == 0 {
		s.createTask(a)
		return
	}
	task := candidates[rand.Intn(len(candidates))]

	transition := s.pickTransition(task)
	if transition == "" {
		return
	}

	err := httpclient.ExecNoResult(s.client(a), model.OpTaskTransition, s.taskParams(task.Num), map[string]string{"transition": transition})
	if err != nil {
		log.Printf("  [%s] transition #%d %s failed: %v", a.name, task.Num, transition, err)
		return
	}
	log.Printf("  [%s] transitioned #%d: %s -> %s", a.name, task.Num, task.State, transition)
	s.refreshTasks()
}

func (s *simulator) assignTask(a actor) {
	candidates := s.nonTerminalTasks()
	if len(candidates) == 0 {
		return
	}
	task := candidates[rand.Intn(len(candidates))]
	target := actors[rand.Intn(len(actors))]

	err := httpclient.ExecNoResult(s.client(a), model.OpTaskUpdate, s.taskParams(task.Num), map[string]any{"assignee": target.name})
	if err != nil {
		log.Printf("  [%s] assign #%d failed: %v", a.name, task.Num, err)
		return
	}
	log.Printf("  [%s] assigned #%d to %s", a.name, task.Num, target.name)
}

func (s *simulator) commentOnTask(a actor) {
	if len(s.tasks) == 0 {
		return
	}
	task := s.tasks[rand.Intn(len(s.tasks))]
	comment := commentBodies[rand.Intn(len(commentBodies))]

	err := httpclient.ExecNoResult(s.client(a), model.OpCommentCreate, s.taskParams(task.Num), map[string]string{"body": comment})
	if err != nil {
		log.Printf("  [%s] comment on #%d failed: %v", a.name, task.Num, err)
		return
	}
	log.Printf("  [%s] commented on #%d", a.name, task.Num)
}

func (s *simulator) nonTerminalTasks() []taskInfo {
	terminal := map[string]bool{"done": true, "cancelled": true, "shipped": true, "wontfix": true}
	var result []taskInfo
	for _, t := range s.tasks {
		if !terminal[t.State] {
			result = append(result, t)
		}
	}
	return result
}

func (s *simulator) pickTransition(task taskInfo) string {
	// Map state to likely transitions for the default workflow.
	transitions := map[string][]string{
		"backlog":     {"start"},
		"in_progress": {"submit", "cancel"},
		"review":      {"approve", "reject"},
		// Custom workflow states.
		"icebox":    {"prioritize"},
		"next":      {"design"},
		"designing": {"build"},
		"building":  {"test"},
		"testing":   {"ship", "reject"},
	}
	opts, ok := transitions[task.State]
	if !ok || len(opts) == 0 {
		return ""
	}
	return opts[rand.Intn(len(opts))]
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
