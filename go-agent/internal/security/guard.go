package security

import (
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Action string

const (
	ActionAllow  Action = "allow"
	ActionReview Action = "review"
	ActionBlock  Action = "block"
)

type Decision struct {
	Action    Action   `json:"action"`
	Reason    string   `json:"reason,omitempty"`
	RiskScore int      `json:"riskScore,omitempty"`
	Signals   []string `json:"signals,omitempty"`
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
	LoopGuardEnabled        bool
	LoopGuardWindowMS       int
	LoopGuardMaxHits        int
	RiskReviewThreshold     int
	RiskBlockThreshold      int
}

type toolPolicyMatcher struct {
	pattern string
	action  Action
}

type Guard struct {
	defaultAction    Action
	toolPolicies     map[string]Action
	toolMatchers     []toolPolicyMatcher
	blockedPhrases   []string
	telemetryTags    map[string]struct{}
	telemetryAction  Action
	sensitiveKeys    map[string]struct{}
	credentialAction Action
	riskReviewAt     int
	riskBlockAt      int
	loopGuard        *ToolLoopGuard
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
	LoopGuardEnabled        *bool             `json:"loop_guard_enabled"`
	LoopGuardWindowMS       *int              `json:"loop_guard_window_ms"`
	LoopGuardMaxHits        *int              `json:"loop_guard_max_hits"`
	RiskReviewThreshold     *int              `json:"risk_review_threshold"`
	RiskBlockThreshold      *int              `json:"risk_block_threshold"`
}

func NewGuard(cfg GuardConfig) *Guard {
	g := &Guard{
		defaultAction:    parseAction(cfg.DefaultAction),
		toolPolicies:     map[string]Action{},
		toolMatchers:     make([]toolPolicyMatcher, 0, 16),
		blockedPhrases:   normalizePhrases(cfg.BlockedMessagePatterns),
		telemetryTags:    normalizeSet(cfg.TelemetryHighRiskTags),
		telemetryAction:  parseAction(cfg.TelemetryAction),
		sensitiveKeys:    normalizeSet(cfg.CredentialSensitiveKeys),
		credentialAction: parseAction(cfg.CredentialLeakAction),
		riskReviewAt:     normalizeThreshold(cfg.RiskReviewThreshold, 70),
		riskBlockAt:      normalizeThreshold(cfg.RiskBlockThreshold, 90),
		loopGuard:        NewToolLoopGuard(time.Duration(cfg.LoopGuardWindowMS)*time.Millisecond, cfg.LoopGuardMaxHits, cfg.LoopGuardEnabled),
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
		g.addToolPolicy(method, action)
	}
	g.loadBundle(cfg.PolicyBundlePath)
	return g
}

func (g *Guard) Evaluate(method string, params map[string]any) Decision {
	canonical := normalizeMethod(method)
	riskScore := 0
	signals := make([]string, 0, 8)

	if action, ok := g.policyActionFor(canonical); ok {
		switch action {
		case ActionBlock:
			return Decision{Action: ActionBlock, Reason: "blocked by tool policy", RiskScore: 100, Signals: []string{"tool_policy:block"}}
		case ActionReview:
			return Decision{Action: ActionReview, Reason: "flagged by tool policy", RiskScore: 80, Signals: []string{"tool_policy:review"}}
		}
	}

	if triggered, hits := g.loopGuard.Register(canonical, params); triggered {
		return Decision{
			Action:    ActionBlock,
			Reason:    "blocked by tool loop guard",
			RiskScore: 100,
			Signals: []string{
				"loop_guard:triggered",
				"loop_guard:hits=" + intString(hits),
			},
		}
	}

	if credentialLeakScanEnabled(canonical) && g.containsCredentialLeak(params) {
		riskScore = maxInt(riskScore, 95)
		signals = append(signals, "credential_leak:key")
		switch g.credentialAction {
		case ActionBlock:
			return Decision{Action: ActionBlock, Reason: "blocked by credential leak policy", RiskScore: riskScore, Signals: signals}
		case ActionReview:
			return Decision{Action: ActionReview, Reason: "credential leak requires review", RiskScore: riskScore, Signals: signals}
		}
	}

	message := firstNonEmptyString(params, "message", "text", "prompt", "command")
	if message != "" {
		lower := strings.ToLower(message)
		for _, phrase := range g.blockedPhrases {
			if strings.Contains(lower, phrase) {
				return Decision{
					Action:    ActionBlock,
					Reason:    "blocked by unsafe message pattern",
					RiskScore: 100,
					Signals:   []string{"blocked_pattern"},
				}
			}
		}
		if looksLikeCredential(lower) {
			riskScore = maxInt(riskScore, 90)
			signals = append(signals, "credential_leak:heuristic")
			switch g.credentialAction {
			case ActionBlock:
				return Decision{Action: ActionBlock, Reason: "blocked by credential content heuristic", RiskScore: riskScore, Signals: signals}
			case ActionReview:
				return Decision{Action: ActionReview, Reason: "credential content requires review", RiskScore: riskScore, Signals: signals}
			}
		}

		messageRisk, messageSignals := analyzePromptRisk(lower)
		if messageRisk > riskScore {
			riskScore = messageRisk
		}
		signals = append(signals, messageSignals...)
	}

	if g.containsTelemetryRisk(params) {
		riskScore = maxInt(riskScore, 85)
		signals = append(signals, "telemetry:high_risk_tag")
		switch g.telemetryAction {
		case ActionBlock:
			return Decision{Action: ActionBlock, Reason: "blocked by telemetry high-risk tag", RiskScore: riskScore, Signals: signals}
		case ActionReview:
			return Decision{Action: ActionReview, Reason: "telemetry high-risk tag requires review", RiskScore: riskScore, Signals: signals}
		}
	}

	if riskScore >= g.riskBlockAt {
		return Decision{
			Action:    ActionBlock,
			Reason:    "blocked by safety risk score",
			RiskScore: riskScore,
			Signals:   dedupeStrings(signals),
		}
	}
	if riskScore >= g.riskReviewAt {
		return Decision{
			Action:    ActionReview,
			Reason:    "review required by safety risk score",
			RiskScore: riskScore,
			Signals:   dedupeStrings(signals),
		}
	}

	switch g.defaultAction {
	case ActionReview:
		return Decision{Action: ActionReview, Reason: "default review policy", RiskScore: riskScore, Signals: dedupeStrings(signals)}
	case ActionBlock:
		return Decision{Action: ActionBlock, Reason: "default block policy", RiskScore: riskScore, Signals: dedupeStrings(signals)}
	default:
		return Decision{Action: ActionAllow, RiskScore: riskScore, Signals: dedupeStrings(signals)}
	}
}

func (g *Guard) Snapshot() map[string]any {
	toolPolicies := make(map[string]string, len(g.toolPolicies))
	for method, action := range g.toolPolicies {
		toolPolicies[method] = string(action)
	}
	for _, matcher := range g.toolMatchers {
		toolPolicies[matcher.pattern] = string(matcher.action)
	}
	return map[string]any{
		"defaultAction":           g.defaultAction,
		"toolPolicies":            toolPolicies,
		"blockedPatterns":         g.blockedPhrases,
		"telemetryAction":         g.telemetryAction,
		"telemetryTags":           keys(g.telemetryTags),
		"credentialLeakAction":    g.credentialAction,
		"credentialSensitiveKeys": keys(g.sensitiveKeys),
		"riskReviewThreshold":     g.riskReviewAt,
		"riskBlockThreshold":      g.riskBlockAt,
		"loopGuard":               g.loopGuard.Snapshot(),
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
			g.addToolPolicy(method, action)
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
	if bundle.RiskReviewThreshold != nil {
		g.riskReviewAt = normalizeThreshold(*bundle.RiskReviewThreshold, g.riskReviewAt)
	}
	if bundle.RiskBlockThreshold != nil {
		g.riskBlockAt = normalizeThreshold(*bundle.RiskBlockThreshold, g.riskBlockAt)
	}
	if g.riskBlockAt < g.riskReviewAt {
		g.riskBlockAt = g.riskReviewAt
	}
	loopSnapshot := g.loopGuard.Snapshot()
	enabled := boolFromAny(loopSnapshot["enabled"], true)
	windowMS := intFromAny(loopSnapshot["windowMs"], 5000)
	maxHits := intFromAny(loopSnapshot["maxHits"], 8)
	if bundle.LoopGuardEnabled != nil {
		enabled = *bundle.LoopGuardEnabled
	}
	if bundle.LoopGuardWindowMS != nil && *bundle.LoopGuardWindowMS > 0 {
		windowMS = *bundle.LoopGuardWindowMS
	}
	if bundle.LoopGuardMaxHits != nil && *bundle.LoopGuardMaxHits > 0 {
		maxHits = *bundle.LoopGuardMaxHits
	}
	g.loopGuard = NewToolLoopGuard(time.Duration(windowMS)*time.Millisecond, maxHits, enabled)
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

func analyzePromptRisk(lower string) (int, []string) {
	score := 0
	signals := make([]string, 0, 6)

	add := func(points int, signal string) {
		score += points
		signals = append(signals, signal)
	}

	if strings.Contains(lower, "ignore previous instructions") {
		add(35, "prompt_injection:ignore_previous_instructions")
	}
	if strings.Contains(lower, "system prompt") || strings.Contains(lower, "developer message") {
		add(30, "prompt_injection:system_prompt_exfil")
	}
	if strings.Contains(lower, "jailbreak") || strings.Contains(lower, "disable safety") {
		add(30, "prompt_injection:safety_bypass")
	}
	if strings.Contains(lower, "sudo ") || strings.Contains(lower, "rm -rf") || strings.Contains(lower, "del /f /s /q") {
		add(25, "command:destructive")
	}
	if strings.Contains(lower, "powershell -enc") || strings.Contains(lower, "base64 -d") {
		add(20, "command:obfuscation")
	}
	if strings.Contains(lower, "curl http") || strings.Contains(lower, "wget http") {
		add(10, "command:remote_fetch")
	}
	if score > 100 {
		score = 100
	}
	return score, signals
}

func (g *Guard) addToolPolicy(pattern string, action Action) {
	normalized := normalizeMethod(pattern)
	if normalized == "" || action == "" {
		return
	}
	if strings.Contains(normalized, "*") {
		g.toolMatchers = append(g.toolMatchers, toolPolicyMatcher{pattern: normalized, action: action})
		return
	}
	g.toolPolicies[normalized] = action
}

func (g *Guard) policyActionFor(method string) (Action, bool) {
	if action, ok := g.toolPolicies[method]; ok {
		return action, true
	}

	bestLen := -1
	var bestAction Action
	for _, matcher := range g.toolMatchers {
		if !wildcardMatch(matcher.pattern, method) {
			continue
		}
		if len(matcher.pattern) > bestLen {
			bestLen = len(matcher.pattern)
			bestAction = matcher.action
		}
	}
	if bestLen >= 0 {
		return bestAction, true
	}
	return "", false
}

func wildcardMatch(pattern string, value string) bool {
	if pattern == "*" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return pattern == value
	}

	parts := strings.Split(pattern, "*")
	index := 0
	anchoredPrefix := !strings.HasPrefix(pattern, "*")
	anchoredSuffix := !strings.HasSuffix(pattern, "*")

	for i, part := range parts {
		if part == "" {
			continue
		}
		found := strings.Index(value[index:], part)
		if found < 0 {
			return false
		}
		if i == 0 && anchoredPrefix && found != 0 {
			return false
		}
		index += found + len(part)
	}

	if anchoredSuffix {
		last := parts[len(parts)-1]
		if last == "" {
			return true
		}
		return strings.HasSuffix(value, last)
	}
	return true
}

func normalizeThreshold(value int, fallback int) int {
	if value <= 0 || value > 100 {
		return fallback
	}
	return value
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func dedupeStrings(items []string) []string {
	if len(items) == 0 {
		return []string{}
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		normalized := strings.TrimSpace(item)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out
}

func intString(v int) string {
	return strconv.Itoa(v)
}

func boolFromAny(v any, fallback bool) bool {
	switch value := v.(type) {
	case bool:
		return value
	default:
		return fallback
	}
}

func intFromAny(v any, fallback int) int {
	switch value := v.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return fallback
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
