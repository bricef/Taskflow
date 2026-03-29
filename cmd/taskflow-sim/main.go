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
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"
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

	sim := &simulator{
		baseURL:   *url,
		boardSlug: *board,
		nextTitle: 0,
	}

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
	baseURL   string
	boardSlug string
	tasks     []taskInfo
	nextTitle int
}

// pickAction returns a weighted random action.
func (s *simulator) pickAction() string {
	if len(s.tasks) == 0 {
		return "create"
	}
	r := rand.Intn(100)
	switch {
	case r < 20:
		return "create"
	case r < 55:
		return "transition"
	case r < 75:
		return "assign"
	default:
		return "comment"
	}
}

func (s *simulator) refreshTasks() {
	var tasks []taskInfo
	err := s.doRequest("GET", fmt.Sprintf("/boards/%s/tasks", s.boardSlug), actors[0].apiKey, nil, &tasks)
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

	body := map[string]any{"title": title, "priority": priority}
	var result taskInfo
	err := s.doRequest("POST", fmt.Sprintf("/boards/%s/tasks", s.boardSlug), a.apiKey, body, &result)
	if err != nil {
		log.Printf("  [%s] create failed: %v", a.name, err)
		return
	}
	log.Printf("  [%s] created #%d %q", a.name, result.Num, title)
	s.tasks = append(s.tasks, result)
}

func (s *simulator) transitionTask(a actor) {
	// Find a non-terminal task.
	candidates := s.nonTerminalTasks()
	if len(candidates) == 0 {
		s.createTask(a)
		return
	}
	task := candidates[rand.Intn(len(candidates))]

	// Get available transitions.
	transition := s.pickTransition(task)
	if transition == "" {
		return
	}

	body := map[string]string{"transition": transition}
	err := s.doRequest("POST", fmt.Sprintf("/boards/%s/tasks/%d/transition", s.boardSlug, task.Num), a.apiKey, body, nil)
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

	body := map[string]any{"assignee": target.name}
	err := s.doRequest("PATCH", fmt.Sprintf("/boards/%s/tasks/%d", s.boardSlug, task.Num), a.apiKey, body, nil)
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

	body := map[string]string{"body": comment}
	err := s.doRequest("POST", fmt.Sprintf("/boards/%s/tasks/%d/comments", s.boardSlug, task.Num), a.apiKey, body, nil)
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

func (s *simulator) doRequest(method, path, apiKey string, body any, out any) error {
	var reqBody *bytes.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reqBody = bytes.NewReader(data)
	} else {
		reqBody = bytes.NewReader(nil)
	}

	req, err := http.NewRequest(method, s.baseURL+path, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errResp map[string]any
		json.NewDecoder(resp.Body).Decode(&errResp)
		if msg, ok := errResp["message"].(string); ok {
			return fmt.Errorf("%s", msg)
		}
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
