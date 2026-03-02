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
	if cfg.Runtime.MemoryMaxEntries != defaultMemoryMaxEntries {
		t.Fatalf("unexpected default runtime.memory_max_entries: %d", cfg.Runtime.MemoryMaxEntries)
	}
	if cfg.Runtime.WebLoginTTLMinutes != defaultWebLoginTTLMinutes {
		t.Fatalf("unexpected default runtime.web_login_ttl_minutes: %d", cfg.Runtime.WebLoginTTLMinutes)
	}
	if cfg.Runtime.ModelCatalogRefreshTTLSeconds != defaultModelCatalogRefreshTTLSeconds {
		t.Fatalf("unexpected default runtime.model_catalog_refresh_ttl_seconds: %d", cfg.Runtime.ModelCatalogRefreshTTLSeconds)
	}
	if !cfg.Runtime.TelegramLiveStreaming {
		t.Fatalf("expected default runtime.telegram_live_streaming=true")
	}
	if cfg.Runtime.TelegramStreamChunkChars != defaultTelegramStreamChunkChars {
		t.Fatalf("unexpected default runtime.telegram_stream_chunk_chars: %d", cfg.Runtime.TelegramStreamChunkChars)
	}
	if cfg.Runtime.TelegramStreamChunkDelayMs != defaultTelegramStreamChunkDelayMs {
		t.Fatalf("unexpected default runtime.telegram_stream_chunk_delay_ms: %d", cfg.Runtime.TelegramStreamChunkDelayMs)
	}
	if !cfg.Runtime.TelegramTypingIndicators {
		t.Fatalf("expected default runtime.telegram_typing_indicators=true")
	}
	if cfg.Runtime.TelegramTypingIntervalMs != defaultTelegramTypingIntervalMs {
		t.Fatalf("unexpected default runtime.telegram_typing_interval_ms: %d", cfg.Runtime.TelegramTypingIntervalMs)
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
	if cfg.Security.EDRTelemetryMaxAgeSecs <= 0 {
		t.Fatalf("expected positive default security.edr_telemetry_max_age_secs")
	}
	if cfg.Security.EDRTelemetryRiskBonus <= 0 {
		t.Fatalf("expected positive default security.edr_telemetry_risk_bonus")
	}
	if cfg.Security.AttestationMismatchRisk <= 0 {
		t.Fatalf("expected positive default security.attestation_mismatch_risk_bonus")
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
web_login_ttl_minutes = 45

[security]
default_action = "review"
telemetry_action = "block"
edr_telemetry_max_age_secs = 60
edr_telemetry_risk_bonus = 30
credential_leak_action = "review"
attestation_expected_sha256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
attestation_mismatch_risk_bonus = 75
policy_bundle_path = "tmp/policy-bundle.json"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed writing test config: %v", err)
	}

	t.Setenv("OPENCLAW_GO_HTTP_BIND", "127.0.0.1:7654")
	t.Setenv("OPENCLAW_GO_STATE_PATH", "tmp/override-state.json")
	t.Setenv("OPENCLAW_GO_WEB_LOGIN_TTL_MINUTES", "180")
	t.Setenv("OPENCLAW_GO_MEMORY_MAX_ENTRIES", "-1")
	t.Setenv("OPENCLAW_GO_MODEL_CATALOG_REFRESH_TTL_SECONDS", "90")
	t.Setenv("OPENCLAW_GO_TELEGRAM_LIVE_STREAMING", "false")
	t.Setenv("OPENCLAW_GO_TELEGRAM_STREAM_CHUNK_CHARS", "420")
	t.Setenv("OPENCLAW_GO_TELEGRAM_STREAM_CHUNK_DELAY_MS", "120")
	t.Setenv("OPENCLAW_GO_TELEGRAM_TYPING_INDICATORS", "false")
	t.Setenv("OPENCLAW_GO_TELEGRAM_TYPING_INTERVAL_MS", "2100")
	t.Setenv("OPENCLAW_GO_TELEGRAM_BOT_TOKEN", "tg-token")
	t.Setenv("OPENCLAW_GO_POLICY_BUNDLE_PATH", "tmp/policy-from-env.json")
	t.Setenv("OPENCLAW_GO_EDR_TELEMETRY_PATH", "tmp/edr-feed.jsonl")
	t.Setenv("OPENCLAW_GO_EDR_TELEMETRY_MAX_AGE_SECS", "120")
	t.Setenv("OPENCLAW_GO_EDR_TELEMETRY_RISK_BONUS", "44")
	t.Setenv("OPENCLAW_GO_ATTESTATION_EXPECTED_SHA256", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	t.Setenv("OPENCLAW_GO_ATTESTATION_MISMATCH_RISK_BONUS", "65")
	t.Setenv("OPENCLAW_GO_ATTESTATION_REPORT_PATH", "tmp/attestation-report.json")
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
	if cfg.Runtime.MemoryMaxEntries != -1 {
		t.Fatalf("runtime.memory_max_entries override not applied: %d", cfg.Runtime.MemoryMaxEntries)
	}
	if cfg.Runtime.WebLoginTTLMinutes != 180 {
		t.Fatalf("runtime.web_login_ttl_minutes override not applied: %d", cfg.Runtime.WebLoginTTLMinutes)
	}
	if cfg.Runtime.ModelCatalogRefreshTTLSeconds != 90 {
		t.Fatalf("runtime.model_catalog_refresh_ttl_seconds override not applied: %d", cfg.Runtime.ModelCatalogRefreshTTLSeconds)
	}
	if cfg.Runtime.TelegramLiveStreaming {
		t.Fatalf("runtime.telegram_live_streaming override not applied")
	}
	if cfg.Runtime.TelegramStreamChunkChars != 420 {
		t.Fatalf("runtime.telegram_stream_chunk_chars override not applied: %d", cfg.Runtime.TelegramStreamChunkChars)
	}
	if cfg.Runtime.TelegramStreamChunkDelayMs != 120 {
		t.Fatalf("runtime.telegram_stream_chunk_delay_ms override not applied: %d", cfg.Runtime.TelegramStreamChunkDelayMs)
	}
	if cfg.Runtime.TelegramTypingIndicators {
		t.Fatalf("runtime.telegram_typing_indicators override not applied")
	}
	if cfg.Runtime.TelegramTypingIntervalMs != 2100 {
		t.Fatalf("runtime.telegram_typing_interval_ms override not applied: %d", cfg.Runtime.TelegramTypingIntervalMs)
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
	if cfg.Security.EDRTelemetryPath != "tmp/edr-feed.jsonl" {
		t.Fatalf("security.edr_telemetry_path override not applied: %s", cfg.Security.EDRTelemetryPath)
	}
	if cfg.Security.EDRTelemetryMaxAgeSecs != 120 {
		t.Fatalf("security.edr_telemetry_max_age_secs override not applied: %d", cfg.Security.EDRTelemetryMaxAgeSecs)
	}
	if cfg.Security.EDRTelemetryRiskBonus != 44 {
		t.Fatalf("security.edr_telemetry_risk_bonus override not applied: %d", cfg.Security.EDRTelemetryRiskBonus)
	}
	if cfg.Security.AttestationExpectedSHA != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("security.attestation_expected_sha256 override not applied")
	}
	if cfg.Security.AttestationMismatchRisk != 65 {
		t.Fatalf("security.attestation_mismatch_risk_bonus override not applied: %d", cfg.Security.AttestationMismatchRisk)
	}
	if cfg.Security.AttestationReportPath != "tmp/attestation-report.json" {
		t.Fatalf("security.attestation_report_path override not applied: %s", cfg.Security.AttestationReportPath)
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

func TestMemoryMaxEntriesValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "openclaw-go.toml")
	content := `
[runtime]
memory_max_entries = -2
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed writing test config: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected runtime.memory_max_entries validation error")
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

func TestWebLoginTTLValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "openclaw-go.toml")
	content := `
[runtime]
web_login_ttl_minutes = 0
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed writing test config: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected runtime.web_login_ttl_minutes validation error")
	}
}

func TestAttestationDigestValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "openclaw-go.toml")
	content := `
[security]
attestation_expected_sha256 = "not-a-sha"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed writing test config: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected attestation digest validation error")
	}
}

func TestChannelAdapterValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "openclaw-go.toml")
	content := `
[channels.discord]
enabled = true
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed writing test config: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected channel adapter validation error")
	}
}
