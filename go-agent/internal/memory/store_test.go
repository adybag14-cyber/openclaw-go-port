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

func TestSemanticRecall(t *testing.T) {
	store := NewStore("memory://semantic", 200)
	store.Append(MessageEntry{SessionID: "s1", Channel: "webchat", Method: "chat.send", Role: "user", Text: "deploy oracle vm with gpt model"})
	store.Append(MessageEntry{SessionID: "s2", Channel: "telegram", Method: "send", Role: "assistant", Text: "tts provider configured"})
	store.Append(MessageEntry{SessionID: "s3", Channel: "webchat", Method: "chat.send", Role: "user", Text: "oracle migration checklist"})

	recall := store.SemanticRecall("oracle vm migration", 2)
	if len(recall) == 0 {
		t.Fatalf("expected semantic recall results")
	}
	if recall[0].SessionID != "s1" && recall[0].SessionID != "s3" {
		t.Fatalf("expected top semantic hit to be oracle-related, got session=%s", recall[0].SessionID)
	}
}

func TestGraphNeighborsAndSynthesis(t *testing.T) {
	store := NewStore("memory://graph", 200)
	store.Append(MessageEntry{SessionID: "abc", Channel: "telegram", Method: "send", Role: "user", Text: "enable telemetry alerts"})
	store.Append(MessageEntry{SessionID: "abc", Channel: "telegram", Method: "send", Role: "assistant", Text: "telemetry alerts enabled"})

	neighbors := store.GraphNeighbors("session:abc", 10)
	if len(neighbors) == 0 {
		t.Fatalf("expected graph neighbors for session node")
	}

	synthesis := store.RecallSynthesis("telemetry alerts", 3)
	count, _ := synthesis["count"].(map[string]any)
	if count == nil {
		t.Fatalf("expected synthesis count object")
	}
	if semantic, _ := count["semantic"].(int); semantic == 0 {
		// JSON-like map values in memory here still int on direct assignment; tolerate conversion fallback.
		if semanticFloat, _ := count["semantic"].(float64); int(semanticFloat) == 0 {
			t.Fatalf("expected semantic synthesis hits, got %v", count["semantic"])
		}
	}
}

func TestStoreStatsAndPersistenceRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "memory.json")
	store := NewStore(path, 100)
	store.Append(MessageEntry{SessionID: "s1", Channel: "webchat", Method: "chat.send", Role: "user", Text: "vector memory test"})
	stats := store.Stats()
	if entries, _ := stats["entries"].(int); entries != 1 {
		if entriesFloat, _ := stats["entries"].(float64); int(entriesFloat) != 1 {
			t.Fatalf("expected entries=1, got %v", stats["entries"])
		}
	}
	if vectors, _ := stats["vectors"].(int); vectors < 1 {
		if vectorsFloat, _ := stats["vectors"].(float64); int(vectorsFloat) < 1 {
			t.Fatalf("expected vectors>=1, got %v", stats["vectors"])
		}
	}

	loaded := NewStore(path, 100)
	loadedStats := loaded.Stats()
	if entries, _ := loadedStats["entries"].(int); entries != 1 {
		if entriesFloat, _ := loadedStats["entries"].(float64); int(entriesFloat) != 1 {
			t.Fatalf("expected persisted entries=1, got %v", loadedStats["entries"])
		}
	}
}
