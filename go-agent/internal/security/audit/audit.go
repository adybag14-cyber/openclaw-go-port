package audit

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/config"
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
	Attempted bool   `json:"attempted"`
	Endpoint  string `json:"endpoint,omitempty"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
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

type Report struct {
	TS       int64       `json:"ts"`
	Summary  Summary     `json:"summary"`
	Findings []Finding   `json:"findings"`
	Deep     *DeepReport `json:"deep,omitempty"`
}

type Options struct {
	Deep bool
}

func Run(cfg config.Config, options Options) Report {
	findings := collectFindings(cfg)
	report := Report{
		TS:       time.Now().UTC().UnixMilli(),
		Findings: findings,
		Summary:  summarize(findings),
	}
	if options.Deep {
		deepGateway := probeGateway(cfg.Gateway.URL)
		deepBridge := probeBrowserBridge(cfg.Runtime.BrowserBridge)
		deepPolicyBundle := probePolicyBundle(cfg.Security.PolicyBundlePath)
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
	return DeepBrowserBridge{
		Attempted: true,
		Endpoint:  endpoint,
		OK:        true,
	}
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
