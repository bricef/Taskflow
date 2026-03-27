package workflow

import "encoding/json"

// DefaultWorkflowJSON is the built-in workflow template:
//
//	backlog → in_progress → review → done
//	              ↑            │
//	              └────────────┘ (reject)
//	         fromAll → cancelled
var DefaultWorkflowJSON = json.RawMessage(`{
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
