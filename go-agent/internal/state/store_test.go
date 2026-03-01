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

func TestDelete(t *testing.T) {
	store := NewStore()
	store.TouchMessage("sess-1", "webchat", "chat.send", "hello")

	if ok := store.Delete("sess-1"); !ok {
		t.Fatalf("expected delete to return true")
	}
	if _, ok := store.Get("sess-1"); ok {
		t.Fatalf("expected sess-1 to be deleted")
	}
	if ok := store.Delete("sess-1"); ok {
		t.Fatalf("expected second delete to return false")
	}
}
