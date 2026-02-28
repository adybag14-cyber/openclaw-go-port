package security

import (
	"encoding/json"
	"os"
	"strings"
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

type GuardConfig struct {
	PolicyBundlePath        string
	DefaultAction           string
	ToolPolicies            map[string]string
	BlockedMessagePatterns  []string
	TelemetryHighRiskTags   []string
	TelemetryAction         string
	CredentialSensitiveKeys []string
	CredentialLeakAction    string
}

type Guard struct {
	defaultAction    Action
	toolPolicies     map[string]Action
	blockedPhrases   []string
	telemetryTags    map[string]struct{}
	telemetryAction  Action
	sensitiveKeys    map[string]struct{}
	credentialAction Action
	lastError        string
}

type policyBundle struct {
	DefaultAction           string            `json:"default_action"`
	ToolPolicies            map[string]string `json:"tool_policies"`
	BlockedMessagePatterns  []string          `json:"blocked_message_patterns"`
	TelemetryHighRiskTags   []string          `json:"telemetry_high_risk_tags"`
	TelemetryAction         string            `json:"telemetry_action"`
	CredentialSensitiveKeys []string          `json:"credential_sensitive_keys"`
	CredentialLeakAction    string            `json:"credential_leak_action"`
}

func NewGuard(cfg GuardConfig) *Guard {
	g := &Guard{
		defaultAction:    parseAction(cfg.DefaultAction),
		toolPolicies:     map[string]Action{},
		blockedPhrases:   normalizePhrases(cfg.BlockedMessagePatterns),
		telemetryTags:    normalizeSet(cfg.TelemetryHighRiskTags),
		telemetryAction:  parseAction(cfg.TelemetryAction),
		sensitiveKeys:    normalizeSet(cfg.CredentialSensitiveKeys),
		credentialAction: parseAction(cfg.CredentialLeakAction),
		lastError:        "",
	}
	if g.defaultAction == "" {
		g.defaultAction = ActionAllow
	}
	if g.telemetryAction == "" {
		g.telemetryAction = ActionReview
	}
	if g.credentialAction == "" {
		g.credentialAction = ActionBlock
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
		switch action {
		case ActionBlock:
			return Decision{Action: ActionBlock, Reason: "blocked by tool policy"}
		case ActionReview:
			return Decision{Action: ActionReview, Reason: "flagged by tool policy"}
		}
	}

	if credentialLeakScanEnabled(canonical) && g.containsCredentialLeak(params) {
		switch g.credentialAction {
		case ActionBlock:
			return Decision{Action: ActionBlock, Reason: "blocked by credential leak policy"}
		case ActionReview:
			return Decision{Action: ActionReview, Reason: "credential leak requires review"}
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
		if looksLikeCredential(lower) {
			switch g.credentialAction {
			case ActionBlock:
				return Decision{Action: ActionBlock, Reason: "blocked by credential content heuristic"}
			case ActionReview:
				return Decision{Action: ActionReview, Reason: "credential content requires review"}
			}
		}
	}

	if g.containsTelemetryRisk(params) {
		switch g.telemetryAction {
		case ActionBlock:
			return Decision{Action: ActionBlock, Reason: "blocked by telemetry high-risk tag"}
		case ActionReview:
			return Decision{Action: ActionReview, Reason: "telemetry high-risk tag requires review"}
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
		"defaultAction":           g.defaultAction,
		"toolPolicies":            toolPolicies,
		"blockedPatterns":         g.blockedPhrases,
		"telemetryAction":         g.telemetryAction,
		"telemetryTags":           keys(g.telemetryTags),
		"credentialLeakAction":    g.credentialAction,
		"credentialSensitiveKeys": keys(g.sensitiveKeys),
		"lastError":               g.lastError,
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
	if action := parseAction(bundle.TelemetryAction); action != "" {
		g.telemetryAction = action
	}
	for tag := range normalizeSet(bundle.TelemetryHighRiskTags) {
		g.telemetryTags[tag] = struct{}{}
	}
	if action := parseAction(bundle.CredentialLeakAction); action != "" {
		g.credentialAction = action
	}
	for key := range normalizeSet(bundle.CredentialSensitiveKeys) {
		g.sensitiveKeys[key] = struct{}{}
	}
}

func (g *Guard) containsTelemetryRisk(params map[string]any) bool {
	if len(g.telemetryTags) == 0 {
		return false
	}
	raw, ok := params["telemetryTags"]
	if !ok {
		raw = params["tags"]
	}
	for _, tag := range flattenStrings(raw) {
		if _, match := g.telemetryTags[strings.ToLower(strings.TrimSpace(tag))]; match {
			return true
		}
	}
	return false
}

func (g *Guard) containsCredentialLeak(params map[string]any) bool {
	if len(g.sensitiveKeys) == 0 {
		return false
	}
	return walkParamsForSensitive(params, g.sensitiveKeys)
}

func walkParamsForSensitive(input any, sensitiveKeys map[string]struct{}) bool {
	switch value := input.(type) {
	case map[string]any:
		for key, entry := range value {
			lowerKey := strings.ToLower(strings.TrimSpace(key))
			if _, sensitive := sensitiveKeys[lowerKey]; sensitive {
				if str, ok := entry.(string); ok && strings.TrimSpace(str) != "" {
					return true
				}
			}
			if walkParamsForSensitive(entry, sensitiveKeys) {
				return true
			}
		}
	case []any:
		for _, entry := range value {
			if walkParamsForSensitive(entry, sensitiveKeys) {
				return true
			}
		}
	}
	return false
}

func credentialLeakScanEnabled(method string) bool {
	switch method {
	case "connect",
		"web.login.start",
		"web.login.wait",
		"auth.oauth.start",
		"auth.oauth.wait",
		"auth.oauth.complete",
		"auth.oauth.logout":
		return false
	default:
		return true
	}
}

func looksLikeCredential(lower string) bool {
	if strings.Contains(lower, "sk-") {
		return true
	}
	if strings.Contains(lower, "ghp_") {
		return true
	}
	if strings.Contains(lower, "akia") {
		return true
	}
	if strings.Contains(lower, "xoxb-") {
		return true
	}
	return false
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

func normalizeSet(items []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, item := range items {
		trimmed := strings.ToLower(strings.TrimSpace(item))
		if trimmed == "" {
			continue
		}
		out[trimmed] = struct{}{}
	}
	return out
}

func keys(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	return out
}

func flattenStrings(raw any) []string {
	switch value := raw.(type) {
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if str, ok := item.(string); ok {
				out = append(out, str)
			}
		}
		return out
	case []string:
		return value
	case string:
		return []string{value}
	default:
		return []string{}
	}
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
