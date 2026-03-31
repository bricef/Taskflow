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
			got := InferPathParams(tt.path)
			if len(got) != len(tt.expect) {
				t.Fatalf("InferPathParams(%q): got %d params, want %d", tt.path, len(got), len(tt.expect))
			}
			for i := range got {
				if got[i] != tt.expect[i] {
					t.Errorf("InferPathParams(%q)[%d]: got %+v, want %+v", tt.path, i, got[i], tt.expect[i])
				}
			}
		})
	}
}

func TestOperationsNoDuplicateNameOrPath(t *testing.T) {
	// Check operations.
	ops := Operations()
	seenName := map[string]bool{}
	seenPath := map[string]bool{}
	for _, op := range ops {
		if seenName[op.Name] {
			t.Errorf("duplicate operation name: %s", op.Name)
		}
		seenName[op.Name] = true

		key := string(op.Action) + " " + op.Path
		if seenPath[key] {
			t.Errorf("duplicate operation (action, path): %s", key)
		}
		seenPath[key] = true
	}

	// Check resources.
	resources := Resources()
	for _, res := range resources {
		if seenName[res.Name] {
			t.Errorf("duplicate name (resource collides with operation): %s", res.Name)
		}
		seenName[res.Name] = true
	}
}

func TestOperationsFieldsPopulated(t *testing.T) {
	for i, op := range Operations() {
		if op.Name == "" {
			t.Errorf("operation %d: Name is empty", i)
		}
		if op.Action == "" {
			t.Errorf("operation %d (%s): Action is empty", i, op.Name)
		}
		if op.Path == "" {
			t.Errorf("operation %d (%s): Path is empty", i, op.Name)
		}
		if op.Summary == "" {
			t.Errorf("operation %d (%s): Summary is empty", i, op.Name)
		}
		if op.MinRole == "" {
			t.Errorf("operation %d (%s): MinRole is empty", i, op.Name)
		}
	}
}

func TestResourcesFieldsPopulated(t *testing.T) {
	for i, res := range Resources() {
		if res.Name == "" {
			t.Errorf("resource %d: Name is empty", i)
		}
		if res.Path == "" {
			t.Errorf("resource %d (%s): Path is empty", i, res.Name)
		}
		if res.Summary == "" {
			t.Errorf("resource %d (%s): Summary is empty", i, res.Name)
		}
		if res.MinRole == "" {
			t.Errorf("resource %d (%s): MinRole is empty", i, res.Name)
		}
		if res.Output == nil {
			t.Errorf("resource %d (%s): Output is nil", i, res.Name)
		}
	}
}

func TestResourcesCount(t *testing.T) {
	if got := len(Resources()); got != 14 {
		t.Errorf("Resources() returned %d, want 14 — update this test if the change was intentional", got)
	}
}

func TestOperationsCount(t *testing.T) {
	if got := len(Operations()); got != 23 {
		t.Errorf("Operations() returned %d, want 23 — update this test if the change was intentional", got)
	}
}

func TestTotalCount(t *testing.T) {
	total := len(Resources()) + len(Operations())
	if total != 37 {
		t.Errorf("Resources() + Operations() = %d, want 37 — the total should not change", total)
	}
}
