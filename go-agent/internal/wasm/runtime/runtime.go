package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/wasm/sandbox"
)

type Module struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Description  string   `json:"description"`
	Capabilities []string `json:"capabilities"`
}

type Runtime struct {
	policy  sandbox.Policy
	modules map[string]Module
}

func NewRuntime() *Runtime {
	r := &Runtime{
		policy:  sandbox.DefaultPolicy(),
		modules: map[string]Module{},
	}
	r.modules["wasm.echo"] = Module{
		ID:           "wasm.echo",
		Name:         "WASM Echo",
		Version:      "0.1.0",
		Description:  "Echo payload for runtime checks",
		Capabilities: []string{"compute"},
	}
	r.modules["wasm.vector.search"] = Module{
		ID:           "wasm.vector.search",
		Name:         "WASM Vector Search",
		Version:      "0.1.0",
		Description:  "Vector index query helper",
		Capabilities: []string{"compute", "filesystem"},
	}
	r.modules["wasm.vision.inspect"] = Module{
		ID:           "wasm.vision.inspect",
		Name:         "WASM Vision Inspect",
		Version:      "0.1.0",
		Description:  "Image inspection runtime helper",
		Capabilities: []string{"compute"},
	}
	return r
}

func (r *Runtime) MarketplaceList() []Module {
	out := make([]Module, 0, len(r.modules))
	for _, module := range r.modules {
		out = append(out, module)
	}
	return out
}

func (r *Runtime) Execute(_ context.Context, moduleID string, input map[string]any) (map[string]any, error) {
	id := strings.ToLower(strings.TrimSpace(moduleID))
	module, ok := r.modules[id]
	if !ok {
		return nil, fmt.Errorf("wasm module %q not found", moduleID)
	}
	decision := r.policy.EvaluateCapabilities(module.Capabilities)
	if !decision.Allowed {
		return nil, fmt.Errorf("sandbox denied module %s: %s", module.ID, decision.Reason)
	}
	return map[string]any{
		"module":    module.ID,
		"status":    "completed",
		"startedAt": time.Now().UTC().Format(time.RFC3339),
		"output": map[string]any{
			"echo": input,
		},
	}, nil
}

func (r *Runtime) Policy() sandbox.Policy {
	return r.policy
}
