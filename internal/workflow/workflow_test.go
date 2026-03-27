package workflow

import (
	"encoding/json"
	"strings"
	"testing"
)

func validWorkflow() json.RawMessage {
	return json.RawMessage(`{
		"states": ["backlog", "in_progress", "review", "done", "cancelled"],
		"initial_state": "backlog",
		"terminal_states": ["done", "cancelled"],
		"transitions": [
			{"from": "backlog", "to": "in_progress", "name": "start"},
			{"from": "in_progress", "to": "review", "name": "submit"},
			{"from": "review", "to": "done", "name": "approve"},
			{"from": "review", "to": "in_progress", "name": "reject"}
		],
		"from_all": [
			{"to": "cancelled", "name": "cancel"}
		]
	}`)
}

func TestParseValid(t *testing.T) {
	w, err := Parse(validWorkflow())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(w.States) != 5 {
		t.Errorf("expected 5 states, got %d", len(w.States))
	}
	if w.InitialState != "backlog" {
		t.Errorf("expected initial_state backlog, got %s", w.InitialState)
	}
	if len(w.TerminalStates) != 2 {
		t.Errorf("expected 2 terminal states, got %d", len(w.TerminalStates))
	}
}

func TestParseFromAllExpansion(t *testing.T) {
	w, err := Parse(validWorkflow())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// fromAll → cancelled should expand to: backlog→cancelled, in_progress→cancelled, review→cancelled
	// (not done→cancelled or cancelled→cancelled since those are terminal/target)
	cancelTransitions := 0
	for _, tr := range w.Transitions {
		if tr.Name == "cancel" {
			cancelTransitions++
			if tr.To != "cancelled" {
				t.Errorf("cancel transition should go to cancelled, got %s", tr.To)
			}
		}
	}
	if cancelTransitions != 3 {
		t.Errorf("expected 3 cancel transitions (from non-terminal states), got %d", cancelTransitions)
	}
}

func TestParseToAllExpansion(t *testing.T) {
	data := json.RawMessage(`{
		"states": ["a", "b", "c", "d"],
		"initial_state": "a",
		"terminal_states": ["d"],
		"transitions": [],
		"to_all": [{"from": "a", "name": "escalate"}]
	}`)
	w, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// toAll from "a" should expand to: a→b, a→c, a→d
	escalateCount := 0
	for _, tr := range w.Transitions {
		if tr.Name == "escalate" {
			escalateCount++
			if tr.From != "a" {
				t.Errorf("escalate should come from a, got %s", tr.From)
			}
		}
	}
	if escalateCount != 3 {
		t.Errorf("expected 3 escalate transitions, got %d", escalateCount)
	}
}

func TestParseBothFromAllAndToAll(t *testing.T) {
	data := json.RawMessage(`{
		"states": ["a", "b", "c"],
		"initial_state": "a",
		"terminal_states": ["c"],
		"transitions": [{"from": "a", "to": "b", "name": "move"}],
		"from_all": [{"to": "c", "name": "finish"}],
		"to_all": [{"from": "a", "name": "spread"}]
	}`)
	w, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// move(a→b), finish(a→c), finish(b→c) = 3 unique edges (toAll duplicates are deduped)
	if len(w.Transitions) != 3 {
		t.Errorf("expected 3 transitions (deduped), got %d", len(w.Transitions))
	}
}

func TestParseNoInitialState(t *testing.T) {
	data := json.RawMessage(`{"states": ["a"], "initial_state": "", "terminal_states": []}`)
	_, err := Parse(data)
	if err == nil || !strings.Contains(err.Error(), "initial_state") {
		t.Fatalf("expected initial_state error, got %v", err)
	}
}

func TestParseInitialStateNotInStates(t *testing.T) {
	data := json.RawMessage(`{"states": ["a"], "initial_state": "b", "terminal_states": []}`)
	_, err := Parse(data)
	if err == nil || !strings.Contains(err.Error(), "not in states") {
		t.Fatalf("expected error about initial_state not in states, got %v", err)
	}
}

func TestParseTerminalStateNotInStates(t *testing.T) {
	data := json.RawMessage(`{"states": ["a"], "initial_state": "a", "terminal_states": ["z"]}`)
	_, err := Parse(data)
	if err == nil || !strings.Contains(err.Error(), "not in states") {
		t.Fatalf("expected error about terminal_state not in states, got %v", err)
	}
}

func TestParseTransitionUnknownState(t *testing.T) {
	data := json.RawMessage(`{
		"states": ["a", "b"],
		"initial_state": "a",
		"terminal_states": ["b"],
		"transitions": [{"from": "a", "to": "z", "name": "bad"}]
	}`)
	_, err := Parse(data)
	if err == nil || !strings.Contains(err.Error(), "unknown state") {
		t.Fatalf("expected unknown state error, got %v", err)
	}
}

func TestParseUnreachableTerminal(t *testing.T) {
	data := json.RawMessage(`{
		"states": ["a", "b", "c"],
		"initial_state": "a",
		"terminal_states": ["c"],
		"transitions": [{"from": "a", "to": "b", "name": "move"}]
	}`)
	_, err := Parse(data)
	if err == nil || !strings.Contains(err.Error(), "reachable") {
		t.Fatalf("expected reachability error, got %v", err)
	}
}

func TestParseDuplicateTransitionName(t *testing.T) {
	data := json.RawMessage(`{
		"states": ["a", "b", "c"],
		"initial_state": "a",
		"terminal_states": ["c"],
		"transitions": [
			{"from": "a", "to": "b", "name": "go"},
			{"from": "b", "to": "c", "name": "go"}
		]
	}`)
	_, err := Parse(data)
	if err == nil || !strings.Contains(err.Error(), "duplicate transition") {
		t.Fatalf("expected duplicate transition error, got %v", err)
	}
}

func TestParseDuplicateStates(t *testing.T) {
	data := json.RawMessage(`{
		"states": ["a", "a"],
		"initial_state": "a",
		"terminal_states": []
	}`)
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for duplicate states")
	}
}

func TestParseEmptyStates(t *testing.T) {
	data := json.RawMessage(`{"states": [], "initial_state": "", "terminal_states": []}`)
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for empty states")
	}
}

// --- Transition Enforcement ---

func TestExecuteValidTransition(t *testing.T) {
	w, _ := Parse(validWorkflow())
	newState, err := w.ExecuteTransition("backlog", "start")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if newState != "in_progress" {
		t.Errorf("expected in_progress, got %s", newState)
	}
}

func TestExecuteInvalidTransitionName(t *testing.T) {
	w, _ := Parse(validWorkflow())
	_, err := w.ExecuteTransition("backlog", "approve")
	if err == nil {
		t.Fatal("expected error for invalid transition")
	}
	if !strings.Contains(err.Error(), "available") {
		t.Errorf("error should list available transitions: %v", err)
	}
}

func TestExecuteWrongState(t *testing.T) {
	w, _ := Parse(validWorkflow())
	_, err := w.ExecuteTransition("review", "start")
	if err == nil {
		t.Fatal("expected error for wrong state")
	}
	if !strings.Contains(err.Error(), "review") {
		t.Errorf("error should mention current state: %v", err)
	}
}

func TestExecuteTerminalState(t *testing.T) {
	w, _ := Parse(validWorkflow())
	_, err := w.ExecuteTransition("done", "start")
	if err == nil {
		t.Fatal("expected error for terminal state")
	}
	if !strings.Contains(err.Error(), "terminal") {
		t.Errorf("error should mention terminal state: %v", err)
	}
}

// --- Workflow Health ---

func TestHealthCheckClean(t *testing.T) {
	w, _ := Parse(validWorkflow())
	issues := w.HealthCheck([]string{"backlog", "in_progress", "review"})
	if len(issues) != 0 {
		t.Errorf("expected no issues, got %v", issues)
	}
}

func TestHealthCheckOrphanedState(t *testing.T) {
	w, _ := Parse(validWorkflow())
	issues := w.HealthCheck([]string{"backlog", "old_state"})
	if len(issues) != 1 || issues[0].Problem != "orphaned" {
		t.Errorf("expected orphaned issue, got %v", issues)
	}
	if issues[0].State != "old_state" {
		t.Errorf("expected state old_state, got %s", issues[0].State)
	}
}

func TestHealthCheckStuckState(t *testing.T) {
	// Create a workflow where "stuck" has no outbound transitions.
	data := json.RawMessage(`{
		"states": ["start", "stuck", "end"],
		"initial_state": "start",
		"terminal_states": ["end"],
		"transitions": [
			{"from": "start", "to": "stuck", "name": "go"},
			{"from": "start", "to": "end", "name": "finish"}
		]
	}`)
	w, _ := Parse(data)
	issues := w.HealthCheck([]string{"stuck", "stuck"})
	if len(issues) != 1 || issues[0].Problem != "stuck" {
		t.Errorf("expected stuck issue, got %v", issues)
	}
	if issues[0].Count != 2 {
		t.Errorf("expected count 2, got %d", issues[0].Count)
	}
}

// --- Default Workflow ---

func TestDefaultWorkflow(t *testing.T) {
	w, err := Parse(DefaultWorkflowJSON)
	if err != nil {
		t.Fatalf("default workflow should be valid: %v", err)
	}

	if len(w.States) != 5 {
		t.Errorf("expected 5 states, got %d", len(w.States))
	}
	if w.InitialState != "backlog" {
		t.Errorf("expected initial_state backlog, got %s", w.InitialState)
	}
	if len(w.TerminalStates) != 2 {
		t.Errorf("expected 2 terminal states, got %d", len(w.TerminalStates))
	}

	// Happy path: backlog → in_progress → review → done
	state := "backlog"
	for _, name := range []string{"start", "submit", "approve"} {
		var err error
		state, err = w.ExecuteTransition(state, name)
		if err != nil {
			t.Fatalf("transition %s failed: %v", name, err)
		}
	}
	if state != "done" {
		t.Errorf("expected done after happy path, got %s", state)
	}
}

func TestDefaultWorkflowReject(t *testing.T) {
	w, _ := Parse(DefaultWorkflowJSON)
	state, _ := w.ExecuteTransition("backlog", "start")
	state, _ = w.ExecuteTransition(state, "submit")
	state, err := w.ExecuteTransition(state, "reject")
	if err != nil {
		t.Fatalf("reject should work from review: %v", err)
	}
	if state != "in_progress" {
		t.Errorf("expected in_progress after reject, got %s", state)
	}
}

func TestDefaultWorkflowCancel(t *testing.T) {
	w, _ := Parse(DefaultWorkflowJSON)
	// Cancel should work from any non-terminal state.
	for _, from := range []string{"backlog", "in_progress", "review"} {
		state, err := w.ExecuteTransition(from, "cancel")
		if err != nil {
			t.Errorf("cancel from %s should work: %v", from, err)
		}
		if state != "cancelled" {
			t.Errorf("expected cancelled, got %s", state)
		}
	}
}
