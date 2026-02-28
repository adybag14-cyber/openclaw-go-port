package session

import "testing"

func TestParseMainSession(t *testing.T) {
	parsed := ParseKey("main")
	if parsed.Kind != KindMain {
		t.Fatalf("expected main kind, got %s", parsed.Kind)
	}
}

func TestParseAgentMainAlias(t *testing.T) {
	parsed := ParseKey("agent:main:main")
	if parsed.Kind != KindMain {
		t.Fatalf("expected main kind, got %s", parsed.Kind)
	}
	if parsed.AgentID != "main" {
		t.Fatalf("expected agent id main, got %s", parsed.AgentID)
	}
	if parsed.Channel != "" {
		t.Fatalf("expected empty channel for agent main alias, got %s", parsed.Channel)
	}
}

func TestParseAgentGroupWithTopic(t *testing.T) {
	parsed := ParseKey("agent:main:telegram:group:123:topic:44")
	if parsed.Kind != KindGroup {
		t.Fatalf("expected group kind, got %s", parsed.Kind)
	}
	if parsed.Channel != "telegram" {
		t.Fatalf("expected telegram channel, got %s", parsed.Channel)
	}
	if parsed.ScopeID != "123" {
		t.Fatalf("expected scope 123, got %s", parsed.ScopeID)
	}
	if parsed.TopicID != "44" {
		t.Fatalf("expected topic 44, got %s", parsed.TopicID)
	}
}

func TestParseAgentChannel(t *testing.T) {
	parsed := ParseKey("agent:ops:discord:channel:help")
	if parsed.Kind != KindChannel {
		t.Fatalf("expected channel kind, got %s", parsed.Kind)
	}
	if parsed.ScopeID != "help" {
		t.Fatalf("expected scope help, got %s", parsed.ScopeID)
	}
}

func TestParseDirectScope(t *testing.T) {
	parsed := ParseKey("agent:main:whatsapp:dm:+15551234567")
	if parsed.Kind != KindDirect {
		t.Fatalf("expected direct kind, got %s", parsed.Kind)
	}
	if parsed.ScopeID != "+15551234567" {
		t.Fatalf("expected direct scope id +15551234567, got %s", parsed.ScopeID)
	}
}

func TestParseInternalScopes(t *testing.T) {
	if ParseKey("cron:daily").Kind != KindCron {
		t.Fatalf("expected cron kind")
	}
	if ParseKey("hook:abc").Kind != KindHook {
		t.Fatalf("expected hook kind")
	}
	if ParseKey("node-123").Kind != KindNode {
		t.Fatalf("expected node kind")
	}
}
