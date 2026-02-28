package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type MessageEntry struct {
	ID        string         `json:"id"`
	SessionID string         `json:"sessionId,omitempty"`
	Channel   string         `json:"channel,omitempty"`
	Method    string         `json:"method"`
	Role      string         `json:"role"`
	Text      string         `json:"text,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
	CreatedAt string         `json:"createdAt"`
}

type Store struct {
	mu         sync.RWMutex
	path       string
	persist    bool
	maxEntries int
	entries    []MessageEntry
	lastError  string
}

type persistedState struct {
	Entries []MessageEntry `json:"entries"`
}

func NewStore(path string, maxEntries int) *Store {
	if maxEntries < 100 {
		maxEntries = 100
	}
	s := &Store{
		path:       strings.TrimSpace(path),
		persist:    shouldPersist(path),
		maxEntries: maxEntries,
		entries:    make([]MessageEntry, 0, maxEntries),
	}
	s.load()
	return s
}

func (s *Store) Append(entry MessageEntry) {
	s.mu.Lock()
	if entry.CreatedAt == "" {
		entry.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if entry.ID == "" {
		entry.ID = "msg-" + time.Now().UTC().Format("20060102150405.000000000")
	}
	s.entries = append(s.entries, entry)
	if len(s.entries) > s.maxEntries {
		start := len(s.entries) - s.maxEntries
		s.entries = append([]MessageEntry(nil), s.entries[start:]...)
	}
	s.mu.Unlock()
	s.persistLockedSnapshot()
}

func (s *Store) HistoryBySession(sessionID string, limit int) []MessageEntry {
	if limit <= 0 {
		limit = 50
	}
	sid := strings.TrimSpace(sessionID)
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]MessageEntry, 0, limit)
	for i := len(s.entries) - 1; i >= 0; i-- {
		entry := s.entries[i]
		if sid != "" && entry.SessionID != sid {
			continue
		}
		out = append(out, entry)
		if len(out) >= limit {
			break
		}
	}
	reverse(out)
	return out
}

func (s *Store) HistoryByChannel(channel string, limit int) []MessageEntry {
	if limit <= 0 {
		limit = 50
	}
	canonical := strings.ToLower(strings.TrimSpace(channel))
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]MessageEntry, 0, limit)
	for i := len(s.entries) - 1; i >= 0; i-- {
		entry := s.entries[i]
		if canonical != "" && strings.ToLower(entry.Channel) != canonical {
			continue
		}
		out = append(out, entry)
		if len(out) >= limit {
			break
		}
	}
	reverse(out)
	return out
}

func (s *Store) LastError() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastError
}

func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

func (s *Store) load() {
	if !s.persist {
		return
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		s.mu.Lock()
		s.lastError = err.Error()
		s.mu.Unlock()
		return
	}
	var data persistedState
	if err := json.Unmarshal(raw, &data); err != nil {
		s.mu.Lock()
		s.lastError = err.Error()
		s.mu.Unlock()
		return
	}
	s.mu.Lock()
	s.entries = append([]MessageEntry(nil), data.Entries...)
	if len(s.entries) > s.maxEntries {
		start := len(s.entries) - s.maxEntries
		s.entries = append([]MessageEntry(nil), s.entries[start:]...)
	}
	s.mu.Unlock()
}

func (s *Store) persistLockedSnapshot() {
	if !s.persist {
		return
	}
	s.mu.RLock()
	payload := persistedState{
		Entries: append([]MessageEntry(nil), s.entries...),
	}
	path := s.path
	s.mu.RUnlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		s.mu.Lock()
		s.lastError = err.Error()
		s.mu.Unlock()
		return
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		s.mu.Lock()
		s.lastError = err.Error()
		s.mu.Unlock()
		return
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		s.mu.Lock()
		s.lastError = err.Error()
		s.mu.Unlock()
		return
	}
}

func shouldPersist(path string) bool {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "memory://") {
		return false
	}
	return true
}

func reverse[T any](items []T) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}
