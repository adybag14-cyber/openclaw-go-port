package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsWhenConfigMissing(t *testing.T) {
	t.Setenv("OPENCLAW_GO_HTTP_BIND", "")

	cfg, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatalf("Load returned error for missing file: %v", err)
	}

	if cfg.Gateway.URL != defaultGatewayURL {
		t.Fatalf("unexpected default gateway.url: %s", cfg.Gateway.URL)
	}
	if cfg.Gateway.Server.HTTPBind != defaultHTTPBind {
		t.Fatalf("unexpected default http bind: %s", cfg.Gateway.Server.HTTPBind)
	}
	if cfg.Runtime.StatePath != defaultStatePath {
		t.Fatalf("unexpected default runtime.state_path: %s", cfg.Runtime.StatePath)
	}
	if cfg.Runtime.Profile != defaultProfile {
		t.Fatalf("unexpected default runtime.profile: %s", cfg.Runtime.Profile)
	}
	if !cfg.Runtime.BrowserBridge.Enabled {
		t.Fatalf("expected default runtime.browser_bridge.enabled=true")
	}
	if cfg.Runtime.BrowserBridge.Endpoint != defaultBrowserBridgeEndpoint {
		t.Fatalf("unexpected default runtime.browser_bridge.endpoint: %s", cfg.Runtime.BrowserBridge.Endpoint)
	}
	if cfg.Runtime.BrowserBridge.RequestTimeoutMs != defaultBrowserBridgeRequestTimeoutMs {
		t.Fatalf("unexpected default runtime.browser_bridge.request_timeout_ms: %d", cfg.Runtime.BrowserBridge.RequestTimeoutMs)
	}
	if cfg.Runtime.BrowserBridge.Retries != defaultBrowserBridgeRetries {
		t.Fatalf("unexpected default runtime.browser_bridge.retries: %d", cfg.Runtime.BrowserBridge.Retries)
	}
	if cfg.Runtime.BrowserBridge.CircuitFailThreshold != defaultBrowserBridgeCircuitFailures {
		t.Fatalf("unexpected default runtime.browser_bridge.circuit_fail_threshold: %d", cfg.Runtime.BrowserBridge.CircuitFailThreshold)
	}
	if cfg.Security.DefaultAction != "allow" {
		t.Fatalf("unexpected default security.default_action: %s", cfg.Security.DefaultAction)
	}
	if cfg.Security.TelemetryAction != "review" {
		t.Fatalf("unexpected default security.telemetry_action: %s", cfg.Security.TelemetryAction)
	}
	if cfg.Security.CredentialLeakAction != "block" {
		t.Fatalf("unexpected default security.credential_leak_action: %s", cfg.Security.CredentialLeakAction)
	}
	if !cfg.Security.LoopGuardEnabled {
		t.Fatalf("expected default security.loop_guard_enabled=true")
	}
	if cfg.Security.LoopGuardWindowMS <= 0 {
		t.Fatalf("expected positive default security.loop_guard_window_ms")
	}
	if cfg.Security.LoopGuardMaxHits <= 0 {
		t.Fatalf("expected positive default security.loop_guard_max_hits")
	}
	if cfg.Security.RiskReviewThreshold <= 0 || cfg.Security.RiskBlockThreshold <= 0 {
		t.Fatalf("expected positive default risk thresholds")
	}
}

func TestLoadTomlAndEnvOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "openclaw-go.toml")
	content := `
[gateway]
url = "ws://example.invalid:9999/ws"

[gateway.server]
bind = "0.0.0.0:9000"
http_bind = "0.0.0.0:9001"
auth_mode = "token"

[runtime]
audit_only = true
state_path = "tmp/openclaw-go-state.json"
profile = "edge"

[security]
default_action = "review"
telemetry_action = "block"
credential_leak_action = "review"
policy_bundle_path = "tmp/policy-bundle.json"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed writing test config: %v", err)
	}

	t.Setenv("OPENCLAW_GO_HTTP_BIND", "127.0.0.1:7654")
	t.Setenv("OPENCLAW_GO_STATE_PATH", "tmp/override-state.json")
	t.Setenv("OPENCLAW_GO_TELEGRAM_BOT_TOKEN", "tg-token")
	t.Setenv("OPENCLAW_GO_POLICY_BUNDLE_PATH", "tmp/policy-from-env.json")
	t.Setenv("OPENCLAW_GO_BROWSER_BRIDGE_ENABLED", "false")
	t.Setenv("OPENCLAW_GO_BROWSER_BRIDGE_ENDPOINT", "http://127.0.0.1:43011")
	t.Setenv("OPENCLAW_GO_BROWSER_BRIDGE_REQUEST_TIMEOUT_MS", "120000")
	t.Setenv("OPENCLAW_GO_BROWSER_BRIDGE_RETRIES", "4")
	t.Setenv("OPENCLAW_GO_BROWSER_BRIDGE_RETRY_BACKOFF_MS", "50")
	t.Setenv("OPENCLAW_GO_BROWSER_BRIDGE_CIRCUIT_FAIL_THRESHOLD", "6")
	t.Setenv("OPENCLAW_GO_BROWSER_BRIDGE_CIRCUIT_COOLDOWN_MS", "9000")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Gateway.URL != "ws://example.invalid:9999/ws" {
		t.Fatalf("gateway.url mismatch: %s", cfg.Gateway.URL)
	}
	if cfg.Gateway.Server.Bind != "0.0.0.0:9000" {
		t.Fatalf("gateway.server.bind mismatch: %s", cfg.Gateway.Server.Bind)
	}
	if cfg.Gateway.Server.HTTPBind != "127.0.0.1:7654" {
		t.Fatalf("env override not applied: %s", cfg.Gateway.Server.HTTPBind)
	}
	if !cfg.Runtime.AuditOnly {
		t.Fatalf("runtime.audit_only expected true")
	}
	if cfg.Runtime.StatePath != "tmp/override-state.json" {
		t.Fatalf("runtime.state_path override not applied: %s", cfg.Runtime.StatePath)
	}
	if cfg.Runtime.Profile != "edge" {
		t.Fatalf("runtime.profile expected edge, got %s", cfg.Runtime.Profile)
	}
	if cfg.Runtime.BrowserBridge.Enabled {
		t.Fatalf("browser bridge enabled env override should be false")
	}
	if cfg.Runtime.BrowserBridge.Endpoint != "http://127.0.0.1:43011" {
		t.Fatalf("browser bridge endpoint override not applied: %s", cfg.Runtime.BrowserBridge.Endpoint)
	}
	if cfg.Runtime.BrowserBridge.RequestTimeoutMs != 120000 {
		t.Fatalf("browser bridge timeout override not applied: %d", cfg.Runtime.BrowserBridge.RequestTimeoutMs)
	}
	if cfg.Runtime.BrowserBridge.Retries != 4 {
		t.Fatalf("browser bridge retries override not applied: %d", cfg.Runtime.BrowserBridge.Retries)
	}
	if cfg.Runtime.BrowserBridge.RetryBackoffMs != 50 {
		t.Fatalf("browser bridge retry backoff override not applied: %d", cfg.Runtime.BrowserBridge.RetryBackoffMs)
	}
	if cfg.Runtime.BrowserBridge.CircuitFailThreshold != 6 {
		t.Fatalf("browser bridge fail threshold override not applied: %d", cfg.Runtime.BrowserBridge.CircuitFailThreshold)
	}
	if cfg.Runtime.BrowserBridge.CircuitCooldownMs != 9000 {
		t.Fatalf("browser bridge cooldown override not applied: %d", cfg.Runtime.BrowserBridge.CircuitCooldownMs)
	}
	if cfg.Channels.Telegram.BotToken != "tg-token" {
		t.Fatalf("telegram token env override not applied")
	}
	if cfg.Security.PolicyBundlePath != "tmp/policy-from-env.json" {
		t.Fatalf("policy bundle env override not applied: %s", cfg.Security.PolicyBundlePath)
	}
	if cfg.Security.DefaultAction != "review" {
		t.Fatalf("security.default_action expected review")
	}
	if cfg.Security.TelemetryAction != "block" {
		t.Fatalf("security.telemetry_action expected block")
	}
	if cfg.Security.CredentialLeakAction != "review" {
		t.Fatalf("security.credential_leak_action expected review")
	}
}

func TestRuntimeProfileValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "openclaw-go.toml")
	content := `
[runtime]
profile = "invalid"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed writing test config: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected runtime.profile validation error")
	}
}

func TestSecurityRiskThresholdValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "openclaw-go.toml")
	content := `
[security]
risk_review_threshold = 80
risk_block_threshold = 70
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed writing test config: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected security threshold validation error")
	}
}

func TestBrowserBridgeValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "openclaw-go.toml")
	content := `
[runtime.browser_bridge]
enabled = true
endpoint = ""
request_timeout_ms = 0
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed writing test config: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected browser bridge validation error")
	}
}
