package state

import "testing"

func TestTouchAndGet(t *testing.T) {
	store := NewStore()
	store.TouchMessage("sess-1", "telegram", "send", "hello")
	store.TouchMessage("sess-1", "telegram", "chat.send", "hi")

	state, ok := store.Get("sess-1")
	if !ok {
		t.Fatalf("expected session state for sess-1")
	}
	if state.MessageCount != 2 {
		t.Fatalf("expected message count=2, got %d", state.MessageCount)
	}
	if state.Channel != "telegram" {
		t.Fatalf("unexpected channel: %s", state.Channel)
	}
	if state.LastMethod != "chat.send" {
		t.Fatalf("unexpected last method: %s", state.LastMethod)
	}
}
