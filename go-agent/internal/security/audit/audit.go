package audit

import (
	"errors"
	"net"
	"net/url"
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

type DeepReport struct {
	Gateway DeepGateway `json:"gateway"`
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
		deep := DeepReport{
			Gateway: probeGateway(cfg.Gateway.URL),
		}
		if !deep.Gateway.OK {
			report.Findings = append(report.Findings, Finding{
				CheckID:  "gateway.deep_probe",
				Severity: SeverityWarn,
				Title:    "Deep gateway probe failed",
				Detail:   deep.Gateway.Error,
			})
			report.Summary = summarize(report.Findings)
		}
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

	if cfg.Runtime.AuditOnly {
		findings = append(findings, Finding{
			CheckID:     "runtime.audit_only.enabled",
			Severity:    SeverityWarn,
			Title:       "Runtime is in audit-only mode",
			Detail:      "runtime.audit_only=true weakens enforcement posture",
			Remediation: "set runtime.audit_only=false in production",
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
	host, port, err := parseGatewayHostPort(trimmed)
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

func parseGatewayHostPort(raw string) (string, string, error) {
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
