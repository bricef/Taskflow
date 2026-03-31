package http

import (
	"encoding/json"
	"flag"
	"os"
	"testing"

	"github.com/bricef/taskflow/internal/model"
)

var updateGolden = flag.Bool("update", false, "update golden files")

func TestOpenAPISpecGolden(t *testing.T) {
	// Build routes the same way the server does: zip operations with handlers.
	// We use nil handlers here because generateOpenAPISpec only reads Operation
	// fields, never the handler.
	ops := model.Operations()
	routes := make([]Route, len(ops))
	for i, op := range ops {
		routes[i] = Route{Operation: op}
	}

	got := generateOpenAPISpec(routes)

	// Re-marshal for stable formatting.
	var parsed any
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("generated spec is not valid JSON: %v", err)
	}
	formatted, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		t.Fatalf("failed to re-marshal spec: %v", err)
	}
	formatted = append(formatted, '\n')

	goldenPath := "testdata/openapi.golden.json"

	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("failed to create testdata dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, formatted, 0o644); err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}
		t.Log("updated golden file")
		return
	}

	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("golden file not found — run 'go test -update' to create it: %v", err)
	}

	if string(formatted) != string(expected) {
		t.Errorf("OpenAPI spec does not match golden file.\n\nRun 'go test ./internal/http/ -update' to regenerate.\n\nTo see the diff:\n  go test ./internal/http/ -run TestOpenAPISpecGolden -update && git diff testdata/openapi.golden.json")
	}
}
