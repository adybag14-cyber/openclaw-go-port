package web

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type LoginStatus string

const (
	LoginPending    LoginStatus = "pending"
	LoginAuthorized LoginStatus = "authorized"
	LoginExpired    LoginStatus = "expired"
	LoginRejected   LoginStatus = "rejected"
)

type Session struct {
	ID                      string      `json:"loginSessionId"`
	Status                  LoginStatus `json:"status"`
	Provider                string      `json:"provider"`
	Model                   string      `json:"model,omitempty"`
	Code                    string      `json:"code,omitempty"`
	VerificationURI         string      `json:"verificationUri"`
	VerificationURIComplete string      `json:"verificationUriComplete"`
	CreatedAt               string      `json:"createdAt"`
	ExpiresAt               string      `json:"expiresAt"`
	AuthorizedAt            string      `json:"authorizedAt,omitempty"`
}

type StartOptions struct {
	Provider string
	Model    string
}

type Manager struct {
	mu       sync.RWMutex
	ttl      time.Duration
	seq      atomic.Uint64
	sessions map[string]*sessionState
}

type sessionState struct {
	session Session
	done    chan struct{}
}

func NewManager(ttl time.Duration) *Manager {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &Manager{
		ttl:      ttl,
		sessions: make(map[string]*sessionState),
	}
}

func (m *Manager) Start(opts StartOptions) Session {
	provider := strings.ToLower(strings.TrimSpace(opts.Provider))
	if provider == "" {
		provider = "chatgpt"
	}
	model := strings.TrimSpace(opts.Model)
	if model == "" {
		model = "gpt-5.2"
	}

	seq := m.seq.Add(1)
	now := time.Now().UTC()
	expires := now.Add(m.ttl)
	id := fmt.Sprintf("web-login-%06d", seq)
	code := fmt.Sprintf("OC-%06d", 100000+seq%900000)

	session := Session{
		ID:                      id,
		Status:                  LoginPending,
		Provider:                provider,
		Model:                   model,
		Code:                    code,
		VerificationURI:         "https://chatgpt.com/",
		VerificationURIComplete: fmt.Sprintf("https://chatgpt.com/?openclaw_code=%s", code),
		CreatedAt:               now.Format(time.RFC3339),
		ExpiresAt:               expires.Format(time.RFC3339),
	}

	m.mu.Lock()
	m.sessions[id] = &sessionState{
		session: session,
		done:    make(chan struct{}),
	}
	m.mu.Unlock()

	return session
}

func (m *Manager) Get(id string) (Session, bool) {
	m.mu.RLock()
	state, ok := m.sessions[id]
	if !ok {
		m.mu.RUnlock()
		return Session{}, false
	}
	session := state.session
	m.mu.RUnlock()
	return m.applyExpiry(session), true
}

func (m *Manager) Wait(ctx context.Context, id string, timeout time.Duration) (Session, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	m.mu.RLock()
	state, ok := m.sessions[id]
	if !ok {
		m.mu.RUnlock()
		return Session{}, errors.New("login session not found")
	}
	doneCh := state.done
	session := m.applyExpiry(state.session)
	m.mu.RUnlock()

	if session.Status != LoginPending {
		return session, nil
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-doneCh:
	case <-timer.C:
	case <-ctx.Done():
	}

	final, ok := m.Get(id)
	if !ok {
		return Session{}, errors.New("login session not found")
	}
	return final, nil
}

func (m *Manager) Complete(id string, code string) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.sessions[id]
	if !ok {
		return Session{}, errors.New("login session not found")
	}
	state.session = m.applyExpiry(state.session)
	if state.session.Status == LoginExpired {
		return state.session, errors.New("login session expired")
	}

	if state.session.Code != "" && strings.TrimSpace(code) != "" && !strings.EqualFold(strings.TrimSpace(code), strings.TrimSpace(state.session.Code)) {
		return state.session, errors.New("invalid login code")
	}

	state.session.Status = LoginAuthorized
	state.session.AuthorizedAt = time.Now().UTC().Format(time.RFC3339)
	select {
	case <-state.done:
	default:
		close(state.done)
	}
	return state.session, nil
}

func (m *Manager) Logout(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.sessions[id]
	if !ok {
		return false
	}
	state.session.Status = LoginRejected
	select {
	case <-state.done:
	default:
		close(state.done)
	}
	return true
}

func (m *Manager) LogoutAll() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, state := range m.sessions {
		state.session.Status = LoginRejected
		select {
		case <-state.done:
		default:
			close(state.done)
		}
		count++
	}
	return count
}

func (m *Manager) HasAuthorizedSession() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, state := range m.sessions {
		session := m.applyExpiry(state.session)
		if session.Status == LoginAuthorized {
			return true
		}
	}
	return false
}

func (m *Manager) applyExpiry(session Session) Session {
	if session.Status != LoginPending {
		return session
	}
	expiresAt, err := time.Parse(time.RFC3339, session.ExpiresAt)
	if err != nil {
		return session
	}
	if time.Now().UTC().After(expiresAt) {
		session.Status = LoginExpired
	}
	return session
}
