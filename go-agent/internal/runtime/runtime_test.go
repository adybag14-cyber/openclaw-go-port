package runtime

import (
	"testing"

	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/config"
)

func TestRuntimeSnapshotWithCoreProfile(t *testing.T) {
	rt := New(config.RuntimeConfig{
		AuditOnly: false,
		StatePath: "memory://runtime",
		Profile:   "core",
	})
	snapshot := rt.Snapshot()
	if snapshot["mode"] != "enforcing" {
		t.Fatalf("expected enforcing runtime mode, got %v", snapshot["mode"])
	}
	if snapshot["profile"] != "core" {
		t.Fatalf("expected core profile, got %v", snapshot["profile"])
	}
}

func TestRuntimeSnapshotWithEdgeProfileAndAuditOnly(t *testing.T) {
	rt := New(config.RuntimeConfig{
		AuditOnly: true,
		StatePath: "memory://runtime",
		Profile:   "edge",
	})
	snapshot := rt.Snapshot()
	if snapshot["mode"] != "audit-only" {
		t.Fatalf("expected audit-only runtime mode, got %v", snapshot["mode"])
	}
	if snapshot["profile"] != "edge" {
		t.Fatalf("expected edge profile, got %v", snapshot["profile"])
	}
}

func TestRuntimeProfileNormalization(t *testing.T) {
	rt := New(config.RuntimeConfig{
		Profile: "unknown",
	})
	if rt.Profile() != ProfileCore {
		t.Fatalf("expected unknown profile to normalize to core")
	}
}
