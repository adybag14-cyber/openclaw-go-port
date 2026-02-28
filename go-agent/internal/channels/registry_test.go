package channels

import (
	"context"
	"testing"
)

func TestRegistrySendAndStatus(t *testing.T) {
	reg := NewRegistry("token", "chat-1")
	status := reg.Status()
	if len(status) < 3 {
		t.Fatalf("expected at least three channels in status, got %d", len(status))
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
