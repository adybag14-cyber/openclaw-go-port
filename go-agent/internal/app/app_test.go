package app

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestRunDoctorOutputsDiagnosticsJSON(t *testing.T) {
	var out bytes.Buffer
	err := Run(context.Background(), Options{
		ConfigPath: "missing.toml",
		Doctor:     true,
		Output:     &out,
	})
	if err != nil {
		t.Fatalf("doctor run failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("doctor output should be JSON: %v", err)
	}
	if payload["doctor"] == nil {
		t.Fatalf("doctor output should include doctor summary")
	}
	if payload["securityAudit"] == nil {
		t.Fatalf("doctor output should include securityAudit report")
	}
	doctor, ok := payload["doctor"].(map[string]any)
	if !ok {
		t.Fatalf("doctor output should include doctor object")
	}
	checks, ok := doctor["checks"].([]any)
	if !ok || len(checks) == 0 {
		t.Fatalf("doctor output should include diagnostic checks")
	}
}

func TestRunListMethodsOutputsMethodCatalog(t *testing.T) {
	var out bytes.Buffer
	err := Run(context.Background(), Options{
		ConfigPath:  "missing.toml",
		ListMethods: true,
		Output:      &out,
	})
	if err != nil {
		t.Fatalf("list-methods run failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("list-methods output should be JSON: %v", err)
	}
	methods, ok := payload["methods"].(map[string]any)
	if !ok {
		t.Fatalf("expected methods object in list-methods output")
	}
	if count, _ := methods["count"].(float64); int(count) < 100 {
		t.Fatalf("expected large method catalog count, got %v", methods["count"])
	}
}
