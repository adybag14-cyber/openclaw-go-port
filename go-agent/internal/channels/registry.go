package channels

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type SendRequest struct {
	Channel   string `json:"channel"`
	To        string `json:"to,omitempty"`
	Message   string `json:"message"`
	SessionID string `json:"sessionId,omitempty"`
}

type SendReceipt struct {
	ID        string         `json:"id"`
	Channel   string         `json:"channel"`
	Provider  string         `json:"provider"`
	To        string         `json:"to,omitempty"`
	Message   string         `json:"message"`
	Status    string         `json:"status"`
	CreatedAt string         `json:"createdAt"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type ChannelStatus struct {
	Name          string   `json:"name"`
	Connected     bool     `json:"connected"`
	Running       bool     `json:"running"`
	DefaultTarget string   `json:"defaultTarget,omitempty"`
	Aliases       []string `json:"aliases,omitempty"`
	LastError     string   `json:"lastError,omitempty"`
}

type Driver interface {
	Name() string
	Aliases() []string
	Status() ChannelStatus
	Send(context.Context, SendRequest) (SendReceipt, error)
	Logout(context.Context, string) (bool, error)
}

type Registry struct {
	mu      sync.RWMutex
	drivers map[string]Driver
	seq     atomic.Uint64
}

func NewRegistry(telegramBotToken string, telegramDefaultTarget string) *Registry {
	r := &Registry{
		drivers: make(map[string]Driver),
	}
	r.register(newWebchatDriver())
	r.register(newCLIChannelDriver())
	r.register(newTelegramDriver(telegramBotToken, telegramDefaultTarget))
	return r
}

func (r *Registry) register(driver Driver) {
	canonical := normalizeChannel(driver.Name())
	r.drivers[canonical] = driver
	for _, alias := range driver.Aliases() {
		r.drivers[normalizeChannel(alias)] = driver
	}
}

func (r *Registry) Resolve(channel string) (Driver, string, bool) {
	normalized := normalizeChannel(channel)
	if normalized == "" {
		normalized = "webchat"
	}
	r.mu.RLock()
	driver, ok := r.drivers[normalized]
	r.mu.RUnlock()
	if !ok {
		return nil, normalized, false
	}
	return driver, normalizeChannel(driver.Name()), true
}

func (r *Registry) Send(ctx context.Context, req SendRequest) (SendReceipt, error) {
	driver, canonical, ok := r.Resolve(req.Channel)
	if !ok {
		return SendReceipt{}, fmt.Errorf("unsupported channel %q", req.Channel)
	}
	req.Channel = canonical
	receipt, err := driver.Send(ctx, req)
	if err != nil {
		return SendReceipt{}, err
	}
	if receipt.ID == "" {
		receipt.ID = fmt.Sprintf("msg-%06d", r.seq.Add(1))
	}
	if receipt.Channel == "" {
		receipt.Channel = canonical
	}
	if receipt.CreatedAt == "" {
		receipt.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if receipt.Status == "" {
		receipt.Status = "delivered"
	}
	return receipt, nil
}

func (r *Registry) Logout(ctx context.Context, channel string, accountID string) (bool, error) {
	driver, _, ok := r.Resolve(channel)
	if !ok {
		return false, errors.New("unknown channel")
	}
	return driver.Logout(ctx, accountID)
}

func (r *Registry) Status() []ChannelStatus {
	r.mu.RLock()
	uniq := map[string]Driver{}
	for _, driver := range r.drivers {
		uniq[normalizeChannel(driver.Name())] = driver
	}
	r.mu.RUnlock()

	out := make([]ChannelStatus, 0, len(uniq))
	for _, driver := range uniq {
		status := driver.Status()
		status.Name = normalizeChannel(driver.Name())
		out = append(out, status)
	}
	return out
}

func normalizeChannel(channel string) string {
	switch strings.ToLower(strings.TrimSpace(channel)) {
	case "", "web", "webchat":
		return "webchat"
	case "telegram", "tg", "tele":
		return "telegram"
	case "cli", "console", "terminal":
		return "cli"
	default:
		return strings.ToLower(strings.TrimSpace(channel))
	}
}

type webchatDriver struct{}

func newWebchatDriver() *webchatDriver { return &webchatDriver{} }
func (d *webchatDriver) Name() string  { return "webchat" }
func (d *webchatDriver) Aliases() []string {
	return []string{"web"}
}
func (d *webchatDriver) Status() ChannelStatus {
	return ChannelStatus{
		Name:      "webchat",
		Connected: true,
		Running:   true,
		Aliases:   d.Aliases(),
	}
}
func (d *webchatDriver) Send(_ context.Context, req SendRequest) (SendReceipt, error) {
	if strings.TrimSpace(req.Message) == "" {
		return SendReceipt{}, errors.New("message is required")
	}
	return SendReceipt{
		Provider: "webchat",
		Channel:  "webchat",
		To:       req.To,
		Message:  req.Message,
		Status:   "delivered",
	}, nil
}
func (d *webchatDriver) Logout(_ context.Context, _ string) (bool, error) {
	return true, nil
}

type cliChannelDriver struct{}

func newCLIChannelDriver() *cliChannelDriver { return &cliChannelDriver{} }
func (d *cliChannelDriver) Name() string     { return "cli" }
func (d *cliChannelDriver) Aliases() []string {
	return []string{"console", "terminal"}
}
func (d *cliChannelDriver) Status() ChannelStatus {
	return ChannelStatus{
		Name:      "cli",
		Connected: true,
		Running:   true,
		Aliases:   d.Aliases(),
	}
}
func (d *cliChannelDriver) Send(_ context.Context, req SendRequest) (SendReceipt, error) {
	if strings.TrimSpace(req.Message) == "" {
		return SendReceipt{}, errors.New("message is required")
	}
	return SendReceipt{
		Provider: "cli",
		Channel:  "cli",
		To:       req.To,
		Message:  req.Message,
		Status:   "queued",
	}, nil
}
func (d *cliChannelDriver) Logout(_ context.Context, _ string) (bool, error) {
	return true, nil
}
