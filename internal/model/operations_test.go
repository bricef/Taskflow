package model

import (
	"testing"
)

func TestInferPathParams(t *testing.T) {
	tests := []struct {
		path   string
		expect []PathParam
	}{
		{
			path:   "/actors",
			expect: nil,
		},
		{
			path:   "/actors/{name}",
			expect: []PathParam{{Name: "name", Type: "string"}},
		},
		{
			path:   "/boards/{slug}",
			expect: []PathParam{{Name: "slug", Type: "string"}},
		},
		{
			path:   "/boards/{slug}/tasks/{num}",
			expect: []PathParam{{Name: "slug", Type: "string"}, {Name: "num", Type: "integer"}},
		},
		{
			path:   "/boards/{slug}/tasks/{num}/transition",
			expect: []PathParam{{Name: "slug", Type: "string"}, {Name: "num", Type: "integer"}},
		},
		{
			path:   "/webhooks/{id}",
			expect: []PathParam{{Name: "id", Type: "integer"}},
		},
		{
			path:   "/comments/{id}",
			expect: []PathParam{{Name: "id", Type: "integer"}},
		},
		{
			path:   "/dependencies/{id}",
			expect: []PathParam{{Name: "id", Type: "integer"}},
		},
		{
			path:   "/attachments/{id}",
			expect: []PathParam{{Name: "id", Type: "integer"}},
		},
		{
			path:   "/boards/{slug}/tasks/{num}/comments",
			expect: []PathParam{{Name: "slug", Type: "string"}, {Name: "num", Type: "integer"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := inferPathParams(tt.path)
			if len(got) != len(tt.expect) {
				t.Fatalf("inferPathParams(%q): got %d params, want %d", tt.path, len(got), len(tt.expect))
			}
			for i := range got {
				if got[i] != tt.expect[i] {
					t.Errorf("inferPathParams(%q)[%d]: got %+v, want %+v", tt.path, i, got[i], tt.expect[i])
				}
			}
		})
	}
}

func TestOperationsNoDuplicateActionPath(t *testing.T) {
	ops := Operations()
	seen := map[string]bool{}
	for _, op := range ops {
		key := string(op.Action) + " " + op.Path
		if seen[key] {
			t.Errorf("duplicate (action, path): %s", key)
		}
		seen[key] = true
	}
}

func TestOperationsFieldsPopulated(t *testing.T) {
	ops := Operations()
	if len(ops) == 0 {
		t.Fatal("Operations() returned no operations")
	}
	for i, op := range ops {
		if op.Action == "" {
			t.Errorf("operation %d: Action is empty", i)
		}
		if op.Path == "" {
			t.Errorf("operation %d: Path is empty", i)
		}
		if op.Summary == "" {
			t.Errorf("operation %d (%s %s): Summary is empty", i, op.Action, op.Path)
		}
		if op.MinRole == "" {
			t.Errorf("operation %d (%s %s): MinRole is empty", i, op.Action, op.Path)
		}
	}
}

func TestOperationsCount(t *testing.T) {
	ops := Operations()
	// Lock down the current count so additions/removals are intentional.
	if got := len(ops); got != 37 {
		t.Errorf("Operations() returned %d operations, want 37 — if this changed intentionally, update this test and the golden files", got)
	}
}
