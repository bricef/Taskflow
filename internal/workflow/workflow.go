// Package workflow implements a state machine engine for task progression.
// Workflows are pure data — this package has no database dependency.
package workflow

import (
	"encoding/json"
	"fmt"
	"slices"
)

// Transition is an expanded, resolved transition between two states.
type Transition struct {
	Name string `json:"name"`
	From string `json:"from"`
	To   string `json:"to"`
}

// Workflow is a parsed, validated, and expanded state machine.
type Workflow struct {
	States         []string     `json:"states"`
	InitialState   string       `json:"initial_state"`
	TerminalStates []string     `json:"terminal_states"`
	Transitions    []Transition `json:"transitions"`
}

// HealthIssue describes a problem found during a workflow health check.
type HealthIssue struct {
	State   string `json:"state"`
	Problem string `json:"problem"` // "orphaned" (state not in workflow) or "stuck" (no outbound transitions)
	Count   int    `json:"count"`
}

// Parse parses a JSON workflow definition, validates it, and expands
// fromAll/toAll shorthands into concrete transitions.
//
// Validation happens in two stages:
//  1. Schema validation (structure, types, required fields) via JSON Schema
//  2. Semantic validation (cross-references, reachability) via custom checks
func Parse(data json.RawMessage) (*Workflow, error) {
	// Stage 1: Schema validation.
	if err := validateSchema(data); err != nil {
		return nil, err
	}

	var raw rawWorkflow
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid workflow JSON: %w", err)
	}

	// Stage 2: Semantic validation — cross-field references that JSON Schema can't express.
	stateSet := make(map[string]bool, len(raw.States))
	for _, s := range raw.States {
		stateSet[s] = true
	}

	if !stateSet[raw.InitialState] {
		return nil, fmt.Errorf("initial_state %q is not in states list", raw.InitialState)
	}

	terminalSet := make(map[string]bool, len(raw.TerminalStates))
	for _, s := range raw.TerminalStates {
		if !stateSet[s] {
			return nil, fmt.Errorf("terminal_state %q is not in states list", s)
		}
		terminalSet[s] = true
	}

	// Expand all transitions.
	var transitions []Transition
	nameSet := make(map[string]bool)
	edgeSet := make(map[[2]string]bool) // dedup expanded transitions

	addTransition := func(name, from, to string) error {
		if !stateSet[from] {
			return fmt.Errorf("transition %q references unknown state %q", name, from)
		}
		if !stateSet[to] {
			return fmt.Errorf("transition %q references unknown state %q", name, to)
		}
		edge := [2]string{from, to}
		if !edgeSet[edge] {
			transitions = append(transitions, Transition{Name: name, From: from, To: to})
			edgeSet[edge] = true
		}
		return nil
	}

	// Explicit transitions.
	for _, t := range raw.Transitions {
		if nameSet[t.Name] {
			return nil, fmt.Errorf("duplicate transition name: %q", t.Name)
		}
		nameSet[t.Name] = true
		if err := addTransition(t.Name, t.From, t.To); err != nil {
			return nil, err
		}
	}

	// fromAll: every non-terminal state → target.
	for _, fa := range raw.FromAll {
		if nameSet[fa.Name] {
			return nil, fmt.Errorf("duplicate transition name: %q", fa.Name)
		}
		nameSet[fa.Name] = true
		for _, s := range raw.States {
			if s == fa.To || terminalSet[s] {
				continue
			}
			if err := addTransition(fa.Name, s, fa.To); err != nil {
				return nil, err
			}
		}
	}

	// toAll: source → every other state.
	for _, ta := range raw.ToAll {
		if nameSet[ta.Name] {
			return nil, fmt.Errorf("duplicate transition name: %q", ta.Name)
		}
		nameSet[ta.Name] = true
		for _, s := range raw.States {
			if s == ta.From {
				continue
			}
			if err := addTransition(ta.Name, ta.From, s); err != nil {
				return nil, err
			}
		}
	}

	w := &Workflow{
		States:         raw.States,
		InitialState:   raw.InitialState,
		TerminalStates: raw.TerminalStates,
		Transitions:    transitions,
	}

	if err := w.checkReachability(); err != nil {
		return nil, err
	}

	return w, nil
}

// AvailableTransitions returns the transitions available from the given state.
func (w *Workflow) AvailableTransitions(currentState string) []Transition {
	var result []Transition
	for _, t := range w.Transitions {
		if t.From == currentState {
			result = append(result, t)
		}
	}
	return result
}

// ExecuteTransition attempts to apply the named transition from the current state.
// Returns the new state, or an error describing why the transition is not allowed.
func (w *Workflow) ExecuteTransition(currentState, transitionName string) (string, error) {
	if w.IsTerminal(currentState) {
		return "", fmt.Errorf("task is in terminal state %q; no transitions available", currentState)
	}

	available := w.AvailableTransitions(currentState)
	for _, t := range available {
		if t.Name == transitionName {
			return t.To, nil
		}
	}

	names := make([]string, len(available))
	for i, t := range available {
		names[i] = t.Name
	}
	return "", fmt.Errorf("no transition %q from state %q; available: %v", transitionName, currentState, names)
}

// IsTerminal returns true if the given state is a terminal state.
func (w *Workflow) IsTerminal(state string) bool {
	return slices.Contains(w.TerminalStates, state)
}

// HealthCheck compares the workflow definition against actual task states
// and reports orphaned states (tasks in states not in the workflow) and
// stuck states (tasks in non-terminal states with no outbound transitions).
func (w *Workflow) HealthCheck(taskStates []string) []HealthIssue {
	// Count tasks per state.
	counts := make(map[string]int)
	for _, s := range taskStates {
		counts[s]++
	}

	// Build set of states that have outbound transitions.
	hasOutbound := make(map[string]bool)
	for _, t := range w.Transitions {
		hasOutbound[t.From] = true
	}

	stateSet := make(map[string]bool, len(w.States))
	for _, s := range w.States {
		stateSet[s] = true
	}

	var issues []HealthIssue

	for state, count := range counts {
		if !stateSet[state] {
			issues = append(issues, HealthIssue{State: state, Problem: "orphaned", Count: count})
		} else if !w.IsTerminal(state) && !hasOutbound[state] {
			issues = append(issues, HealthIssue{State: state, Problem: "stuck", Count: count})
		}
	}

	return issues
}

// checkReachability verifies that at least one terminal state is reachable
// from the initial state via the transition graph.
func (w *Workflow) checkReachability() error {
	if len(w.TerminalStates) == 0 {
		return nil // no terminals to reach
	}

	reachable := make(map[string]bool)
	var visit func(string)
	visit = func(s string) {
		if reachable[s] {
			return
		}
		reachable[s] = true
		for _, t := range w.Transitions {
			if t.From == s {
				visit(t.To)
			}
		}
	}
	visit(w.InitialState)

	for _, ts := range w.TerminalStates {
		if reachable[ts] {
			return nil // at least one terminal is reachable
		}
	}
	return fmt.Errorf("no terminal state is reachable from initial state %q", w.InitialState)
}

// rawWorkflow matches the JSON schema before expansion.
type rawWorkflow struct {
	States         []string        `json:"states"`
	InitialState   string          `json:"initial_state"`
	TerminalStates []string        `json:"terminal_states"`
	Transitions    []rawTransition `json:"transitions"`
	FromAll        []rawFromAll    `json:"from_all"`
	ToAll          []rawToAll      `json:"to_all"`
}

type rawTransition struct {
	From string `json:"from"`
	To   string `json:"to"`
	Name string `json:"name"`
}

type rawFromAll struct {
	To   string `json:"to"`
	Name string `json:"name"`
}

type rawToAll struct {
	From string `json:"from"`
	Name string `json:"name"`
}
