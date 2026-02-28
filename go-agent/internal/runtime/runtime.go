package runtime

import (
	"strings"

	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/config"
)

type Profile string

const (
	ProfileCore Profile = "core"
	ProfileEdge Profile = "edge"
)

type Runtime struct {
	auditOnly bool
	statePath string
	profile   Profile
}

func New(cfg config.RuntimeConfig) *Runtime {
	return &Runtime{
		auditOnly: cfg.AuditOnly,
		statePath: strings.TrimSpace(cfg.StatePath),
		profile:   normalizeProfile(cfg.Profile),
	}
}

func (r *Runtime) Snapshot() map[string]any {
	mode := "enforcing"
	if r.auditOnly {
		mode = "audit-only"
	}
	return map[string]any{
		"auditOnly": r.auditOnly,
		"statePath": r.statePath,
		"profile":   string(r.profile),
		"mode":      mode,
	}
}

func (r *Runtime) Profile() Profile {
	return r.profile
}

func (r *Runtime) AuditOnly() bool {
	return r.auditOnly
}

func normalizeProfile(raw string) Profile {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(ProfileEdge):
		return ProfileEdge
	default:
		return ProfileCore
	}
}
