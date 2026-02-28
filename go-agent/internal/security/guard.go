package security

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/config"
)

type Action string

const (
	ActionAllow  Action = "allow"
	ActionReview Action = "review"
	ActionBlock  Action = "block"
)

type Decision struct {
	Action Action `json:"action"`
	Reason string `json:"reason,omitempty"`
}

type Guard struct {
	defaultAction  Action
	toolPolicies   map[string]Action
	blockedPhrases []string
	lastError      string
}

type policyBundle struct {
	DefaultAction          string            `json:"default_action"`
	ToolPolicies           map[string]string `json:"tool_policies"`
	BlockedMessagePatterns []string          `json:"blocked_message_patterns"`
}

func NewGuard(cfg config.SecurityConfig) *Guard {
	g := &Guard{
		defaultAction:  parseAction(cfg.DefaultAction),
		toolPolicies:   map[string]Action{},
		blockedPhrases: normalizePhrases(cfg.BlockedMessagePatterns),
		lastError:      "",
	}
	if g.defaultAction == "" {
		g.defaultAction = ActionAllow
	}
	for method, policy := range cfg.ToolPolicies {
		action := parseAction(policy)
		if action == "" {
			continue
		}
		g.toolPolicies[normalizeMethod(method)] = action
	}
	g.loadBundle(cfg.PolicyBundlePath)
	return g
}

func (g *Guard) Evaluate(method string, params map[string]any) Decision {
	canonical := normalizeMethod(method)

	if action, ok := g.toolPolicies[canonical]; ok {
		if action == ActionBlock {
			return Decision{Action: ActionBlock, Reason: "blocked by tool policy"}
		}
		if action == ActionReview {
			return Decision{Action: ActionReview, Reason: "flagged by tool policy"}
		}
	}

	message := firstNonEmptyString(params, "message", "text", "prompt", "command")
	if message != "" {
		lower := strings.ToLower(message)
		for _, phrase := range g.blockedPhrases {
			if strings.Contains(lower, phrase) {
				return Decision{
					Action: ActionBlock,
					Reason: "blocked by unsafe message pattern",
				}
			}
		}
	}

	switch g.defaultAction {
	case ActionReview:
		return Decision{Action: ActionReview, Reason: "default review policy"}
	case ActionBlock:
		return Decision{Action: ActionBlock, Reason: "default block policy"}
	default:
		return Decision{Action: ActionAllow}
	}
}

func (g *Guard) Snapshot() map[string]any {
	toolPolicies := make(map[string]string, len(g.toolPolicies))
	for method, action := range g.toolPolicies {
		toolPolicies[method] = string(action)
	}
	return map[string]any{
		"defaultAction":   g.defaultAction,
		"toolPolicies":    toolPolicies,
		"blockedPatterns": g.blockedPhrases,
		"lastError":       g.lastError,
	}
}

func (g *Guard) loadBundle(path string) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" || strings.HasPrefix(strings.ToLower(trimmed), "memory://") {
		return
	}

	raw, err := os.ReadFile(trimmed)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		g.lastError = err.Error()
		return
	}
	var bundle policyBundle
	if err := json.Unmarshal(raw, &bundle); err != nil {
		g.lastError = err.Error()
		return
	}
	if action := parseAction(bundle.DefaultAction); action != "" {
		g.defaultAction = action
	}
	for method, policy := range bundle.ToolPolicies {
		if action := parseAction(policy); action != "" {
			g.toolPolicies[normalizeMethod(method)] = action
		}
	}
	if len(bundle.BlockedMessagePatterns) > 0 {
		g.blockedPhrases = append(g.blockedPhrases, normalizePhrases(bundle.BlockedMessagePatterns)...)
	}
}

func parseAction(raw string) Action {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "allow":
		return ActionAllow
	case "review":
		return ActionReview
	case "block":
		return ActionBlock
	default:
		return ""
	}
}

func normalizeMethod(method string) string {
	return strings.ToLower(strings.TrimSpace(method))
}

func normalizePhrases(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.ToLower(strings.TrimSpace(item))
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func firstNonEmptyString(params map[string]any, keys ...string) string {
	for _, key := range keys {
		raw, ok := params[key]
		if !ok {
			continue
		}
		if value, ok := raw.(string); ok {
			trimmed := strings.TrimSpace(value)
			if trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}
