package web

import (
	"context"
	"errors"
	"fmt"
	"sort"
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
	provider := normalizeProviderAlias(opts.Provider)
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
	verificationURI := providerVerificationURI(provider)

	session := Session{
		ID:                      id,
		Status:                  LoginPending,
		Provider:                provider,
		Model:                   model,
		Code:                    code,
		VerificationURI:         verificationURI,
		VerificationURIComplete: fmt.Sprintf("%s?openclaw_code=%s", strings.TrimRight(verificationURI, "/"), code),
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

func (m *Manager) IsAuthorized(id string) bool {
	session, ok := m.Get(id)
	if !ok {
		return false
	}
	return session.Status == LoginAuthorized
}

func (m *Manager) List() []Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Session, 0, len(m.sessions))
	for _, state := range m.sessions {
		out = append(out, m.applyExpiry(state.session))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt < out[j].CreatedAt
	})
	return out
}

func (m *Manager) Summary() map[string]any {
	sessions := m.List()
	byProvider := map[string]map[string]int{}
	pending := 0
	authorized := 0
	expired := 0
	rejected := 0
	for _, session := range sessions {
		provider := strings.ToLower(strings.TrimSpace(session.Provider))
		if provider == "" {
			provider = "unknown"
		}
		bucket, ok := byProvider[provider]
		if !ok {
			bucket = map[string]int{
				"total":      0,
				"pending":    0,
				"authorized": 0,
				"expired":    0,
				"rejected":   0,
			}
			byProvider[provider] = bucket
		}
		bucket["total"]++
		switch session.Status {
		case LoginAuthorized:
			authorized++
			bucket["authorized"]++
		case LoginExpired:
			expired++
			bucket["expired"]++
		case LoginRejected:
			rejected++
			bucket["rejected"]++
		default:
			pending++
			bucket["pending"]++
		}
	}
	return map[string]any{
		"total":      len(sessions),
		"pending":    pending,
		"authorized": authorized,
		"expired":    expired,
		"rejected":   rejected,
		"byProvider": byProvider,
	}
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

func providerVerificationURI(provider string) string {
	switch normalizeProviderAlias(provider) {
	case "claude":
		return "https://claude.ai/"
	case "gemini":
		return "https://aistudio.google.com/"
	case "openrouter":
		return "https://openrouter.ai/"
	case "opencode":
		return "https://opencode.ai/"
	case "minimax":
		return "https://chat.minimax.io/"
	case "kimi":
		return "https://kimi.com/"
	case "zhipuai":
		return "https://open.bigmodel.cn/"
	case "zai":
		return "https://chat.z.ai/"
	case "inception":
		return "https://chat.inceptionlabs.ai/"
	case "qwen":
		return "https://chat.qwen.ai/"
	case "chatgpt", "codex", "openai":
		fallthrough
	default:
		return "https://chatgpt.com/"
	}
}

func normalizeProviderAlias(provider string) string {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	switch normalized {
	case "openai", "openai-chatgpt", "chatgpt-web", "chatgpt.com":
		return "chatgpt"
	case "openai-codex", "codex-cli", "openai-codex-cli":
		return "codex"
	case "anthropic", "claude-cli", "claude-code", "claude-desktop":
		return "claude"
	case "google", "google-gemini", "google-gemini-cli", "gemini-cli":
		return "gemini"
	case "qwen-portal", "qwen-cli", "qwen-chat", "qwen35", "qwen3.5", "qwen-3.5", "copaw", "qwen-copaw", "qwen-agent":
		return "qwen"
	case "minimax-portal", "minimax-cli":
		return "minimax"
	case "kimi-code", "kimi-coding", "kimi-for-coding":
		return "kimi"
	case "opencode-zen", "opencode-ai", "opencode-go", "opencode_free", "opencodefree":
		return "opencode"
	case "zhipu", "zhipu-ai", "bigmodel", "bigmodel-cn", "zhipuai-coding", "zhipu-coding":
		return "zhipuai"
	case "z.ai", "z-ai", "zaiweb", "zai-web", "glm", "glm5", "glm-5":
		return "zai"
	case "inception-labs", "inceptionlabs", "mercury", "mercury2", "mercury-2":
		return "inception"
	default:
		return normalized
	}
}
