package audit

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/config"
)

func TestRunReportsCriticalWhenAuthNone(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Server.AuthMode = "none"

	report := Run(cfg, Options{})
	if report.Summary.Critical < 1 {
		t.Fatalf("expected at least one critical finding, got %+v", report.Summary)
	}
}

func TestRunDeepGatewayProbe(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind local listener: %v", err)
	}
	defer listener.Close()

	cfg := config.Default()
	cfg.Gateway.URL = "ws://" + listener.Addr().String() + "/gateway"

	report := Run(cfg, Options{Deep: true})
	if report.Deep == nil {
		t.Fatalf("expected deep report")
	}
	if !report.Deep.Gateway.OK {
		t.Fatalf("expected deep gateway probe to pass, got error=%s", report.Deep.Gateway.Error)
	}
}

func TestRunWarnsWhenCredentialPolicyKeysMissing(t *testing.T) {
	cfg := config.Default()
	cfg.Security.CredentialSensitiveKeys = []string{}

	report := Run(cfg, Options{})
	if report.Summary.Critical < 1 {
		t.Fatalf("expected critical finding for missing credential keys")
	}
}

func TestRunFlagsPublicGatewayBinds(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Server.AuthMode = "token"
	cfg.Gateway.Token = "secret"
	cfg.Gateway.Server.Bind = "0.0.0.0:8765"
	cfg.Gateway.Server.HTTPBind = "0.0.0.0:8766"

	report := Run(cfg, Options{})
	if !hasFinding(report, "gateway.bind.public") {
		t.Fatalf("expected gateway.bind.public finding")
	}
	if !hasFinding(report, "gateway.http_bind.public") {
		t.Fatalf("expected gateway.http_bind.public finding")
	}
}

func TestRunDetectsInvalidPolicyBundleFile(t *testing.T) {
	cfg := config.Default()
	cfg.Security.PolicyBundlePath = filepath.Join(t.TempDir(), "policy.json")
	if err := os.WriteFile(cfg.Security.PolicyBundlePath, []byte("{not-json"), 0o600); err != nil {
		t.Fatalf("failed to write policy bundle fixture: %v", err)
	}

	report := Run(cfg, Options{})
	if !hasFinding(report, "security.policy_bundle.parse_failed") {
		t.Fatalf("expected security.policy_bundle.parse_failed finding")
	}
}

func TestRunReportsTelemetryAndAttestationPostureFindings(t *testing.T) {
	cfg := config.Default()
	report := Run(cfg, Options{})

	if !hasFinding(report, "security.edr_telemetry.unset") {
		t.Fatalf("expected security.edr_telemetry.unset finding")
	}
	if !hasFinding(report, "security.attestation.expected_sha_unset") {
		t.Fatalf("expected security.attestation.expected_sha_unset finding")
	}
	if !hasFinding(report, "security.attestation.report_path_unset") {
		t.Fatalf("expected security.attestation.report_path_unset finding")
	}
}

func TestRunDeepBrowserBridgeProbe(t *testing.T) {
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer bridge.Close()

	cfg := config.Default()
	cfg.Runtime.BrowserBridge.Enabled = true
	cfg.Runtime.BrowserBridge.Endpoint = bridge.URL

	report := Run(cfg, Options{Deep: true})
	if report.Deep == nil {
		t.Fatalf("expected deep report")
	}
	if !report.Deep.BrowserBridge.OK {
		t.Fatalf("expected deep browser bridge probe to pass, got error=%s", report.Deep.BrowserBridge.Error)
	}
	if report.Deep.BrowserBridge.HealthStatus != http.StatusOK {
		t.Fatalf("expected deep browser bridge health status 200, got %d", report.Deep.BrowserBridge.HealthStatus)
	}
}

func TestRunParityCheckIDCorpus(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Server.AuthMode = "none"
	cfg.Gateway.Server.Bind = "0.0.0.0:8765"
	cfg.Gateway.Server.HTTPBind = "0.0.0.0:8766"
	cfg.Runtime.AuditOnly = true
	cfg.Runtime.StatePath = "memory://state"
	cfg.Runtime.BrowserBridge.Endpoint = "http://0.0.0.0:43010"
	cfg.Security.PolicyBundlePath = filepath.Join(t.TempDir(), "missing-policy.json")
	cfg.Security.LoopGuardEnabled = false
	cfg.Security.RiskReviewThreshold = 20
	cfg.Security.RiskBlockThreshold = 30

	report := Run(cfg, Options{})
	got := checkIDs(report)
	want := []string{
		"gateway.auth.none",
		"gateway.bind.public",
		"gateway.http_bind.public",
		"runtime.audit_only.enabled",
		"runtime.state_path.in_memory",
		"runtime.browser_bridge.endpoint.public",
		"security.loop_guard.disabled",
		"security.risk_thresholds.permissive",
		"security.policy_bundle.stat_failed",
	}
	for _, expected := range want {
		if !slices.Contains(got, expected) {
			t.Fatalf("expected parity check id %s in report ids=%v", expected, got)
		}
	}
}

func TestRunFixAppliesRemediationsAndPersistsConfig(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Server.AuthMode = "none"
	cfg.Gateway.Server.Bind = "0.0.0.0:8765"
	cfg.Gateway.Server.HTTPBind = "0.0.0.0:8766"
	cfg.Runtime.StatePath = "memory://state"
	cfg.Runtime.BrowserBridge.Endpoint = "http://0.0.0.0:43010"
	cfg.Security.LoopGuardEnabled = false
	cfg.Security.LoopGuardWindowMS = 0
	cfg.Security.LoopGuardMaxHits = 0
	cfg.Security.RiskReviewThreshold = 20
	cfg.Security.RiskBlockThreshold = 30
	cfg.Security.BlockedMessagePatterns = []string{}
	cfg.Security.CredentialSensitiveKeys = []string{}
	cfg.Security.PolicyBundlePath = "memory://policy"

	configPath := filepath.Join(t.TempDir(), "openclaw-go.toml")
	report := Run(cfg, Options{
		Fix:        true,
		ConfigPath: configPath,
	})
	if report.Fix == nil {
		t.Fatalf("expected fix report")
	}
	if !report.Fix.OK {
		t.Fatalf("expected fix report ok, errors=%v", report.Fix.Errors)
	}
	if len(report.Fix.Changes) == 0 {
		t.Fatalf("expected fix to apply changes")
	}
	if hasFinding(report, "gateway.auth.none") {
		t.Fatalf("expected auth.none finding to be remediated")
	}
	if hasFinding(report, "security.credential_keys.empty") {
		t.Fatalf("expected credential keys finding to be remediated")
	}

	loaded, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("expected remediated config to load: %v", err)
	}
	if loaded.Gateway.Server.AuthMode != "auto" {
		t.Fatalf("expected auth mode auto, got %s", loaded.Gateway.Server.AuthMode)
	}
	if strings.HasPrefix(strings.ToLower(loaded.Runtime.StatePath), "memory://") {
		t.Fatalf("expected persisted runtime state path, got %s", loaded.Runtime.StatePath)
	}
	if !loaded.Security.LoopGuardEnabled {
		t.Fatalf("expected loop guard to be enabled after remediation")
	}
	if len(loaded.Security.BlockedMessagePatterns) == 0 {
		t.Fatalf("expected blocked patterns restored after remediation")
	}
	if len(loaded.Security.CredentialSensitiveKeys) == 0 {
		t.Fatalf("expected credential keys restored after remediation")
	}
	if strings.TrimSpace(loaded.Security.PolicyBundlePath) == "" {
		t.Fatalf("expected policy bundle path to be set")
	}
	if strings.TrimSpace(loaded.Security.EDRTelemetryPath) == "" {
		t.Fatalf("expected edr telemetry path to be set")
	}
	if strings.TrimSpace(loaded.Security.AttestationReportPath) == "" {
		t.Fatalf("expected attestation report path to be set")
	}
	if _, err := os.Stat(loaded.Security.PolicyBundlePath); err != nil {
		t.Fatalf("expected policy bundle file to exist: %v", err)
	}
}

func TestRunFixIsIdempotentAfterRemediation(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Server.AuthMode = "none"
	cfg.Security.PolicyBundlePath = "memory://policy"

	configPath := filepath.Join(t.TempDir(), "openclaw-go.toml")
	first := Run(cfg, Options{
		Fix:        true,
		ConfigPath: configPath,
	})
	if first.Fix == nil || !first.Fix.OK {
		t.Fatalf("expected first fix run to succeed")
	}

	loaded, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("failed to load remediated config: %v", err)
	}
	second := Run(loaded, Options{
		Fix:        true,
		ConfigPath: configPath,
	})
	if second.Fix == nil || !second.Fix.OK {
		t.Fatalf("expected second fix run to succeed")
	}
	if len(second.Fix.Changes) != 0 {
		t.Fatalf("expected second fix run to be idempotent, got changes=%v", second.Fix.Changes)
	}
}

func hasFinding(report Report, checkID string) bool {
	for _, finding := range report.Findings {
		if finding.CheckID == checkID {
			return true
		}
	}
	return false
}

func checkIDs(report Report) []string {
	out := make([]string, 0, len(report.Findings))
	for _, finding := range report.Findings {
		out = append(out, finding.CheckID)
	}
	return out
}
