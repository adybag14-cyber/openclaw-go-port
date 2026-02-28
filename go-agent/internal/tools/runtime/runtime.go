package runtime

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type Request struct {
	Tool      string         `json:"tool"`
	SessionID string         `json:"session_id,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
}

type InvokeResult struct {
	Provider string `json:"provider"`
	Output   any    `json:"output"`
}

type Provider interface {
	Name() string
	Supports(tool string) bool
	Invoke(ctx context.Context, req Request) (any, error)
	Catalog() []ToolSpec
}

type ToolSpec struct {
	Tool        string `json:"tool"`
	Provider    string `json:"provider"`
	Description string `json:"description"`
}

type Runtime struct {
	providers []Provider
}

func New() *Runtime {
	return &Runtime{
		providers: make([]Provider, 0, 4),
	}
}

func NewDefault() *Runtime {
	rt := New()
	rt.RegisterProvider(NewBuiltinBridgeProvider())
	return rt
}

func (r *Runtime) RegisterProvider(provider Provider) {
	r.providers = append(r.providers, provider)
}

func (r *Runtime) Invoke(ctx context.Context, req Request) (InvokeResult, error) {
	normalizedTool := strings.ToLower(strings.TrimSpace(req.Tool))
	if normalizedTool == "" {
		return InvokeResult{}, errors.New("tool name is required")
	}
	req.Tool = normalizedTool
	for _, provider := range r.providers {
		if !provider.Supports(req.Tool) {
			continue
		}
		output, err := provider.Invoke(ctx, req)
		if err != nil {
			return InvokeResult{}, err
		}
		return InvokeResult{
			Provider: provider.Name(),
			Output:   output,
		}, nil
	}
	return InvokeResult{}, fmt.Errorf("no provider for tool %q", req.Tool)
}

func (r *Runtime) Catalog() []ToolSpec {
	var out []ToolSpec
	for _, provider := range r.providers {
		out = append(out, provider.Catalog()...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Tool == out[j].Tool {
			return out[i].Provider < out[j].Provider
		}
		return out[i].Tool < out[j].Tool
	})
	return out
}

type BuiltinBridgeProvider struct{}

func NewBuiltinBridgeProvider() *BuiltinBridgeProvider {
	return &BuiltinBridgeProvider{}
}

func (b *BuiltinBridgeProvider) Name() string {
	return "builtin-bridge"
}

func (b *BuiltinBridgeProvider) Supports(tool string) bool {
	switch strings.ToLower(strings.TrimSpace(tool)) {
	case "browser.request", "browser.open", "tool.echo":
		return true
	default:
		return false
	}
}

func (b *BuiltinBridgeProvider) Invoke(_ context.Context, req Request) (any, error) {
	switch req.Tool {
	case "browser.request":
		url := toString(req.Input["url"], "")
		method := strings.ToUpper(toString(req.Input["method"], "GET"))
		return map[string]any{
			"status":   200,
			"ok":       true,
			"url":      url,
			"method":   method,
			"response": "bridge request accepted",
		}, nil
	case "browser.open":
		url := toString(req.Input["url"], "")
		return map[string]any{
			"status": 200,
			"ok":     true,
			"url":    url,
			"opened": true,
		}, nil
	case "tool.echo":
		return map[string]any{
			"status": 200,
			"ok":     true,
			"echo":   req.Input,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported builtin tool %q", req.Tool)
	}
}

func (b *BuiltinBridgeProvider) Catalog() []ToolSpec {
	return []ToolSpec{
		{Tool: "browser.open", Provider: b.Name(), Description: "Open browser URL through bridge runtime"},
		{Tool: "browser.request", Provider: b.Name(), Description: "Send browser bridge HTTP-like request"},
		{Tool: "tool.echo", Provider: b.Name(), Description: "Echo request payload for smoke validation"},
	}
}

func toString(value any, fallback string) string {
	raw, ok := value.(string)
	if !ok {
		return fallback
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}
