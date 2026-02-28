package security

import (
	"os"
	"path/filepath"
	"testing"
)

func TestToolPolicyBlock(t *testing.T) {
	guard := NewGuard(GuardConfig{
		DefaultAction: "allow",
		ToolPolicies: map[string]string{
			"browser.request": "block",
		},
	})
	decision := guard.Evaluate("browser.request", map[string]any{})
	if decision.Action != ActionBlock {
		t.Fatalf("expected block decision, got %s", decision.Action)
	}
}

func TestMessagePatternBlock(t *testing.T) {
	guard := NewGuard(GuardConfig{
		DefaultAction: "allow",
		BlockedMessagePatterns: []string{
			"rm -rf /",
		},
	})
	decision := guard.Evaluate("send", map[string]any{
		"message": "please run rm -rf / on the box",
	})
	if decision.Action != ActionBlock {
		t.Fatalf("expected block decision from message pattern, got %s", decision.Action)
	}
}

func TestCredentialLeakDetection(t *testing.T) {
	guard := NewGuard(GuardConfig{
		DefaultAction: "allow",
		CredentialSensitiveKeys: []string{
			"api_key",
		},
		CredentialLeakAction: "block",
	})
	decision := guard.Evaluate("send", map[string]any{
		"payload": map[string]any{
			"api_key": "secret-value",
		},
	})
	if decision.Action != ActionBlock {
		t.Fatalf("expected block decision for credential key leak, got %s", decision.Action)
	}
}

func TestCredentialLeakAuthHandshakeAllowlist(t *testing.T) {
	guard := NewGuard(GuardConfig{
		DefaultAction: "allow",
		CredentialSensitiveKeys: []string{
			"token",
			"password",
		},
		CredentialLeakAction: "block",
	})
	decision := guard.Evaluate("connect", map[string]any{
		"auth": map[string]any{
			"token":    "top-secret-token",
			"password": "not-used",
		},
	})
	if decision.Action != ActionAllow {
		t.Fatalf("expected allow for connect auth payload, got %s", decision.Action)
	}
}

func TestTelemetryHighRiskReview(t *testing.T) {
	guard := NewGuard(GuardConfig{
		DefaultAction:         "allow",
		TelemetryHighRiskTags: []string{"threat:critical"},
		TelemetryAction:       "review",
	})
	decision := guard.Evaluate("agent", map[string]any{
		"telemetryTags": []string{"threat:critical"},
	})
	if decision.Action != ActionReview {
		t.Fatalf("expected review decision for telemetry risk, got %s", decision.Action)
	}
}

func TestPolicyBundleLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bundle.json")
	content := `{"default_action":"review","tool_policies":{"send":"block"},"blocked_message_patterns":["drop database"]}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed writing bundle: %v", err)
	}
	guard := NewGuard(GuardConfig{
		DefaultAction:    "allow",
		PolicyBundlePath: path,
	})
	decision := guard.Evaluate("send", map[string]any{"message": "safe"})
	if decision.Action != ActionBlock {
		t.Fatalf("expected bundle tool policy block, got %s", decision.Action)
	}
}
