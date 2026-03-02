package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/channels"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/scheduler"
	toolruntime "github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/tools/runtime"
)

type telegramPollEnvelope struct {
	OK          bool                    `json:"ok"`
	Result      []telegramInboundUpdate `json:"result"`
	Description string                  `json:"description"`
	ErrorCode   int                     `json:"error_code"`
}

type telegramInboundUpdate struct {
	UpdateID      int64                 `json:"update_id"`
	Message       *telegramInboundEntry `json:"message,omitempty"`
	EditedMessage *telegramInboundEntry `json:"edited_message,omitempty"`
	ChannelPost   *telegramInboundEntry `json:"channel_post,omitempty"`
}

type telegramInboundEntry struct {
	MessageID int64               `json:"message_id"`
	Text      string              `json:"text"`
	Caption   string              `json:"caption"`
	Chat      telegramInboundChat `json:"chat"`
	From      telegramInboundUser `json:"from"`
}

type telegramInboundChat struct {
	ID int64 `json:"id"`
}

type telegramInboundUser struct {
	ID       int64  `json:"id"`
	IsBot    bool   `json:"is_bot"`
	Username string `json:"username"`
}

func (u telegramInboundUpdate) pickMessage() *telegramInboundEntry {
	if u.Message != nil {
		return u.Message
	}
	if u.EditedMessage != nil {
		return u.EditedMessage
	}
	if u.ChannelPost != nil {
		return u.ChannelPost
	}
	return nil
}

func (s *Server) startBackgroundRuntimes(ctx context.Context) {
	if strings.TrimSpace(s.cfg.Channels.Telegram.BotToken) != "" {
		s.backgroundWG.Add(1)
		go func() {
			defer s.backgroundWG.Done()
			s.runTelegramPollingRuntime(ctx)
		}()
	}
}

func (s *Server) runTelegramPollingRuntime(ctx context.Context) {
	token := strings.TrimSpace(s.cfg.Channels.Telegram.BotToken)
	if token == "" {
		return
	}
	client := &http.Client{
		Timeout: 35 * time.Second,
	}
	var offset int64
	backoff := time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		updates, err := s.fetchTelegramUpdates(ctx, client, token, offset)
		if err != nil {
			wait := backoff
			if wait > 20*time.Second {
				wait = 20 * time.Second
			}
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
			continue
		}

		backoff = time.Second
		for _, update := range updates {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			_ = s.processTelegramUpdate(ctx, update)
		}
	}
}

func (s *Server) fetchTelegramUpdates(ctx context.Context, client *http.Client, token string, offset int64) ([]telegramInboundUpdate, error) {
	payload := map[string]any{
		"offset":          offset,
		"timeout":         25,
		"allowed_updates": []string{"message", "edited_message", "channel_post"},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("%s/bot%s/getUpdates", channels.TelegramAPIBase(), token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("telegram getUpdates HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed telegramPollEnvelope
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("telegram getUpdates invalid response: %w", err)
	}
	if !parsed.OK {
		if strings.TrimSpace(parsed.Description) != "" {
			return nil, errors.New(strings.TrimSpace(parsed.Description))
		}
		return nil, fmt.Errorf("telegram getUpdates failed with code %d", parsed.ErrorCode)
	}
	return parsed.Result, nil
}

func (s *Server) processTelegramUpdate(ctx context.Context, update telegramInboundUpdate) error {
	message := update.pickMessage()
	if message == nil || message.From.IsBot {
		return nil
	}

	text := strings.TrimSpace(message.Text)
	if text == "" {
		text = strings.TrimSpace(message.Caption)
	}
	if text == "" {
		return nil
	}

	target := strconv.FormatInt(message.Chat.ID, 10)
	sessionID := fmt.Sprintf("tg-chat-%d", message.Chat.ID)

	if strings.HasPrefix(text, "/") {
		job := scheduler.Job{
			Method:    "send",
			SessionID: sessionID,
			Params: map[string]any{
				"channel": "telegram",
				"to":      target,
				"message": text,
			},
		}
		result, handled, err := s.handleTelegramCommand(job, text)
		if !handled {
			return nil
		}
		if err != nil {
			_, sendErr := s.channels.Send(ctx, channels.SendRequest{
				Channel:   "telegram",
				To:        target,
				Message:   "Command failed: " + conciseError(err, 260),
				SessionID: sessionID,
			})
			return sendErr
		}
		replyMessage := telegramReplyMessageFromCommandResult(result)
		if strings.TrimSpace(replyMessage) == "" {
			return nil
		}
		_, err = s.channels.Send(ctx, channels.SendRequest{
			Channel:   "telegram",
			To:        target,
			Message:   replyMessage,
			SessionID: sessionID,
		})
		return err
	}

	s.recordChannelMemory(sessionID, "telegram", "telegram.inbound", "user", text, map[string]any{
		"target": target,
		"source": "telegram-polling",
	})

	stopTyping := s.startTelegramTypingLoop(ctx, target)
	defer stopTyping()

	replyText, replyMeta, err := s.buildTelegramAssistantReply(ctx, target, sessionID, text)
	if err != nil {
		replyText = "I hit an upstream error while generating a response: " + conciseError(err, 240)
		replyMeta = map[string]any{
			"source": "telegram-polling",
			"error":  conciseError(err, 400),
		}
	}

	receipt, sendErr := s.sendTelegramReply(ctx, target, sessionID, replyText)
	if sendErr != nil {
		return sendErr
	}
	meta := map[string]any{
		"reply":   replyMeta,
		"receipt": receipt.Metadata,
		"target":  target,
		"source":  "telegram-polling",
	}
	s.recordChannelMemory(sessionID, "telegram", "telegram.reply", "assistant", replyText, meta)
	return nil
}

func (s *Server) buildTelegramAssistantReply(ctx context.Context, target string, sessionID string, userMessage string) (string, map[string]any, error) {
	provider, model := s.compat.getTelegramModelSelection(target)
	if strings.TrimSpace(provider) == "" {
		provider = "chatgpt"
	}
	if strings.TrimSpace(model) == "" {
		model = "gpt-5.2"
	}

	timeout := time.Duration(s.cfg.Runtime.BrowserBridge.RequestTimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	invokeCtx, cancel := context.WithTimeout(ctx, timeout+10*time.Second)
	defer cancel()

	invokeResult, err := s.tools.Invoke(invokeCtx, toolruntime.Request{
		Tool:      "browser.request",
		SessionID: sessionID,
		Input: map[string]any{
			"provider": provider,
			"model":    model,
			"messages": []map[string]any{
				{"role": "user", "content": userMessage},
			},
		},
	})
	if err != nil {
		return "", map[string]any{
			"provider": provider,
			"model":    model,
		}, err
	}

	assistant := extractAssistantTextFromInvokeOutput(invokeResult.Output)
	if strings.TrimSpace(assistant) == "" {
		return "", map[string]any{
			"provider":     provider,
			"model":        model,
			"toolProvider": invokeResult.Provider,
		}, errors.New("assistant response was empty")
	}

	return assistant, map[string]any{
		"provider":     provider,
		"model":        model,
		"toolProvider": invokeResult.Provider,
	}, nil
}

func (s *Server) startTelegramTypingLoop(ctx context.Context, target string) context.CancelFunc {
	if !s.cfg.Runtime.TelegramTypingIndicators {
		return func() {}
	}
	loopCtx, cancel := context.WithCancel(ctx)
	_ = s.channels.SendTyping(loopCtx, "telegram", target)

	interval := time.Duration(s.cfg.Runtime.TelegramTypingIntervalMs) * time.Millisecond
	if interval < time.Second {
		interval = time.Second
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-loopCtx.Done():
				return
			case <-ticker.C:
				_ = s.channels.SendTyping(loopCtx, "telegram", target)
			}
		}
	}()
	return cancel
}

func (s *Server) sendTelegramReply(ctx context.Context, target string, sessionID string, message string) (channels.SendReceipt, error) {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return channels.SendReceipt{}, errors.New("telegram reply is empty")
	}

	if !s.cfg.Runtime.TelegramLiveStreaming {
		return s.channels.Send(ctx, channels.SendRequest{
			Channel:   "telegram",
			To:        target,
			Message:   trimmed,
			SessionID: sessionID,
		})
	}

	chunks := splitTelegramStreamingChunks(trimmed, s.cfg.Runtime.TelegramStreamChunkChars)
	if len(chunks) <= 1 {
		return s.channels.Send(ctx, channels.SendRequest{
			Channel:   "telegram",
			To:        target,
			Message:   trimmed,
			SessionID: sessionID,
		})
	}

	delay := time.Duration(s.cfg.Runtime.TelegramStreamChunkDelayMs) * time.Millisecond
	if delay < 0 {
		delay = 0
	}

	var first channels.SendReceipt
	last := channels.SendReceipt{}
	streamMessageIDs := make([]any, 0, len(chunks))
	for idx, chunk := range chunks {
		if idx > 0 && delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return channels.SendReceipt{}, ctx.Err()
			case <-timer.C:
			}
		}
		if s.cfg.Runtime.TelegramTypingIndicators {
			_ = s.channels.SendTyping(ctx, "telegram", target)
		}
		receipt, err := s.channels.Send(ctx, channels.SendRequest{
			Channel:   "telegram",
			To:        target,
			Message:   chunk,
			SessionID: sessionID,
		})
		if err != nil {
			return channels.SendReceipt{}, err
		}
		if idx == 0 {
			first = receipt
		}
		last = receipt
		if receipt.Metadata != nil {
			if messageID, ok := receipt.Metadata["messageId"]; ok {
				streamMessageIDs = append(streamMessageIDs, messageID)
			}
		}
	}

	last.Message = trimmed
	if last.Metadata == nil {
		last.Metadata = map[string]any{}
	}
	last.Metadata["streaming"] = true
	last.Metadata["streamChunkCount"] = len(chunks)
	last.Metadata["streamChunkChars"] = s.cfg.Runtime.TelegramStreamChunkChars
	last.Metadata["streamMessageIds"] = streamMessageIDs
	if first.ID != "" {
		last.ID = first.ID
		last.CreatedAt = first.CreatedAt
	}
	return last, nil
}

func splitTelegramStreamingChunks(message string, maxRunes int) []string {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return []string{}
	}
	if maxRunes <= 0 {
		maxRunes = 700
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
			minSplit := start + (maxRunes / 2)
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

func telegramReplyMessageFromCommandResult(result map[string]any) string {
	if result == nil {
		return ""
	}
	switch payload := result["result"].(type) {
	case channels.SendReceipt:
		return strings.TrimSpace(payload.Message)
	case *channels.SendReceipt:
		if payload == nil {
			return ""
		}
		return strings.TrimSpace(payload.Message)
	case map[string]any:
		return strings.TrimSpace(toString(payload["message"], ""))
	default:
		return ""
	}
}

func extractAssistantTextFromInvokeOutput(raw any) string {
	output, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	if assistant := strings.TrimSpace(toString(output["assistantText"], "")); assistant != "" {
		return assistant
	}
	response, ok := output["response"].(map[string]any)
	if !ok {
		return ""
	}
	choices, ok := response["choices"].([]any)
	if !ok || len(choices) == 0 {
		return strings.TrimSpace(toString(response["text"], ""))
	}
	first, ok := choices[0].(map[string]any)
	if !ok {
		return ""
	}
	if message, ok := first["message"].(map[string]any); ok {
		if content := strings.TrimSpace(toString(message["content"], "")); content != "" {
			return content
		}
	}
	return strings.TrimSpace(toString(first["text"], ""))
}

func conciseError(err error, limit int) string {
	if err == nil {
		return ""
	}
	text := strings.TrimSpace(err.Error())
	if limit > 0 && len(text) > limit {
		return text[:limit] + "..."
	}
	return text
}
