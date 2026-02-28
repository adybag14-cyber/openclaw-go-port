package routines

import (
	"context"
	"testing"
)

func TestListAndRunRoutine(t *testing.T) {
	m := NewManager()
	list := m.List()
	if len(list) < 1 {
		t.Fatalf("expected at least one default routine")
	}

	result, err := m.Run(context.Background(), "edge-wasm-smoke", map[string]any{"target": "module-a"})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("expected completed status, got %s", result.Status)
	}
}

func TestRunUnknownRoutine(t *testing.T) {
	m := NewManager()
	_, err := m.Run(context.Background(), "unknown-routine", nil)
	if err == nil {
		t.Fatalf("expected error for unknown routine")
	}
}
