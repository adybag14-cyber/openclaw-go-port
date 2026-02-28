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
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
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
	for _, capability := range capabilities {
		switch strings.ToLower(strings.TrimSpace(capability)) {
		case "network":
			if !p.AllowNetwork {
				return Decision{Allowed: false, Reason: "network capability denied by sandbox policy"}
			}
		case "filesystem":
			if !p.AllowFilesystem {
				return Decision{Allowed: false, Reason: "filesystem capability denied by sandbox policy"}
			}
		}
	}
	return Decision{Allowed: true}
}
