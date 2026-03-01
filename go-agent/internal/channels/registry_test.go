package channels

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/config"
)

func TestRegistrySendAndStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/bottoken/sendMessage" {
			t.Fatalf("unexpected telegram path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		if payload["chat_id"] != "chat-1" {
			t.Fatalf("unexpected chat_id payload: %v", payload["chat_id"])
		}
		if payload["text"] != "hello" {
			t.Fatalf("unexpected text payload: %v", payload["text"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":101,"date":1700000000,"chat":{"id":1}}}`))
	}))
	defer server.Close()
	t.Setenv("OPENCLAW_GO_TELEGRAM_API_BASE", server.URL)

	reg := NewRegistry("token", "chat-1")
	status := reg.Status()
	if len(status) < 10 {
		t.Fatalf("expected broad channel status catalog, got %d", len(status))
	}

	receipt, err := reg.Send(context.Background(), SendRequest{
		Channel: "telegram",
		Message: "hello",
	})
	if err != nil {
		t.Fatalf("telegram send failed: %v", err)
	}
	if receipt.Channel != "telegram" {
		t.Fatalf("unexpected receipt channel: %s", receipt.Channel)
	}
	if receipt.Status != "delivered" {
		t.Fatalf("unexpected receipt status: %s", receipt.Status)
	}
}

func TestGenericChannelWebhookSend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	cfg := config.Default().Channels
	cfg.WhatsApp = config.ChannelAdapterConfig{
		Enabled:       true,
		Token:         "secret",
		DefaultTarget: "chat-1",
		WebhookURL:    server.URL,
		AuthHeader:    "Authorization",
		AuthPrefix:    "Bearer",
	}
	reg := NewRegistryFromConfig(cfg)

	receipt, err := reg.Send(context.Background(), SendRequest{
		Channel: "wa",
		Message: "hello webhook",
	})
	if err != nil {
		t.Fatalf("whatsapp webhook send failed: %v", err)
	}
	if receipt.Channel != "whatsapp" {
		t.Fatalf("unexpected channel: %s", receipt.Channel)
	}
	if receipt.Status != "delivered" {
		t.Fatalf("unexpected status: %s", receipt.Status)
	}
}

func TestGenericChannelDisabled(t *testing.T) {
	cfg := config.Default().Channels
	cfg.Discord = config.ChannelAdapterConfig{Enabled: false}
	reg := NewRegistryFromConfig(cfg)
	_, err := reg.Send(context.Background(), SendRequest{
		Channel: "discord",
		Message: "hello",
	})
	if err == nil {
		t.Fatalf("expected disabled channel send to fail")
	}
}

func TestGenericChannelTokenReadySend(t *testing.T) {
	cfg := config.Default().Channels
	cfg.Slack = config.ChannelAdapterConfig{
		Enabled:       true,
		Token:         "slack-token",
		DefaultTarget: "room-1",
	}
	reg := NewRegistryFromConfig(cfg)
	receipt, err := reg.Send(context.Background(), SendRequest{
		Channel: "slack",
		Message: "hello slack",
	})
	if err != nil {
		t.Fatalf("slack token-ready send failed: %v", err)
	}
	if receipt.Channel != "slack" {
		t.Fatalf("unexpected channel: %s", receipt.Channel)
	}
}

func TestTelegramRequiresToken(t *testing.T) {
	reg := NewRegistry("", "")
	_, err := reg.Send(context.Background(), SendRequest{
		Channel: "telegram",
		Message: "hello",
	})
	if err == nil {
		t.Fatalf("expected telegram send to fail when token missing")
	}
}

func TestTelegramLongMessageSplitsAcrossMultipleSends(t *testing.T) {
	requestCount := 0
	chunkLengths := make([]int, 0, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/bottoken/sendMessage" {
			t.Fatalf("unexpected telegram path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		text, _ := payload["text"].(string)
		chunkLengths = append(chunkLengths, len([]rune(text)))
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":101,"date":1700000000,"chat":{"id":1}}}`))
	}))
	defer server.Close()
	t.Setenv("OPENCLAW_GO_TELEGRAM_API_BASE", server.URL)

	reg := NewRegistry("token", "chat-1")
	longMessage := strings.Repeat("a", maxTelegramMessageRunes+321)
	receipt, err := reg.Send(context.Background(), SendRequest{
		Channel: "telegram",
		Message: longMessage,
	})
	if err != nil {
		t.Fatalf("telegram send failed: %v", err)
	}
	if requestCount < 2 {
		t.Fatalf("expected at least 2 sendMessage requests, got %d", requestCount)
	}
	for _, length := range chunkLengths {
		if length > maxTelegramMessageRunes {
			t.Fatalf("chunk exceeds telegram max size: %d", length)
		}
	}
	meta := receipt.Metadata
	if chunked, _ := meta["chunked"].(bool); !chunked {
		t.Fatalf("expected chunked metadata true")
	}
}
