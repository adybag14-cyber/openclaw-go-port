package runtime

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/wasm/sandbox"
	"github.com/tetratelabs/wazero"
)

type Module struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Description  string   `json:"description"`
	Capabilities []string `json:"capabilities"`
	WIT          string   `json:"wit,omitempty"`
	EntryPoint   string   `json:"entryPoint,omitempty"`
	Engine       string   `json:"engine,omitempty"`
	WasmBase64   string   `json:"wasmBase64,omitempty"`
	WasmPath     string   `json:"wasmPath,omitempty"`
	Binary       []byte   `json:"-"`
}

type Runtime struct {
	mu      sync.RWMutex
	policy  sandbox.Policy
	modules map[string]Module
}

func NewRuntime() *Runtime {
	r := &Runtime{
		policy:  sandbox.DefaultPolicy(),
		modules: map[string]Module{},
	}
	_ = r.InstallModule(Module{
		ID:           "wasm.echo",
		Name:         "WASM Echo",
		Version:      "0.1.0",
		Description:  "Echo payload for runtime checks",
		Capabilities: []string{"compute"},
		EntryPoint:   "main",
		WIT:          "component openclaw:echo@0.1.0",
	})
	_ = r.InstallModule(Module{
		ID:           "wasm.vector.search",
		Name:         "WASM Vector Search",
		Version:      "0.1.0",
		Description:  "Vector index query helper",
		Capabilities: []string{"compute", "filesystem"},
		EntryPoint:   "search",
		WIT:          "component openclaw:vector_search@0.1.0",
	})
	_ = r.InstallModule(Module{
		ID:           "wasm.vision.inspect",
		Name:         "WASM Vision Inspect",
		Version:      "0.1.0",
		Description:  "Image inspection runtime helper",
		Capabilities: []string{"compute"},
		EntryPoint:   "inspect",
		WIT:          "component openclaw:vision_inspect@0.1.0",
	})
	return r
}

func (r *Runtime) MarketplaceList() []Module {
	r.mu.RLock()
	out := make([]Module, 0, len(r.modules))
	for _, module := range r.modules {
		out = append(out, module)
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func (r *Runtime) InstallModule(module Module) error {
	normalized, err := normalizeModule(module)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.modules[normalized.ID] = normalized
	r.mu.Unlock()
	return nil
}

func (r *Runtime) RemoveModule(moduleID string) bool {
	id := strings.ToLower(strings.TrimSpace(moduleID))
	if id == "" {
		return false
	}
	r.mu.Lock()
	_, ok := r.modules[id]
	if ok {
		delete(r.modules, id)
	}
	r.mu.Unlock()
	return ok
}

func (r *Runtime) SetPolicy(policy sandbox.Policy) {
	r.mu.Lock()
	r.policy = policy
	r.mu.Unlock()
}

func (r *Runtime) Execute(ctx context.Context, moduleID string, input map[string]any) (map[string]any, error) {
	return r.execute(ctx, moduleID, input)
}

func (r *Runtime) execute(ctx context.Context, moduleID string, input map[string]any) (map[string]any, error) {
	id := strings.ToLower(strings.TrimSpace(moduleID))
	r.mu.RLock()
	module, ok := r.modules[id]
	policy := r.policy
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("wasm module %q not found", moduleID)
	}

	timeoutMs := toInt(input["timeoutMs"], policy.MaxDurationMS)
	memoryMB := toInt(input["memoryMB"], policy.MaxMemoryMB)
	if timeoutMs > policy.MaxDurationMS {
		return nil, fmt.Errorf("sandbox denied module %s: timeout exceeds policy limit (%d > %d)", module.ID, timeoutMs, policy.MaxDurationMS)
	}
	if memoryMB > policy.MaxMemoryMB {
		return nil, fmt.Errorf("sandbox denied module %s: memory exceeds policy limit (%d > %d)", module.ID, memoryMB, policy.MaxMemoryMB)
	}

	requiredCapabilities := uniqueLowerCapabilities(module.Capabilities, toStringSlice(input["requiredCapabilities"]))
	decision := policy.EvaluateCapabilities(requiredCapabilities)
	if !decision.Allowed {
		return nil, fmt.Errorf("sandbox denied module %s: %s", module.ID, decision.Reason)
	}

	var output map[string]any
	var err error
	if len(module.Binary) > 0 {
		output, err = executeCompiledModule(ctx, module, input, timeoutMs)
		if err != nil {
			return nil, fmt.Errorf("wasm module %s execution failed: %w", module.ID, err)
		}
	} else {
		output = map[string]any{
			"engine": "synthetic",
			"echo":   input,
		}
	}

	return map[string]any{
		"module":    module.ID,
		"status":    "completed",
		"startedAt": time.Now().UTC().Format(time.RFC3339),
		"engine":    module.Engine,
		"limits": map[string]any{
			"timeoutMs": timeoutMs,
			"memoryMB":  memoryMB,
		},
		"capabilities": requiredCapabilities,
		"output":       output,
	}, nil
}

func (r *Runtime) Policy() sandbox.Policy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.policy
}

func normalizeModule(module Module) (Module, error) {
	id := strings.ToLower(strings.TrimSpace(module.ID))
	if id == "" {
		return Module{}, fmt.Errorf("module id is required")
	}
	name := strings.TrimSpace(module.Name)
	if name == "" {
		name = id
	}
	version := strings.TrimSpace(module.Version)
	if version == "" {
		version = "0.1.0"
	}
	desc := strings.TrimSpace(module.Description)
	entryPoint := strings.TrimSpace(module.EntryPoint)
	if entryPoint == "" {
		entryPoint = "main"
	}
	wit := strings.TrimSpace(module.WIT)
	engine := strings.ToLower(strings.TrimSpace(module.Engine))
	binary, err := resolveModuleBinary(module)
	if err != nil {
		return Module{}, err
	}
	if engine == "" {
		if len(binary) > 0 {
			engine = "wazero"
		} else {
			engine = "synthetic"
		}
	}
	normalizedCaps := uniqueLowerCapabilities(module.Capabilities, nil)

	return Module{
		ID:           id,
		Name:         name,
		Version:      version,
		Description:  desc,
		Capabilities: normalizedCaps,
		WIT:          wit,
		EntryPoint:   entryPoint,
		Engine:       engine,
		WasmPath:     strings.TrimSpace(module.WasmPath),
		Binary:       binary,
	}, nil
}

func uniqueLowerCapabilities(primary []string, secondary []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(primary)+len(secondary))
	appendCap := func(cap string) {
		normalized := strings.ToLower(strings.TrimSpace(cap))
		if normalized == "" {
			return
		}
		if _, ok := seen[normalized]; ok {
			return
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	for _, cap := range primary {
		appendCap(cap)
	}
	for _, cap := range secondary {
		appendCap(cap)
	}
	return out
}

func toInt(value any, fallback int) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return fallback
	}
}

func toStringSlice(value any) []string {
	switch v := value.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, entry := range v {
			if trimmed := strings.TrimSpace(entry); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(v))
		for _, entry := range v {
			if text, ok := entry.(string); ok {
				if trimmed := strings.TrimSpace(text); trimmed != "" {
					out = append(out, trimmed)
				}
			}
		}
		return out
	default:
		return []string{}
	}
}

func resolveModuleBinary(module Module) ([]byte, error) {
	if len(module.Binary) > 0 {
		return append([]byte(nil), module.Binary...), nil
	}
	if encoded := strings.TrimSpace(module.WasmBase64); encoded != "" {
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("invalid wasmBase64 for module %q: %w", module.ID, err)
		}
		return decoded, nil
	}
	if path := strings.TrimSpace(module.WasmPath); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed reading wasmPath for module %q: %w", module.ID, err)
		}
		return raw, nil
	}
	return nil, nil
}

func executeCompiledModule(ctx context.Context, module Module, input map[string]any, timeoutMs int) (map[string]any, error) {
	callCtx := ctx
	cancel := func() {}
	if timeoutMs > 0 {
		callCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	}
	defer cancel()

	rt := wazero.NewRuntime(callCtx)
	defer func() {
		_ = rt.Close(callCtx)
	}()

	compiled, err := rt.CompileModule(callCtx, module.Binary)
	if err != nil {
		return nil, fmt.Errorf("compile failed: %w", err)
	}
	inst, err := rt.InstantiateModule(callCtx, compiled, wazero.NewModuleConfig())
	if err != nil {
		return nil, fmt.Errorf("instantiate failed: %w", err)
	}

	entry := strings.TrimSpace(module.EntryPoint)
	if entry == "" {
		entry = "main"
	}
	fn := inst.ExportedFunction(entry)
	if fn == nil {
		return nil, fmt.Errorf("exported function %q not found", entry)
	}

	args, err := parseWasmArgs(input["args"])
	if err != nil {
		return nil, err
	}

	results, err := fn.Call(callCtx, args...)
	if err != nil {
		return nil, err
	}
	converted := make([]uint64, len(results))
	for idx, value := range results {
		converted[idx] = value
	}

	output := map[string]any{
		"engine":   "wazero",
		"function": entry,
		"results":  converted,
	}
	if len(converted) == 1 {
		output["result"] = converted[0]
	}
	return output, nil
}

func parseWasmArgs(raw any) ([]uint64, error) {
	if raw == nil {
		return []uint64{}, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("wasm args must be an array")
	}
	out := make([]uint64, 0, len(items))
	for _, item := range items {
		switch value := item.(type) {
		case uint64:
			out = append(out, value)
		case int:
			if value < 0 {
				return nil, fmt.Errorf("wasm args cannot contain negative integers")
			}
			out = append(out, uint64(value))
		case int64:
			if value < 0 {
				return nil, fmt.Errorf("wasm args cannot contain negative integers")
			}
			out = append(out, uint64(value))
		case float64:
			if value < 0 {
				return nil, fmt.Errorf("wasm args cannot contain negative numbers")
			}
			out = append(out, uint64(value))
		default:
			return nil, fmt.Errorf("unsupported wasm arg type %T", item)
		}
	}
	return out, nil
}
