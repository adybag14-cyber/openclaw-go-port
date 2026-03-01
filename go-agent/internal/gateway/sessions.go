package gateway

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type ClientSession struct {
	ID            string    `json:"id"`
	ClientID      string    `json:"clientId,omitempty"`
	Channel       string    `json:"channel,omitempty"`
	Role          string    `json:"role"`
	Scopes        []string  `json:"scopes"`
	AuthMode      string    `json:"authMode"`
	Authenticated bool      `json:"authenticated"`
	CreatedAt     string    `json:"createdAt"`
	LastSeenAt    string    `json:"lastSeenAt"`
	createdAtRaw  time.Time `json:"-"`
	lastSeenRaw   time.Time `json:"-"`
}

type SessionRegistry struct {
	mu       sync.RWMutex
	seq      atomic.Uint64
	sessions map[string]ClientSession
}

func NewSessionRegistry() *SessionRegistry {
	return &SessionRegistry{
		sessions: make(map[string]ClientSession),
	}
}

func (r *SessionRegistry) Create(clientID, channel, role string, scopes []string, authMode string, authenticated bool) ClientSession {
	now := time.Now().UTC()
	id := fmt.Sprintf("sess-%06d", r.seq.Add(1))
	session := ClientSession{
		ID:            id,
		ClientID:      clientID,
		Channel:       channel,
		Role:          role,
		Scopes:        append([]string(nil), scopes...),
		AuthMode:      authMode,
		Authenticated: authenticated,
		CreatedAt:     now.Format(time.RFC3339),
		LastSeenAt:    now.Format(time.RFC3339),
		createdAtRaw:  now,
		lastSeenRaw:   now,
	}
	r.mu.Lock()
	r.sessions[id] = session
	r.mu.Unlock()
	return session
}

func (r *SessionRegistry) UpdateChannel(id string, channel string) {
	r.mu.Lock()
	session, ok := r.sessions[id]
	if !ok {
		r.mu.Unlock()
		return
	}
	session.Channel = channel
	r.sessions[id] = session
	r.mu.Unlock()
}

func (r *SessionRegistry) Touch(id string) {
	r.mu.Lock()
	session, ok := r.sessions[id]
	if !ok {
		r.mu.Unlock()
		return
	}
	now := time.Now().UTC()
	session.LastSeenAt = now.Format(time.RFC3339)
	session.lastSeenRaw = now
	r.sessions[id] = session
	r.mu.Unlock()
}

func (r *SessionRegistry) Get(id string) (ClientSession, bool) {
	r.mu.RLock()
	session, ok := r.sessions[id]
	r.mu.RUnlock()
	return session, ok
}

func (r *SessionRegistry) List() []ClientSession {
	r.mu.RLock()
	out := make([]ClientSession, 0, len(r.sessions))
	for _, session := range r.sessions {
		out = append(out, session)
	}
	r.mu.RUnlock()
	return out
}

func (r *SessionRegistry) Count() int {
	r.mu.RLock()
	count := len(r.sessions)
	r.mu.RUnlock()
	return count
}

func (r *SessionRegistry) Delete(id string) bool {
	r.mu.Lock()
	_, ok := r.sessions[id]
	if ok {
		delete(r.sessions, id)
	}
	r.mu.Unlock()
	return ok
}
