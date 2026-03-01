package web

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestStartWaitCompleteFlow(t *testing.T) {
	m := NewManager(5 * time.Minute)
	session := m.Start(StartOptions{
		Provider: "chatgpt",
		Model:    "gpt-5.2",
	})
	if session.Status != LoginPending {
		t.Fatalf("expected pending status, got %s", session.Status)
	}

	waitPending, err := m.Wait(context.Background(), session.ID, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("wait failed: %v", err)
	}
	if waitPending.Status != LoginPending {
		t.Fatalf("expected pending while waiting pre-complete, got %s", waitPending.Status)
	}

	if _, err := m.Complete(session.ID, session.Code); err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	waitAuthorized, err := m.Wait(context.Background(), session.ID, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("wait failed after complete: %v", err)
	}
	if waitAuthorized.Status != LoginAuthorized {
		t.Fatalf("expected authorized, got %s", waitAuthorized.Status)
	}
	if !m.HasAuthorizedSession() {
		t.Fatalf("expected authorized session to be detected")
	}
}

func TestCompleteRejectsWrongCode(t *testing.T) {
	m := NewManager(5 * time.Minute)
	session := m.Start(StartOptions{})
	if _, err := m.Complete(session.ID, "WRONG-CODE"); err == nil {
		t.Fatalf("expected invalid code error")
	}
}

func TestIsAuthorized(t *testing.T) {
	m := NewManager(5 * time.Minute)
	session := m.Start(StartOptions{})
	if m.IsAuthorized(session.ID) {
		t.Fatalf("pending session should not be authorized")
	}
	if _, err := m.Complete(session.ID, session.Code); err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if !m.IsAuthorized(session.ID) {
		t.Fatalf("completed session should be authorized")
	}
	if m.IsAuthorized("missing-session") {
		t.Fatalf("missing session should not be authorized")
	}
}

func TestProviderVerificationURI(t *testing.T) {
	m := NewManager(5 * time.Minute)
	cases := []struct {
		provider string
		prefix   string
	}{
		{provider: "chatgpt", prefix: "https://chatgpt.com/"},
		{provider: "codex", prefix: "https://chatgpt.com/"},
		{provider: "openrouter", prefix: "https://openrouter.ai/"},
		{provider: "kimi", prefix: "https://kimi.com/"},
		{provider: "qwen", prefix: "https://chat.qwen.ai/"},
	}
	for _, tc := range cases {
		session := m.Start(StartOptions{Provider: tc.provider, Model: "auto"})
		if session.VerificationURI != tc.prefix {
			t.Fatalf("provider %s verification URI mismatch: got=%s want=%s", tc.provider, session.VerificationURI, tc.prefix)
		}
		if !strings.HasPrefix(session.VerificationURIComplete, strings.TrimRight(tc.prefix, "/")) {
			t.Fatalf("provider %s verification URI complete should start with %s, got %s", tc.provider, strings.TrimRight(tc.prefix, "/"), session.VerificationURIComplete)
		}
	}
}

func TestSummaryByProviderAndStatus(t *testing.T) {
	m := NewManager(5 * time.Minute)
	chatgpt := m.Start(StartOptions{Provider: "chatgpt", Model: "gpt-5.2"})
	codex := m.Start(StartOptions{Provider: "codex", Model: "gpt-5.2"})
	if _, err := m.Complete(codex.ID, codex.Code); err != nil {
		t.Fatalf("complete codex failed: %v", err)
	}
	if !m.Logout(chatgpt.ID) {
		t.Fatalf("expected logout chatgpt session")
	}

	summary := m.Summary()
	total, _ := summary["total"].(int)
	if total != 2 {
		t.Fatalf("expected total=2, got %v", summary["total"])
	}
	authorized, _ := summary["authorized"].(int)
	if authorized != 1 {
		t.Fatalf("expected authorized=1, got %v", summary["authorized"])
	}
	rejected, _ := summary["rejected"].(int)
	if rejected != 1 {
		t.Fatalf("expected rejected=1, got %v", summary["rejected"])
	}
	byProvider, _ := summary["byProvider"].(map[string]map[string]int)
	if byProvider == nil {
		t.Fatalf("expected byProvider map")
	}
	if byProvider["codex"]["authorized"] != 1 {
		t.Fatalf("expected codex authorized=1, got %v", byProvider["codex"]["authorized"])
	}
	if byProvider["chatgpt"]["rejected"] != 1 {
		t.Fatalf("expected chatgpt rejected=1, got %v", byProvider["chatgpt"]["rejected"])
	}
}
