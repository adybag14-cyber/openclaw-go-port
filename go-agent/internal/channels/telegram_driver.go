package channels

import (
	"context"
	"errors"
	"strings"
)

type telegramDriver struct {
	botToken      string
	defaultTarget string
	connected     bool
	lastError     string
}

func newTelegramDriver(botToken string, defaultTarget string) *telegramDriver {
	token := strings.TrimSpace(botToken)
	return &telegramDriver{
		botToken:      token,
		defaultTarget: strings.TrimSpace(defaultTarget),
		connected:     token != "",
		lastError:     "",
	}
}

func (d *telegramDriver) Name() string { return "telegram" }
func (d *telegramDriver) Aliases() []string {
	return []string{"tg", "tele"}
}
func (d *telegramDriver) Status() ChannelStatus {
	return ChannelStatus{
		Name:          "telegram",
		Connected:     d.connected,
		Running:       true,
		DefaultTarget: d.defaultTarget,
		Aliases:       d.Aliases(),
		LastError:     d.lastError,
	}
}
func (d *telegramDriver) Send(_ context.Context, req SendRequest) (SendReceipt, error) {
	if !d.connected {
		return SendReceipt{}, errors.New("telegram bot token is not configured")
	}
	message := strings.TrimSpace(req.Message)
	if message == "" {
		return SendReceipt{}, errors.New("message is required")
	}
	target := strings.TrimSpace(req.To)
	if target == "" {
		target = d.defaultTarget
	}
	return SendReceipt{
		Provider: "telegram",
		Channel:  "telegram",
		To:       target,
		Message:  message,
		Status:   "delivered",
	}, nil
}
func (d *telegramDriver) Logout(_ context.Context, _ string) (bool, error) {
	d.connected = false
	d.lastError = "logged out by operator"
	return true, nil
}
