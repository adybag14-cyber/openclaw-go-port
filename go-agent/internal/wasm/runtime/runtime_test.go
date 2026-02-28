package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/wasm/sandbox"
)

const wasmConst42MainBase64 = "AGFzbQEAAAABBQFgAAF/AwIBAAcIAQRtYWluAAAKBgEEAEEqCw=="

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

func TestExecuteUsesWazeroWhenModuleBytesProvided(t *testing.T) {
	rt := NewRuntime()
	err := rt.InstallModule(Module{
		ID:           "wasm.custom.const42",
		Name:         "Const 42",
		Version:      "1.0.0",
		Capabilities: []string{"compute"},
		EntryPoint:   "main",
		WasmBase64:   wasmConst42MainBase64,
	})
	if err != nil {
		t.Fatalf("install module failed: %v", err)
	}

	result, err := rt.Execute(context.Background(), "wasm.custom.const42", map[string]any{})
	if err != nil {
		t.Fatalf("execute module failed: %v", err)
	}
	if result["engine"] != "wazero" {
		t.Fatalf("expected top-level engine wazero, got %v", result["engine"])
	}
	output, ok := result["output"].(map[string]any)
	if !ok {
		t.Fatalf("expected output object, got %T", result["output"])
	}
	if output["engine"] != "wazero" {
		t.Fatalf("expected output engine wazero, got %v", output["engine"])
	}
	switch value := output["result"].(type) {
	case uint64:
		if value != 42 {
			t.Fatalf("expected wasm result 42, got %d", value)
		}
	case int:
		if value != 42 {
			t.Fatalf("expected wasm result 42, got %d", value)
		}
	default:
		t.Fatalf("unexpected wasm result type/value: %T %v", output["result"], output["result"])
	}
}

func TestInstallRejectsInvalidWasmBase64(t *testing.T) {
	rt := NewRuntime()
	err := rt.InstallModule(Module{
		ID:           "wasm.invalid.encoding",
		Capabilities: []string{"compute"},
		WasmBase64:   "%%%not-base64%%%",
	})
	if err == nil {
		t.Fatalf("expected invalid wasmBase64 install error")
	}
}

func TestExecuteFailsWhenEntryPointMissingInCompiledModule(t *testing.T) {
	rt := NewRuntime()
	err := rt.InstallModule(Module{
		ID:           "wasm.const.missing-fn",
		Capabilities: []string{"compute"},
		EntryPoint:   "missing_fn",
		WasmBase64:   wasmConst42MainBase64,
	})
	if err != nil {
		t.Fatalf("install module failed: %v", err)
	}
	_, err = rt.Execute(context.Background(), "wasm.const.missing-fn", map[string]any{})
	if err == nil {
		t.Fatalf("expected missing exported function error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "exported function") {
		t.Fatalf("unexpected error for missing exported function: %v", err)
	}
}

func TestExecuteFailsWhenWasmArgsContainUnsupportedType(t *testing.T) {
	rt := NewRuntime()
	err := rt.InstallModule(Module{
		ID:           "wasm.const.bad-args",
		Capabilities: []string{"compute"},
		EntryPoint:   "main",
		WasmBase64:   wasmConst42MainBase64,
	})
	if err != nil {
		t.Fatalf("install module failed: %v", err)
	}
	_, err = rt.Execute(context.Background(), "wasm.const.bad-args", map[string]any{
		"args": []any{"bad"},
	})
	if err == nil {
		t.Fatalf("expected error for unsupported wasm arg type")
	}
}
