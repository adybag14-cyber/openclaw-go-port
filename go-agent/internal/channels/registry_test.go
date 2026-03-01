package channels

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/config"
)

func TestRegistrySendAndStatus(t *testing.T) {
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
