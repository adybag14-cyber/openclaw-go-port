package memory

import (
	"path/filepath"
	"testing"
)

func TestStoreAppendAndHistory(t *testing.T) {
	store := NewStore("memory://history", 200)
	store.Append(MessageEntry{SessionID: "s1", Channel: "webchat", Method: "chat.send", Role: "user", Text: "hello"})
	store.Append(MessageEntry{SessionID: "s1", Channel: "webchat", Method: "chat.send", Role: "assistant", Text: "hi"})
	store.Append(MessageEntry{SessionID: "s2", Channel: "telegram", Method: "send", Role: "user", Text: "ping"})

	history := store.HistoryBySession("s1", 10)
	if len(history) != 2 {
		t.Fatalf("expected 2 session entries, got %d", len(history))
	}
	channelHistory := store.HistoryByChannel("telegram", 10)
	if len(channelHistory) != 1 {
		t.Fatalf("expected 1 telegram entry, got %d", len(channelHistory))
	}
}

func TestStorePersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "memory.json")
	store := NewStore(path, 100)
	store.Append(MessageEntry{SessionID: "s1", Channel: "webchat", Method: "chat.send", Role: "user", Text: "persist"})

	loaded := NewStore(path, 100)
	history := loaded.HistoryBySession("s1", 10)
	if len(history) != 1 {
		t.Fatalf("expected persisted history, got %d entries", len(history))
	}
	if history[0].Text != "persist" {
		t.Fatalf("unexpected persisted payload: %+v", history[0])
	}
}
