package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const defaultTelegramAPIBase = "https://api.telegram.org"

func TelegramAPIBase() string {
	if custom := strings.TrimSpace(os.Getenv("OPENCLAW_GO_TELEGRAM_API_BASE")); custom != "" {
		return strings.TrimRight(custom, "/")
	}
	return defaultTelegramAPIBase
}

type telegramDriver struct {
	mu sync.RWMutex

	botToken      string
	defaultTarget string
	connected     bool
	lastError     string
	httpClient    *http.Client
}

type telegramSendMessageResult struct {
	MessageID int64 `json:"message_id"`
	Date      int64 `json:"date"`
	Chat      struct {
		ID int64 `json:"id"`
	} `json:"chat"`
}

type telegramAPIResponse[T any] struct {
	OK          bool   `json:"ok"`
	Result      T      `json:"result"`
	Description string `json:"description"`
	ErrorCode   int    `json:"error_code"`
}

func newTelegramDriver(botToken string, defaultTarget string) *telegramDriver {
	token := strings.TrimSpace(botToken)
	return &telegramDriver{
		botToken:      token,
		defaultTarget: strings.TrimSpace(defaultTarget),
		connected:     token != "",
		lastError:     "",
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (d *telegramDriver) Name() string { return "telegram" }

func (d *telegramDriver) Aliases() []string {
	return []string{"tg", "tele"}
}

func (d *telegramDriver) Status() ChannelStatus {
	d.mu.RLock()
	connected := d.connected
	lastError := d.lastError
	d.mu.RUnlock()
	return ChannelStatus{
		Name:          "telegram",
		Connected:     connected,
		Running:       true,
		DefaultTarget: d.defaultTarget,
		Aliases:       d.Aliases(),
		LastError:     lastError,
	}
}

func (d *telegramDriver) Send(ctx context.Context, req SendRequest) (SendReceipt, error) {
	token := strings.TrimSpace(d.botToken)
	if token == "" {
		d.setDisconnected("telegram bot token is not configured")
		return SendReceipt{}, errors.New("telegram bot token is not configured")
	}

	message := strings.TrimSpace(req.Message)
	if message == "" {
		return SendReceipt{}, errors.New("message is required")
	}

	target := strings.TrimSpace(req.To)
	if target == "" {
		target = strings.TrimSpace(d.defaultTarget)
	}
	if target == "" {
		return SendReceipt{}, errors.New("telegram target chat id is required")
	}

	payload := map[string]any{
		"chat_id": target,
		"text":    message,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return SendReceipt{}, err
	}

	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", TelegramAPIBase(), token)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return SendReceipt{}, err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := d.httpClient.Do(request)
	if err != nil {
		d.setDisconnected(err.Error())
		return SendReceipt{}, err
	}
	defer response.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		description := strings.TrimSpace(string(body))
		if description == "" {
			description = fmt.Sprintf("telegram sendMessage returned HTTP %d", response.StatusCode)
		}
		d.setDisconnected(description)
		return SendReceipt{}, fmt.Errorf("telegram sendMessage failed: %s", description)
	}

	var parsed telegramAPIResponse[telegramSendMessageResult]
	if err := json.Unmarshal(body, &parsed); err != nil {
		d.setDisconnected("invalid telegram response: " + err.Error())
		return SendReceipt{}, fmt.Errorf("telegram sendMessage invalid response: %w", err)
	}
	if !parsed.OK {
		description := strings.TrimSpace(parsed.Description)
		if description == "" {
			description = "telegram sendMessage rejected request"
		}
		d.setDisconnected(description)
		return SendReceipt{}, errors.New(description)
	}

	d.setConnected()
	receipt := SendReceipt{
		Provider: "telegram",
		Channel:  "telegram",
		To:       target,
		Message:  message,
		Status:   "delivered",
		Metadata: map[string]any{
			"mode":      "telegram-bot-api",
			"messageId": parsed.Result.MessageID,
			"chatId":    parsed.Result.Chat.ID,
			"date":      parsed.Result.Date,
		},
	}
	return receipt, nil
}

func (d *telegramDriver) Logout(_ context.Context, _ string) (bool, error) {
	d.setDisconnected("logged out by operator")
	return true, nil
}

func (d *telegramDriver) setConnected() {
	d.mu.Lock()
	d.connected = true
	d.lastError = ""
	d.mu.Unlock()
}

func (d *telegramDriver) setDisconnected(reason string) {
	d.mu.Lock()
	d.connected = false
	d.lastError = strings.TrimSpace(reason)
	d.mu.Unlock()
}
