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
const maxTelegramMessageRunes = 4096

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

	chunks := splitTelegramMessage(message, maxTelegramMessageRunes)
	if len(chunks) == 0 {
		return SendReceipt{}, errors.New("message is required")
	}

	messageIDs := make([]int64, 0, len(chunks))
	lastResult := telegramSendMessageResult{}
	for _, chunk := range chunks {
		sent, err := d.sendSingleMessage(ctx, token, target, chunk)
		if err != nil {
			return SendReceipt{}, err
		}
		lastResult = sent
		messageIDs = append(messageIDs, sent.MessageID)
	}
	d.setConnected()

	metadata := map[string]any{
		"mode":       "telegram-bot-api",
		"messageId":  lastResult.MessageID,
		"chatId":     lastResult.Chat.ID,
		"date":       lastResult.Date,
		"chunked":    len(chunks) > 1,
		"chunkCount": len(chunks),
		"messageIds": messageIDs,
	}
	if len(messageIDs) > 0 {
		metadata["firstMessageId"] = messageIDs[0]
	}

	receipt := SendReceipt{
		Provider: "telegram",
		Channel:  "telegram",
		To:       target,
		Message:  message,
		Status:   "delivered",
		Metadata: metadata,
	}
	return receipt, nil
}

func (d *telegramDriver) sendSingleMessage(ctx context.Context, token string, target string, message string) (telegramSendMessageResult, error) {
	payload := map[string]any{
		"chat_id": target,
		"text":    message,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return telegramSendMessageResult{}, err
	}

	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", TelegramAPIBase(), token)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return telegramSendMessageResult{}, err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := d.httpClient.Do(request)
	if err != nil {
		d.setDisconnected(err.Error())
		return telegramSendMessageResult{}, err
	}
	defer response.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		description := strings.TrimSpace(string(body))
		if description == "" {
			description = fmt.Sprintf("telegram sendMessage returned HTTP %d", response.StatusCode)
		}
		d.setDisconnected(description)
		return telegramSendMessageResult{}, fmt.Errorf("telegram sendMessage failed: %s", description)
	}

	var parsed telegramAPIResponse[telegramSendMessageResult]
	if err := json.Unmarshal(body, &parsed); err != nil {
		d.setDisconnected("invalid telegram response: " + err.Error())
		return telegramSendMessageResult{}, fmt.Errorf("telegram sendMessage invalid response: %w", err)
	}
	if !parsed.OK {
		description := strings.TrimSpace(parsed.Description)
		if description == "" {
			description = "telegram sendMessage rejected request"
		}
		d.setDisconnected(description)
		return telegramSendMessageResult{}, errors.New(description)
	}

	return parsed.Result, nil
}

func (d *telegramDriver) SendTyping(ctx context.Context, target string) error {
	token := strings.TrimSpace(d.botToken)
	if token == "" {
		d.setDisconnected("telegram bot token is not configured")
		return errors.New("telegram bot token is not configured")
	}

	resolvedTarget := strings.TrimSpace(target)
	if resolvedTarget == "" {
		resolvedTarget = strings.TrimSpace(d.defaultTarget)
	}
	if resolvedTarget == "" {
		return errors.New("telegram target chat id is required")
	}

	payload := map[string]any{
		"chat_id": resolvedTarget,
		"action":  "typing",
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	endpoint := fmt.Sprintf("%s/bot%s/sendChatAction", TelegramAPIBase(), token)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := d.httpClient.Do(request)
	if err != nil {
		d.setDisconnected(err.Error())
		return err
	}
	defer response.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		description := strings.TrimSpace(string(body))
		if description == "" {
			description = fmt.Sprintf("telegram sendChatAction returned HTTP %d", response.StatusCode)
		}
		d.setDisconnected(description)
		return fmt.Errorf("telegram sendChatAction failed: %s", description)
	}

	var parsed telegramAPIResponse[bool]
	if err := json.Unmarshal(body, &parsed); err != nil {
		d.setDisconnected("invalid telegram response: " + err.Error())
		return fmt.Errorf("telegram sendChatAction invalid response: %w", err)
	}
	if !parsed.OK {
		description := strings.TrimSpace(parsed.Description)
		if description == "" {
			description = "telegram sendChatAction rejected request"
		}
		d.setDisconnected(description)
		return errors.New(description)
	}

	d.setConnected()
	return nil
}

func splitTelegramMessage(message string, maxRunes int) []string {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return []string{}
	}
	if maxRunes <= 0 {
		return []string{trimmed}
	}

	runes := []rune(trimmed)
	if len(runes) <= maxRunes {
		return []string{trimmed}
	}

	chunks := make([]string, 0, (len(runes)/maxRunes)+1)
	for start := 0; start < len(runes); {
		end := start + maxRunes
		if end > len(runes) {
			end = len(runes)
		}
		if end < len(runes) {
			split := end
			minSplit := start + maxRunes/2
			if minSplit < start {
				minSplit = start
			}
			for i := end; i > minSplit; i-- {
				if runes[i-1] == '\n' || runes[i-1] == ' ' {
					split = i
					break
				}
			}
			end = split
		}
		if end <= start {
			end = start + maxRunes
			if end > len(runes) {
				end = len(runes)
			}
		}
		part := strings.TrimSpace(string(runes[start:end]))
		if part != "" {
			chunks = append(chunks, part)
		}
		start = end
	}
	if len(chunks) == 0 {
		return []string{trimmed}
	}
	return chunks
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
