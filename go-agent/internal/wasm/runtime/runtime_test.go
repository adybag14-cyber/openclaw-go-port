package runtime

import (
	"context"
	"testing"
)

func TestMarketplaceList(t *testing.T) {
	rt := NewRuntime()
	list := rt.MarketplaceList()
	if len(list) < 1 {
		t.Fatalf("expected wasm marketplace entries")
	}
}

func TestExecuteAllowsSafeModule(t *testing.T) {
	rt := NewRuntime()
	output, err := rt.Execute(context.Background(), "wasm.echo", map[string]any{"hello": "world"})
	if err != nil {
		t.Fatalf("execute wasm.echo failed: %v", err)
	}
	if output["status"] != "completed" {
		t.Fatalf("unexpected execute status: %v", output["status"])
	}
}

func TestExecuteRejectsDeniedCapability(t *testing.T) {
	rt := NewRuntime()
	_, err := rt.Execute(context.Background(), "wasm.vector.search", map[string]any{})
	if err == nil {
		t.Fatalf("expected sandbox denial for filesystem capability")
	}
}
