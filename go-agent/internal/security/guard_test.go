package security

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/config"
)

func TestToolPolicyBlock(t *testing.T) {
	guard := NewGuard(config.SecurityConfig{
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
	guard := NewGuard(config.SecurityConfig{
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

func TestPolicyBundleLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bundle.json")
	content := `{"default_action":"review","tool_policies":{"send":"block"},"blocked_message_patterns":["drop database"]}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed writing bundle: %v", err)
	}
	guard := NewGuard(config.SecurityConfig{
		DefaultAction:    "allow",
		PolicyBundlePath: path,
	})
	decision := guard.Evaluate("send", map[string]any{"message": "safe"})
	if decision.Action != ActionBlock {
		t.Fatalf("expected bundle tool policy block, got %s", decision.Action)
	}
}
