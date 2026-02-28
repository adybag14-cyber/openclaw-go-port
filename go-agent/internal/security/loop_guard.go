package security

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type ToolLoopGuard struct {
	mu       sync.Mutex
	window   time.Duration
	maxHits  int
	history  map[string][]time.Time
	disabled bool
}

func NewToolLoopGuard(window time.Duration, maxHits int, enabled bool) *ToolLoopGuard {
	if window <= 0 {
		window = 5 * time.Second
	}
	if maxHits <= 0 {
		maxHits = 8
	}
	return &ToolLoopGuard{
		window:   window,
		maxHits:  maxHits,
		history:  map[string][]time.Time{},
		disabled: !enabled,
	}
}

func (g *ToolLoopGuard) Register(method string, params map[string]any) (triggered bool, count int) {
	if g == nil || g.disabled {
		return false, 0
	}
	key := loopGuardKey(strings.ToLower(strings.TrimSpace(method)), params)
	now := time.Now().UTC()

	g.mu.Lock()
	defer g.mu.Unlock()

	events := g.history[key]
	threshold := now.Add(-g.window)
	pruned := make([]time.Time, 0, len(events)+1)
	for _, event := range events {
		if event.After(threshold) {
			pruned = append(pruned, event)
		}
	}
	pruned = append(pruned, now)
	g.history[key] = pruned

	count = len(pruned)
	if count > g.maxHits {
		return true, count
	}
	return false, count
}

func (g *ToolLoopGuard) Snapshot() map[string]any {
	if g == nil {
		return map[string]any{
			"enabled": false,
		}
	}
	return map[string]any{
		"enabled":  !g.disabled,
		"windowMs": g.window.Milliseconds(),
		"maxHits":  g.maxHits,
	}
}

func loopGuardKey(method string, params map[string]any) string {
	sessionID := strings.TrimSpace(firstString(params, "sessionId", "session_id", "id"))
	channel := strings.TrimSpace(firstString(params, "channel", "source"))
	clientID := strings.TrimSpace(firstString(params, "clientId", "client_id", "userId", "user_id"))

	identity := "global"
	switch {
	case sessionID != "":
		identity = "session:" + sessionID
	case clientID != "":
		identity = "client:" + clientID
	case channel != "":
		identity = "channel:" + strings.ToLower(channel)
	}

	return fmt.Sprintf("%s|%s", identity, method)
}

func firstString(params map[string]any, keys ...string) string {
	for _, key := range keys {
		if params == nil {
			break
		}
		raw, ok := params[key]
		if !ok {
			continue
		}
		value, ok := raw.(string)
		if !ok {
			continue
		}
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
