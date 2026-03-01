package gateway

import "testing"

func TestSessionRegistryDelete(t *testing.T) {
	registry := NewSessionRegistry()
	session := registry.Create("client-1", "webchat", "client", []string{"chat"}, "none", true)

	if ok := registry.Delete(session.ID); !ok {
		t.Fatalf("expected delete to return true")
	}
	if _, ok := registry.Get(session.ID); ok {
		t.Fatalf("expected deleted session to be absent")
	}
	if ok := registry.Delete(session.ID); ok {
		t.Fatalf("expected deleting absent session to return false")
	}
}
