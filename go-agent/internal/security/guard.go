package security

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
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
	EDRTelemetryPath        string
	EDRTelemetryMaxAgeSecs  int
	EDRTelemetryRiskBonus   int
	CredentialSensitiveKeys []string
	CredentialLeakAction    string
	AttestationExpectedSHA  string
	AttestationReportPath   string
	AttestationMismatchRisk int
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

type telemetryAlert struct {
	Reason string
	Tag    string
	Risk   int
}

type telemetryCache struct {
	checkedAt time.Time
	alert     *telemetryAlert
}

type attestationSnapshot struct {
	ExecutablePath  string `json:"executablePath"`
	ExecutableSHA   string `json:"executableSha256"`
	ExpectedSHA     string `json:"expectedSha256,omitempty"`
	Verified        bool   `json:"verified"`
	StartedAtMS     int64  `json:"startedAtMs"`
	ReportWritePath string `json:"reportPath,omitempty"`
	Error           string `json:"error,omitempty"`
}

type Guard struct {
	defaultAction    Action
	toolPolicies     map[string]Action
	toolMatchers     []toolPolicyMatcher
	blockedPhrases   []string
	telemetryTags    map[string]struct{}
	telemetryAction  Action
	telemetryPath    string
	telemetryMaxAge  time.Duration
	telemetryRisk    int
	telemetryCache   telemetryCache
	telemetryMu      sync.Mutex
	sensitiveKeys    map[string]struct{}
	credentialAction Action
	attestation      attestationSnapshot
	attestationRisk  int
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

var toolPolicyGroups = map[string][]string{
	"group:edge": {
		"edge.*",
	},
	"group:browser": {
		"browser.*",
		"web.login.*",
		"auth.oauth.*",
	},
	"group:messaging": {
		"send",
		"chat.send",
		"sessions.send",
		"poll",
	},
	"group:sessions": {
		"sessions.*",
		"session.status",
	},
	"group:system": {
		"connect",
		"status",
		"health",
		"config.*",
		"security.audit",
	},
	"group:nodes": {
		"node.*",
		"canvas.present",
	},
}

func NewGuard(cfg GuardConfig) *Guard {
	g := &Guard{
		defaultAction:    parseAction(cfg.DefaultAction),
		toolPolicies:     map[string]Action{},
		toolMatchers:     make([]toolPolicyMatcher, 0, 16),
		blockedPhrases:   normalizePhrases(cfg.BlockedMessagePatterns),
		telemetryTags:    normalizeSet(cfg.TelemetryHighRiskTags),
		telemetryAction:  parseAction(cfg.TelemetryAction),
		telemetryPath:    strings.TrimSpace(cfg.EDRTelemetryPath),
		telemetryMaxAge:  time.Duration(maxInt(cfg.EDRTelemetryMaxAgeSecs, 1)) * time.Second,
		telemetryRisk:    normalizeThreshold(cfg.EDRTelemetryRiskBonus, 45),
		telemetryCache:   telemetryCache{},
		sensitiveKeys:    normalizeSet(cfg.CredentialSensitiveKeys),
		credentialAction: parseAction(cfg.CredentialLeakAction),
		attestationRisk:  normalizeThreshold(cfg.AttestationMismatchRisk, 55),
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
	g.attestation = buildAttestationSnapshot(strings.TrimSpace(cfg.AttestationExpectedSHA), strings.TrimSpace(cfg.AttestationReportPath))
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

	if !g.attestation.Verified {
		riskScore = clampRisk(riskScore + maxInt(g.attestationRisk, 1))
		signals = append(signals, "runtime_attestation_mismatch")
		expected := strings.TrimSpace(g.attestation.ExpectedSHA)
		if expected == "" {
			expected = "unset-expected-sha256"
		}
		signals = append(signals, "attestation:expected="+expected)
	}

	if alert := g.recentTelemetryAlert(); alert != nil {
		riskScore = clampRisk(riskScore + maxInt(alert.Risk, 1))
		signals = append(signals, alert.Tag)
		switch g.telemetryAction {
		case ActionBlock:
			return Decision{Action: ActionBlock, Reason: alert.Reason, RiskScore: riskScore, Signals: dedupeStrings(signals)}
		case ActionReview:
			return Decision{Action: ActionReview, Reason: alert.Reason, RiskScore: riskScore, Signals: dedupeStrings(signals)}
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
		"defaultAction":   g.defaultAction,
		"toolPolicies":    toolPolicies,
		"blockedPatterns": g.blockedPhrases,
		"telemetryAction": g.telemetryAction,
		"telemetryTags":   keys(g.telemetryTags),
		"telemetryFeed": map[string]any{
			"path":          g.telemetryPath,
			"maxAgeSeconds": int(g.telemetryMaxAge / time.Second),
			"riskBonus":     g.telemetryRisk,
		},
		"credentialLeakAction":    g.credentialAction,
		"credentialSensitiveKeys": keys(g.sensitiveKeys),
		"attestation":             g.attestation,
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

func (g *Guard) recentTelemetryAlert() *telemetryAlert {
	if strings.TrimSpace(g.telemetryPath) == "" {
		return nil
	}
	now := time.Now().UTC()

	g.telemetryMu.Lock()
	if !g.telemetryCache.checkedAt.IsZero() && now.Sub(g.telemetryCache.checkedAt) < 2*time.Second {
		cached := g.telemetryCache.alert
		g.telemetryMu.Unlock()
		return cached
	}
	g.telemetryMu.Unlock()

	raw, err := os.ReadFile(g.telemetryPath)
	if err != nil {
		if os.IsNotExist(err) {
			g.telemetryMu.Lock()
			g.telemetryCache.checkedAt = now
			g.telemetryCache.alert = nil
			g.telemetryMu.Unlock()
			return nil
		}
		g.lastError = err.Error()
		g.telemetryMu.Lock()
		g.telemetryCache.checkedAt = now
		g.telemetryCache.alert = nil
		g.telemetryMu.Unlock()
		return nil
	}

	clip := raw
	const maxBytes = 2 * 1024 * 1024
	if len(clip) > maxBytes {
		clip = clip[len(clip)-maxBytes:]
	}

	lines := strings.Split(string(clip), "\n")
	scanned := 0
	var found *telemetryAlert
	for i := len(lines) - 1; i >= 0; i-- {
		if scanned >= 256 {
			break
		}
		scanned++
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			continue
		}
		if !g.telemetryEventRecent(payload, now) {
			continue
		}
		alert := g.classifyTelemetryPayload(payload)
		if alert != nil {
			found = alert
			break
		}
	}

	g.telemetryMu.Lock()
	g.telemetryCache.checkedAt = now
	g.telemetryCache.alert = found
	g.telemetryMu.Unlock()
	return found
}

func (g *Guard) telemetryEventRecent(payload map[string]any, now time.Time) bool {
	timestamp := now
	if value := firstTimeField(payload, "observedAtMs", "observed_at_ms", "timestampMs", "timestamp_ms", "ts"); !value.IsZero() {
		timestamp = value
	}
	return now.Sub(timestamp) <= g.telemetryMaxAge
}

func (g *Guard) classifyTelemetryPayload(payload map[string]any) *telemetryAlert {
	severity := strings.ToLower(strings.TrimSpace(firstStringField(payload, "severity", "level")))
	highSeverity := severity == "critical" || severity == "high" || severity == "severe" || severity == "emergency"
	highTag := ""
	for _, tag := range flattenStrings(payload["tags"]) {
		normalized := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(tag, " ", "_")))
		if _, ok := g.telemetryTags[normalized]; ok {
			highTag = normalized
			break
		}
	}
	blocked := boolField(payload, "blocked", "quarantined")
	if !highSeverity && highTag == "" && !blocked {
		return nil
	}

	reason := ""
	switch {
	case highTag != "":
		reason = "telemetry high-risk tag detected: " + highTag
	case highSeverity:
		reason = "telemetry severity detected: " + severity
	default:
		reason = "telemetry event indicates blocked/quarantined host activity"
	}
	return &telemetryAlert{
		Reason: reason,
		Tag:    "edr_telemetry_alert",
		Risk:   maxInt(g.telemetryRisk, 1),
	}
}

func buildAttestationSnapshot(expectedSHA string, reportPath string) attestationSnapshot {
	snapshot := attestationSnapshot{
		ExpectedSHA: strings.ToLower(strings.TrimSpace(expectedSHA)),
		Verified:    true,
		StartedAtMS: time.Now().UTC().UnixMilli(),
	}

	exePath, err := os.Executable()
	if err != nil {
		snapshot.Error = err.Error()
		snapshot.Verified = snapshot.ExpectedSHA == ""
		return snapshot
	}
	snapshot.ExecutablePath = exePath
	snapshot.ExecutableSHA, err = fileSHA256(exePath)
	if err != nil {
		snapshot.Error = err.Error()
		snapshot.Verified = snapshot.ExpectedSHA == ""
	} else if snapshot.ExpectedSHA != "" {
		snapshot.Verified = strings.EqualFold(snapshot.ExpectedSHA, snapshot.ExecutableSHA)
	}

	reportPath = strings.TrimSpace(reportPath)
	if reportPath != "" {
		snapshot.ReportWritePath = reportPath
		if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err == nil {
			if payload, err := json.MarshalIndent(snapshot, "", "  "); err == nil {
				_ = os.WriteFile(reportPath, payload, 0o644)
			}
		}
	}
	return snapshot
}

func fileSHA256(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func firstStringField(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		if value, ok := raw.(string); ok {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func firstTimeField(payload map[string]any, keys ...string) time.Time {
	for _, key := range keys {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		switch value := raw.(type) {
		case float64:
			if value <= 0 {
				continue
			}
			return time.UnixMilli(int64(value)).UTC()
		case int64:
			if value <= 0 {
				continue
			}
			return time.UnixMilli(value).UTC()
		case int:
			if value <= 0 {
				continue
			}
			return time.UnixMilli(int64(value)).UTC()
		case string:
			trimmed := strings.TrimSpace(value)
			if trimmed == "" {
				continue
			}
			if parsed, err := strconv.ParseInt(trimmed, 10, 64); err == nil && parsed > 0 {
				return time.UnixMilli(parsed).UTC()
			}
		}
	}
	return time.Time{}
}

func boolField(payload map[string]any, keys ...string) bool {
	for _, key := range keys {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		switch value := raw.(type) {
		case bool:
			if value {
				return true
			}
		case string:
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "1", "true", "yes", "on":
				return true
			}
		}
	}
	return false
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
	if strings.HasPrefix(normalized, "group:") {
		for _, expanded := range expandToolPolicyGroup(normalized) {
			g.addToolPolicy(expanded, action)
		}
		return
	}
	if strings.Contains(normalized, "*") {
		g.toolMatchers = append(g.toolMatchers, toolPolicyMatcher{pattern: normalized, action: action})
		return
	}
	g.toolPolicies[normalized] = action
}

func expandToolPolicyGroup(group string) []string {
	if patterns, ok := toolPolicyGroups[group]; ok {
		return append([]string(nil), patterns...)
	}
	return []string{}
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

func clampRisk(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
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
