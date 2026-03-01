package web

import (
	"context"
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
