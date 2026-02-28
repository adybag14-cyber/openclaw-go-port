package state

import (
	"strings"
	"sync"
	"time"
)

type SessionState struct {
	SessionID    string `json:"sessionId"`
	Channel      string `json:"channel,omitempty"`
	LastMethod   string `json:"lastMethod,omitempty"`
	LastMessage  string `json:"lastMessage,omitempty"`
	MessageCount int    `json:"messageCount"`
	UpdatedAt    string `json:"updatedAt"`
}

type Store struct {
	mu       sync.RWMutex
	sessions map[string]SessionState
}

func NewStore() *Store {
	return &Store{
		sessions: make(map[string]SessionState),
	}
}

func (s *Store) TouchMessage(sessionID string, channel string, method string, message string) {
	sid := strings.TrimSpace(sessionID)
	if sid == "" {
		return
	}
	s.mu.Lock()
	state := s.sessions[sid]
	state.SessionID = sid
	if strings.TrimSpace(channel) != "" {
		state.Channel = strings.ToLower(strings.TrimSpace(channel))
	}
	state.LastMethod = strings.TrimSpace(method)
	state.LastMessage = strings.TrimSpace(message)
	state.MessageCount++
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	s.sessions[sid] = state
	s.mu.Unlock()
}

func (s *Store) Get(sessionID string) (SessionState, bool) {
	s.mu.RLock()
	state, ok := s.sessions[strings.TrimSpace(sessionID)]
	s.mu.RUnlock()
	return state, ok
}

func (s *Store) List() []SessionState {
	s.mu.RLock()
	out := make([]SessionState, 0, len(s.sessions))
	for _, state := range s.sessions {
		out = append(out, state)
	}
	s.mu.RUnlock()
	return out
}
