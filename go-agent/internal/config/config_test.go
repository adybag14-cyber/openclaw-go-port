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
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed writing test config: %v", err)
	}

	t.Setenv("OPENCLAW_GO_HTTP_BIND", "127.0.0.1:7654")

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
}
