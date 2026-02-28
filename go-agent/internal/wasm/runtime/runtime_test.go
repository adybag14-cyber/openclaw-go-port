package runtime

import (
	"context"
	"testing"

	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/wasm/sandbox"
)

func TestMarketplaceListDeterministicAndNonEmpty(t *testing.T) {
	rt := NewRuntime()
	list := rt.MarketplaceList()
	if len(list) < 1 {
		t.Fatalf("expected wasm marketplace entries")
	}
	for i := 1; i < len(list); i++ {
		if list[i-1].ID > list[i].ID {
			t.Fatalf("marketplace list should be sorted by ID")
		}
	}
}

func TestExecuteAllowsSafeModuleWithinLimits(t *testing.T) {
	rt := NewRuntime()
	output, err := rt.Execute(context.Background(), "wasm.echo", map[string]any{
		"hello":     "world",
		"timeoutMs": 1000,
		"memoryMB":  64,
	})
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

func TestExecuteRejectsTimeoutAndMemoryOverPolicy(t *testing.T) {
	rt := NewRuntime()
	_, err := rt.Execute(context.Background(), "wasm.echo", map[string]any{
		"timeoutMs": 999999,
	})
	if err == nil {
		t.Fatalf("expected timeout policy denial")
	}

	_, err = rt.Execute(context.Background(), "wasm.echo", map[string]any{
		"memoryMB": 999999,
	})
	if err == nil {
		t.Fatalf("expected memory policy denial")
	}
}

func TestInstallAndRemoveModuleLifecycle(t *testing.T) {
	rt := NewRuntime()
	err := rt.InstallModule(Module{
		ID:           "wasm.custom.math",
		Name:         "Custom Math",
		Version:      "1.0.0",
		Description:  "math helper",
		Capabilities: []string{"compute"},
	})
	if err != nil {
		t.Fatalf("install module failed: %v", err)
	}

	_, err = rt.Execute(context.Background(), "wasm.custom.math", map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("execute installed module failed: %v", err)
	}

	removed := rt.RemoveModule("wasm.custom.math")
	if !removed {
		t.Fatalf("expected module removal to succeed")
	}
	_, err = rt.Execute(context.Background(), "wasm.custom.math", map[string]any{})
	if err == nil {
		t.Fatalf("expected removed module to be unavailable")
	}
}

func TestSetPolicyAllowsNetworkWhenEnabled(t *testing.T) {
	rt := NewRuntime()
	err := rt.InstallModule(Module{
		ID:           "wasm.net.echo",
		Name:         "Network Echo",
		Version:      "1.0.0",
		Capabilities: []string{"compute", "network"},
	})
	if err != nil {
		t.Fatalf("install network module failed: %v", err)
	}

	_, err = rt.Execute(context.Background(), "wasm.net.echo", map[string]any{})
	if err == nil {
		t.Fatalf("expected default policy to block network capability")
	}

	rt.SetPolicy(sandbox.Policy{
		MaxMemoryMB:     512,
		MaxDurationMS:   10000,
		AllowNetwork:    true,
		AllowFilesystem: false,
	})
	_, err = rt.Execute(context.Background(), "wasm.net.echo", map[string]any{})
	if err != nil {
		t.Fatalf("expected policy override to allow network capability: %v", err)
	}
}
