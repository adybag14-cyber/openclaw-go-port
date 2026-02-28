package sandbox

import (
	"strings"
	"time"
)

type Policy struct {
	MaxMemoryMB     int  `json:"maxMemoryMB"`
	MaxDurationMS   int  `json:"maxDurationMs"`
	AllowNetwork    bool `json:"allowNetwork"`
	AllowFilesystem bool `json:"allowFilesystem"`
}

type Decision struct {
	Allowed            bool     `json:"allowed"`
	Reason             string   `json:"reason,omitempty"`
	DeniedCapabilities []string `json:"deniedCapabilities,omitempty"`
}

func DefaultPolicy() Policy {
	return Policy{
		MaxMemoryMB:     256,
		MaxDurationMS:   int((5 * time.Second).Milliseconds()),
		AllowNetwork:    false,
		AllowFilesystem: false,
	}
}

func (p Policy) EvaluateCapabilities(capabilities []string) Decision {
	denied := make([]string, 0, 2)
	for _, capability := range capabilities {
		switch strings.ToLower(strings.TrimSpace(capability)) {
		case "network":
			if !p.AllowNetwork {
				denied = append(denied, "network")
			}
		case "filesystem":
			if !p.AllowFilesystem {
				denied = append(denied, "filesystem")
			}
		}
	}
	if len(denied) > 0 {
		return Decision{
			Allowed:            false,
			Reason:             "one or more capabilities denied by sandbox policy",
			DeniedCapabilities: denied,
		}
	}
	return Decision{Allowed: true}
}
