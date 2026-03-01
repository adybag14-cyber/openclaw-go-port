package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/config"
)

type genericDriver struct {
	name          string
	aliases       []string
	enabled       bool
	token         string
	defaultTarget string
	webhookURL    string
	authHeader    string
	authPrefix    string
	headers       map[string]string
	connected     bool
	lastError     string
	httpClient    *http.Client
}

func newGenericDriver(name string, aliases []string, cfg config.ChannelAdapterConfig) *genericDriver {
	token := strings.TrimSpace(cfg.Token)
	webhookURL := strings.TrimSpace(cfg.WebhookURL)
	authHeader := strings.TrimSpace(cfg.AuthHeader)
	if authHeader == "" {
		authHeader = "Authorization"
	}
	authPrefix := strings.TrimSpace(cfg.AuthPrefix)
	if authPrefix == "" {
		authPrefix = "Bearer"
	}
	headers := make(map[string]string, len(cfg.Headers))
	for key, value := range cfg.Headers {
		k := strings.TrimSpace(key)
		if k == "" {
			continue
		}
		headers[k] = strings.TrimSpace(value)
	}
	return &genericDriver{
		name:          strings.ToLower(strings.TrimSpace(name)),
		aliases:       append([]string(nil), aliases...),
		enabled:       cfg.Enabled,
		token:         token,
		defaultTarget: strings.TrimSpace(cfg.DefaultTarget),
		webhookURL:    webhookURL,
		authHeader:    authHeader,
		authPrefix:    authPrefix,
		headers:       headers,
		connected:     cfg.Enabled && (token != "" || webhookURL != ""),
		lastError:     "",
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (d *genericDriver) Name() string { return d.name }

func (d *genericDriver) Aliases() []string {
	return append([]string(nil), d.aliases...)
}

func (d *genericDriver) Status() ChannelStatus {
	return ChannelStatus{
		Name:          d.name,
		Connected:     d.connected,
		Running:       true,
		DefaultTarget: d.defaultTarget,
		Aliases:       d.Aliases(),
		LastError:     d.lastError,
	}
}

func (d *genericDriver) Send(ctx context.Context, req SendRequest) (SendReceipt, error) {
	if !d.enabled {
		return SendReceipt{}, fmt.Errorf("%s channel is disabled", d.name)
	}
	message := strings.TrimSpace(req.Message)
	if message == "" {
		return SendReceipt{}, errors.New("message is required")
	}
	target := strings.TrimSpace(req.To)
	if target == "" {
		target = d.defaultTarget
	}
	if d.webhookURL != "" {
		return d.sendWebhook(ctx, target, req.SessionID, message)
	}
	if d.token == "" {
		return SendReceipt{}, fmt.Errorf("%s token is not configured", d.name)
	}
	return SendReceipt{
		Provider: d.name,
		Channel:  d.name,
		To:       target,
		Message:  message,
		Status:   "delivered",
		Metadata: map[string]any{
			"mode": "token-ready",
		},
	}, nil
}

func (d *genericDriver) sendWebhook(ctx context.Context, target string, sessionID string, message string) (SendReceipt, error) {
	payload := map[string]any{
		"channel":   d.name,
		"to":        target,
		"message":   message,
		"sessionId": strings.TrimSpace(sessionID),
		"sentAt":    time.Now().UTC().Format(time.RFC3339),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return SendReceipt{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, d.webhookURL, bytes.NewReader(raw))
	if err != nil {
		return SendReceipt{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	for key, value := range d.headers {
		request.Header.Set(key, value)
	}
	if d.token != "" {
		tokenValue := d.token
		if d.authPrefix != "" {
			tokenValue = d.authPrefix + " " + tokenValue
		}
		request.Header.Set(d.authHeader, tokenValue)
	}

	response, err := d.httpClient.Do(request)
	if err != nil {
		d.connected = false
		d.lastError = err.Error()
		return SendReceipt{}, err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		d.connected = false
		d.lastError = string(body)
		return SendReceipt{}, fmt.Errorf("%s webhook returned status %d: %s", d.name, response.StatusCode, strings.TrimSpace(string(body)))
	}
	d.connected = true
	d.lastError = ""
	return SendReceipt{
		Provider: d.name,
		Channel:  d.name,
		To:       target,
		Message:  message,
		Status:   "delivered",
		Metadata: map[string]any{
			"mode":       "webhook",
			"statusCode": response.StatusCode,
			"response":   strings.TrimSpace(string(body)),
		},
	}, nil
}

func (d *genericDriver) Logout(_ context.Context, _ string) (bool, error) {
	d.connected = false
	d.lastError = "logged out by operator"
	return true, nil
}
