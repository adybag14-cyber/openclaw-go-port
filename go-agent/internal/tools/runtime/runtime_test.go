package runtime

import (
	"context"
	"testing"
)

func TestDefaultCatalogAndInvoke(t *testing.T) {
	rt := NewDefault()
	catalog := rt.Catalog()
	if len(catalog) < 3 {
		t.Fatalf("expected default catalog entries, got %d", len(catalog))
	}

	result, err := rt.Invoke(context.Background(), Request{
		Tool: "browser.request",
		Input: map[string]any{
			"url":    "https://example.com",
			"method": "post",
		},
	})
	if err != nil {
		t.Fatalf("invoke failed: %v", err)
	}
	if result.Provider != "builtin-bridge" {
		t.Fatalf("unexpected provider: %s", result.Provider)
	}

	output, ok := result.Output.(map[string]any)
	if !ok {
		t.Fatalf("unexpected output type: %T", result.Output)
	}
	if output["status"] != 200 {
		t.Fatalf("unexpected status output: %v", output["status"])
	}
}

func TestInvokeUnknownTool(t *testing.T) {
	rt := NewDefault()
	_, err := rt.Invoke(context.Background(), Request{
		Tool: "does.not.exist",
	})
	if err == nil {
		t.Fatalf("expected error for unknown tool")
	}
}
