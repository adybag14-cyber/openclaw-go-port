package app

import (
	"net"
	"net/url"
	"os"
	osexec "os/exec"
	"strconv"
	"strings"

	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/config"
	securityaudit "github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/security/audit"
)

type doctorCheck struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

func buildDoctorChecks(cfg config.Config, report securityaudit.Report) []doctorCheck {
	checks := make([]doctorCheck, 0, 12)
	authMode := strings.ToLower(strings.TrimSpace(cfg.Gateway.Server.AuthMode))
	tokenSet := strings.TrimSpace(cfg.Gateway.Token) != ""
	passwordSet := strings.TrimSpace(cfg.Gateway.Password) != ""
	authOK := true
	authMessage := authMode
	authDetail := "not required"
	switch authMode {
	case "token":
		authOK = tokenSet
		authDetail = "gateway.token is required when auth_mode=token"
	case "password":
		authOK = passwordSet
		authDetail = "gateway.password is required when auth_mode=password"
	case "auto":
		authMessage = "auto"
		authDetail = "auto mode derives from gateway.token or gateway.password"
	case "", "none":
		authMessage = "none"
		authDetail = "auth disabled"
	default:
		authOK = false
		authDetail = "unsupported gateway auth mode"
	}
	checks = append(checks, doctorCheck{
		ID:      "gateway.auth_secret",
		Status:  passFail(authOK),
		Message: authMessage,
		Detail:  authDetail,
	})

	checks = append(checks, doctorCheck{
		ID:      "gateway.bind_scope",
		Status:  passWarn(isLoopbackBind(cfg.Gateway.Server.Bind)),
		Message: strings.TrimSpace(cfg.Gateway.Server.Bind),
		Detail:  "prefer loopback bind for control endpoints",
	})
	checks = append(checks, doctorCheck{
		ID:      "gateway.http_bind_scope",
		Status:  passWarn(isLoopbackBind(cfg.Gateway.Server.HTTPBind)),
		Message: strings.TrimSpace(cfg.Gateway.Server.HTTPBind),
		Detail:  "prefer loopback bind for control endpoints",
	})

	bridgeEndpoint := strings.TrimSpace(cfg.Runtime.BrowserBridge.Endpoint)
	bridgeLoopback := endpointLoopback(bridgeEndpoint)
	bridgeStatus := "pass"
	bridgeDetail := "browser bridge enabled and loopback-scoped"
	if !cfg.Runtime.BrowserBridge.Enabled {
		bridgeStatus = "warn"
		bridgeDetail = "browser bridge disabled"
	} else if bridgeEndpoint == "" {
		bridgeStatus = "fail"
		bridgeDetail = "browser bridge endpoint is required when enabled"
	} else if !bridgeLoopback {
		bridgeStatus = "warn"
		bridgeDetail = "browser bridge endpoint is non-loopback"
	}
	checks = append(checks, doctorCheck{
		ID:      "runtime.browser_bridge",
		Status:  bridgeStatus,
		Message: bridgeEndpoint,
		Detail:  bridgeDetail,
	})

	statePath := strings.TrimSpace(cfg.Runtime.StatePath)
	stateStatus := "pass"
	stateDetail := "runtime state path is persisted"
	if strings.HasPrefix(strings.ToLower(statePath), "memory://") {
		stateStatus = "warn"
		stateDetail = "runtime state path is in-memory and non-persistent"
	}
	checks = append(checks, doctorCheck{
		ID:      "runtime.state_path",
		Status:  stateStatus,
		Message: statePath,
		Detail:  stateDetail,
	})

	policyPath := strings.TrimSpace(cfg.Security.PolicyBundlePath)
	policyStatus := "pass"
	policyDetail := "persisted policy bundle configured"
	switch {
	case policyPath == "" || strings.HasPrefix(strings.ToLower(policyPath), "memory://"):
		policyStatus = "warn"
		policyDetail = "policy bundle is unset or memory-backed"
	default:
		info, err := os.Stat(policyPath)
		if err != nil {
			policyStatus = "warn"
			policyDetail = err.Error()
		} else if info.IsDir() {
			policyStatus = "warn"
			policyDetail = "policy bundle path points to a directory"
		}
	}
	checks = append(checks, doctorCheck{
		ID:      "security.policy_bundle",
		Status:  policyStatus,
		Message: policyPath,
		Detail:  policyDetail,
	})

	checks = append(checks, doctorCheck{
		ID:      "security.loop_guard",
		Status:  passWarn(cfg.Security.LoopGuardEnabled),
		Message: boolLabel(cfg.Security.LoopGuardEnabled),
		Detail:  "loop guard protects against repetitive tool loops",
	})
	checks = append(checks, doctorCheck{
		ID:      "security.risk_thresholds",
		Status:  passWarn(cfg.Security.RiskReviewThreshold >= 40 && cfg.Security.RiskBlockThreshold >= 60),
		Message: "review=" + intString(cfg.Security.RiskReviewThreshold) + " block=" + intString(cfg.Security.RiskBlockThreshold),
		Detail:  "higher thresholds reduce permissive risk posture",
	})
	checks = append(checks, doctorCheck{
		ID:      "security.audit.summary",
		Status:  auditStatus(report),
		Message: "critical=" + intString(report.Summary.Critical) + " warn=" + intString(report.Summary.Warn) + " info=" + intString(report.Summary.Info),
		Detail:  "derived from security.audit findings",
	})

	if report.Deep != nil {
		checks = append(checks, doctorCheck{
			ID:      "security.deep.gateway",
			Status:  passWarn(report.Deep.Gateway.OK),
			Message: report.Deep.Gateway.URL,
			Detail:  detailOr(report.Deep.Gateway.Error, "gateway deep probe"),
		})
		checks = append(checks, doctorCheck{
			ID:      "security.deep.browser_bridge",
			Status:  passWarn(report.Deep.BrowserBridge.OK || !report.Deep.BrowserBridge.Attempted),
			Message: report.Deep.BrowserBridge.Endpoint,
			Detail:  detailOr(report.Deep.BrowserBridge.Error, "browser bridge deep probe"),
		})
		checks = append(checks, doctorCheck{
			ID:      "security.deep.policy_bundle",
			Status:  passWarn(report.Deep.PolicyBundle.ParseOK || !report.Deep.PolicyBundle.Attempted),
			Message: report.Deep.PolicyBundle.Path,
			Detail:  detailOr(report.Deep.PolicyBundle.Error, "policy bundle deep probe"),
		})
	}

	dockerAvailable := commandAvailable("docker")
	wasmtimeAvailable := commandAvailable("wasmtime")
	checks = append(checks, doctorCheck{
		ID:      "docker.binary",
		Status:  passWarn(dockerAvailable),
		Message: boolLabel(dockerAvailable),
		Detail:  "docker is used by parity validation workflows",
	})
	checks = append(checks, doctorCheck{
		ID:      "wasmtime.binary",
		Status:  passWarn(wasmtimeAvailable),
		Message: boolLabel(wasmtimeAvailable),
		Detail:  "optional wasm inspection utility",
	})

	return checks
}

func commandAvailable(name string) bool {
	return osexec.Command(name, "--version").Run() == nil
}

func passWarn(ok bool) string {
	if ok {
		return "pass"
	}
	return "warn"
}

func passFail(ok bool) string {
	if ok {
		return "pass"
	}
	return "fail"
}

func auditStatus(report securityaudit.Report) string {
	if report.Summary.Critical > 0 {
		return "fail"
	}
	if report.Summary.Warn > 0 {
		return "warn"
	}
	return "pass"
}

func boolLabel(ok bool) string {
	if ok {
		return "enabled"
	}
	return "disabled"
}

func endpointLoopback(raw string) bool {
	host := strings.TrimSpace(raw)
	if host == "" {
		return false
	}
	if strings.Contains(host, "://") {
		parsed, err := url.Parse(host)
		if err != nil {
			return false
		}
		host = parsed.Hostname()
	} else if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	return isLoopbackHost(host)
}

func isLoopbackBind(bind string) bool {
	trimmed := strings.TrimSpace(bind)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, ":") {
		return false
	}
	if parsedHost, _, err := net.SplitHostPort(trimmed); err == nil {
		return isLoopbackHost(parsedHost)
	}
	return isLoopbackHost(trimmed)
}

func isLoopbackHost(host string) bool {
	normalized := strings.ToLower(strings.Trim(strings.TrimSpace(host), "[]"))
	switch normalized {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	ip := net.ParseIP(normalized)
	return ip != nil && ip.IsLoopback()
}

func detailOr(raw string, fallback string) string {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	return raw
}

func intString(v int) string {
	return strconv.Itoa(v)
}
