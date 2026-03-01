package gateway

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	osexec "os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	webbridge "github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/bridge/web"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/memory"
)

type compatState struct {
	mu sync.RWMutex

	talkConfig map[string]any
	talkMode   string

	ttsEnabled  bool
	ttsProvider string

	voiceWakeEnabled bool
	heartbeats       bool
	heartbeatEveryMs int
	lastHeartbeatAt  string

	presence map[string]any
	events   []map[string]any

	models                   []map[string]any
	telegramModelByTarget    map[string]string
	telegramProviderByTarget map[string]string
	telegramAuthByTarget     map[string]string
	telegramAuthByScope      map[string]string
	providerAPIKeys          map[string]string

	agentSeq   int
	agents     map[string]map[string]any
	agentFiles map[string]map[string]map[string]any

	skillSeq        int
	installedSkills map[string]map[string]any

	cronSeq  int
	cronJobs map[string]map[string]any
	cronRuns []map[string]any

	wizard map[string]any

	devicePairSeq  int
	devicePairs    map[string]map[string]any
	deviceTokenSeq int
	deviceTokens   map[string]map[string]any

	nodePairSeq int
	nodePairs   map[string]map[string]any
	nodes       map[string]map[string]any
	nodeEvents  []map[string]any

	approvalSeq       int
	globalApprovals   map[string]any
	nodeApprovals     map[string]map[string]any
	pendingApprovals  map[string]map[string]any
	configOverlay     map[string]any
	sessionTombstones map[string]bool

	updateSeq  int
	updateJobs map[string]map[string]any
}

func newCompatState() *compatState {
	now := time.Now().UTC().Format(time.RFC3339)
	return &compatState{
		talkConfig: map[string]any{
			"voice":       "alloy",
			"language":    "en",
			"temperature": 0.2,
		},
		talkMode:         "balanced",
		ttsEnabled:       true,
		ttsProvider:      "native",
		voiceWakeEnabled: false,
		heartbeats:       false,
		heartbeatEveryMs: 15000,
		lastHeartbeatAt:  "",
		presence: map[string]any{
			"state":     "online",
			"updatedAt": now,
		},
		events: make([]map[string]any, 0, 128),
		models: []map[string]any{
			{
				"id":         "gpt-5.2",
				"name":       "GPT-5.2",
				"mode":       "auto",
				"provider":   "chatgpt",
				"capability": "general",
				"aliases":    []string{"auto", "default"},
			},
			{
				"id":         "gpt-5.2-thinking",
				"name":       "GPT-5.2 Thinking",
				"mode":       "thinking",
				"provider":   "chatgpt",
				"capability": "reasoning",
				"aliases":    []string{"thinking", "extended", "extended-thinking"},
			},
			{
				"id":         "gpt-5.2-pro",
				"name":       "GPT-5.2 Pro",
				"mode":       "pro",
				"provider":   "chatgpt",
				"capability": "research",
				"aliases":    []string{"pro", "extended-pro"},
			},
			{
				"id":         "gpt-5.1-mini",
				"name":       "GPT-5.1 Mini",
				"mode":       "instant",
				"provider":   "chatgpt",
				"capability": "fast-response",
				"aliases":    []string{"instant", "mini", "fast"},
			},
			{
				"id":         "gpt-5.3-codex",
				"name":       "GPT-5.3 Codex",
				"mode":       "pro",
				"provider":   "codex",
				"capability": "coding",
				"aliases":    []string{"codex", "gpt5.3-codex", "gpt-5.3-codex"},
			},
			{
				"id":         "qwen3.5-397b-a17b",
				"name":       "Qwen 3.5 397B",
				"mode":       "thinking",
				"provider":   "qwen",
				"capability": "reasoning",
				"aliases":    []string{"qwen3.5", "qwen35", "qwen-3.5"},
			},
			{
				"id":         "qwen3.5-plus",
				"name":       "Qwen 3.5 Plus",
				"mode":       "pro",
				"provider":   "qwen",
				"capability": "general",
				"aliases":    []string{"qwen-plus"},
			},
			{
				"id":         "qwen3.5-flash",
				"name":       "Qwen 3.5 Flash",
				"mode":       "instant",
				"provider":   "qwen",
				"capability": "fast-response",
				"aliases":    []string{"qwen-flash"},
			},
			{
				"id":         "inception/mercury",
				"name":       "Mercury 2",
				"mode":       "thinking",
				"provider":   "inception",
				"capability": "reasoning",
				"aliases":    []string{"mercury", "mercury2", "mercury-2"},
			},
			{
				"id":         "opencode/glm-5-free",
				"name":       "OpenCode GLM-5 Free",
				"mode":       "instant",
				"provider":   "opencode",
				"capability": "coding",
				"aliases":    []string{"glm-5-free", "opencode-free"},
			},
			{
				"id":         "opencode/kimi-k2.5-free",
				"name":       "OpenCode Kimi K2.5 Free",
				"mode":       "instant",
				"provider":   "opencode",
				"capability": "coding",
				"aliases":    []string{"kimi-k2.5-free"},
			},
			{
				"id":         "openrouter/qwen/qwen3-coder:free",
				"name":       "OpenRouter Qwen3 Coder Free",
				"mode":       "instant",
				"provider":   "openrouter",
				"capability": "coding",
				"aliases":    []string{"qwen3-coder-free"},
			},
			{
				"id":         "openrouter/google/gemini-2.0-flash-exp:free",
				"name":       "OpenRouter Gemini 2.0 Flash Free",
				"mode":       "instant",
				"provider":   "openrouter",
				"capability": "general",
				"aliases":    []string{"gemini-2.0-flash-free", "gemini-free"},
			},
		},
		telegramModelByTarget:    map[string]string{},
		telegramProviderByTarget: map[string]string{},
		telegramAuthByTarget:     map[string]string{},
		telegramAuthByScope:      map[string]string{},
		providerAPIKeys:          map[string]string{},
		agentSeq:                 0,
		agents:                   map[string]map[string]any{},
		agentFiles:               map[string]map[string]map[string]any{},
		skillSeq:                 0,
		installedSkills:          map[string]map[string]any{},
		cronSeq:                  0,
		cronJobs:                 map[string]map[string]any{},
		cronRuns:                 make([]map[string]any, 0, 256),
		wizard:                   map[string]any{"active": false, "step": 0, "status": "idle"},
		devicePairSeq:            0,
		devicePairs:              map[string]map[string]any{},
		deviceTokenSeq:           0,
		deviceTokens:             map[string]map[string]any{},
		nodePairSeq:              0,
		nodePairs:                map[string]map[string]any{},
		nodes:                    map[string]map[string]any{"node-local": {"nodeId": "node-local", "name": "local", "status": "online"}},
		nodeEvents:               make([]map[string]any, 0, 256),
		approvalSeq:              0,
		globalApprovals:          map[string]any{"mode": "prompt", "updatedAt": now},
		nodeApprovals:            map[string]map[string]any{},
		pendingApprovals:         map[string]map[string]any{},
		configOverlay:            map[string]any{},
		sessionTombstones:        map[string]bool{},
		updateSeq:                0,
		updateJobs:               map[string]map[string]any{},
	}
}

func (c *compatState) listModelIDs() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, 0, len(c.models))
	for _, item := range c.models {
		id := strings.ToLower(toString(item["id"], ""))
		if id != "" {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

func (c *compatState) listModelDescriptors() []map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	items := make([]map[string]any, 0, len(c.models))
	for _, item := range c.models {
		items = append(items, cloneMap(item))
	}
	return items
}

func normalizeModelAlias(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("_", "-", ".", "-", " ", "-", "/", "-")
	return replacer.Replace(normalized)
}

func normalizeProviderAlias(value string) string {
	return normalizeProviderID(value)
}

func (c *compatState) listModelProviders() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	seen := map[string]struct{}{}
	out := make([]string, 0, len(c.models))
	for _, item := range c.models {
		provider := normalizeProviderAlias(toString(item["provider"], ""))
		if provider == "" {
			continue
		}
		if _, ok := seen[provider]; ok {
			continue
		}
		seen[provider] = struct{}{}
		out = append(out, provider)
	}
	sort.Strings(out)
	return out
}

func (c *compatState) listModelIDsForProvider(provider string) []string {
	normalizedProvider := normalizeProviderAlias(provider)
	if normalizedProvider == "" {
		return c.listModelIDs()
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, 0, len(c.models))
	for _, item := range c.models {
		catalogProvider := normalizeProviderAlias(toString(item["provider"], ""))
		if catalogProvider != normalizedProvider {
			continue
		}
		modelID := strings.ToLower(strings.TrimSpace(toString(item["id"], "")))
		if modelID == "" {
			continue
		}
		out = append(out, modelID)
	}
	sort.Strings(out)
	return out
}

func (c *compatState) defaultModelForProvider(provider string) (string, bool) {
	normalizedProvider := normalizeProviderAlias(provider)
	if normalizedProvider == "" {
		return "", false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, item := range c.models {
		catalogProvider := normalizeProviderAlias(toString(item["provider"], ""))
		if catalogProvider != normalizedProvider {
			continue
		}
		modelID := strings.ToLower(strings.TrimSpace(toString(item["id"], "")))
		if modelID == "" {
			continue
		}
		return modelID, true
	}
	return "", false
}

func (c *compatState) hasModelProvider(provider string) bool {
	_, ok := c.defaultModelForProvider(provider)
	return ok
}

func (c *compatState) lookupModelDescriptor(model string) (map[string]any, bool) {
	modelID, _, ok := c.resolveModelChoice(model)
	if !ok {
		return map[string]any{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, item := range c.models {
		if strings.EqualFold(toString(item["id"], ""), modelID) {
			return cloneMap(item), true
		}
	}
	return map[string]any{}, false
}

func (c *compatState) providerForModel(model string) string {
	descriptor, ok := c.lookupModelDescriptor(model)
	if !ok {
		return ""
	}
	return normalizeProviderAlias(toString(descriptor["provider"], ""))
}

func (c *compatState) resolveModelChoice(model string) (string, string, bool) {
	normalized := normalizeModelAlias(model)
	if normalized == "" {
		return "", "", false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, item := range c.models {
		modelID := strings.ToLower(strings.TrimSpace(toString(item["id"], "")))
		if modelID == normalized || normalizeModelAlias(modelID) == normalized {
			return modelID, "", true
		}
	}

	aliasMap := map[string]string{
		"auto":              "gpt-5.2",
		"default":           "gpt-5.2",
		"gpt5-2":            "gpt-5.2",
		"gpt-5-2":           "gpt-5.2",
		"instant":           "gpt-5.1-mini",
		"fast":              "gpt-5.1-mini",
		"mini":              "gpt-5.1-mini",
		"thinking":          "gpt-5.2-thinking",
		"extended":          "gpt-5.2-thinking",
		"extended-thinking": "gpt-5.2-thinking",
		"reasoning":         "gpt-5.2-thinking",
		"pro":               "gpt-5.2-pro",
		"extended-pro":      "gpt-5.2-pro",
		"research":          "gpt-5.2-pro",
	}
	if mapped, ok := aliasMap[normalized]; ok {
		return mapped, normalized, true
	}

	for _, item := range c.models {
		modelID := strings.ToLower(strings.TrimSpace(toString(item["id"], "")))
		mode := normalizeModelAlias(toString(item["mode"], ""))
		if mode != "" && mode == normalized {
			return modelID, normalized, true
		}
		name := normalizeModelAlias(toString(item["name"], ""))
		if name != "" && name == normalized {
			return modelID, normalized, true
		}
		for _, alias := range toStringSlice(item["aliases"]) {
			if normalizeModelAlias(alias) == normalized {
				return modelID, normalized, true
			}
		}
	}

	return "", "", false
}

func (c *compatState) isKnownModel(model string) bool {
	_, _, ok := c.resolveModelChoice(model)
	return ok
}

func (c *compatState) nextTelegramModel(target string) string {
	current := c.getTelegramModel(target)
	models := c.listModelIDs()
	if len(models) == 0 {
		return c.setTelegramModel(target, "gpt-5.2")
	}
	currentIndex := -1
	for idx, model := range models {
		if strings.EqualFold(model, current) {
			currentIndex = idx
			break
		}
	}
	nextIndex := 0
	if currentIndex >= 0 {
		nextIndex = (currentIndex + 1) % len(models)
	}
	return c.setTelegramModel(target, models[nextIndex])
}

func (c *compatState) getTelegramModelProvider(target string) string {
	targetKey := strings.ToLower(strings.TrimSpace(target))
	c.mu.RLock()
	provider := normalizeProviderAlias(c.telegramProviderByTarget[targetKey])
	model := strings.ToLower(strings.TrimSpace(c.telegramModelByTarget[targetKey]))
	c.mu.RUnlock()
	if provider != "" {
		return provider
	}
	if descriptorProvider := c.providerForModel(model); descriptorProvider != "" {
		return descriptorProvider
	}
	return "chatgpt"
}

func (c *compatState) getTelegramModel(target string) string {
	targetKey := strings.ToLower(strings.TrimSpace(target))
	c.mu.RLock()
	model := strings.ToLower(strings.TrimSpace(c.telegramModelByTarget[targetKey]))
	c.mu.RUnlock()
	if model == "" {
		provider := c.getTelegramModelProvider(target)
		if fallback, ok := c.defaultModelForProvider(provider); ok {
			return fallback
		}
		return "gpt-5.2"
	}
	return model
}

func (c *compatState) setTelegramModelSelection(target string, provider string, model string) (string, string) {
	targetKey := strings.ToLower(strings.TrimSpace(target))
	normalized := strings.ToLower(strings.TrimSpace(model))
	if normalized == "" {
		normalized = "gpt-5.2"
	}
	normalizedProvider := normalizeProviderAlias(provider)
	if normalizedProvider == "" {
		normalizedProvider = c.providerForModel(normalized)
	}
	if normalizedProvider == "" {
		normalizedProvider = "chatgpt"
	}
	c.mu.Lock()
	c.telegramModelByTarget[targetKey] = normalized
	c.telegramProviderByTarget[targetKey] = normalizedProvider
	c.mu.Unlock()
	return normalizedProvider, normalized
}

func (c *compatState) setTelegramModel(target string, model string) string {
	provider := c.getTelegramModelProvider(target)
	if matchedProvider := c.providerForModel(model); matchedProvider != "" {
		provider = matchedProvider
	}
	_, selected := c.setTelegramModelSelection(target, provider, model)
	return selected
}

func (c *compatState) getTelegramModelSelection(target string) (string, string) {
	provider := c.getTelegramModelProvider(target)
	model := c.getTelegramModel(target)
	if provider == "" {
		provider = c.providerForModel(model)
	}
	if provider == "" {
		provider = "chatgpt"
	}
	if model == "" {
		model = "gpt-5.2"
	}
	return provider, model
}

func (c *compatState) getTelegramAuth(target string) string {
	targetKey := strings.ToLower(strings.TrimSpace(target))
	c.mu.RLock()
	loginID := strings.TrimSpace(c.telegramAuthByTarget[targetKey])
	c.mu.RUnlock()
	return loginID
}

func (c *compatState) setTelegramAuth(target string, loginID string) {
	targetKey := strings.ToLower(strings.TrimSpace(target))
	c.mu.Lock()
	c.telegramAuthByTarget[targetKey] = strings.TrimSpace(loginID)
	c.mu.Unlock()
}

func normalizeAuthAccount(account string) string {
	return strings.ToLower(strings.TrimSpace(account))
}

func telegramAuthScopeKey(target string, provider string, account string) string {
	targetKey := strings.ToLower(strings.TrimSpace(target))
	providerKey := normalizeProviderAlias(provider)
	accountKey := normalizeAuthAccount(account)
	if providerKey == "" {
		providerKey = "_default"
	}
	if accountKey == "" {
		accountKey = "_default"
	}
	return strings.Join([]string{targetKey, providerKey, accountKey}, "|")
}

func (c *compatState) setTelegramAuthScoped(target string, provider string, account string, loginID string) {
	targetKey := strings.ToLower(strings.TrimSpace(target))
	scopeKey := telegramAuthScopeKey(target, provider, account)
	normalizedLoginID := strings.TrimSpace(loginID)
	c.mu.Lock()
	previousScoped := strings.TrimSpace(c.telegramAuthByScope[scopeKey])
	c.telegramAuthByScope[scopeKey] = normalizedLoginID
	if normalizedLoginID != "" {
		c.telegramAuthByTarget[targetKey] = normalizedLoginID
		c.mu.Unlock()
		return
	}

	currentTarget := strings.TrimSpace(c.telegramAuthByTarget[targetKey])
	if currentTarget != "" && currentTarget != previousScoped {
		c.mu.Unlock()
		return
	}
	fallback := ""
	prefix := targetKey + "|"
	for key, candidate := range c.telegramAuthByScope {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		fallback = candidate
		break
	}
	c.telegramAuthByTarget[targetKey] = fallback
	c.mu.Unlock()
}

func (c *compatState) getTelegramAuthScoped(target string, provider string, account string) string {
	targetKey := strings.ToLower(strings.TrimSpace(target))
	providerKey := normalizeProviderAlias(provider)
	accountKey := normalizeAuthAccount(account)

	c.mu.RLock()
	defer c.mu.RUnlock()

	if providerKey == "" && accountKey == "" {
		return strings.TrimSpace(c.telegramAuthByTarget[targetKey])
	}

	if exact := strings.TrimSpace(c.telegramAuthByScope[telegramAuthScopeKey(target, providerKey, accountKey)]); exact != "" {
		return exact
	}

	if providerKey != "" && accountKey == "" {
		prefix := strings.Join([]string{targetKey, providerKey, ""}, "|")
		for key, loginID := range c.telegramAuthByScope {
			if strings.HasPrefix(key, prefix) {
				if trimmed := strings.TrimSpace(loginID); trimmed != "" {
					return trimmed
				}
			}
		}
	}

	if providerKey == "" && accountKey != "" {
		suffix := "|" + accountKey
		prefix := targetKey + "|"
		for key, loginID := range c.telegramAuthByScope {
			if strings.HasPrefix(key, prefix) && strings.HasSuffix(key, suffix) {
				if trimmed := strings.TrimSpace(loginID); trimmed != "" {
					return trimmed
				}
			}
		}
	}

	return strings.TrimSpace(c.telegramAuthByTarget[targetKey])
}

func (c *compatState) setProviderAPIKey(provider string, apiKey string) bool {
	normalizedProvider := normalizeProviderAlias(provider)
	normalizedKey := strings.TrimSpace(apiKey)
	if normalizedProvider == "" || normalizedKey == "" {
		return false
	}
	c.mu.Lock()
	c.providerAPIKeys[normalizedProvider] = normalizedKey
	c.mu.Unlock()
	return true
}

func (c *compatState) hasProviderAPIKey(provider string) bool {
	normalizedProvider := normalizeProviderAlias(provider)
	if normalizedProvider == "" {
		return false
	}
	c.mu.RLock()
	_, ok := c.providerAPIKeys[normalizedProvider]
	c.mu.RUnlock()
	return ok
}

func (c *compatState) edgeTopologySnapshot() map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()

	nodeCount := len(c.nodes)
	onlineNodes := 0
	for _, node := range c.nodes {
		if strings.EqualFold(toString(node["status"], ""), "online") {
			onlineNodes++
		}
	}

	approvedPairs := 0
	pendingPairs := 0
	rejectedPairs := 0
	approvedPeers := map[string]struct{}{}
	for _, pair := range c.nodePairs {
		status := strings.ToLower(toString(pair["status"], "pending"))
		nodeID := strings.ToLower(strings.TrimSpace(toString(pair["nodeId"], "")))
		switch status {
		case "approved":
			approvedPairs++
			if nodeID != "" && nodeID != "node-local" {
				approvedPeers[nodeID] = struct{}{}
			}
		case "rejected", "denied":
			rejectedPairs++
		default:
			pendingPairs++
		}
	}

	pendingApprovals := 0
	approvedApprovals := 0
	rejectedApprovals := 0
	for _, approval := range c.pendingApprovals {
		status := strings.ToLower(toString(approval["status"], "pending"))
		switch status {
		case "approved":
			approvedApprovals++
		case "rejected":
			rejectedApprovals++
		default:
			pendingApprovals++
		}
	}

	return map[string]any{
		"nodes":             nodeCount,
		"onlineNodes":       onlineNodes,
		"approvedPairs":     approvedPairs,
		"pendingPairs":      pendingPairs,
		"rejectedPairs":     rejectedPairs,
		"approvedPeers":     len(approvedPeers),
		"pendingApprovals":  pendingApprovals,
		"approvedApprovals": approvedApprovals,
		"rejectedApprovals": rejectedApprovals,
		"nodeEvents":        len(c.nodeEvents),
	}
}

func (c *compatState) mergeConfig(params map[string]any) map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, value := range params {
		if strings.TrimSpace(key) == "" {
			continue
		}
		c.configOverlay[strings.TrimSpace(key)] = value
	}
	return cloneMap(c.configOverlay)
}

func (c *compatState) configSnapshot() map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneMap(c.configOverlay)
}

func (c *compatState) touchHeartbeat(enabled bool, intervalMs int) map[string]any {
	if intervalMs <= 0 {
		intervalMs = 15000
	}
	now := time.Now().UTC().Format(time.RFC3339)
	c.mu.Lock()
	c.heartbeats = enabled
	c.heartbeatEveryMs = intervalMs
	if enabled {
		c.lastHeartbeatAt = now
	}
	heartbeat := map[string]any{
		"enabled":    c.heartbeats,
		"intervalMs": c.heartbeatEveryMs,
		"lastAt":     c.lastHeartbeatAt,
	}
	c.mu.Unlock()
	return heartbeat
}

func (c *compatState) heartbeatSnapshot() map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return map[string]any{
		"enabled":    c.heartbeats,
		"intervalMs": c.heartbeatEveryMs,
		"lastAt":     c.lastHeartbeatAt,
	}
}

func (c *compatState) addEvent(kind string, payload map[string]any) map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := map[string]any{
		"id":        fmt.Sprintf("evt-%06d", len(c.events)+1),
		"type":      kind,
		"payload":   cloneMap(payload),
		"createdAt": time.Now().UTC().Format(time.RFC3339),
	}
	c.events = append(c.events, entry)
	if len(c.events) > 256 {
		c.events = append([]map[string]any(nil), c.events[len(c.events)-256:]...)
	}
	return cloneMap(entry)
}

func (c *compatState) listEvents(limit int) []map[string]any {
	if limit <= 0 {
		limit = 20
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.events) == 0 {
		return []map[string]any{}
	}
	if limit > len(c.events) {
		limit = len(c.events)
	}
	start := len(c.events) - limit
	out := make([]map[string]any, 0, limit)
	for _, entry := range c.events[start:] {
		out = append(out, cloneMap(entry))
	}
	return out
}

func (c *compatState) createUpdateJob(target string, dryRun bool, force bool, channel string) map[string]any {
	c.mu.Lock()
	c.updateSeq++
	jobID := fmt.Sprintf("update-%06d", c.updateSeq)
	now := time.Now().UTC().Format(time.RFC3339)
	job := map[string]any{
		"jobId":           jobID,
		"status":          "queued",
		"phase":           "queued",
		"progress":        0,
		"targetVersion":   target,
		"requestedBy":     valueOrDefault(channel, "gateway"),
		"dryRun":          dryRun,
		"force":           force,
		"startedAt":       now,
		"updatedAt":       now,
		"transitionCount": 0,
	}
	if dryRun {
		job["status"] = "completed"
		job["phase"] = "dry-run"
		job["progress"] = 100
		job["completedAt"] = now
		job["transitionCount"] = 1
	}
	c.updateJobs[jobID] = cloneMap(job)
	c.mu.Unlock()
	return cloneMap(job)
}

func (c *compatState) updateUpdateJob(jobID string, status string, phase string, progress int, detail map[string]any) (map[string]any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	job, ok := c.updateJobs[jobID]
	if !ok {
		return map[string]any{}, false
	}
	now := time.Now().UTC().Format(time.RFC3339)
	job["status"] = strings.TrimSpace(status)
	job["phase"] = strings.TrimSpace(phase)
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}
	job["progress"] = progress
	job["updatedAt"] = now
	job["transitionCount"] = toInt(job["transitionCount"], 0) + 1
	if detail != nil {
		for key, value := range detail {
			job[key] = value
		}
	}
	c.updateJobs[jobID] = cloneMap(job)
	return cloneMap(job), true
}

func (c *compatState) completeUpdateJob(jobID string, applied bool, releaseNotes []string) (map[string]any, bool) {
	detail := map[string]any{
		"applied":      applied,
		"releaseNotes": append([]string(nil), releaseNotes...),
		"completedAt":  time.Now().UTC().Format(time.RFC3339),
	}
	return c.updateUpdateJob(jobID, "completed", "finalize", 100, detail)
}

func (c *compatState) failUpdateJob(jobID string, reason string) (map[string]any, bool) {
	detail := map[string]any{
		"error":       strings.TrimSpace(reason),
		"completedAt": time.Now().UTC().Format(time.RFC3339),
	}
	return c.updateUpdateJob(jobID, "failed", "failed", 100, detail)
}

func (c *compatState) getUpdateJob(jobID string) (map[string]any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	job, ok := c.updateJobs[strings.TrimSpace(jobID)]
	if !ok {
		return map[string]any{}, false
	}
	return cloneMap(job), true
}

func (c *compatState) listUpdateJobs(limit int) []map[string]any {
	if limit <= 0 {
		limit = 20
	}
	c.mu.RLock()
	items := make([]map[string]any, 0, len(c.updateJobs))
	for _, job := range c.updateJobs {
		items = append(items, cloneMap(job))
	}
	c.mu.RUnlock()
	sort.Slice(items, func(i, j int) bool {
		return toString(items[i]["startedAt"], "") < toString(items[j]["startedAt"], "")
	})
	if len(items) > limit {
		items = items[len(items)-limit:]
	}
	return items
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func cloneMapList(items []map[string]any) []map[string]any {
	if len(items) == 0 {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, cloneMap(item))
	}
	return out
}

func resolveSessionID(params map[string]any) string {
	return toString(params["sessionId"], toString(params["id"], ""))
}

func countWords(text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	return len(strings.Fields(text))
}

func asStringMap(v any) map[string]any {
	switch value := v.(type) {
	case map[string]any:
		return cloneMap(value)
	default:
		return map[string]any{}
	}
}

func (s *Server) handleCompatMethod(ctx context.Context, requestID string, canonical string, params map[string]any) (map[string]any, *dispatchError) {
	switch canonical {
	case "doctor.memory.status":
		return s.handleCompatDoctorMemory(), nil
	case "usage.status":
		return s.handleCompatUsageStatus(), nil
	case "usage.cost":
		return s.handleCompatUsageCost(), nil
	case "last-heartbeat":
		return s.compat.heartbeatSnapshot(), nil
	case "set-heartbeats":
		return s.compat.touchHeartbeat(
			toBool(params["enabled"], true),
			toInt(params["intervalMs"], toInt(params["interval_ms"], 15000)),
		), nil
	case "system-presence":
		return s.handleCompatSystemPresence(params), nil
	case "system-event":
		return map[string]any{
			"event": s.compat.addEvent(toString(params["type"], "system"), params),
		}, nil
	case "wake":
		heartbeat := s.compat.touchHeartbeat(true, toInt(params["intervalMs"], 15000))
		return map[string]any{
			"ok":        true,
			"awakened":  true,
			"heartbeat": heartbeat,
		}, nil
	case "talk.config":
		return s.handleCompatTalkConfig(params), nil
	case "talk.mode":
		return s.handleCompatTalkMode(params), nil
	case "tts.status":
		return s.handleCompatTTSStatus(), nil
	case "tts.enable":
		return s.handleCompatTTSEnable(true), nil
	case "tts.disable":
		return s.handleCompatTTSEnable(false), nil
	case "tts.providers":
		return s.handleCompatTTSProviders(), nil
	case "tts.setprovider":
		return s.handleCompatSetTTSProvider(params)
	case "tts.convert":
		return s.handleCompatTTSConvert(params)
	case "voicewake.get":
		return s.handleCompatVoiceWake(nil), nil
	case "voicewake.set":
		return s.handleCompatVoiceWake(params), nil
	case "models.list":
		return s.handleCompatModelsList(params)
	case "agent.identity.get":
		return map[string]any{
			"id":        "openclaw-go",
			"service":   s.build.Service,
			"version":   s.build.Version,
			"runtime":   s.runtime.Snapshot(),
			"authMode":  s.resolveAuthMode(),
			"startedAt": s.startedAt.UTC().Format(time.RFC3339),
		}, nil
	case "agents.list":
		return s.handleCompatAgentsList(), nil
	case "agents.create":
		return s.handleCompatAgentCreate(params), nil
	case "agents.update":
		return s.handleCompatAgentUpdate(params)
	case "agents.delete":
		return s.handleCompatAgentDelete(params)
	case "agents.files.list":
		return s.handleCompatAgentFilesList(params)
	case "agents.files.get":
		return s.handleCompatAgentFilesGet(params)
	case "agents.files.set":
		return s.handleCompatAgentFilesSet(params)
	case "skills.status":
		return s.handleCompatSkillsStatus(), nil
	case "skills.bins":
		return s.handleCompatSkillsBins(), nil
	case "skills.install":
		return s.handleCompatSkillsInstall(params), nil
	case "skills.update":
		return s.handleCompatSkillsUpdate(params), nil
	case "secrets.reload":
		return map[string]any{
			"ok":         true,
			"reloadedAt": time.Now().UTC().Format(time.RFC3339),
			"count":      len(toStringSlice(params["keys"])),
		}, nil
	case "update.run":
		return s.handleCompatUpdateRun(requestID, params), nil
	case "cron.list":
		return s.handleCompatCronList(), nil
	case "cron.status":
		return s.handleCompatCronStatus(), nil
	case "cron.add":
		return s.handleCompatCronAdd(params), nil
	case "cron.update":
		return s.handleCompatCronUpdate(params)
	case "cron.remove":
		return s.handleCompatCronRemove(params)
	case "cron.run":
		return s.handleCompatCronRun(params)
	case "cron.runs":
		return s.handleCompatCronRuns(params), nil
	case "auth.oauth.providers":
		return s.handleCompatAuthOAuthProviders(params)
	case "auth.oauth.import":
		return s.handleCompatOAuthImport(params)
	case "wizard.start":
		return s.handleCompatWizardStart(params), nil
	case "wizard.next":
		return s.handleCompatWizardNext(params)
	case "wizard.cancel":
		return s.handleCompatWizardCancel(params), nil
	case "wizard.status":
		return s.handleCompatWizardStatus(), nil
	case "device.pair.list":
		return s.handleCompatDevicePairList(), nil
	case "device.pair.approve":
		return s.handleCompatDevicePairUpdate(params, "approved")
	case "device.pair.reject":
		return s.handleCompatDevicePairUpdate(params, "rejected")
	case "device.pair.remove":
		return s.handleCompatDevicePairRemove(params)
	case "device.token.rotate":
		return s.handleCompatDeviceTokenRotate(params), nil
	case "device.token.revoke":
		return s.handleCompatDeviceTokenRevoke(params), nil
	case "node.pair.request":
		return s.handleCompatNodePairRequest(params), nil
	case "node.pair.list":
		return s.handleCompatNodePairList(), nil
	case "node.pair.approve":
		return s.handleCompatNodePairUpdate(params, "approved")
	case "node.pair.reject":
		return s.handleCompatNodePairUpdate(params, "rejected")
	case "node.pair.verify":
		return s.handleCompatNodePairUpdate(params, "verified")
	case "node.rename":
		return s.handleCompatNodeRename(params)
	case "node.list":
		return s.handleCompatNodeList(), nil
	case "node.describe":
		return s.handleCompatNodeDescribe(params)
	case "node.invoke":
		return s.handleCompatNodeInvoke(params), nil
	case "node.invoke.result":
		return s.handleCompatNodeInvokeResult(params), nil
	case "node.event":
		return s.handleCompatNodeEvent(params), nil
	case "push.test":
		return map[string]any{
			"ok":        true,
			"channel":   toString(params["channel"], "webchat"),
			"messageId": fmt.Sprintf("push-%d", time.Now().UTC().UnixNano()),
		}, nil
	case "canvas.present":
		return map[string]any{
			"ok":          true,
			"frameRef":    toString(params["frameRef"], "canvas://latest"),
			"presentedAt": time.Now().UTC().Format(time.RFC3339),
		}, nil
	case "exec.approvals.get":
		return s.handleCompatExecApprovalsGet(), nil
	case "exec.approvals.set":
		return s.handleCompatExecApprovalsSet(params), nil
	case "exec.approvals.node.get":
		return s.handleCompatExecApprovalsNodeGet(params), nil
	case "exec.approvals.node.set":
		return s.handleCompatExecApprovalsNodeSet(params), nil
	case "exec.approval.request":
		return s.handleCompatExecApprovalRequest(params), nil
	case "exec.approval.waitdecision":
		return s.handleCompatExecApprovalWait(ctx, params)
	case "exec.approval.resolve":
		return s.handleCompatExecApprovalResolve(params), nil
	case "poll":
		return s.handleCompatPoll(params), nil
	case "chat.abort":
		return map[string]any{
			"ok":      true,
			"jobId":   toString(params["jobId"], ""),
			"aborted": true,
		}, nil
	case "chat.inject":
		return s.handleCompatChatInject(params), nil
	case "config.set":
		return s.handleCompatConfigSet(params)
	case "config.patch":
		return s.handleCompatConfigPatch(params), nil
	case "config.apply":
		return s.handleCompatConfigApply(params), nil
	case "config.schema":
		return s.handleCompatConfigSchema(), nil
	case "logs.tail":
		return s.handleCompatLogsTail(params), nil
	case "sessions.preview":
		return s.handleCompatSessionsPreview(params), nil
	case "sessions.patch":
		return s.handleCompatSessionsPatch(params)
	case "sessions.resolve":
		return s.handleCompatSessionsResolve(params)
	case "sessions.reset":
		return s.handleCompatSessionsReset(params), nil
	case "sessions.delete":
		return s.handleCompatSessionsDelete(params), nil
	case "sessions.compact":
		return s.handleCompatSessionsCompact(params), nil
	case "sessions.usage":
		return s.handleCompatSessionsUsage(params), nil
	case "sessions.usage.timeseries":
		return s.handleCompatSessionsUsageTimeseries(params), nil
	case "sessions.usage.logs":
		return s.handleCompatSessionsUsageLogs(params), nil
	default:
		return nil, &dispatchError{
			Code:    -32601,
			Message: "compat method not implemented",
			Details: map[string]any{
				"method":    canonical,
				"requestId": requestID,
			},
		}
	}
}

func (s *Server) handleCompatDoctorMemory() map[string]any {
	lastError := s.memory.LastError()
	return map[string]any{
		"healthy":      strings.TrimSpace(lastError) == "",
		"entryCount":   s.memory.Count(),
		"statePath":    s.cfg.Runtime.StatePath,
		"persisted":    !strings.HasPrefix(strings.ToLower(strings.TrimSpace(s.cfg.Runtime.StatePath)), "memory://"),
		"lastError":    lastError,
		"checkedAt":    time.Now().UTC().Format(time.RFC3339),
		"maxRetention": 10000,
		"stats":        s.memory.Stats(),
	}
}

func (s *Server) handleCompatUsageStatus() map[string]any {
	entries := s.memory.HistoryBySession("", 5000)
	messageCount := len(entries)
	tokenEstimate := 0
	for _, entry := range entries {
		tokenEstimate += countWords(entry.Text)
	}
	return map[string]any{
		"window": map[string]any{
			"messages": messageCount,
			"tokens":   tokenEstimate,
		},
		"sessions":  s.sessions.Count(),
		"updatedAt": time.Now().UTC().Format(time.RFC3339),
	}
}

func (s *Server) handleCompatUsageCost() map[string]any {
	status := s.handleCompatUsageStatus()
	window := asStringMap(status["window"])
	tokens := toInt(window["tokens"], 0)
	costUSD := float64(tokens) * 0.000002
	return map[string]any{
		"currency": "USD",
		"tokens":   tokens,
		"cost":     costUSD,
		"window":   window,
	}
}

func (s *Server) handleCompatSystemPresence(params map[string]any) map[string]any {
	s.compat.mu.Lock()
	if len(params) > 0 {
		for key, value := range params {
			s.compat.presence[key] = value
		}
		s.compat.presence["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	}
	presence := cloneMap(s.compat.presence)
	s.compat.mu.Unlock()
	return map[string]any{
		"presence": presence,
	}
}

func (s *Server) handleCompatTalkConfig(params map[string]any) map[string]any {
	s.compat.mu.Lock()
	incoming := asStringMap(params["config"])
	if len(incoming) == 0 {
		for key, value := range params {
			if key == "sessionId" || key == "id" {
				continue
			}
			incoming[key] = value
		}
	}
	for key, value := range incoming {
		s.compat.talkConfig[key] = value
	}
	config := cloneMap(s.compat.talkConfig)
	s.compat.mu.Unlock()
	return map[string]any{
		"config": config,
	}
}

func (s *Server) handleCompatTalkMode(params map[string]any) map[string]any {
	s.compat.mu.Lock()
	mode := toString(params["mode"], "")
	if mode != "" {
		s.compat.talkMode = mode
	}
	current := s.compat.talkMode
	s.compat.mu.Unlock()
	return map[string]any{
		"mode": current,
	}
}

const (
	defaultCompatTTSProvider  = "native"
	defaultCompatTTSFormat    = "wav"
	defaultKittenTTSTimeoutMs = 25000
)

func normalizeTTSProviderID(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.NewReplacer("_", "-", " ", "-", ".", "-").Replace(normalized)
	switch normalized {
	case "", "default", "local":
		return defaultCompatTTSProvider
	case "native", "edge", "edge-tts":
		return "native"
	case "openai", "voice", "openai-voice", "openai-tts", "openai-voice-bridge":
		return "openai-voice"
	case "elevenlabs", "eleven-labs", "11labs":
		return "elevenlabs"
	case "kittentts", "kitten-tts", "kitten-tts-cli", "kittytts":
		return "kittentts"
	default:
		return normalized
	}
}

func supportedCompatTTSProviders() []string {
	return []string{"native", "openai-voice", "kittentts", "elevenlabs"}
}

func (s *Server) isSupportedCompatTTSProvider(provider string) bool {
	normalized := normalizeTTSProviderID(provider)
	for _, item := range supportedCompatTTSProviders() {
		if item == normalized {
			return true
		}
	}
	return false
}

func resolveKittenTTSBinary() (string, string) {
	for _, envKey := range []string{"OPENCLAW_GO_KITTENTTS_BIN", "OPENCLAW_GO_TTS_KITTENTTS_BIN"} {
		candidate := strings.TrimSpace(os.Getenv(envKey))
		if candidate == "" {
			continue
		}
		if path, err := osexec.LookPath(candidate); err == nil {
			return path, envKey
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate, envKey
		}
	}
	if path, err := osexec.LookPath("kittentts"); err == nil {
		return path, "PATH"
	}
	return "", ""
}

func (s *Server) compatTTSProviderCatalog() []map[string]any {
	kittenttsPath, kittenttsSource := resolveKittenTTSBinary()
	elevenlabsConfigured := strings.TrimSpace(os.Getenv("ELEVENLABS_API_KEY")) != "" || s.compat.hasProviderAPIKey("elevenlabs")
	openAIVoiceEnabled := s.cfg.Runtime.BrowserBridge.Enabled && strings.TrimSpace(s.cfg.Runtime.BrowserBridge.Endpoint) != ""

	kittenttsReason := "kittentts binary not found"
	if kittenttsPath != "" {
		kittenttsReason = fmt.Sprintf("kittentts binary found via %s", kittenttsSource)
	}

	return []map[string]any{
		{
			"id":        "native",
			"name":      "Native Synth",
			"enabled":   true,
			"available": true,
			"reason":    "built-in synthetic fallback",
		},
		{
			"id":        "openai-voice",
			"name":      "OpenAI Voice",
			"enabled":   openAIVoiceEnabled,
			"available": openAIVoiceEnabled,
			"reason": map[bool]string{
				true:  "browser bridge configured",
				false: "browser bridge disabled or endpoint missing",
			}[openAIVoiceEnabled],
		},
		{
			"id":        "kittentts",
			"name":      "KittenTTS",
			"enabled":   kittenttsPath != "",
			"available": kittenttsPath != "",
			"reason":    kittenttsReason,
		},
		{
			"id":           "elevenlabs",
			"name":         "ElevenLabs",
			"enabled":      elevenlabsConfigured,
			"available":    elevenlabsConfigured,
			"requiresAuth": true,
			"reason": map[bool]string{
				true:  "api key available",
				false: "api key missing",
			}[elevenlabsConfigured],
		},
	}
}

func (s *Server) compatTTSProviderInfo(provider string) map[string]any {
	normalized := normalizeTTSProviderID(provider)
	for _, item := range s.compatTTSProviderCatalog() {
		if normalizeTTSProviderID(toString(item["id"], "")) == normalized {
			return cloneMap(item)
		}
	}
	return map[string]any{
		"id":        normalized,
		"name":      normalized,
		"enabled":   false,
		"available": false,
		"reason":    "provider not supported",
	}
}

func (s *Server) handleCompatTTSProviders() map[string]any {
	status := s.handleCompatTTSStatus()
	return map[string]any{
		"providers": s.compatTTSProviderCatalog(),
		"current": map[string]any{
			"provider":  toString(status["provider"], defaultCompatTTSProvider),
			"enabled":   toBool(status["enabled"], true),
			"available": toBool(status["available"], true),
		},
	}
}

func (s *Server) handleCompatTTSStatus() map[string]any {
	s.compat.mu.RLock()
	enabled := s.compat.ttsEnabled
	provider := normalizeTTSProviderID(s.compat.ttsProvider)
	if provider == "" {
		provider = defaultCompatTTSProvider
	}
	s.compat.mu.RUnlock()
	info := s.compatTTSProviderInfo(provider)
	return map[string]any{
		"enabled":    enabled,
		"provider":   provider,
		"available":  toBool(info["available"], false),
		"providerId": toString(info["id"], provider),
		"name":       toString(info["name"], provider),
		"reason":     toString(info["reason"], ""),
	}
}

func (s *Server) handleCompatTTSEnable(enabled bool) map[string]any {
	s.compat.mu.Lock()
	s.compat.ttsEnabled = enabled
	provider := normalizeTTSProviderID(s.compat.ttsProvider)
	if provider == "" {
		provider = defaultCompatTTSProvider
		s.compat.ttsProvider = provider
	}
	s.compat.mu.Unlock()
	info := s.compatTTSProviderInfo(provider)
	return map[string]any{
		"enabled":   enabled,
		"provider":  provider,
		"available": toBool(info["available"], false),
		"reason":    toString(info["reason"], ""),
	}
}

func (s *Server) handleCompatSetTTSProvider(params map[string]any) (map[string]any, *dispatchError) {
	provider := normalizeTTSProviderID(toString(params["provider"], ""))
	if provider == "" {
		return nil, &dispatchError{
			Code:    -32602,
			Message: "missing provider",
		}
	}
	if !s.isSupportedCompatTTSProvider(provider) {
		return nil, &dispatchError{
			Code:    -32602,
			Message: "unsupported provider",
			Details: map[string]any{
				"provider":  provider,
				"supported": supportedCompatTTSProviders(),
			},
		}
	}

	s.compat.mu.Lock()
	s.compat.ttsProvider = provider
	enabled := s.compat.ttsEnabled
	s.compat.mu.Unlock()

	info := s.compatTTSProviderInfo(provider)
	return map[string]any{
		"provider":  provider,
		"enabled":   enabled,
		"available": toBool(info["available"], false),
		"reason":    toString(info["reason"], ""),
	}, nil
}

func normalizeTTSOutputFormat(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.TrimPrefix(normalized, ".")
	if normalized == "" {
		return defaultCompatTTSFormat
	}
	switch normalized {
	case "wav", "mp3", "ogg", "flac", "opus":
		return normalized
	default:
		return defaultCompatTTSFormat
	}
}

func parseEnvInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	return toInt(raw, fallback)
}

func buildSyntheticTTSWave(text string) []byte {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		runes = []rune("openclaw")
	}

	const (
		sampleRate     = 16000
		samplesPerRune = 320
		amplitude      = 0.35
	)

	totalSamples := len(runes) * samplesPerRune
	if totalSamples < sampleRate/2 {
		totalSamples = sampleRate / 2
	}
	pcm := make([]int16, totalSamples)
	for i := 0; i < totalSamples; i++ {
		r := runes[(i/samplesPerRune)%len(runes)]
		frequency := 180.0 + float64(int(r)%48)*9.0
		phase := float64(i%samplesPerRune) / float64(samplesPerRune)
		envelope := 1.0
		if phase < 0.08 {
			envelope = phase / 0.08
		} else if phase > 0.92 {
			envelope = (1.0 - phase) / 0.08
		}
		if envelope < 0 {
			envelope = 0
		}
		angle := 2.0 * math.Pi * frequency * float64(i) / sampleRate
		pcm[i] = int16(math.Sin(angle) * envelope * amplitude * 32767.0)
	}

	dataLen := len(pcm) * 2
	buf := bytes.NewBuffer(make([]byte, 0, 44+dataLen))
	_ = binary.Write(buf, binary.LittleEndian, []byte("RIFF"))
	_ = binary.Write(buf, binary.LittleEndian, uint32(36+dataLen))
	_ = binary.Write(buf, binary.LittleEndian, []byte("WAVE"))
	_ = binary.Write(buf, binary.LittleEndian, []byte("fmt "))
	_ = binary.Write(buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1)) // PCM
	_ = binary.Write(buf, binary.LittleEndian, uint16(1)) // mono
	_ = binary.Write(buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(buf, binary.LittleEndian, uint32(sampleRate*2))
	_ = binary.Write(buf, binary.LittleEndian, uint16(2))
	_ = binary.Write(buf, binary.LittleEndian, uint16(16))
	_ = binary.Write(buf, binary.LittleEndian, []byte("data"))
	_ = binary.Write(buf, binary.LittleEndian, uint32(dataLen))
	for _, sample := range pcm {
		_ = binary.Write(buf, binary.LittleEndian, sample)
	}
	return buf.Bytes()
}

func truncateForMetadata(value string, limit int) string {
	trimmed := strings.TrimSpace(value)
	if limit <= 0 || len(trimmed) <= limit {
		return trimmed
	}
	return trimmed[:limit] + "..."
}

func (s *Server) runKittenTTSSynthesis(text string, outputFormat string) ([]byte, string, map[string]any, error) {
	binPath, source := resolveKittenTTSBinary()
	if binPath == "" {
		return nil, outputFormat, map[string]any{
			"engine": "kittentts",
		}, fmt.Errorf("kittentts binary not found; set OPENCLAW_GO_KITTENTTS_BIN or install kittentts in PATH")
	}

	argsRaw := strings.TrimSpace(os.Getenv("OPENCLAW_GO_KITTENTTS_ARGS"))
	args := []string{}
	if argsRaw != "" {
		args = strings.Fields(argsRaw)
	}

	tmpDir, err := os.MkdirTemp("", "openclaw-go-kittentts-*")
	if err != nil {
		return nil, outputFormat, map[string]any{"engine": "kittentts"}, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	outputPath := filepath.Join(tmpDir, "tts."+normalizeTTSOutputFormat(outputFormat))
	usesTextPlaceholder := false
	usesOutputPlaceholder := false
	for idx := range args {
		if strings.Contains(args[idx], "{{text}}") {
			args[idx] = strings.ReplaceAll(args[idx], "{{text}}", text)
			usesTextPlaceholder = true
		}
		if strings.Contains(args[idx], "{{output}}") {
			args[idx] = strings.ReplaceAll(args[idx], "{{output}}", outputPath)
			usesOutputPlaceholder = true
		}
	}

	timeoutMs := parseEnvInt("OPENCLAW_GO_KITTENTTS_TIMEOUT_MS", defaultKittenTTSTimeoutMs)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	cmd := osexec.CommandContext(ctx, binPath, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if !usesTextPlaceholder {
		cmd.Stdin = strings.NewReader(text)
	}

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, outputFormat, map[string]any{
				"engine":        "kittentts",
				"timeoutMs":     timeoutMs,
				"binary":        binPath,
				"binarySource":  source,
				"stderrPreview": truncateForMetadata(stderr.String(), 320),
			}, fmt.Errorf("kittentts execution timed out after %dms", timeoutMs)
		}
		return nil, outputFormat, map[string]any{
			"engine":        "kittentts",
			"binary":        binPath,
			"binarySource":  source,
			"stderrPreview": truncateForMetadata(stderr.String(), 320),
		}, fmt.Errorf("kittentts execution failed: %w", err)
	}

	audioBytes := []byte{}
	format := normalizeTTSOutputFormat(outputFormat)
	if usesOutputPlaceholder {
		if fileBytes, readErr := os.ReadFile(outputPath); readErr == nil {
			audioBytes = fileBytes
			ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(outputPath)), ".")
			if ext != "" {
				format = normalizeTTSOutputFormat(ext)
			}
		}
	}
	if len(audioBytes) == 0 {
		audioBytes = append(audioBytes, stdout.Bytes()...)
	}
	if len(audioBytes) == 0 {
		return nil, format, map[string]any{
			"engine":        "kittentts",
			"binary":        binPath,
			"binarySource":  source,
			"stderrPreview": truncateForMetadata(stderr.String(), 320),
		}, fmt.Errorf("kittentts produced no audio output")
	}

	return audioBytes, format, map[string]any{
		"engine":              "kittentts",
		"binary":              binPath,
		"binarySource":        source,
		"argsConfigured":      argsRaw != "",
		"textPlaceholderUsed": usesTextPlaceholder,
		"outputFileUsed":      usesOutputPlaceholder,
		"stderrPreview":       truncateForMetadata(stderr.String(), 320),
	}, nil
}

func (s *Server) handleCompatTTSConvert(params map[string]any) (map[string]any, *dispatchError) {
	text := toString(params["text"], toString(params["message"], ""))
	if strings.TrimSpace(text) == "" {
		return nil, &dispatchError{
			Code:    -32602,
			Message: "missing text",
		}
	}

	status := s.handleCompatTTSStatus()
	if !toBool(status["enabled"], true) {
		return nil, &dispatchError{
			Code:    -32050,
			Message: "tts is disabled",
		}
	}

	requestedProvider := normalizeTTSProviderID(toString(params["provider"], toString(status["provider"], defaultCompatTTSProvider)))
	if !s.isSupportedCompatTTSProvider(requestedProvider) {
		return nil, &dispatchError{
			Code:    -32602,
			Message: "unsupported provider",
			Details: map[string]any{
				"provider":  requestedProvider,
				"supported": supportedCompatTTSProviders(),
			},
		}
	}

	requireRealAudio := toBool(params["requireRealAudio"], toBool(params["require_real_audio"], false))
	requestedFormat := normalizeTTSOutputFormat(toString(params["format"], toString(params["outputFormat"], defaultCompatTTSFormat)))

	audioBytes := []byte{}
	outputFormat := requestedFormat
	realAudio := false
	engine := "synthetic"
	providerUsed := requestedProvider
	synthesisError := ""
	debug := map[string]any{"engine": "synthetic"}

	switch requestedProvider {
	case "kittentts":
		var err error
		audioBytes, outputFormat, debug, err = s.runKittenTTSSynthesis(text, requestedFormat)
		if err != nil {
			synthesisError = err.Error()
		} else {
			realAudio = true
			engine = "kittentts"
		}
	case "native":
		audioBytes = buildSyntheticTTSWave(text)
		outputFormat = "wav"
		engine = "native-synthetic"
	case "openai-voice", "elevenlabs":
		synthesisError = fmt.Sprintf("%s real synthesis bridge is not configured in this runtime", requestedProvider)
	default:
		synthesisError = fmt.Sprintf("provider %s is not implemented", requestedProvider)
	}

	if synthesisError != "" && requireRealAudio {
		return nil, &dispatchError{
			Code:    -32050,
			Message: "real tts synthesis failed",
			Details: map[string]any{
				"provider": requestedProvider,
				"error":    synthesisError,
			},
		}
	}

	if len(audioBytes) == 0 {
		audioBytes = buildSyntheticTTSWave(text)
		outputFormat = "wav"
		if providerUsed != "native" {
			providerUsed = "native"
		}
		engine = "synthetic-fallback"
	}

	audioRef := fmt.Sprintf("memory://tts/%d.%s", time.Now().UTC().UnixNano(), outputFormat)
	return map[string]any{
		"ok":                true,
		"text":              text,
		"requestedProvider": requestedProvider,
		"provider":          providerUsed,
		"audioRef":          audioRef,
		"bytes":             len(audioBytes),
		"audioBase64":       base64.StdEncoding.EncodeToString(audioBytes),
		"outputFormat":      outputFormat,
		"realAudio":         realAudio,
		"engine":            engine,
		"fallback":          providerUsed != requestedProvider,
		"synthesisError":    synthesisError,
		"synthesizedAt":     time.Now().UTC().Format(time.RFC3339),
		"debug":             debug,
	}, nil
}

func (s *Server) handleCompatModelsList(params map[string]any) (map[string]any, *dispatchError) {
	allowed := map[string]struct{}{
		"provider": {},
		"limit":    {},
	}
	for key := range params {
		if _, ok := allowed[key]; !ok {
			return nil, &dispatchError{
				Code:    -32602,
				Message: fmt.Sprintf("invalid models.list params: unknown field %q", key),
				Details: map[string]any{"field": key},
			}
		}
	}

	requestedProvider := normalizeProviderAlias(toString(params["provider"], ""))
	models := s.compat.listModelDescriptors()
	filtered := make([]map[string]any, 0, len(models))
	for _, model := range models {
		provider := normalizeProviderAlias(toString(model["provider"], ""))
		if requestedProvider != "" && provider != requestedProvider {
			continue
		}
		filtered = append(filtered, model)
	}
	sort.Slice(filtered, func(i, j int) bool {
		pi := normalizeProviderAlias(toString(filtered[i]["provider"], ""))
		pj := normalizeProviderAlias(toString(filtered[j]["provider"], ""))
		if pi == pj {
			return strings.ToLower(toString(filtered[i]["id"], "")) < strings.ToLower(toString(filtered[j]["id"], ""))
		}
		return pi < pj
	})

	limit := toInt(params["limit"], 0)
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}

	providers := make([]string, 0, len(filtered))
	seen := map[string]struct{}{}
	for _, model := range filtered {
		provider := normalizeProviderAlias(toString(model["provider"], ""))
		if provider == "" {
			continue
		}
		if _, ok := seen[provider]; ok {
			continue
		}
		seen[provider] = struct{}{}
		providers = append(providers, provider)
	}
	sort.Strings(providers)

	payload := map[string]any{
		"count":  len(filtered),
		"models": filtered,
	}
	if requestedProvider != "" {
		payload["providerRequested"] = requestedProvider
	}
	payload["providers"] = providers
	return payload, nil
}

func (s *Server) handleCompatVoiceWake(params map[string]any) map[string]any {
	s.compat.mu.Lock()
	if params != nil && len(params) > 0 {
		s.compat.voiceWakeEnabled = toBool(params["enabled"], s.compat.voiceWakeEnabled)
	}
	enabled := s.compat.voiceWakeEnabled
	s.compat.mu.Unlock()
	return map[string]any{
		"enabled": enabled,
	}
}

func (s *Server) handleCompatAgentsList() map[string]any {
	s.compat.mu.RLock()
	items := make([]map[string]any, 0, len(s.compat.agents))
	for _, agent := range s.compat.agents {
		items = append(items, cloneMap(agent))
	}
	s.compat.mu.RUnlock()
	sort.Slice(items, func(i, j int) bool {
		return toString(items[i]["agentId"], "") < toString(items[j]["agentId"], "")
	})
	return map[string]any{
		"count": len(items),
		"items": items,
	}
}

func (s *Server) handleCompatAgentCreate(params map[string]any) map[string]any {
	s.compat.mu.Lock()
	s.compat.agentSeq++
	agentID := fmt.Sprintf("agent-%04d", s.compat.agentSeq)
	agent := map[string]any{
		"agentId":     agentID,
		"name":        toString(params["name"], agentID),
		"description": toString(params["description"], ""),
		"model":       toString(params["model"], "gpt-5.2"),
		"createdAt":   time.Now().UTC().Format(time.RFC3339),
		"updatedAt":   time.Now().UTC().Format(time.RFC3339),
		"status":      "ready",
	}
	s.compat.agents[agentID] = agent
	if _, ok := s.compat.agentFiles[agentID]; !ok {
		s.compat.agentFiles[agentID] = map[string]map[string]any{}
	}
	s.compat.mu.Unlock()
	return map[string]any{"agent": cloneMap(agent)}
}

func (s *Server) handleCompatAgentUpdate(params map[string]any) (map[string]any, *dispatchError) {
	agentID := toString(params["agentId"], toString(params["id"], ""))
	if agentID == "" {
		return nil, &dispatchError{Code: -32602, Message: "missing agentId"}
	}
	s.compat.mu.Lock()
	agent, ok := s.compat.agents[agentID]
	if !ok {
		s.compat.mu.Unlock()
		return nil, &dispatchError{
			Code:    -32004,
			Message: "agent not found",
			Details: map[string]any{"agentId": agentID},
		}
	}
	for _, key := range []string{"name", "description", "model", "status"} {
		if value, exists := params[key]; exists {
			agent[key] = value
		}
	}
	agent["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	s.compat.agents[agentID] = agent
	s.compat.mu.Unlock()
	return map[string]any{"agent": cloneMap(agent)}, nil
}

func (s *Server) handleCompatAgentDelete(params map[string]any) (map[string]any, *dispatchError) {
	agentID := toString(params["agentId"], toString(params["id"], ""))
	if agentID == "" {
		return nil, &dispatchError{Code: -32602, Message: "missing agentId"}
	}
	s.compat.mu.Lock()
	_, exists := s.compat.agents[agentID]
	if exists {
		delete(s.compat.agents, agentID)
		delete(s.compat.agentFiles, agentID)
	}
	s.compat.mu.Unlock()
	return map[string]any{
		"ok":      exists,
		"agentId": agentID,
	}, nil
}

func (s *Server) handleCompatAgentFilesList(params map[string]any) (map[string]any, *dispatchError) {
	agentID := toString(params["agentId"], "")
	if agentID == "" {
		return nil, &dispatchError{Code: -32602, Message: "missing agentId"}
	}
	s.compat.mu.RLock()
	files := s.compat.agentFiles[agentID]
	items := make([]map[string]any, 0, len(files))
	for _, file := range files {
		items = append(items, cloneMap(file))
	}
	s.compat.mu.RUnlock()
	sort.Slice(items, func(i, j int) bool {
		return toString(items[i]["fileId"], "") < toString(items[j]["fileId"], "")
	})
	return map[string]any{"count": len(items), "items": items}, nil
}

func (s *Server) handleCompatAgentFilesGet(params map[string]any) (map[string]any, *dispatchError) {
	agentID := toString(params["agentId"], "")
	fileID := toString(params["fileId"], toString(params["id"], ""))
	if agentID == "" || fileID == "" {
		return nil, &dispatchError{Code: -32602, Message: "missing agentId or fileId"}
	}
	s.compat.mu.RLock()
	files := s.compat.agentFiles[agentID]
	file, ok := files[fileID]
	s.compat.mu.RUnlock()
	if !ok {
		return nil, &dispatchError{
			Code:    -32004,
			Message: "file not found",
			Details: map[string]any{"agentId": agentID, "fileId": fileID},
		}
	}
	return map[string]any{"file": cloneMap(file)}, nil
}

func (s *Server) handleCompatAgentFilesSet(params map[string]any) (map[string]any, *dispatchError) {
	agentID := toString(params["agentId"], "")
	if agentID == "" {
		return nil, &dispatchError{Code: -32602, Message: "missing agentId"}
	}
	fileID := toString(params["fileId"], "")
	if fileID == "" {
		fileID = fmt.Sprintf("file-%d", time.Now().UTC().UnixNano())
	}
	entry := map[string]any{
		"agentId":   agentID,
		"fileId":    fileID,
		"path":      toString(params["path"], ""),
		"content":   toString(params["content"], ""),
		"updatedAt": time.Now().UTC().Format(time.RFC3339),
	}
	s.compat.mu.Lock()
	if _, ok := s.compat.agentFiles[agentID]; !ok {
		s.compat.agentFiles[agentID] = map[string]map[string]any{}
	}
	s.compat.agentFiles[agentID][fileID] = entry
	s.compat.mu.Unlock()
	return map[string]any{"file": cloneMap(entry)}, nil
}

func (s *Server) handleCompatSkillsStatus() map[string]any {
	s.compat.mu.RLock()
	items := make([]map[string]any, 0, len(s.compat.installedSkills))
	for _, skill := range s.compat.installedSkills {
		items = append(items, cloneMap(skill))
	}
	s.compat.mu.RUnlock()
	sort.Slice(items, func(i, j int) bool {
		return toString(items[i]["name"], "") < toString(items[j]["name"], "")
	})
	return map[string]any{
		"count": len(items),
		"items": items,
	}
}

func (s *Server) handleCompatSkillsBins() map[string]any {
	status := s.handleCompatSkillsStatus()
	raw, _ := status["items"].([]map[string]any)
	bins := make([]string, 0, len(raw))
	for _, entry := range raw {
		name := toString(entry["name"], "")
		if name != "" {
			bins = append(bins, fmt.Sprintf("bin/%s", name))
		}
	}
	sort.Strings(bins)
	return map[string]any{
		"count": len(bins),
		"bins":  bins,
	}
}

func (s *Server) handleCompatSkillsInstall(params map[string]any) map[string]any {
	name := toString(params["name"], toString(params["skill"], ""))
	if name == "" {
		name = fmt.Sprintf("skill-%d", time.Now().UTC().Unix())
	}
	key := strings.ToLower(strings.TrimSpace(name))
	s.compat.mu.Lock()
	s.compat.skillSeq++
	entry := map[string]any{
		"id":        fmt.Sprintf("skill-%04d", s.compat.skillSeq),
		"name":      name,
		"source":    toString(params["source"], "local"),
		"version":   toString(params["version"], "latest"),
		"updatedAt": time.Now().UTC().Format(time.RFC3339),
		"installed": true,
	}
	s.compat.installedSkills[key] = entry
	s.compat.mu.Unlock()
	return map[string]any{
		"ok":    true,
		"skill": cloneMap(entry),
	}
}

func (s *Server) handleCompatSkillsUpdate(params map[string]any) map[string]any {
	name := strings.ToLower(toString(params["name"], toString(params["skill"], "")))
	if name == "" {
		return s.handleCompatSkillsInstall(params)
	}
	version := toString(params["version"], "latest")
	s.compat.mu.Lock()
	entry, ok := s.compat.installedSkills[name]
	if !ok {
		s.compat.skillSeq++
		entry = map[string]any{
			"id":        fmt.Sprintf("skill-%04d", s.compat.skillSeq),
			"name":      name,
			"source":    "local",
			"installed": true,
		}
	}
	entry["version"] = version
	entry["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	s.compat.installedSkills[name] = entry
	s.compat.mu.Unlock()
	return map[string]any{
		"ok":    true,
		"skill": cloneMap(entry),
	}
}

func (s *Server) handleCompatAuthOAuthProviders(params map[string]any) (map[string]any, *dispatchError) {
	allowed := map[string]struct{}{
		"provider": {},
	}
	for key := range params {
		if _, ok := allowed[key]; !ok {
			return nil, &dispatchError{
				Code:    -32602,
				Message: fmt.Sprintf("invalid auth.oauth.providers params: unknown field %q", key),
				Details: map[string]any{"field": key},
			}
		}
	}

	requestedProvider := normalizeProviderAlias(toString(params["provider"], ""))
	providers := authProviderCatalogPayload(func(provider string) bool {
		return s.compat.hasProviderAPIKey(provider)
	})
	filtered := make([]map[string]any, 0, len(providers))
	for _, provider := range providers {
		id := normalizeProviderAlias(toString(provider["id"], ""))
		if requestedProvider != "" && id != requestedProvider {
			continue
		}
		filtered = append(filtered, provider)
	}
	sort.Slice(filtered, func(i, j int) bool {
		return toString(filtered[i]["id"], "") < toString(filtered[j]["id"], "")
	})

	payload := map[string]any{
		"count":     len(filtered),
		"providers": filtered,
	}
	if requestedProvider != "" {
		payload["providerRequested"] = requestedProvider
	}
	return payload, nil
}

func (s *Server) handleCompatOAuthImport(params map[string]any) (map[string]any, *dispatchError) {
	allowed := map[string]struct{}{
		"provider":       {},
		"model":          {},
		"loginSessionId": {},
		"code":           {},
	}
	for key := range params {
		if _, ok := allowed[key]; !ok {
			return nil, &dispatchError{
				Code:    -32602,
				Message: fmt.Sprintf("invalid auth.oauth.import params: unknown field %q", key),
				Details: map[string]any{"field": key},
			}
		}
	}

	providerRaw := toString(params["provider"], "chatgpt")
	providerEntry, ok := resolveOAuthProviderCatalogEntry(providerRaw)
	if !ok {
		return nil, &dispatchError{
			Code:    -32602,
			Message: "unknown oauth provider",
			Details: map[string]any{
				"provider": providerRaw,
				"known":    knownAuthProviders(),
			},
		}
	}
	provider := providerEntry.ID
	model := toString(params["model"], "")
	if strings.TrimSpace(model) == "" {
		if fallbackModel, ok := s.compat.defaultModelForProvider(provider); ok {
			model = fallbackModel
		} else {
			model = "gpt-5.2"
		}
	}
	loginID := toString(params["loginSessionId"], "")
	code := strings.TrimSpace(toString(params["code"], ""))

	if loginID == "" {
		session := s.webLogin.Start(webLoginStartOptions(provider, model))
		loginID = session.ID
		if code == "" {
			code = session.Code
		}
	} else if code == "" {
		session, ok := s.webLogin.Get(loginID)
		if !ok {
			return nil, &dispatchError{
				Code:    -32004,
				Message: "login session not found",
				Details: map[string]any{"loginSessionId": loginID},
			}
		}
		code = session.Code
	}

	session, err := s.webLogin.Complete(loginID, code)
	if err != nil {
		return nil, &dispatchError{
			Code:    -32041,
			Message: err.Error(),
			Details: map[string]any{"loginSessionId": loginID},
		}
	}
	return map[string]any{
		"imported":            true,
		"providerId":          providerEntry.ID,
		"providerDisplayName": providerEntry.DisplayName,
		"login":               session,
	}, nil
}

func webLoginStartOptions(provider string, model string) webbridge.StartOptions {
	return webbridge.StartOptions{
		Provider: provider,
		Model:    model,
	}
}

func (s *Server) handleCompatWizardStart(params map[string]any) map[string]any {
	flow := toString(params["flow"], "default")
	s.compat.mu.Lock()
	s.compat.wizard["active"] = true
	s.compat.wizard["status"] = "running"
	s.compat.wizard["step"] = 1
	s.compat.wizard["flow"] = flow
	s.compat.wizard["startedAt"] = time.Now().UTC().Format(time.RFC3339)
	snapshot := cloneMap(s.compat.wizard)
	s.compat.mu.Unlock()
	return map[string]any{"wizard": snapshot}
}

func (s *Server) handleCompatWizardNext(params map[string]any) (map[string]any, *dispatchError) {
	s.compat.mu.Lock()
	active := toBool(s.compat.wizard["active"], false)
	if !active {
		s.compat.mu.Unlock()
		return nil, &dispatchError{
			Code:    -32004,
			Message: "wizard is not active",
		}
	}
	step := toInt(s.compat.wizard["step"], 0) + 1
	s.compat.wizard["step"] = step
	if done := toBool(params["done"], false); done {
		s.compat.wizard["active"] = false
		s.compat.wizard["status"] = "completed"
	}
	s.compat.wizard["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	snapshot := cloneMap(s.compat.wizard)
	s.compat.mu.Unlock()
	return map[string]any{"wizard": snapshot}, nil
}

func (s *Server) handleCompatWizardCancel(params map[string]any) map[string]any {
	reason := toString(params["reason"], "cancelled")
	s.compat.mu.Lock()
	s.compat.wizard["active"] = false
	s.compat.wizard["status"] = "cancelled"
	s.compat.wizard["reason"] = reason
	s.compat.wizard["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	snapshot := cloneMap(s.compat.wizard)
	s.compat.mu.Unlock()
	return map[string]any{"wizard": snapshot}
}

func (s *Server) handleCompatWizardStatus() map[string]any {
	s.compat.mu.RLock()
	snapshot := cloneMap(s.compat.wizard)
	s.compat.mu.RUnlock()
	return map[string]any{"wizard": snapshot}
}

func (s *Server) handleCompatDevicePairList() map[string]any {
	s.compat.mu.RLock()
	items := make([]map[string]any, 0, len(s.compat.devicePairs))
	for _, pair := range s.compat.devicePairs {
		items = append(items, cloneMap(pair))
	}
	s.compat.mu.RUnlock()
	return map[string]any{"count": len(items), "items": items}
}

func (s *Server) handleCompatDevicePairUpdate(params map[string]any, status string) (map[string]any, *dispatchError) {
	pairID := toString(params["pairId"], toString(params["id"], ""))
	if pairID == "" {
		return nil, &dispatchError{Code: -32602, Message: "missing pairId"}
	}
	s.compat.mu.Lock()
	pair, ok := s.compat.devicePairs[pairID]
	if !ok {
		pair = map[string]any{
			"pairId":    pairID,
			"deviceId":  toString(params["deviceId"], pairID),
			"createdAt": time.Now().UTC().Format(time.RFC3339),
		}
	}
	pair["status"] = status
	pair["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	s.compat.devicePairs[pairID] = pair
	s.compat.mu.Unlock()
	return map[string]any{"pair": cloneMap(pair)}, nil
}

func (s *Server) handleCompatDevicePairRemove(params map[string]any) (map[string]any, *dispatchError) {
	pairID := toString(params["pairId"], toString(params["id"], ""))
	if pairID == "" {
		return nil, &dispatchError{Code: -32602, Message: "missing pairId"}
	}
	s.compat.mu.Lock()
	_, ok := s.compat.devicePairs[pairID]
	if ok {
		delete(s.compat.devicePairs, pairID)
	}
	s.compat.mu.Unlock()
	return map[string]any{"ok": ok, "pairId": pairID}, nil
}

func (s *Server) handleCompatDeviceTokenRotate(params map[string]any) map[string]any {
	deviceID := toString(params["deviceId"], "default-device")
	s.compat.mu.Lock()
	s.compat.deviceTokenSeq++
	tokenID := fmt.Sprintf("token-%04d", s.compat.deviceTokenSeq)
	token := map[string]any{
		"tokenId":   tokenID,
		"deviceId":  deviceID,
		"value":     fmt.Sprintf("tok-%d", time.Now().UTC().UnixNano()),
		"revoked":   false,
		"createdAt": time.Now().UTC().Format(time.RFC3339),
	}
	s.compat.deviceTokens[tokenID] = token
	s.compat.mu.Unlock()
	return map[string]any{"token": cloneMap(token)}
}

func (s *Server) handleCompatDeviceTokenRevoke(params map[string]any) map[string]any {
	tokenID := toString(params["tokenId"], "")
	s.compat.mu.Lock()
	revoked := 0
	if tokenID == "" {
		for _, token := range s.compat.deviceTokens {
			token["revoked"] = true
			revoked++
		}
	} else if token, ok := s.compat.deviceTokens[tokenID]; ok {
		token["revoked"] = true
		revoked = 1
	}
	s.compat.mu.Unlock()
	return map[string]any{"ok": revoked > 0, "revoked": revoked}
}

func (s *Server) handleCompatNodePairRequest(params map[string]any) map[string]any {
	nodeID := toString(params["nodeId"], fmt.Sprintf("node-%d", time.Now().UTC().UnixNano()))
	s.compat.mu.Lock()
	s.compat.nodePairSeq++
	pairID := fmt.Sprintf("node-pair-%04d", s.compat.nodePairSeq)
	entry := map[string]any{
		"pairId":    pairID,
		"nodeId":    nodeID,
		"status":    "pending",
		"createdAt": time.Now().UTC().Format(time.RFC3339),
	}
	s.compat.nodePairs[pairID] = entry
	if _, ok := s.compat.nodes[nodeID]; !ok {
		s.compat.nodes[nodeID] = map[string]any{
			"nodeId":    nodeID,
			"name":      toString(params["name"], nodeID),
			"status":    "pairing",
			"createdAt": time.Now().UTC().Format(time.RFC3339),
		}
	}
	s.compat.mu.Unlock()
	return map[string]any{"pair": cloneMap(entry)}
}

func (s *Server) handleCompatNodePairList() map[string]any {
	s.compat.mu.RLock()
	items := make([]map[string]any, 0, len(s.compat.nodePairs))
	for _, pair := range s.compat.nodePairs {
		items = append(items, cloneMap(pair))
	}
	s.compat.mu.RUnlock()
	return map[string]any{"count": len(items), "items": items}
}

func (s *Server) handleCompatNodePairUpdate(params map[string]any, status string) (map[string]any, *dispatchError) {
	pairID := toString(params["pairId"], "")
	if pairID == "" {
		return nil, &dispatchError{Code: -32602, Message: "missing pairId"}
	}
	s.compat.mu.Lock()
	pair, ok := s.compat.nodePairs[pairID]
	if !ok {
		s.compat.mu.Unlock()
		return nil, &dispatchError{Code: -32004, Message: "node pair not found", Details: map[string]any{"pairId": pairID}}
	}
	pair["status"] = status
	pair["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	s.compat.nodePairs[pairID] = pair
	nodeID := toString(pair["nodeId"], "")
	if node, exists := s.compat.nodes[nodeID]; exists && status == "approved" {
		node["status"] = "online"
		node["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
		s.compat.nodes[nodeID] = node
	}
	s.compat.mu.Unlock()
	return map[string]any{"pair": cloneMap(pair)}, nil
}

func (s *Server) handleCompatNodeRename(params map[string]any) (map[string]any, *dispatchError) {
	nodeID := toString(params["nodeId"], "")
	name := toString(params["name"], "")
	if nodeID == "" || name == "" {
		return nil, &dispatchError{Code: -32602, Message: "missing nodeId or name"}
	}
	s.compat.mu.Lock()
	node, ok := s.compat.nodes[nodeID]
	if !ok {
		s.compat.mu.Unlock()
		return nil, &dispatchError{Code: -32004, Message: "node not found", Details: map[string]any{"nodeId": nodeID}}
	}
	node["name"] = name
	node["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	s.compat.nodes[nodeID] = node
	s.compat.mu.Unlock()
	return map[string]any{"node": cloneMap(node)}, nil
}

func (s *Server) handleCompatNodeList() map[string]any {
	s.compat.mu.RLock()
	items := make([]map[string]any, 0, len(s.compat.nodes))
	for _, node := range s.compat.nodes {
		items = append(items, cloneMap(node))
	}
	s.compat.mu.RUnlock()
	return map[string]any{"count": len(items), "items": items}
}

func (s *Server) handleCompatNodeDescribe(params map[string]any) (map[string]any, *dispatchError) {
	nodeID := toString(params["nodeId"], "")
	if nodeID == "" {
		return nil, &dispatchError{Code: -32602, Message: "missing nodeId"}
	}
	s.compat.mu.RLock()
	node, ok := s.compat.nodes[nodeID]
	s.compat.mu.RUnlock()
	if !ok {
		return nil, &dispatchError{Code: -32004, Message: "node not found", Details: map[string]any{"nodeId": nodeID}}
	}
	return map[string]any{"node": cloneMap(node)}, nil
}

func (s *Server) handleCompatNodeInvoke(params map[string]any) map[string]any {
	nodeID := toString(params["nodeId"], "node-local")
	resultID := fmt.Sprintf("invoke-%d", time.Now().UTC().UnixNano())
	event := map[string]any{
		"eventId":   fmt.Sprintf("node-event-%d", time.Now().UTC().UnixNano()),
		"nodeId":    nodeID,
		"type":      "invoke",
		"payload":   cloneMap(params),
		"createdAt": time.Now().UTC().Format(time.RFC3339),
		"resultId":  resultID,
	}
	s.compat.mu.Lock()
	s.compat.nodeEvents = append(s.compat.nodeEvents, event)
	if len(s.compat.nodeEvents) > 256 {
		s.compat.nodeEvents = append([]map[string]any(nil), s.compat.nodeEvents[len(s.compat.nodeEvents)-256:]...)
	}
	s.compat.mu.Unlock()
	return map[string]any{
		"accepted": true,
		"nodeId":   nodeID,
		"resultId": resultID,
	}
}

func (s *Server) handleCompatNodeInvokeResult(params map[string]any) map[string]any {
	return map[string]any{
		"resultId": toString(params["resultId"], ""),
		"status":   "completed",
		"output":   cloneMap(params),
	}
}

func (s *Server) handleCompatNodeEvent(params map[string]any) map[string]any {
	event := map[string]any{
		"eventId":   fmt.Sprintf("node-event-%d", time.Now().UTC().UnixNano()),
		"nodeId":    toString(params["nodeId"], "node-local"),
		"type":      toString(params["type"], "custom"),
		"payload":   cloneMap(params),
		"createdAt": time.Now().UTC().Format(time.RFC3339),
	}
	s.compat.mu.Lock()
	s.compat.nodeEvents = append(s.compat.nodeEvents, event)
	if len(s.compat.nodeEvents) > 256 {
		s.compat.nodeEvents = append([]map[string]any(nil), s.compat.nodeEvents[len(s.compat.nodeEvents)-256:]...)
	}
	s.compat.mu.Unlock()
	return map[string]any{"event": cloneMap(event)}
}

func (s *Server) handleCompatExecApprovalsGet() map[string]any {
	s.compat.mu.RLock()
	approvals := cloneMap(s.compat.globalApprovals)
	s.compat.mu.RUnlock()
	return map[string]any{"approvals": approvals}
}

func (s *Server) handleCompatExecApprovalsSet(params map[string]any) map[string]any {
	incoming := asStringMap(params["approvals"])
	if len(incoming) == 0 {
		incoming = cloneMap(params)
	}
	s.compat.mu.Lock()
	for key, value := range incoming {
		s.compat.globalApprovals[key] = value
	}
	s.compat.globalApprovals["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	out := cloneMap(s.compat.globalApprovals)
	s.compat.mu.Unlock()
	return map[string]any{"approvals": out}
}

func (s *Server) handleCompatExecApprovalsNodeGet(params map[string]any) map[string]any {
	nodeID := toString(params["nodeId"], "node-local")
	s.compat.mu.RLock()
	nodePolicy, ok := s.compat.nodeApprovals[nodeID]
	if !ok {
		nodePolicy = map[string]any{
			"nodeId": nodeID,
			"mode":   toString(s.compat.globalApprovals["mode"], "prompt"),
		}
	}
	snapshot := cloneMap(nodePolicy)
	s.compat.mu.RUnlock()
	return map[string]any{"approvals": snapshot}
}

func (s *Server) handleCompatExecApprovalsNodeSet(params map[string]any) map[string]any {
	nodeID := toString(params["nodeId"], "")
	if nodeID == "" {
		nodeID = "node-local"
	}
	incoming := asStringMap(params["approvals"])
	if len(incoming) == 0 {
		incoming = cloneMap(params)
	}
	s.compat.mu.Lock()
	existing, ok := s.compat.nodeApprovals[nodeID]
	if !ok {
		existing = map[string]any{"nodeId": nodeID}
	}
	for key, value := range incoming {
		existing[key] = value
	}
	existing["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	s.compat.nodeApprovals[nodeID] = existing
	out := cloneMap(existing)
	s.compat.mu.Unlock()
	return map[string]any{"approvals": out}
}

func (s *Server) handleCompatExecApprovalRequest(params map[string]any) map[string]any {
	s.compat.mu.Lock()
	s.compat.approvalSeq++
	approvalID := fmt.Sprintf("approval-%06d", s.compat.approvalSeq)
	entry := map[string]any{
		"approvalId": approvalID,
		"status":     "pending",
		"method":     toString(params["method"], ""),
		"reason":     toString(params["reason"], ""),
		"createdAt":  time.Now().UTC().Format(time.RFC3339),
	}
	s.compat.pendingApprovals[approvalID] = entry
	s.compat.mu.Unlock()
	return map[string]any{"approval": cloneMap(entry)}
}

func (s *Server) handleCompatExecApprovalWait(ctx context.Context, params map[string]any) (map[string]any, *dispatchError) {
	approvalID := toString(params["approvalId"], "")
	if approvalID == "" {
		return nil, &dispatchError{Code: -32602, Message: "missing approvalId"}
	}
	timeout := toDurationMs(params["timeoutMs"], 5000)
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
	s.compat.mu.RLock()
	entry, ok := s.compat.pendingApprovals[approvalID]
	s.compat.mu.RUnlock()
	if !ok {
		return nil, &dispatchError{Code: -32004, Message: "approval not found", Details: map[string]any{"approvalId": approvalID}}
	}
	return map[string]any{"approval": cloneMap(entry)}, nil
}

func (s *Server) handleCompatExecApprovalResolve(params map[string]any) map[string]any {
	approvalID := toString(params["approvalId"], "")
	status := strings.ToLower(toString(params["status"], "approved"))
	if status != "approved" && status != "rejected" {
		status = "approved"
	}
	s.compat.mu.Lock()
	entry, ok := s.compat.pendingApprovals[approvalID]
	if !ok {
		entry = map[string]any{
			"approvalId": approvalID,
			"createdAt":  time.Now().UTC().Format(time.RFC3339),
		}
	}
	entry["status"] = status
	entry["resolvedAt"] = time.Now().UTC().Format(time.RFC3339)
	s.compat.pendingApprovals[approvalID] = entry
	s.compat.mu.Unlock()
	return map[string]any{"approval": cloneMap(entry)}
}

func (s *Server) handleCompatCronList() map[string]any {
	s.compat.mu.RLock()
	items := make([]map[string]any, 0, len(s.compat.cronJobs))
	for _, job := range s.compat.cronJobs {
		items = append(items, cloneMap(job))
	}
	s.compat.mu.RUnlock()
	sort.Slice(items, func(i, j int) bool {
		return toString(items[i]["cronId"], "") < toString(items[j]["cronId"], "")
	})
	return map[string]any{"count": len(items), "items": items}
}

func (s *Server) handleCompatCronStatus() map[string]any {
	s.compat.mu.RLock()
	jobCount := len(s.compat.cronJobs)
	runCount := len(s.compat.cronRuns)
	s.compat.mu.RUnlock()
	return map[string]any{
		"running": false,
		"jobs":    jobCount,
		"runs":    runCount,
	}
}

func (s *Server) handleCompatCronAdd(params map[string]any) map[string]any {
	s.compat.mu.Lock()
	s.compat.cronSeq++
	cronID := fmt.Sprintf("cron-%04d", s.compat.cronSeq)
	entry := map[string]any{
		"cronId":        cronID,
		"name":          toString(params["name"], cronID),
		"schedule":      toString(params["schedule"], "@hourly"),
		"method":        toString(params["method"], "agent"),
		"enabled":       toBool(params["enabled"], true),
		"createdAt":     time.Now().UTC().Format(time.RFC3339),
		"updatedAt":     time.Now().UTC().Format(time.RFC3339),
		"lastRunAt":     "",
		"lastRunStatus": "",
	}
	s.compat.cronJobs[cronID] = entry
	s.compat.mu.Unlock()
	return map[string]any{"job": cloneMap(entry)}
}

func (s *Server) handleCompatCronUpdate(params map[string]any) (map[string]any, *dispatchError) {
	cronID := toString(params["cronId"], toString(params["id"], ""))
	if cronID == "" {
		return nil, &dispatchError{Code: -32602, Message: "missing cronId"}
	}
	s.compat.mu.Lock()
	entry, ok := s.compat.cronJobs[cronID]
	if !ok {
		s.compat.mu.Unlock()
		return nil, &dispatchError{Code: -32004, Message: "cron job not found", Details: map[string]any{"cronId": cronID}}
	}
	for _, key := range []string{"name", "schedule", "method", "enabled"} {
		if value, exists := params[key]; exists {
			entry[key] = value
		}
	}
	entry["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	s.compat.cronJobs[cronID] = entry
	s.compat.mu.Unlock()
	return map[string]any{"job": cloneMap(entry)}, nil
}

func (s *Server) handleCompatCronRemove(params map[string]any) (map[string]any, *dispatchError) {
	cronID := toString(params["cronId"], toString(params["id"], ""))
	if cronID == "" {
		return nil, &dispatchError{Code: -32602, Message: "missing cronId"}
	}
	s.compat.mu.Lock()
	_, ok := s.compat.cronJobs[cronID]
	if ok {
		delete(s.compat.cronJobs, cronID)
	}
	s.compat.mu.Unlock()
	return map[string]any{"ok": ok, "cronId": cronID}, nil
}

func (s *Server) handleCompatCronRun(params map[string]any) (map[string]any, *dispatchError) {
	cronID := toString(params["cronId"], toString(params["id"], ""))
	if cronID == "" {
		return nil, &dispatchError{Code: -32602, Message: "missing cronId"}
	}
	s.compat.mu.Lock()
	job, ok := s.compat.cronJobs[cronID]
	if !ok {
		s.compat.mu.Unlock()
		return nil, &dispatchError{Code: -32004, Message: "cron job not found", Details: map[string]any{"cronId": cronID}}
	}
	run := map[string]any{
		"runId":     fmt.Sprintf("cron-run-%d", time.Now().UTC().UnixNano()),
		"cronId":    cronID,
		"status":    "completed",
		"startedAt": time.Now().UTC().Format(time.RFC3339),
		"endedAt":   time.Now().UTC().Format(time.RFC3339),
	}
	s.compat.cronRuns = append(s.compat.cronRuns, run)
	if len(s.compat.cronRuns) > 256 {
		s.compat.cronRuns = append([]map[string]any(nil), s.compat.cronRuns[len(s.compat.cronRuns)-256:]...)
	}
	job["lastRunAt"] = run["endedAt"]
	job["lastRunStatus"] = run["status"]
	s.compat.cronJobs[cronID] = job
	s.compat.mu.Unlock()
	return map[string]any{"run": cloneMap(run)}, nil
}

func (s *Server) handleCompatCronRuns(params map[string]any) map[string]any {
	limit := toInt(params["limit"], 25)
	s.compat.mu.RLock()
	runs := cloneMapList(s.compat.cronRuns)
	s.compat.mu.RUnlock()
	if limit > 0 && len(runs) > limit {
		runs = runs[len(runs)-limit:]
	}
	return map[string]any{"count": len(runs), "items": runs}
}

func (s *Server) handleCompatUpdateRun(requestID string, params map[string]any) map[string]any {
	target := normalizeOptionalText(firstNonEmptyValue(params, "targetVersion", "target"), 128)
	if target == "" {
		target = "latest"
	}
	channel := normalizeOptionalText(firstNonEmptyValue(params, "channel", "requestedBy"), 64)
	force := toBool(params["force"], false)
	dryRun := toBool(params["dryRun"], false)
	simulateFailure := toBool(params["simulateFailure"], false)

	job := s.compat.createUpdateJob(target, dryRun, force, channel)
	jobID := toString(job["jobId"], "")

	if !dryRun && jobID != "" {
		go func() {
			time.Sleep(15 * time.Millisecond)
			_, _ = s.compat.updateUpdateJob(jobID, "running", "download", 35, map[string]any{
				"requestId": requestID,
			})

			time.Sleep(15 * time.Millisecond)
			_, _ = s.compat.updateUpdateJob(jobID, "running", "apply", 75, nil)

			if envTruthy("OPENCLAW_GO_UPDATE_FAIL") || simulateFailure {
				_, _ = s.compat.failUpdateJob(jobID, "simulated update failure")
				return
			}
			_, _ = s.compat.completeUpdateJob(jobID, true, []string{
				"gateway contracts refreshed",
				"runtime compatibility checks passed",
			})
		}()
	}

	return map[string]any{
		"ok":        true,
		"requestId": requestID,
		"target":    target,
		"job":       job,
		"status":    toString(job["status"], "queued"),
		"phase":     toString(job["phase"], "queued"),
		"scheduledAt": toString(
			job["startedAt"],
			time.Now().UTC().Format(time.RFC3339),
		),
	}
}

func (s *Server) handleCompatPoll(params map[string]any) map[string]any {
	jobID := normalizeOptionalText(firstNonEmptyValue(params, "jobId"), 128)
	if jobID != "" {
		if job, ok := s.compat.getUpdateJob(jobID); ok {
			return map[string]any{
				"count": 1,
				"items": []map[string]any{
					{
						"kind": "update",
						"job":  job,
					},
				},
				"job":    job,
				"jobId":  jobID,
				"status": toString(job["status"], "unknown"),
			}
		}
	}

	limit := toInt(params["limit"], 25)
	events := s.compat.listEvents(limit)
	updateJobs := s.compat.listUpdateJobs(limit)
	return map[string]any{
		"count":         len(events),
		"items":         events,
		"updateCount":   len(updateJobs),
		"updateJobs":    updateJobs,
		"hasUpdateJobs": len(updateJobs) > 0,
	}
}

func (s *Server) handleCompatChatInject(params map[string]any) map[string]any {
	sessionID := resolveSessionID(params)
	channel := strings.ToLower(toString(params["channel"], "webchat"))
	role := strings.ToLower(toString(params["role"], "assistant"))
	text := toString(params["text"], toString(params["message"], ""))
	entry := memory.MessageEntry{
		SessionID: sessionID,
		Channel:   channel,
		Method:    "chat.inject",
		Role:      role,
		Text:      text,
		Payload:   cloneMap(params),
	}
	s.memory.Append(entry)
	s.state.TouchMessage(sessionID, channel, "chat.inject", text)
	return map[string]any{
		"ok":      true,
		"session": sessionID,
		"channel": channel,
		"role":    role,
	}
}

func (s *Server) handleCompatConfigSet(params map[string]any) (map[string]any, *dispatchError) {
	key := strings.TrimSpace(toString(params["key"], ""))
	if key == "" {
		return nil, &dispatchError{Code: -32602, Message: "missing key"}
	}
	value, ok := params["value"]
	if !ok {
		return nil, &dispatchError{Code: -32602, Message: "missing value"}
	}
	snapshot := s.compat.mergeConfig(map[string]any{key: value})
	return map[string]any{
		"ok":      true,
		"key":     key,
		"value":   value,
		"overlay": snapshot,
	}, nil
}

func (s *Server) handleCompatConfigPatch(params map[string]any) map[string]any {
	patch := asStringMap(params["patch"])
	if len(patch) == 0 {
		patch = cloneMap(params)
		delete(patch, "sessionId")
		delete(patch, "id")
	}
	snapshot := s.compat.mergeConfig(patch)
	return map[string]any{
		"ok":      true,
		"overlay": snapshot,
	}
}

func (s *Server) handleCompatConfigApply(_ map[string]any) map[string]any {
	return map[string]any{
		"ok":        true,
		"applied":   true,
		"overlay":   s.compat.configSnapshot(),
		"appliedAt": time.Now().UTC().Format(time.RFC3339),
	}
}

func (s *Server) handleCompatConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"gateway": map[string]any{
				"type": "object",
			},
			"runtime": map[string]any{
				"type": "object",
			},
			"channels": map[string]any{
				"type": "object",
			},
			"security": map[string]any{
				"type": "object",
			},
		},
	}
}

func (s *Server) handleCompatLogsTail(params map[string]any) map[string]any {
	limit := toInt(params["limit"], 50)
	history := s.memory.HistoryBySession("", limit)
	lines := make([]string, 0, len(history))
	for _, entry := range history {
		lines = append(lines, fmt.Sprintf("%s [%s] %s", entry.CreatedAt, entry.Method, entry.Text))
	}
	return map[string]any{
		"count": len(lines),
		"lines": lines,
	}
}

func (s *Server) handleCompatSessionsPreview(params map[string]any) map[string]any {
	limit := toInt(params["limit"], 50)
	items := s.sessions.List()
	previews := make([]map[string]any, 0, len(items))
	for _, session := range items {
		if s.compat.isSessionDeleted(session.ID) {
			continue
		}
		previews = append(previews, map[string]any{
			"sessionId":     session.ID,
			"channel":       session.Channel,
			"lastSeenAt":    session.LastSeenAt,
			"authenticated": session.Authenticated,
		})
	}
	if limit > 0 && len(previews) > limit {
		previews = previews[:limit]
	}
	return map[string]any{
		"count": len(previews),
		"items": previews,
	}
}

func (s *Server) handleCompatSessionsPatch(params map[string]any) (map[string]any, *dispatchError) {
	sessionID := resolveSessionID(params)
	if sessionID == "" {
		return nil, &dispatchError{Code: -32602, Message: "missing sessionId"}
	}
	session, ok := s.sessions.Get(sessionID)
	if !ok {
		return nil, &dispatchError{Code: -32004, Message: "session not found", Details: map[string]any{"sessionId": sessionID}}
	}
	if channel := strings.TrimSpace(toString(params["channel"], "")); channel != "" {
		s.sessions.UpdateChannel(sessionID, channel)
		session.Channel = channel
	}
	return map[string]any{
		"session": session,
	}, nil
}

func (s *Server) handleCompatSessionsResolve(params map[string]any) (map[string]any, *dispatchError) {
	sessionID := resolveSessionID(params)
	if sessionID == "" {
		return nil, &dispatchError{Code: -32602, Message: "missing sessionId"}
	}
	session, ok := s.sessions.Get(sessionID)
	if !ok || s.compat.isSessionDeleted(sessionID) {
		return nil, &dispatchError{Code: -32004, Message: "session not found", Details: map[string]any{"sessionId": sessionID}}
	}
	state, ok := s.state.Get(sessionID)
	resolvedState := any(nil)
	if ok {
		resolvedState = state
	}
	return map[string]any{
		"session":    session,
		"state":      resolvedState,
		"stateFound": ok,
	}, nil
}

func (s *Server) handleCompatSessionsReset(params map[string]any) map[string]any {
	sessionID := resolveSessionID(params)
	if sessionID == "" {
		return map[string]any{"ok": false, "reason": "missing sessionId"}
	}
	removedMessages := s.memory.RemoveSession(sessionID)
	clearedState := s.state.Delete(sessionID)
	s.compat.clearSessionTombstone(sessionID)
	return map[string]any{
		"ok":              true,
		"sessionId":       sessionID,
		"removedMessages": removedMessages,
		"clearedState":    clearedState,
		"resetAt":         time.Now().UTC().Format(time.RFC3339),
	}
}

func (s *Server) handleCompatSessionsDelete(params map[string]any) map[string]any {
	sessionID := resolveSessionID(params)
	if sessionID == "" {
		return map[string]any{"ok": false, "reason": "missing sessionId"}
	}
	removedMessages := s.memory.RemoveSession(sessionID)
	removedState := s.state.Delete(sessionID)
	removedSession := s.sessions.Delete(sessionID)
	s.compat.markSessionDeleted(sessionID)
	return map[string]any{
		"ok":              true,
		"sessionId":       sessionID,
		"removedMessages": removedMessages,
		"removedState":    removedState,
		"removedSession":  removedSession,
		"deletedAt":       time.Now().UTC().Format(time.RFC3339),
	}
}

func (s *Server) handleCompatSessionsCompact(params map[string]any) map[string]any {
	limit := toInt(params["limit"], 1000)
	before := s.memory.Count()
	compacted := s.memory.Trim(limit)
	after := s.memory.Count()
	return map[string]any{
		"ok":        true,
		"limit":     limit,
		"before":    before,
		"after":     after,
		"count":     after,
		"compacted": compacted,
	}
}

func (s *Server) handleCompatSessionsUsage(params map[string]any) map[string]any {
	sessionID := resolveSessionID(params)
	history := s.memory.HistoryBySession(sessionID, toInt(params["limit"], 5000))
	totalTokens := 0
	for _, entry := range history {
		totalTokens += countWords(entry.Text)
	}
	return map[string]any{
		"sessionId": sessionID,
		"messages":  len(history),
		"tokens":    totalTokens,
	}
}

func (s *Server) handleCompatSessionsUsageTimeseries(params map[string]any) map[string]any {
	sessionID := resolveSessionID(params)
	history := s.memory.HistoryBySession(sessionID, toInt(params["limit"], 500))
	buckets := map[string]int{}
	for _, entry := range history {
		timestamp := entry.CreatedAt
		if len(timestamp) >= 13 {
			timestamp = timestamp[:13] + ":00:00Z"
		}
		buckets[timestamp]++
	}
	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	items := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		items = append(items, map[string]any{
			"bucket":   key,
			"messages": buckets[key],
		})
	}
	return map[string]any{
		"sessionId": sessionID,
		"count":     len(items),
		"items":     items,
	}
}

func (s *Server) handleCompatSessionsUsageLogs(params map[string]any) map[string]any {
	sessionID := resolveSessionID(params)
	limit := toInt(params["limit"], 100)
	history := s.memory.HistoryBySession(sessionID, limit)
	logs := make([]map[string]any, 0, len(history))
	for _, entry := range history {
		logs = append(logs, map[string]any{
			"id":        entry.ID,
			"createdAt": entry.CreatedAt,
			"method":    entry.Method,
			"role":      entry.Role,
			"text":      entry.Text,
		})
	}
	return map[string]any{
		"sessionId": sessionID,
		"count":     len(logs),
		"items":     logs,
	}
}

func (c *compatState) markSessionDeleted(sessionID string) {
	c.mu.Lock()
	c.sessionTombstones[sessionID] = true
	c.mu.Unlock()
}

func (c *compatState) clearSessionTombstone(sessionID string) {
	c.mu.Lock()
	delete(c.sessionTombstones, sessionID)
	c.mu.Unlock()
}

func (c *compatState) isSessionDeleted(sessionID string) bool {
	c.mu.RLock()
	deleted := c.sessionTombstones[sessionID]
	c.mu.RUnlock()
	return deleted
}
