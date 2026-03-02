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
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected bridge endpoint: %s", r.URL.Path)
		}
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
