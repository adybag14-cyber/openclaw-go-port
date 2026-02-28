package session

import "strings"

type Kind string

const (
	KindMain    Kind = "main"
	KindDirect  Kind = "direct"
	KindGroup   Kind = "group"
	KindChannel Kind = "channel"
	KindCron    Kind = "cron"
	KindHook    Kind = "hook"
	KindNode    Kind = "node"
	KindOther   Kind = "other"
)

type Descriptor struct {
	Key     string `json:"key"`
	Kind    Kind   `json:"kind"`
	AgentID string `json:"agentId,omitempty"`
	Channel string `json:"channel,omitempty"`
	ScopeID string `json:"scopeId,omitempty"`
	TopicID string `json:"topicId,omitempty"`
}

func ParseKey(sessionKey string) Descriptor {
	trimmed := strings.TrimSpace(sessionKey)
	lower := strings.ToLower(trimmed)
	if lower == "main" {
		return Descriptor{Key: trimmed, Kind: KindMain}
	}
	if strings.HasPrefix(lower, "cron:") {
		return Descriptor{
			Key:     trimmed,
			Kind:    KindCron,
			Channel: "internal",
			ScopeID: trimmed[len("cron:"):],
		}
	}
	if strings.HasPrefix(lower, "hook:") {
		return Descriptor{
			Key:     trimmed,
			Kind:    KindHook,
			Channel: "internal",
			ScopeID: trimmed[len("hook:"):],
		}
	}
	if strings.HasPrefix(lower, "node-") {
		return Descriptor{
			Key:     trimmed,
			Kind:    KindNode,
			Channel: "internal",
			ScopeID: trimmed[len("node-"):],
		}
	}
	return parseAgentSessionKey(trimmed)
}

func parseAgentSessionKey(key string) Descriptor {
	parts := strings.Split(key, ":")
	if len(parts) == 0 || !strings.EqualFold(parts[0], "agent") {
		return Descriptor{
			Key:  key,
			Kind: KindOther,
		}
	}

	descriptor := Descriptor{
		Key:  key,
		Kind: KindOther,
	}
	if len(parts) >= 2 {
		descriptor.AgentID = parts[1]
	}
	if len(parts) >= 3 {
		descriptor.Channel = parts[2]
	}
	rest := []string{}
	if len(parts) >= 4 {
		rest = parts[3:]
	}
	if len(rest) == 0 {
		if strings.EqualFold(descriptor.Channel, "main") {
			descriptor.Kind = KindMain
			descriptor.ScopeID = "main"
			descriptor.Channel = ""
		}
		return descriptor
	}
	if len(rest) == 1 {
		if strings.EqualFold(rest[0], "main") {
			descriptor.Kind = KindMain
		}
		descriptor.ScopeID = rest[0]
		return descriptor
	}

	switch strings.ToLower(rest[0]) {
	case "dm":
		descriptor.Kind = KindDirect
		descriptor.ScopeID = strings.Join(rest[1:], ":")
	case "group":
		descriptor.Kind = KindGroup
		descriptor.ScopeID, descriptor.TopicID = parseTopicScope(rest[1:])
	case "channel":
		descriptor.Kind = KindChannel
		descriptor.ScopeID, descriptor.TopicID = parseTopicScope(rest[1:])
	default:
		descriptor.ScopeID = strings.Join(rest, ":")
	}
	return descriptor
}

func parseTopicScope(parts []string) (string, string) {
	if len(parts) == 0 {
		return "", ""
	}
	for i := 0; i+1 < len(parts); i++ {
		if strings.EqualFold(parts[i], "topic") {
			return strings.Join(parts[:i], ":"), parts[i+1]
		}
	}
	return strings.Join(parts, ":"), ""
}
