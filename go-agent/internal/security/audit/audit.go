package audit

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/config"
	"github.com/pelletier/go-toml/v2"
)

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarn     Severity = "warn"
	SeverityCritical Severity = "critical"
)

type Finding struct {
	CheckID     string   `json:"checkId"`
	Severity    Severity `json:"severity"`
	Title       string   `json:"title"`
	Detail      string   `json:"detail"`
	Remediation string   `json:"remediation,omitempty"`
}

type Summary struct {
	Critical int `json:"critical"`
	Warn     int `json:"warn"`
	Info     int `json:"info"`
}

type DeepGateway struct {
	Attempted bool   `json:"attempted"`
	URL       string `json:"url,omitempty"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
}

type DeepBrowserBridge struct {
	Attempted    bool   `json:"attempted"`
	Endpoint     string `json:"endpoint,omitempty"`
	HealthURL    string `json:"healthUrl,omitempty"`
	HealthStatus int    `json:"healthStatus,omitempty"`
	OK           bool   `json:"ok"`
	Error        string `json:"error,omitempty"`
}

type DeepPolicyBundle struct {
	Attempted bool   `json:"attempted"`
	Path      string `json:"path,omitempty"`
	Exists    bool   `json:"exists"`
	ParseOK   bool   `json:"parseOk"`
	Error     string `json:"error,omitempty"`
}

type DeepReport struct {
	Gateway       DeepGateway       `json:"gateway"`
	BrowserBridge DeepBrowserBridge `json:"browserBridge"`
	PolicyBundle  DeepPolicyBundle  `json:"policyBundle"`
}

type FixAction struct {
	Kind    string `json:"kind"`
	Target  string `json:"target"`
	OK      bool   `json:"ok"`
	Skipped string `json:"skipped,omitempty"`
	Error   string `json:"error,omitempty"`
}

type FixResult struct {
	OK      bool        `json:"ok"`
	Changes []string    `json:"changes"`
	Actions []FixAction `json:"actions"`
	Errors  []string    `json:"errors"`
}

type Report struct {
	TS       int64       `json:"ts"`
	Summary  Summary     `json:"summary"`
	Findings []Finding   `json:"findings"`
	Deep     *DeepReport `json:"deep,omitempty"`
	Fix      *FixResult  `json:"fix,omitempty"`
}

type Options struct {
	Deep       bool
	Fix        bool
	ConfigPath string
}

func Run(cfg config.Config, options Options) Report {
	effectiveCfg := cfg
	var fixResult *FixResult
	if options.Fix {
		fixedCfg, result := applyFixes(cfg, options.ConfigPath)
		effectiveCfg = fixedCfg
		fixResult = result
	}

	findings := collectFindings(effectiveCfg)
	report := Report{
		TS:       time.Now().UTC().UnixMilli(),
		Findings: findings,
		Summary:  summarize(findings),
		Fix:      fixResult,
	}
	if options.Deep {
		deepGateway := probeGateway(effectiveCfg.Gateway.URL)
		deepBridge := probeBrowserBridge(effectiveCfg.Runtime.BrowserBridge)
		deepPolicyBundle := probePolicyBundle(effectiveCfg.Security.PolicyBundlePath)
		deep := DeepReport{
			Gateway:       deepGateway,
			BrowserBridge: deepBridge,
			PolicyBundle:  deepPolicyBundle,
		}
		if !deep.Gateway.OK {
			report.Findings = append(report.Findings, Finding{
				CheckID:  "gateway.deep_probe",
				Severity: SeverityWarn,
				Title:    "Deep gateway probe failed",
				Detail:   deep.Gateway.Error,
			})
		}
		if deep.BrowserBridge.Attempted && !deep.BrowserBridge.OK {
			report.Findings = append(report.Findings, Finding{
				CheckID:  "browser_bridge.deep_probe",
				Severity: SeverityWarn,
				Title:    "Deep browser bridge probe failed",
				Detail:   deep.BrowserBridge.Error,
			})
		}
		if deep.PolicyBundle.Attempted && !deep.PolicyBundle.ParseOK {
			report.Findings = append(report.Findings, Finding{
				CheckID:  "security.policy_bundle.deep_probe",
				Severity: SeverityWarn,
				Title:    "Deep policy bundle probe failed",
				Detail:   deep.PolicyBundle.Error,
			})
		}
		report.Summary = summarize(report.Findings)
		report.Deep = &deep
	}
	return report
}

func collectFindings(cfg config.Config) []Finding {
	findings := []Finding{}
	authMode := strings.ToLower(strings.TrimSpace(cfg.Gateway.Server.AuthMode))
	hasToken := strings.TrimSpace(cfg.Gateway.Token) != ""
	hasPassword := strings.TrimSpace(cfg.Gateway.Password) != ""

	if authMode == "none" {
		findings = append(findings, Finding{
			CheckID:  "gateway.auth.none",
			Severity: SeverityCritical,
			Title:    "Gateway auth is disabled",
			Detail:   "gateway.server.auth_mode is set to none",
		})
	} else if authMode == "auto" && !hasToken && !hasPassword {
		findings = append(findings, Finding{
			CheckID:     "gateway.auth.auto_unset",
			Severity:    SeverityWarn,
			Title:       "Gateway auth auto mode has no explicit secret",
			Detail:      "gateway.server.auth_mode is auto but token/password is empty",
			Remediation: "set gateway.token or gateway.password explicitly",
		})
	}
	if !isLoopbackBind(cfg.Gateway.Server.Bind) {
		severity := SeverityWarn
		if authMode == "none" {
			severity = SeverityCritical
		}
		findings = append(findings, Finding{
			CheckID:     "gateway.bind.public",
			Severity:    severity,
			Title:       "Gateway bind is publicly reachable",
			Detail:      fmt.Sprintf("gateway.server.bind is `%s`", strings.TrimSpace(cfg.Gateway.Server.Bind)),
			Remediation: "bind to loopback unless explicit external exposure is required",
		})
	}
	if !isLoopbackBind(cfg.Gateway.Server.HTTPBind) {
		severity := SeverityWarn
		if authMode == "none" {
			severity = SeverityCritical
		}
		findings = append(findings, Finding{
			CheckID:     "gateway.http_bind.public",
			Severity:    severity,
			Title:       "Gateway HTTP bind is publicly reachable",
			Detail:      fmt.Sprintf("gateway.server.http_bind is `%s`", strings.TrimSpace(cfg.Gateway.Server.HTTPBind)),
			Remediation: "bind to loopback unless explicit external exposure is required",
		})
	}

	if cfg.Runtime.AuditOnly {
		findings = append(findings, Finding{
			CheckID:     "runtime.audit_only.enabled",
			Severity:    SeverityWarn,
			Title:       "Runtime is in audit-only mode",
			Detail:      "runtime.audit_only=true weakens enforcement posture",
			Remediation: "set runtime.audit_only=false in production",
		})
	}
	if isMemoryPath(cfg.Runtime.StatePath) {
		findings = append(findings, Finding{
			CheckID:     "runtime.state_path.in_memory",
			Severity:    SeverityInfo,
			Title:       "Runtime state path is in-memory",
			Detail:      "runtime.state_path uses memory:// and is not persisted across restarts",
			Remediation: "set runtime.state_path to a persisted file path for production",
		})
	}
	bridgeHost, _, _ := parseURLHostPort(cfg.Runtime.BrowserBridge.Endpoint)
	if cfg.Runtime.BrowserBridge.Enabled && bridgeHost != "" && !isLoopbackHost(bridgeHost) {
		findings = append(findings, Finding{
			CheckID:     "runtime.browser_bridge.endpoint.public",
			Severity:    SeverityWarn,
			Title:       "Browser bridge endpoint is non-loopback",
			Detail:      fmt.Sprintf("runtime.browser_bridge.endpoint is `%s`", strings.TrimSpace(cfg.Runtime.BrowserBridge.Endpoint)),
			Remediation: "prefer loopback browser bridge endpoint unless remote bridge is explicitly trusted",
		})
	}
	if !cfg.Security.LoopGuardEnabled {
		findings = append(findings, Finding{
			CheckID:     "security.loop_guard.disabled",
			Severity:    SeverityWarn,
			Title:       "Security loop guard is disabled",
			Detail:      "security.loop_guard_enabled=false weakens replay/loop defense",
			Remediation: "enable security.loop_guard_enabled for production",
		})
	}
	if cfg.Security.LoopGuardEnabled && (cfg.Security.LoopGuardWindowMS <= 0 || cfg.Security.LoopGuardMaxHits <= 0) {
		findings = append(findings, Finding{
			CheckID:     "security.loop_guard.thresholds.invalid",
			Severity:    SeverityWarn,
			Title:       "Security loop guard thresholds are invalid",
			Detail:      "loop guard requires positive window and max hits",
			Remediation: "set security.loop_guard_window_ms and security.loop_guard_max_hits to positive values",
		})
	}
	if cfg.Security.RiskReviewThreshold < 40 || cfg.Security.RiskBlockThreshold < 60 {
		findings = append(findings, Finding{
			CheckID:     "security.risk_thresholds.permissive",
			Severity:    SeverityWarn,
			Title:       "Security risk thresholds are permissive",
			Detail:      fmt.Sprintf("review=%d block=%d", cfg.Security.RiskReviewThreshold, cfg.Security.RiskBlockThreshold),
			Remediation: "raise risk thresholds for tighter block/review posture",
		})
	}
	edrPath := strings.TrimSpace(cfg.Security.EDRTelemetryPath)
	if edrPath == "" {
		findings = append(findings, Finding{
			CheckID:     "security.edr_telemetry.unset",
			Severity:    SeverityInfo,
			Title:       "EDR telemetry feed path is not configured",
			Detail:      "security.edr_telemetry_path is empty",
			Remediation: "set security.edr_telemetry_path to a readable JSONL telemetry feed path",
		})
	} else {
		edrStat, err := os.Stat(edrPath)
		if err != nil {
			findings = append(findings, Finding{
				CheckID:     "security.edr_telemetry.stat_failed",
				Severity:    SeverityWarn,
				Title:       "EDR telemetry feed cannot be inspected",
				Detail:      err.Error(),
				Remediation: "ensure security.edr_telemetry_path exists and is readable",
			})
		} else if edrStat.IsDir() {
			findings = append(findings, Finding{
				CheckID:     "security.edr_telemetry.is_dir",
				Severity:    SeverityWarn,
				Title:       "EDR telemetry feed path is a directory",
				Detail:      "security.edr_telemetry_path must point to a JSONL file",
				Remediation: "set security.edr_telemetry_path to a JSONL telemetry file path",
			})
		}
	}
	if strings.TrimSpace(cfg.Security.AttestationExpectedSHA) == "" {
		findings = append(findings, Finding{
			CheckID:     "security.attestation.expected_sha_unset",
			Severity:    SeverityInfo,
			Title:       "Runtime attestation expected digest is not configured",
			Detail:      "security.attestation_expected_sha256 is empty",
			Remediation: "set security.attestation_expected_sha256 to enforce runtime binary verification",
		})
	}
	if strings.TrimSpace(cfg.Security.AttestationReportPath) == "" {
		findings = append(findings, Finding{
			CheckID:     "security.attestation.report_path_unset",
			Severity:    SeverityInfo,
			Title:       "Runtime attestation report path is not configured",
			Detail:      "security.attestation_report_path is empty",
			Remediation: "set security.attestation_report_path to persist attestation snapshots",
		})
	}
	if len(cfg.Security.BlockedMessagePatterns) == 0 {
		findings = append(findings, Finding{
			CheckID:  "security.blocked_patterns.empty",
			Severity: SeverityWarn,
			Title:    "Blocked message patterns are empty",
			Detail:   "security.blocked_message_patterns has no deny signatures",
		})
	}
	if len(cfg.Security.CredentialSensitiveKeys) == 0 {
		findings = append(findings, Finding{
			CheckID:  "security.credential_keys.empty",
			Severity: SeverityCritical,
			Title:    "Sensitive credential keys are not configured",
			Detail:   "security.credential_sensitive_keys is empty",
		})
	}
	policyPath := strings.TrimSpace(cfg.Security.PolicyBundlePath)
	if policyPath == "" || strings.HasPrefix(strings.ToLower(policyPath), "memory://") {
		findings = append(findings, Finding{
			CheckID:     "security.policy_bundle.unset",
			Severity:    SeverityInfo,
			Title:       "Signed policy bundle is not configured",
			Detail:      "security.policy_bundle_path is empty or in-memory",
			Remediation: "set a persisted signed policy bundle path for production",
		})
	} else {
		policyStat, err := os.Stat(policyPath)
		if err != nil {
			findings = append(findings, Finding{
				CheckID:     "security.policy_bundle.stat_failed",
				Severity:    SeverityWarn,
				Title:       "Policy bundle path inspection failed",
				Detail:      err.Error(),
				Remediation: "ensure security.policy_bundle_path exists and is readable",
			})
		} else if policyStat.IsDir() {
			findings = append(findings, Finding{
				CheckID:     "security.policy_bundle.is_dir",
				Severity:    SeverityWarn,
				Title:       "Policy bundle path is a directory",
				Detail:      "security.policy_bundle_path must point to a file",
				Remediation: "set security.policy_bundle_path to a JSON policy file",
			})
		} else {
			raw, readErr := os.ReadFile(policyPath)
			if readErr != nil {
				findings = append(findings, Finding{
					CheckID:     "security.policy_bundle.read_failed",
					Severity:    SeverityWarn,
					Title:       "Policy bundle cannot be read",
					Detail:      readErr.Error(),
					Remediation: "ensure policy bundle permissions allow read access",
				})
			} else {
				var parsed map[string]any
				if err := json.Unmarshal(raw, &parsed); err != nil {
					findings = append(findings, Finding{
						CheckID:     "security.policy_bundle.parse_failed",
						Severity:    SeverityWarn,
						Title:       "Policy bundle is not valid JSON",
						Detail:      err.Error(),
						Remediation: "repair policy bundle JSON syntax",
					})
				}
			}
		}
	}
	return findings
}

func applyFixes(cfg config.Config, configPath string) (config.Config, *FixResult) {
	result := &FixResult{
		OK:      true,
		Changes: make([]string, 0, 16),
		Actions: make([]FixAction, 0, 8),
		Errors:  make([]string, 0, 8),
	}

	targetPath := strings.TrimSpace(configPath)
	if targetPath == "" {
		targetPath = "openclaw-go.toml"
	}
	absPath, absErr := filepath.Abs(targetPath)
	if absErr == nil {
		targetPath = absPath
	}
	baseDir := filepath.Dir(targetPath)
	defaults := config.Default()
	next := cfg

	recordChange := func(change string) {
		result.Changes = append(result.Changes, change)
	}

	authMode := strings.ToLower(strings.TrimSpace(next.Gateway.Server.AuthMode))
	if authMode == "none" {
		next.Gateway.Server.AuthMode = "auto"
		recordChange("set gateway.server.auth_mode to auto")
	}
	if !isLoopbackBind(next.Gateway.Server.Bind) {
		next.Gateway.Server.Bind = defaults.Gateway.Server.Bind
		recordChange("set gateway.server.bind to loopback default")
	}
	if !isLoopbackBind(next.Gateway.Server.HTTPBind) {
		next.Gateway.Server.HTTPBind = defaults.Gateway.Server.HTTPBind
		recordChange("set gateway.server.http_bind to loopback default")
	}

	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(next.Runtime.StatePath)), "memory://") {
		next.Runtime.StatePath = filepath.Join(baseDir, "openclaw-go-state.json")
		recordChange("set runtime.state_path to persisted file path")
	}

	if next.Runtime.BrowserBridge.Enabled {
		host, _, _ := parseURLHostPort(next.Runtime.BrowserBridge.Endpoint)
		if host != "" && !isLoopbackHost(host) {
			next.Runtime.BrowserBridge.Endpoint = defaults.Runtime.BrowserBridge.Endpoint
			recordChange("set runtime.browser_bridge.endpoint to loopback default")
		}
	}

	if !next.Security.LoopGuardEnabled {
		next.Security.LoopGuardEnabled = true
		recordChange("enabled security.loop_guard_enabled")
	}
	if next.Security.LoopGuardWindowMS <= 0 {
		next.Security.LoopGuardWindowMS = defaults.Security.LoopGuardWindowMS
		recordChange("set security.loop_guard_window_ms to default")
	}
	if next.Security.LoopGuardMaxHits <= 0 {
		next.Security.LoopGuardMaxHits = defaults.Security.LoopGuardMaxHits
		recordChange("set security.loop_guard_max_hits to default")
	}

	if len(next.Security.BlockedMessagePatterns) == 0 {
		next.Security.BlockedMessagePatterns = append([]string(nil), defaults.Security.BlockedMessagePatterns...)
		recordChange("restored default security.blocked_message_patterns")
	}
	if len(next.Security.CredentialSensitiveKeys) == 0 {
		next.Security.CredentialSensitiveKeys = append([]string(nil), defaults.Security.CredentialSensitiveKeys...)
		recordChange("restored default security.credential_sensitive_keys")
	}

	if next.Security.RiskReviewThreshold < 40 || next.Security.RiskReviewThreshold > 100 {
		next.Security.RiskReviewThreshold = defaults.Security.RiskReviewThreshold
		recordChange("set security.risk_review_threshold to default")
	}
	if next.Security.RiskBlockThreshold < 60 || next.Security.RiskBlockThreshold > 100 || next.Security.RiskBlockThreshold < next.Security.RiskReviewThreshold {
		next.Security.RiskBlockThreshold = defaults.Security.RiskBlockThreshold
		if next.Security.RiskBlockThreshold < next.Security.RiskReviewThreshold {
			next.Security.RiskBlockThreshold = next.Security.RiskReviewThreshold
		}
		recordChange("set security.risk_block_threshold to default")
	}
	if strings.TrimSpace(next.Security.EDRTelemetryPath) == "" {
		next.Security.EDRTelemetryPath = filepath.Join(baseDir, "edr-telemetry.jsonl")
		recordChange("set security.edr_telemetry_path to persisted telemetry feed path")
	}
	if next.Security.EDRTelemetryMaxAgeSecs <= 0 {
		next.Security.EDRTelemetryMaxAgeSecs = defaults.Security.EDRTelemetryMaxAgeSecs
		recordChange("set security.edr_telemetry_max_age_secs to default")
	}
	if next.Security.EDRTelemetryRiskBonus <= 0 || next.Security.EDRTelemetryRiskBonus > 100 {
		next.Security.EDRTelemetryRiskBonus = defaults.Security.EDRTelemetryRiskBonus
		recordChange("set security.edr_telemetry_risk_bonus to default")
	}
	if strings.TrimSpace(next.Security.AttestationReportPath) == "" {
		next.Security.AttestationReportPath = filepath.Join(baseDir, "attestation-report.json")
		recordChange("set security.attestation_report_path to persisted file path")
	}
	if next.Security.AttestationMismatchRisk <= 0 || next.Security.AttestationMismatchRisk > 100 {
		next.Security.AttestationMismatchRisk = defaults.Security.AttestationMismatchRisk
		recordChange("set security.attestation_mismatch_risk_bonus to default")
	}

	policyPath := strings.TrimSpace(next.Security.PolicyBundlePath)
	if policyPath == "" || strings.HasPrefix(strings.ToLower(policyPath), "memory://") {
		next.Security.PolicyBundlePath = filepath.Join(baseDir, "security-policy.json")
		recordChange("set security.policy_bundle_path to persisted file path")
	}

	if mkdirErr := os.MkdirAll(baseDir, 0o755); mkdirErr != nil {
		result.OK = false
		result.Errors = append(result.Errors, fmt.Sprintf("failed to create config directory %s: %v", baseDir, mkdirErr))
		result.Actions = append(result.Actions, FixAction{
			Kind:   "mkdir",
			Target: baseDir,
			OK:     false,
			Error:  mkdirErr.Error(),
		})
	} else {
		result.Actions = append(result.Actions, FixAction{
			Kind:   "mkdir",
			Target: baseDir,
			OK:     true,
		})
	}

	policyAction := ensurePolicyBundleFile(next.Security.PolicyBundlePath, defaults.Security.BlockedMessagePatterns)
	result.Actions = append(result.Actions, policyAction)
	if !policyAction.OK && policyAction.Error != "" {
		result.OK = false
		result.Errors = append(result.Errors, policyAction.Error)
	}
	if policyAction.OK && policyAction.Kind == "write" && strings.TrimSpace(policyAction.Skipped) == "" {
		recordChange("created security policy bundle file")
	}

	tomlBytes, marshalErr := toml.Marshal(next)
	if marshalErr != nil {
		result.OK = false
		result.Errors = append(result.Errors, fmt.Sprintf("failed to serialize remediated config: %v", marshalErr))
		result.Actions = append(result.Actions, FixAction{
			Kind:   "write",
			Target: targetPath,
			OK:     false,
			Error:  marshalErr.Error(),
		})
		return cfg, result
	}

	if writeErr := os.WriteFile(targetPath, tomlBytes, 0o600); writeErr != nil {
		result.OK = false
		result.Errors = append(result.Errors, fmt.Sprintf("failed to write remediated config: %v", writeErr))
		result.Actions = append(result.Actions, FixAction{
			Kind:   "write",
			Target: targetPath,
			OK:     false,
			Error:  writeErr.Error(),
		})
		return cfg, result
	}
	result.Actions = append(result.Actions, FixAction{
		Kind:   "write",
		Target: targetPath,
		OK:     true,
	})

	if chmodErr := os.Chmod(targetPath, 0o600); chmodErr != nil {
		result.Actions = append(result.Actions, FixAction{
			Kind:    "chmod",
			Target:  targetPath,
			OK:      false,
			Skipped: chmodErr.Error(),
		})
	} else {
		result.Actions = append(result.Actions, FixAction{
			Kind:   "chmod",
			Target: targetPath,
			OK:     true,
		})
	}

	if len(result.Errors) > 0 {
		result.OK = false
	}
	return next, result
}

func ensurePolicyBundleFile(path string, blockedPatterns []string) FixAction {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" || strings.HasPrefix(strings.ToLower(trimmed), "memory://") {
		return FixAction{
			Kind:    "write",
			Target:  trimmed,
			OK:      false,
			Skipped: "policy bundle path is unset",
		}
	}

	if info, err := os.Stat(trimmed); err == nil && !info.IsDir() {
		return FixAction{
			Kind:    "write",
			Target:  trimmed,
			OK:      true,
			Skipped: "policy bundle file already exists",
		}
	}

	parent := filepath.Dir(trimmed)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return FixAction{
			Kind:   "write",
			Target: trimmed,
			OK:     false,
			Error:  err.Error(),
		}
	}

	payload := map[string]any{
		"version":                  1,
		"generatedBy":              "openclaw-go-security-audit-fix",
		"generatedAt":              time.Now().UTC().Format(time.RFC3339),
		"default_action":           "allow",
		"tool_policies":            map[string]any{},
		"blocked_message_patterns": blockedPatterns,
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return FixAction{
			Kind:   "write",
			Target: trimmed,
			OK:     false,
			Error:  err.Error(),
		}
	}
	if err := os.WriteFile(trimmed, raw, 0o600); err != nil {
		return FixAction{
			Kind:   "write",
			Target: trimmed,
			OK:     false,
			Error:  err.Error(),
		}
	}
	return FixAction{
		Kind:   "write",
		Target: trimmed,
		OK:     true,
	}
}

func summarize(findings []Finding) Summary {
	out := Summary{}
	for _, finding := range findings {
		switch finding.Severity {
		case SeverityCritical:
			out.Critical++
		case SeverityWarn:
			out.Warn++
		default:
			out.Info++
		}
	}
	return out
}

func probeGateway(gatewayURL string) DeepGateway {
	trimmed := strings.TrimSpace(gatewayURL)
	if trimmed == "" {
		return DeepGateway{
			Attempted: false,
			URL:       gatewayURL,
			OK:        false,
			Error:     "gateway url is empty",
		}
	}
	host, port, err := parseURLHostPort(trimmed)
	if err != nil {
		return DeepGateway{
			Attempted: false,
			URL:       trimmed,
			OK:        false,
			Error:     err.Error(),
		}
	}
	conn, dialErr := net.DialTimeout("tcp", net.JoinHostPort(host, port), 1500*time.Millisecond)
	if dialErr != nil {
		return DeepGateway{
			Attempted: true,
			URL:       trimmed,
			OK:        false,
			Error:     dialErr.Error(),
		}
	}
	_ = conn.Close()
	return DeepGateway{
		Attempted: true,
		URL:       trimmed,
		OK:        true,
	}
}

func probeBrowserBridge(cfg config.BrowserBridgeConfig) DeepBrowserBridge {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if !cfg.Enabled {
		return DeepBrowserBridge{
			Attempted: false,
			Endpoint:  endpoint,
			OK:        true,
		}
	}
	if endpoint == "" {
		return DeepBrowserBridge{
			Attempted: false,
			Endpoint:  endpoint,
			OK:        false,
			Error:     "runtime browser bridge endpoint is empty",
		}
	}
	host, port, err := parseURLHostPort(endpoint)
	if err != nil {
		return DeepBrowserBridge{
			Attempted: false,
			Endpoint:  endpoint,
			OK:        false,
			Error:     err.Error(),
		}
	}
	conn, dialErr := net.DialTimeout("tcp", net.JoinHostPort(host, port), 1500*time.Millisecond)
	if dialErr != nil {
		return DeepBrowserBridge{
			Attempted: true,
			Endpoint:  endpoint,
			OK:        false,
			Error:     dialErr.Error(),
		}
	}
	_ = conn.Close()

	healthURL, healthErr := browserBridgeHealthURL(endpoint)
	if healthErr != nil {
		return DeepBrowserBridge{
			Attempted: true,
			Endpoint:  endpoint,
			OK:        false,
			Error:     healthErr.Error(),
		}
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, reqErr := client.Get(healthURL)
	if reqErr != nil {
		return DeepBrowserBridge{
			Attempted: true,
			Endpoint:  endpoint,
			HealthURL: healthURL,
			OK:        false,
			Error:     reqErr.Error(),
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return DeepBrowserBridge{
			Attempted:    true,
			Endpoint:     endpoint,
			HealthURL:    healthURL,
			HealthStatus: resp.StatusCode,
			OK:           false,
			Error:        fmt.Sprintf("health probe returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body))),
		}
	}
	return DeepBrowserBridge{
		Attempted:    true,
		Endpoint:     endpoint,
		HealthURL:    healthURL,
		HealthStatus: resp.StatusCode,
		OK:           true,
	}
}

func browserBridgeHealthURL(endpoint string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return "", err
	}
	switch strings.ToLower(parsed.Scheme) {
	case "ws":
		parsed.Scheme = "http"
	case "wss":
		parsed.Scheme = "https"
	case "http", "https":
	default:
		if parsed.Scheme == "" {
			parsed.Scheme = "http"
		}
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		path = "/health"
	} else {
		path += "/health"
	}
	parsed.Path = path
	return parsed.String(), nil
}

func probePolicyBundle(policyBundlePath string) DeepPolicyBundle {
	trimmed := strings.TrimSpace(policyBundlePath)
	if trimmed == "" || strings.HasPrefix(strings.ToLower(trimmed), "memory://") {
		return DeepPolicyBundle{
			Attempted: false,
			Path:      trimmed,
			Exists:    false,
			ParseOK:   true,
		}
	}

	absPath := trimmed
	if !filepath.IsAbs(absPath) {
		if resolved, err := filepath.Abs(trimmed); err == nil {
			absPath = resolved
		}
	}
	info, statErr := os.Stat(absPath)
	if statErr != nil {
		return DeepPolicyBundle{
			Attempted: true,
			Path:      absPath,
			Exists:    false,
			ParseOK:   false,
			Error:     statErr.Error(),
		}
	}
	if info.IsDir() {
		return DeepPolicyBundle{
			Attempted: true,
			Path:      absPath,
			Exists:    true,
			ParseOK:   false,
			Error:     "policy bundle path is a directory",
		}
	}
	raw, readErr := os.ReadFile(absPath)
	if readErr != nil {
		return DeepPolicyBundle{
			Attempted: true,
			Path:      absPath,
			Exists:    true,
			ParseOK:   false,
			Error:     readErr.Error(),
		}
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return DeepPolicyBundle{
			Attempted: true,
			Path:      absPath,
			Exists:    true,
			ParseOK:   false,
			Error:     err.Error(),
		}
	}
	return DeepPolicyBundle{
		Attempted: true,
		Path:      absPath,
		Exists:    true,
		ParseOK:   true,
	}
}

func parseURLHostPort(raw string) (string, string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", "", err
	}
	host := parsed.Hostname()
	if host == "" {
		return "", "", &url.Error{Op: "parse", URL: raw, Err: errors.New("missing host")}
	}
	port := parsed.Port()
	if port == "" {
		switch strings.ToLower(parsed.Scheme) {
		case "ws", "http":
			port = "80"
		case "wss", "https":
			port = "443"
		default:
			port = "80"
		}
	}
	return host, port, nil
}

func isMemoryPath(path string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(path)), "memory://")
}

func isLoopbackBind(bind string) bool {
	trimmed := strings.TrimSpace(bind)
	if trimmed == "" {
		return false
	}

	host := trimmed
	if strings.Contains(trimmed, "://") {
		parsedHost, _, err := parseURLHostPort(trimmed)
		if err != nil {
			return false
		}
		host = parsedHost
	} else {
		if strings.HasPrefix(trimmed, ":") {
			host = "0.0.0.0"
		} else {
			if parsedHost, _, err := net.SplitHostPort(trimmed); err == nil {
				host = parsedHost
			}
		}
	}
	host = strings.Trim(host, "[]")
	return isLoopbackHost(host)
}

func isLoopbackHost(host string) bool {
	normalized := strings.ToLower(strings.TrimSpace(host))
	switch normalized {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	ip := net.ParseIP(normalized)
	return ip != nil && ip.IsLoopback()
}
