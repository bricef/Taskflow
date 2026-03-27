package model

// Workflow defines a state machine for task progression on a board.
// The workflow engine (Increment 2) will parse and validate this;
// for now it is stored and retrieved as a JSON blob on Board.Workflow.
type Workflow struct {
	States         []string             `json:"states"`
	InitialState   string               `json:"initial_state"`
	TerminalStates []string             `json:"terminal_states"`
	Transitions    []WorkflowTransition `json:"transitions"`
	FromAll        []WorkflowFromAll    `json:"from_all,omitempty"` // Transitions reachable from every non-terminal state.
	ToAll          []WorkflowToAll      `json:"to_all,omitempty"`   // Transitions from a specific state to every other state.
}

type WorkflowTransition struct {
	From string `json:"from"`
	To   string `json:"to"`
	Name string `json:"name"`
}

// WorkflowFromAll defines a transition that expands to: every non-terminal state → To.
type WorkflowFromAll struct {
	To   string `json:"to"`
	Name string `json:"name"`
}

// WorkflowToAll defines a transition that expands to: From → every other state.
type WorkflowToAll struct {
	From string `json:"from"`
	Name string `json:"name"`
}
