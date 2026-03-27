package workflow

import (
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// SchemaJSON is the JSON Schema for workflow definitions.
// It validates structure and types; semantic rules (reachability,
// cross-references between states and transitions) are checked
// separately in Parse().
const SchemaJSON = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "TaskFlow Workflow",
  "description": "A state machine definition for task progression on a board.",
  "type": "object",
  "required": ["states", "initial_state", "terminal_states"],
  "additionalProperties": false,
  "properties": {
    "states": {
      "description": "All valid states in this workflow.",
      "type": "array",
      "items": { "type": "string", "minLength": 1 },
      "minItems": 1,
      "uniqueItems": true
    },
    "initial_state": {
      "description": "The state assigned to newly created tasks. Must be one of 'states'.",
      "type": "string",
      "minLength": 1
    },
    "terminal_states": {
      "description": "States from which no further transitions are possible (e.g., done, cancelled).",
      "type": "array",
      "items": { "type": "string", "minLength": 1 },
      "uniqueItems": true
    },
    "transitions": {
      "description": "Explicit transitions between specific states.",
      "type": "array",
      "items": {
        "type": "object",
        "required": ["from", "to", "name"],
        "additionalProperties": false,
        "properties": {
          "from": { "type": "string", "minLength": 1, "description": "Source state." },
          "to":   { "type": "string", "minLength": 1, "description": "Target state." },
          "name": { "type": "string", "minLength": 1, "description": "Transition name (used in API calls)." }
        }
      },
      "default": []
    },
    "from_all": {
      "description": "Shorthand: expands to a transition from every non-terminal state to the target.",
      "type": "array",
      "items": {
        "type": "object",
        "required": ["to", "name"],
        "additionalProperties": false,
        "properties": {
          "to":   { "type": "string", "minLength": 1, "description": "Target state." },
          "name": { "type": "string", "minLength": 1, "description": "Transition name." }
        }
      },
      "default": []
    },
    "to_all": {
      "description": "Shorthand: expands to a transition from the source to every other state.",
      "type": "array",
      "items": {
        "type": "object",
        "required": ["from", "name"],
        "additionalProperties": false,
        "properties": {
          "from": { "type": "string", "minLength": 1, "description": "Source state." },
          "name": { "type": "string", "minLength": 1, "description": "Transition name." }
        }
      },
      "default": []
    }
  }
}`

var compiledSchema *jsonschema.Schema

func init() {
	var schemaDoc any
	if err := json.Unmarshal([]byte(SchemaJSON), &schemaDoc); err != nil {
		panic(fmt.Sprintf("workflow: invalid schema JSON: %v", err))
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("workflow.json", schemaDoc); err != nil {
		panic(fmt.Sprintf("workflow: invalid schema: %v", err))
	}
	var err error
	compiledSchema, err = c.Compile("workflow.json")
	if err != nil {
		panic(fmt.Sprintf("workflow: failed to compile schema: %v", err))
	}
}

// validateSchema validates the raw JSON against the workflow JSON Schema.
// Returns a structured error describing what's wrong, or nil if valid.
func validateSchema(data json.RawMessage) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	err := compiledSchema.Validate(v)
	if err != nil {
		return fmt.Errorf("workflow schema validation failed: %w", err)
	}
	return nil
}
