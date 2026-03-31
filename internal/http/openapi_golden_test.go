package http

import (
	"encoding/json"
	"flag"
	"os"
	"testing"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/transport"
)

var updateGolden = flag.Bool("update", false, "update golden files")

func TestOpenAPISpecGolden(t *testing.T) {
	// Build routes the same way the server does: resources + operations.
	// Handlers are nil because generateOpenAPISpec never calls them.
	var routes []Route

	for _, res := range model.Resources() {
		routes = append(routes, Route{
			Name:    res.Name,
			Path:    res.Path,
			Summary: res.Summary,
			MinRole: res.MinRole,
			Output:  res.Output,
			Params:  res.QueryParams(),
			Method:  "GET",
			Status:  200,
		})
	}

	for _, op := range model.Operations() {
		routes = append(routes, Route{
			Name:    op.Name,
			Path:    op.Path,
			Summary: op.Summary,
			MinRole: op.MinRole,
			Input:   op.Input,
			Output:  op.Output,
			Method:  transport.MethodForAction(op.Action),
			Status:  transport.StatusForAction(op.Action),
		})
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
