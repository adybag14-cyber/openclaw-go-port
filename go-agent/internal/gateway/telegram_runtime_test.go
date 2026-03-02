package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	webbridge "github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/bridge/web"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/buildinfo"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/config"
)

func TestProcessTelegramUpdateCommandSendsReply(t *testing.T) {
	var mu sync.Mutex
	messages := make([]string, 0, 4)
	telegramAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bottoken/sendMessage" {
			t.Fatalf("unexpected telegram endpoint: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		mu.Lock()
		messages = append(messages, strings.TrimSpace(toString(payload["text"], "")))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":31,"chat":{"id":777},"date":1700000000}}`))
	}))
	defer telegramAPI.Close()
	t.Setenv("OPENCLAW_GO_TELEGRAM_API_BASE", telegramAPI.URL)

	cfg := config.Default()
	cfg.Channels.Telegram.BotToken = "token"
	cfg.Runtime.StatePath = "memory://telegram-runtime-command"

	s := New(cfg, buildinfo.Default())
	defer s.Close()

	update := telegramInboundUpdate{
		UpdateID: 1,
		Message: &telegramInboundEntry{
			Text: "/tts@openclaw_bot status",
			Chat: telegramInboundChat{ID: 777},
			From: telegramInboundUser{ID: 11, IsBot: false},
		},
	}
	if err := s.processTelegramUpdate(context.Background(), update); err != nil {
		t.Fatalf("processTelegramUpdate command failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(messages) != 1 {
		t.Fatalf("expected one outbound telegram message, got %d", len(messages))
	}
	if !strings.Contains(messages[0], "TTS is") {
		t.Fatalf("expected /tts status response, got: %s", messages[0])
	}
}

func TestProcessTelegramUpdateTextBridgesAssistantReply(t *testing.T) {
	var bridgeMu sync.Mutex
	var bridgePayload map[string]any
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected bridge endpoint: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		bridgeMu.Lock()
		bridgePayload = payload
		bridgeMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cmpl-test","model":"gpt-5.2","choices":[{"message":{"role":"assistant","content":"bridge-response-ok"}}]}`))
	}))
	defer bridge.Close()

	var mu sync.Mutex
	messages := make([]string, 0, 4)
	typingCalls := 0
	telegramAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bottoken/sendMessage":
			body, _ := io.ReadAll(r.Body)
			var payload map[string]any
			_ = json.Unmarshal(body, &payload)
			mu.Lock()
			messages = append(messages, strings.TrimSpace(toString(payload["text"], "")))
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":44,"chat":{"id":888},"date":1700000000}}`))
		case "/bottoken/sendChatAction":
			typingCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
		default:
			t.Fatalf("unexpected telegram endpoint: %s", r.URL.Path)
		}
	}))
	defer telegramAPI.Close()
	t.Setenv("OPENCLAW_GO_TELEGRAM_API_BASE", telegramAPI.URL)

	cfg := config.Default()
	cfg.Channels.Telegram.BotToken = "token"
	cfg.Runtime.StatePath = "memory://telegram-runtime-bridge"
	cfg.Runtime.BrowserBridge.Enabled = true
	cfg.Runtime.BrowserBridge.Endpoint = bridge.URL
	cfg.Runtime.BrowserBridge.RequestTimeoutMs = 5000

	s := New(cfg, buildinfo.Default())
	defer s.Close()

	update := telegramInboundUpdate{
		UpdateID: 2,
		Message: &telegramInboundEntry{
			Text: "hello from phone",
			Chat: telegramInboundChat{ID: 888},
			From: telegramInboundUser{ID: 22, IsBot: false},
		},
	}
	if err := s.processTelegramUpdate(context.Background(), update); err != nil {
		t.Fatalf("processTelegramUpdate text failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(messages) != 1 {
		t.Fatalf("expected one outbound telegram message, got %d", len(messages))
	}
	if messages[0] != "bridge-response-ok" {
		t.Fatalf("expected bridged assistant reply, got: %s", messages[0])
	}
	if typingCalls == 0 {
		t.Fatalf("expected at least one typing indicator call")
	}
	bridgeMu.Lock()
	defer bridgeMu.Unlock()
	rawMessages, ok := bridgePayload["messages"].([]any)
	if !ok || len(rawMessages) < 2 {
		t.Fatalf("expected bridge payload to include system + user context, got: %#v", bridgePayload["messages"])
	}
}

func TestProcessTelegramUpdateStartCommandSendsHelp(t *testing.T) {
	var mu sync.Mutex
	messages := make([]string, 0, 4)
	telegramAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bottoken/sendMessage" {
			t.Fatalf("unexpected telegram endpoint: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		mu.Lock()
		messages = append(messages, strings.TrimSpace(toString(payload["text"], "")))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":61,"chat":{"id":999},"date":1700000000}}`))
	}))
	defer telegramAPI.Close()
	t.Setenv("OPENCLAW_GO_TELEGRAM_API_BASE", telegramAPI.URL)

	cfg := config.Default()
	cfg.Channels.Telegram.BotToken = "token"
	cfg.Runtime.StatePath = "memory://telegram-runtime-start"

	s := New(cfg, buildinfo.Default())
	defer s.Close()

	update := telegramInboundUpdate{
		UpdateID: 3,
		Message: &telegramInboundEntry{
			Text: "/start",
			Chat: telegramInboundChat{ID: 999},
			From: telegramInboundUser{ID: 33, IsBot: false},
		},
	}
	if err := s.processTelegramUpdate(context.Background(), update); err != nil {
		t.Fatalf("processTelegramUpdate start failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(messages) != 1 {
		t.Fatalf("expected one outbound telegram message, got %d", len(messages))
	}
	if !strings.Contains(messages[0], "Commands: /model, /auth, /set, /tts") {
		t.Fatalf("expected /start help response, got: %s", messages[0])
	}
}

func TestProcessTelegramUpdateTextStreamsLongReplies(t *testing.T) {
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected bridge endpoint: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cmpl-test","model":"gpt-5.2","choices":[{"message":{"role":"assistant","content":"This is a long bridged response that should be split into multiple telegram streaming chunks for live delivery."}}]}`))
	}))
	defer bridge.Close()

	var mu sync.Mutex
	messages := make([]string, 0, 8)
	typingCalls := 0
	telegramAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bottoken/sendMessage":
			body, _ := io.ReadAll(r.Body)
			var payload map[string]any
			_ = json.Unmarshal(body, &payload)
			mu.Lock()
			messages = append(messages, strings.TrimSpace(toString(payload["text"], "")))
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":72,"chat":{"id":889},"date":1700000000}}`))
		case "/bottoken/sendChatAction":
			typingCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
		default:
			t.Fatalf("unexpected telegram endpoint: %s", r.URL.Path)
		}
	}))
	defer telegramAPI.Close()
	t.Setenv("OPENCLAW_GO_TELEGRAM_API_BASE", telegramAPI.URL)

	cfg := config.Default()
	cfg.Channels.Telegram.BotToken = "token"
	cfg.Runtime.StatePath = "memory://telegram-runtime-streaming"
	cfg.Runtime.BrowserBridge.Enabled = true
	cfg.Runtime.BrowserBridge.Endpoint = bridge.URL
	cfg.Runtime.BrowserBridge.RequestTimeoutMs = 5000
	cfg.Runtime.TelegramLiveStreaming = true
	cfg.Runtime.TelegramStreamChunkChars = 30
	cfg.Runtime.TelegramStreamChunkDelayMs = 0
	cfg.Runtime.TelegramTypingIndicators = true
	cfg.Runtime.TelegramTypingIntervalMs = 1000

	s := New(cfg, buildinfo.Default())
	defer s.Close()

	update := telegramInboundUpdate{
		UpdateID: 4,
		Message: &telegramInboundEntry{
			Text: "stream me",
			Chat: telegramInboundChat{ID: 889},
			From: telegramInboundUser{ID: 44, IsBot: false},
		},
	}
	if err := s.processTelegramUpdate(context.Background(), update); err != nil {
		t.Fatalf("processTelegramUpdate streaming failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(messages) < 2 {
		t.Fatalf("expected streamed reply to produce multiple messages, got %d", len(messages))
	}
	if typingCalls == 0 {
		t.Fatalf("expected typing indicators during streamed reply")
	}
}

func TestProcessTelegramUpdateUsesSessionHistoryAndAuthScope(t *testing.T) {
	var bridgeMu sync.Mutex
	bridgePayloads := make([]map[string]any, 0, 4)
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected bridge endpoint: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		bridgeMu.Lock()
		bridgePayloads = append(bridgePayloads, payload)
		bridgeMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cmpl-test","model":"gpt-5.2","choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer bridge.Close()

	telegramAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bottoken/sendMessage", "/bottoken/sendChatAction":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":101,"chat":{"id":901},"date":1700000000}}`))
		default:
			t.Fatalf("unexpected telegram endpoint: %s", r.URL.Path)
		}
	}))
	defer telegramAPI.Close()
	t.Setenv("OPENCLAW_GO_TELEGRAM_API_BASE", telegramAPI.URL)

	cfg := config.Default()
	cfg.Channels.Telegram.BotToken = "token"
	cfg.Runtime.StatePath = "memory://telegram-runtime-history-auth"
	cfg.Runtime.BrowserBridge.Enabled = true
	cfg.Runtime.BrowserBridge.Endpoint = bridge.URL
	cfg.Runtime.BrowserBridge.RequestTimeoutMs = 5000
	cfg.Runtime.TelegramStreamChunkDelayMs = 0

	s := New(cfg, buildinfo.Default())
	defer s.Close()

	login := s.webLogin.Start(webbridge.StartOptions{Provider: "chatgpt", Model: "gpt-5.2"})
	if _, err := s.webLogin.Complete(login.ID, login.Code); err != nil {
		t.Fatalf("failed to authorize login session: %v", err)
	}
	s.compat.setTelegramAuthScoped("901", "chatgpt", "", login.ID)

	first := telegramInboundUpdate{
		UpdateID: 10,
		Message: &telegramInboundEntry{
			Text: "my name is ady",
			Chat: telegramInboundChat{ID: 901},
			From: telegramInboundUser{ID: 55, IsBot: false},
		},
	}
	second := telegramInboundUpdate{
		UpdateID: 11,
		Message: &telegramInboundEntry{
			Text: "what is my name?",
			Chat: telegramInboundChat{ID: 901},
			From: telegramInboundUser{ID: 55, IsBot: false},
		},
	}
	if err := s.processTelegramUpdate(context.Background(), first); err != nil {
		t.Fatalf("first telegram update failed: %v", err)
	}
	if err := s.processTelegramUpdate(context.Background(), second); err != nil {
		t.Fatalf("second telegram update failed: %v", err)
	}

	bridgeMu.Lock()
	defer bridgeMu.Unlock()
	if len(bridgePayloads) < 2 {
		t.Fatalf("expected at least two bridge payloads, got %d", len(bridgePayloads))
	}
	secondPayload := bridgePayloads[len(bridgePayloads)-1]
	if got := strings.TrimSpace(toString(secondPayload["loginSessionId"], "")); got != login.ID {
		t.Fatalf("expected loginSessionId %q in second payload, got %q", login.ID, got)
	}
	rawMessages, ok := secondPayload["messages"].([]any)
	if !ok || len(rawMessages) < 3 {
		t.Fatalf("expected second payload to contain rich context, got %#v", secondPayload["messages"])
	}
	foundPriorFact := false
	for _, raw := range rawMessages {
		msg, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		content := strings.ToLower(strings.TrimSpace(toString(msg["content"], "")))
		if strings.Contains(content, "my name is ady") {
			foundPriorFact = true
			break
		}
	}
	if !foundPriorFact {
		t.Fatalf("expected second payload messages to include prior session memory")
	}
}

func TestProcessTelegramUpdateFallsBackToLatestAuthorizedProviderSession(t *testing.T) {
	var bridgeMu sync.Mutex
	var bridgePayload map[string]any
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected bridge endpoint: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		bridgeMu.Lock()
		bridgePayload = payload
		bridgeMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cmpl-test","model":"gpt-5.2","choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer bridge.Close()

	telegramAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bottoken/sendMessage", "/bottoken/sendChatAction":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":222,"chat":{"id":902},"date":1700000000}}`))
		default:
			t.Fatalf("unexpected telegram endpoint: %s", r.URL.Path)
		}
	}))
	defer telegramAPI.Close()
	t.Setenv("OPENCLAW_GO_TELEGRAM_API_BASE", telegramAPI.URL)

	cfg := config.Default()
	cfg.Channels.Telegram.BotToken = "token"
	cfg.Runtime.StatePath = "memory://telegram-runtime-fallback-auth"
	cfg.Runtime.BrowserBridge.Enabled = true
	cfg.Runtime.BrowserBridge.Endpoint = bridge.URL
	cfg.Runtime.BrowserBridge.RequestTimeoutMs = 5000

	s := New(cfg, buildinfo.Default())
	defer s.Close()

	stale := s.webLogin.Start(webbridge.StartOptions{Provider: "chatgpt", Model: "gpt-5.2"})
	s.compat.setTelegramAuthScoped("902", "chatgpt", "", stale.ID)

	authorized := s.webLogin.Start(webbridge.StartOptions{Provider: "chatgpt", Model: "gpt-5.2"})
	if _, err := s.webLogin.Complete(authorized.ID, authorized.Code); err != nil {
		t.Fatalf("failed to authorize fallback session: %v", err)
	}

	update := telegramInboundUpdate{
		UpdateID: 12,
		Message: &telegramInboundEntry{
			Text: "hello fallback auth",
			Chat: telegramInboundChat{ID: 902},
			From: telegramInboundUser{ID: 66, IsBot: false},
		},
	}
	if err := s.processTelegramUpdate(context.Background(), update); err != nil {
		t.Fatalf("processTelegramUpdate fallback auth failed: %v", err)
	}

	bridgeMu.Lock()
	defer bridgeMu.Unlock()
	if bridgePayload == nil {
		t.Fatalf("expected bridge payload to be captured")
	}
	if got := strings.TrimSpace(toString(bridgePayload["loginSessionId"], "")); got != authorized.ID {
		t.Fatalf("expected fallback authorized loginSessionId %q, got %q", authorized.ID, got)
	}
}

func TestProcessTelegramUpdateFallsBackAcrossProviders(t *testing.T) {
	var bridgeMu sync.Mutex
	bridgePayloads := make([]map[string]any, 0, 4)
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		bridgeMu.Lock()
		bridgePayloads = append(bridgePayloads, payload)
		bridgeMu.Unlock()

		if strings.EqualFold(toString(payload["provider"], ""), "qwen") {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"qwen login missing"}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cmpl-fallback","model":"gpt-5.2","choices":[{"message":{"role":"assistant","content":"fallback-provider-ok"}}]}`))
	}))
	defer bridge.Close()

	var mu sync.Mutex
	messages := make([]string, 0, 4)
	telegramAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bottoken/sendMessage":
			body, _ := io.ReadAll(r.Body)
			var payload map[string]any
			_ = json.Unmarshal(body, &payload)
			mu.Lock()
			messages = append(messages, strings.TrimSpace(toString(payload["text"], "")))
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":333,"chat":{"id":903},"date":1700000000}}`))
		case "/bottoken/sendChatAction":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
		default:
			t.Fatalf("unexpected telegram endpoint: %s", r.URL.Path)
		}
	}))
	defer telegramAPI.Close()
	t.Setenv("OPENCLAW_GO_TELEGRAM_API_BASE", telegramAPI.URL)

	cfg := config.Default()
	cfg.Channels.Telegram.BotToken = "token"
	cfg.Runtime.StatePath = "memory://telegram-runtime-provider-fallback"
	cfg.Runtime.BrowserBridge.Enabled = true
	cfg.Runtime.BrowserBridge.Endpoint = bridge.URL
	cfg.Runtime.BrowserBridge.RequestTimeoutMs = 5000

	s := New(cfg, buildinfo.Default())
	defer s.Close()

	// Explicitly select a provider that will fail in the bridge mock.
	s.compat.setTelegramModelSelection("903", "qwen", "qwen3.5-plus")

	authorized := s.webLogin.Start(webbridge.StartOptions{Provider: "chatgpt", Model: "gpt-5.2"})
	if _, err := s.webLogin.Complete(authorized.ID, authorized.Code); err != nil {
		t.Fatalf("failed to authorize fallback provider session: %v", err)
	}

	update := telegramInboundUpdate{
		UpdateID: 13,
		Message: &telegramInboundEntry{
			Text: "can you still reply if qwen fails?",
			Chat: telegramInboundChat{ID: 903},
			From: telegramInboundUser{ID: 77, IsBot: false},
		},
	}
	if err := s.processTelegramUpdate(context.Background(), update); err != nil {
		t.Fatalf("processTelegramUpdate provider failover failed: %v", err)
	}

	mu.Lock()
	if len(messages) != 1 {
		mu.Unlock()
		t.Fatalf("expected one outbound telegram message, got %d", len(messages))
	}
	if messages[0] != "fallback-provider-ok" {
		got := messages[0]
		mu.Unlock()
		t.Fatalf("expected fallback reply text, got %q", got)
	}
	mu.Unlock()

	bridgeMu.Lock()
	defer bridgeMu.Unlock()
	if len(bridgePayloads) < 2 {
		t.Fatalf("expected at least 2 bridge attempts (primary + fallback), got %d", len(bridgePayloads))
	}
	firstProvider := strings.ToLower(strings.TrimSpace(toString(bridgePayloads[0]["provider"], "")))
	if firstProvider != "qwen" {
		t.Fatalf("expected first attempt provider=qwen, got %q", firstProvider)
	}
	seenQwen := false
	seenChatGPT := false
	chatGPTLoginID := ""
	for _, payload := range bridgePayloads {
		provider := strings.ToLower(strings.TrimSpace(toString(payload["provider"], "")))
		switch provider {
		case "qwen":
			seenQwen = true
		case "chatgpt":
			seenChatGPT = true
			if chatGPTLoginID == "" {
				chatGPTLoginID = strings.TrimSpace(toString(payload["loginSessionId"], ""))
			}
		}
	}
	if !seenQwen {
		t.Fatalf("expected at least one qwen attempt in bridge payloads")
	}
	if !seenChatGPT {
		t.Fatalf("expected fallback attempt for chatgpt provider")
	}
	if chatGPTLoginID != authorized.ID {
		t.Fatalf("expected fallback attempt loginSessionId %q, got %q", authorized.ID, chatGPTLoginID)
	}
}

func TestProcessTelegramUpdateForwardsProviderAPIKeyToBridge(t *testing.T) {
	var bridgeMu sync.Mutex
	var bridgePayload map[string]any
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		bridgeMu.Lock()
		bridgePayload = payload
		bridgeMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cmpl-key","model":"openrouter/auto","choices":[{"message":{"role":"assistant","content":"key-forward-ok"}}]}`))
	}))
	defer bridge.Close()

	telegramAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bottoken/sendMessage", "/bottoken/sendChatAction":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":444,"chat":{"id":904},"date":1700000000}}`))
		default:
			t.Fatalf("unexpected telegram endpoint: %s", r.URL.Path)
		}
	}))
	defer telegramAPI.Close()
	t.Setenv("OPENCLAW_GO_TELEGRAM_API_BASE", telegramAPI.URL)

	cfg := config.Default()
	cfg.Channels.Telegram.BotToken = "token"
	cfg.Runtime.StatePath = "memory://telegram-runtime-provider-key"
	cfg.Runtime.BrowserBridge.Enabled = true
	cfg.Runtime.BrowserBridge.Endpoint = bridge.URL
	cfg.Runtime.BrowserBridge.RequestTimeoutMs = 5000

	s := New(cfg, buildinfo.Default())
	defer s.Close()

	s.compat.setProviderAPIKey("openrouter", "sk-or-test")
	s.compat.setTelegramModelSelection("904", "openrouter", "openrouter/auto")

	update := telegramInboundUpdate{
		UpdateID: 14,
		Message: &telegramInboundEntry{
			Text: "use provider key",
			Chat: telegramInboundChat{ID: 904},
			From: telegramInboundUser{ID: 88, IsBot: false},
		},
	}
	if err := s.processTelegramUpdate(context.Background(), update); err != nil {
		t.Fatalf("processTelegramUpdate provider key failed: %v", err)
	}

	bridgeMu.Lock()
	defer bridgeMu.Unlock()
	if bridgePayload == nil {
		t.Fatalf("expected bridge payload to be captured")
	}
	if got := strings.TrimSpace(toString(bridgePayload["provider"], "")); got != "openrouter" {
		t.Fatalf("expected provider openrouter, got %q", got)
	}
	if got := strings.TrimSpace(toString(bridgePayload["apiKey"], "")); got != "sk-or-test" {
		t.Fatalf("expected apiKey forwarding, got %q", got)
	}
	if got := strings.TrimSpace(toString(bridgePayload["api_key"], "")); got != "sk-or-test" {
		t.Fatalf("expected api_key forwarding, got %q", got)
	}
}

func TestTrimTelegramMessagesToBudgetKeepsMostRecentMessages(t *testing.T) {
	messages := []map[string]any{
		{"role": "system", "content": "1234567890"},
		{"role": "user", "content": "aaaa"},
		{"role": "assistant", "content": "bbbb"},
		{"role": "user", "content": "cccc"},
	}

	trimmed := trimTelegramMessagesToBudget(messages, 18)
	if len(trimmed) != 3 {
		t.Fatalf("expected 3 messages after budget trim, got %d", len(trimmed))
	}
	if toString(trimmed[0]["role"], "") != "system" {
		t.Fatalf("expected first message to remain system, got %v", trimmed[0]["role"])
	}
	if toString(trimmed[len(trimmed)-1]["content"], "") != "cccc" {
		t.Fatalf("expected newest user message to be retained")
	}
}

func TestTrimTelegramMessagesToBudgetTruncatesNewestUserWhenNeeded(t *testing.T) {
	messages := []map[string]any{
		{"role": "system", "content": "1234567890"},
		{"role": "assistant", "content": "short"},
		{"role": "user", "content": "this is a very long user message that exceeds the budget"},
	}

	trimmed := trimTelegramMessagesToBudget(messages, 18)
	if len(trimmed) != 2 {
		t.Fatalf("expected only system + newest user messages, got %d", len(trimmed))
	}
	if toString(trimmed[1]["role"], "") != "user" {
		t.Fatalf("expected last message to remain user, got %v", trimmed[1]["role"])
	}
	content := toString(trimmed[1]["content"], "")
	if strings.TrimSpace(content) == "" {
		t.Fatalf("expected truncated user message to remain non-empty")
	}
	if len(content) > 8 {
		t.Fatalf("expected truncated user message to fit remaining budget, got %d chars", len(content))
	}
}
