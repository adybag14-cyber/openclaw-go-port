package gateway

import (
	"fmt"
	"strings"
	"time"

	webbridge "github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/bridge/web"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/channels"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/scheduler"
)

func (s *Server) handleTelegramCommand(job scheduler.Job, message string) (map[string]any, bool, error) {
	trimmed := strings.TrimSpace(message)
	if !strings.HasPrefix(trimmed, "/") {
		return nil, false, nil
	}
	target := strings.TrimSpace(toString(job.Params["to"], s.cfg.Channels.Telegram.DefaultTarget))
	if target == "" {
		target = "default"
	}

	command := strings.TrimPrefix(trimmed, "/")
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return nil, false, nil
	}
	root := strings.ToLower(parts[0])

	var (
		reply channels.SendReceipt
		err   error
	)
	switch root {
	case "model":
		reply, err = s.handleTelegramModelCommand(target, parts[1:])
	case "auth":
		reply, err = s.handleTelegramAuthCommand(target, parts[1:])
	case "tts":
		reply, err = s.handleTelegramTTSCommand(target, command, parts[1:])
	default:
		reply = telegramCommandReceipt(target, fmt.Sprintf("Unknown command `%s`. Supported: /model, /auth, /tts", root), map[string]any{
			"type":      "unknown",
			"command":   root,
			"supported": []string{"model", "auth", "tts"},
		})
	}
	if err != nil {
		return nil, true, err
	}
	s.recordMemory(job, "user", message, map[string]any{
		"channel": "telegram",
		"to":      target,
		"command": true,
	})
	s.recordMemory(job, "assistant", reply.Message, map[string]any{
		"channel":  "telegram",
		"to":       target,
		"metadata": reply.Metadata,
	})
	return map[string]any{
		"status": "accepted",
		"method": job.Method,
		"result": reply,
	}, true, nil
}

func (s *Server) handleTelegramModelCommand(target string, args []string) (channels.SendReceipt, error) {
	current := s.compat.getTelegramModel(target)
	descriptors := s.compat.listModelDescriptors()
	if len(args) == 0 || strings.EqualFold(args[0], "list") || strings.EqualFold(args[0], "status") {
		available := s.compat.listModelIDs()
		return telegramCommandReceipt(target, fmt.Sprintf("Current model: `%s`\nAvailable: %s", current, strings.Join(available, ", ")), map[string]any{
			"type":            "model.status",
			"currentModel":    current,
			"availableModels": available,
			"models":          descriptors,
		}), nil
	}

	action := strings.ToLower(strings.TrimSpace(args[0]))
	switch action {
	case "next":
		selected := s.compat.nextTelegramModel(target)
		return telegramCommandReceipt(target, fmt.Sprintf("Model advanced to `%s` for `%s`.", selected, target), map[string]any{
			"type":         "model.next",
			"currentModel": selected,
			"target":       target,
		}), nil
	case "reset":
		selected := s.compat.setTelegramModel(target, "gpt-5.2")
		return telegramCommandReceipt(target, fmt.Sprintf("Model reset to `%s` for `%s`.", selected, target), map[string]any{
			"type":         "model.reset",
			"currentModel": selected,
			"target":       target,
		}), nil
	}

	resolvedModel, aliasUsed, ok := s.compat.resolveModelChoice(action)
	if !ok {
		available := s.compat.listModelIDs()
		return telegramCommandReceipt(target, fmt.Sprintf("Unknown model `%s`. Available: %s", action, strings.Join(available, ", ")), map[string]any{
			"type":            "model.invalid",
			"requestedModel":  action,
			"availableModels": available,
		}), nil
	}
	selected := s.compat.setTelegramModel(target, resolvedModel)
	return telegramCommandReceipt(target, fmt.Sprintf("Model set to `%s` for `%s`.", selected, target), map[string]any{
		"type":         "model.set",
		"requested":    action,
		"aliasUsed":    aliasUsed,
		"currentModel": selected,
		"target":       target,
	}), nil
}

func (s *Server) handleTelegramAuthCommand(target string, args []string) (channels.SendReceipt, error) {
	action := "start"
	if len(args) > 0 {
		action = strings.ToLower(strings.TrimSpace(args[0]))
	}

	switch action {
	case "start":
		if existingID := s.compat.getTelegramAuth(target); existingID != "" {
			if existing, ok := s.webLogin.Get(existingID); ok && existing.Status == webbridge.LoginPending {
				return telegramCommandReceipt(target, fmt.Sprintf("Auth already pending.\nOpen: %s\nThen run: `/auth complete %s`", existing.VerificationURIComplete, existing.Code), map[string]any{
					"type":                    "auth.start",
					"target":                  target,
					"loginSessionId":          existing.ID,
					"code":                    existing.Code,
					"status":                  existing.Status,
					"verificationUri":         existing.VerificationURI,
					"verificationUriComplete": existing.VerificationURIComplete,
					"expiresAt":               existing.ExpiresAt,
				}), nil
			}
		}

		model := s.compat.getTelegramModel(target)
		login := s.webLogin.Start(webbridge.StartOptions{
			Provider: "chatgpt",
			Model:    model,
		})
		s.compat.setTelegramAuth(target, login.ID)
		return telegramCommandReceipt(target, fmt.Sprintf("Auth started.\nOpen: %s\nIf prompted, use code `%s`.\nThen run: `/auth complete %s`", login.VerificationURIComplete, login.Code, login.Code), map[string]any{
			"type":                    "auth.start",
			"target":                  target,
			"loginSessionId":          login.ID,
			"code":                    login.Code,
			"verificationUri":         login.VerificationURI,
			"verificationUriComplete": login.VerificationURIComplete,
			"expiresAt":               login.ExpiresAt,
			"model":                   model,
			"status":                  login.Status,
		}), nil
	case "status":
		loginID := s.compat.getTelegramAuth(target)
		if loginID == "" {
			return telegramCommandReceipt(target, "No active auth flow for this Telegram target.", map[string]any{
				"type":   "auth.status",
				"target": target,
				"status": "none",
			}), nil
		}
		login, ok := s.webLogin.Get(loginID)
		if !ok {
			return telegramCommandReceipt(target, "Auth session expired or missing. Run `/auth` again.", map[string]any{
				"type":           "auth.status",
				"target":         target,
				"status":         "missing",
				"loginSessionId": loginID,
			}), nil
		}
		expiresInSec := authExpiresInSeconds(login.ExpiresAt)
		message := fmt.Sprintf("Auth status: `%s` (session `%s`).", login.Status, login.ID)
		if login.Status == webbridge.LoginPending {
			message += fmt.Sprintf("\nOpen: %s", login.VerificationURIComplete)
			message += fmt.Sprintf("\nThen run: `/auth complete %s`", login.Code)
		}
		return telegramCommandReceipt(target, message, map[string]any{
			"type":             "auth.status",
			"target":           target,
			"login":            login,
			"expiresInSeconds": expiresInSec,
		}), nil
	case "url":
		loginID := s.compat.getTelegramAuth(target)
		if loginID == "" {
			return telegramCommandReceipt(target, "No active auth flow. Run `/auth` first.", map[string]any{
				"type":   "auth.url",
				"target": target,
				"status": "none",
			}), nil
		}
		login, ok := s.webLogin.Get(loginID)
		if !ok {
			s.compat.setTelegramAuth(target, "")
			return telegramCommandReceipt(target, "Auth session expired or missing. Run `/auth` again.", map[string]any{
				"type":           "auth.url",
				"target":         target,
				"status":         "missing",
				"loginSessionId": loginID,
			}), nil
		}
		return telegramCommandReceipt(target, fmt.Sprintf("Auth URL: %s\nCode: `%s`", login.VerificationURIComplete, login.Code), map[string]any{
			"type":                    "auth.url",
			"target":                  target,
			"loginSessionId":          login.ID,
			"verificationUri":         login.VerificationURI,
			"verificationUriComplete": login.VerificationURIComplete,
			"code":                    login.Code,
			"status":                  login.Status,
		}), nil
	case "complete":
		loginID := s.compat.getTelegramAuth(target)
		if loginID == "" {
			return telegramCommandReceipt(target, "No pending auth session. Run `/auth` first.", map[string]any{
				"type":   "auth.complete",
				"target": target,
				"error":  "missing_session",
			}), nil
		}

		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			if login, ok := s.webLogin.Get(loginID); ok && login.Status == webbridge.LoginAuthorized {
				return telegramCommandReceipt(target, fmt.Sprintf("Auth already completed. Session `%s` is `%s`.", login.ID, login.Status), map[string]any{
					"type":   "auth.complete",
					"target": target,
					"login":  login,
				}), nil
			}
			return telegramCommandReceipt(target, "Missing code. Usage: `/auth complete <CODE>`", map[string]any{
				"type":   "auth.complete",
				"target": target,
				"error":  "missing_code",
			}), nil
		}
		code := strings.TrimSpace(args[1])
		login, err := s.webLogin.Complete(loginID, code)
		if err != nil {
			return telegramCommandReceipt(target, fmt.Sprintf("Auth failed: %s", err.Error()), map[string]any{
				"type":           "auth.complete",
				"target":         target,
				"loginSessionId": loginID,
				"error":          err.Error(),
			}), nil
		}
		return telegramCommandReceipt(target, fmt.Sprintf("Auth completed. Session `%s` is `%s`.", login.ID, login.Status), map[string]any{
			"type":   "auth.complete",
			"target": target,
			"login":  login,
		}), nil
	case "cancel", "logout":
		loginID := s.compat.getTelegramAuth(target)
		if loginID == "" {
			return telegramCommandReceipt(target, "No active auth session for this target.", map[string]any{
				"type":   "auth.cancel",
				"target": target,
				"status": "none",
			}), nil
		}
		revoked := s.webLogin.Logout(loginID)
		s.compat.setTelegramAuth(target, "")
		return telegramCommandReceipt(target, fmt.Sprintf("Auth session `%s` cancelled.", loginID), map[string]any{
			"type":           "auth.cancel",
			"target":         target,
			"loginSessionId": loginID,
			"revoked":        revoked,
		}), nil
	default:
		return telegramCommandReceipt(target, "Unknown `/auth` action. Use `/auth`, `/auth status`, `/auth url`, `/auth complete <CODE>`, or `/auth cancel`.", map[string]any{
			"type":   "auth.invalid",
			"target": target,
			"action": action,
		}), nil
	}
}

func (s *Server) handleTelegramTTSCommand(target string, rawCommand string, args []string) (channels.SendReceipt, error) {
	action := "status"
	if len(args) > 0 {
		action = strings.ToLower(strings.TrimSpace(args[0]))
	}

	switch action {
	case "status":
		status := s.handleCompatTTSStatus()
		enabled := toBool(status["enabled"], false)
		provider := toString(status["provider"], "native")
		return telegramCommandReceipt(target, fmt.Sprintf("TTS is `%t` via `%s`.", enabled, provider), map[string]any{
			"type":     "tts.status",
			"target":   target,
			"enabled":  enabled,
			"provider": provider,
		}), nil
	case "on", "enable":
		state := s.handleCompatTTSEnable(true)
		return telegramCommandReceipt(target, fmt.Sprintf("TTS enabled via `%s`.", toString(state["provider"], "native")), map[string]any{
			"type":     "tts.enable",
			"target":   target,
			"enabled":  true,
			"provider": toString(state["provider"], "native"),
		}), nil
	case "off", "disable":
		state := s.handleCompatTTSEnable(false)
		return telegramCommandReceipt(target, fmt.Sprintf("TTS disabled (provider `%s`).", toString(state["provider"], "native")), map[string]any{
			"type":     "tts.disable",
			"target":   target,
			"enabled":  false,
			"provider": toString(state["provider"], "native"),
		}), nil
	case "provider":
		if len(args) < 2 {
			return telegramCommandReceipt(target, "Missing provider. Usage: `/tts provider <NAME>`", map[string]any{
				"type":   "tts.provider",
				"target": target,
				"error":  "missing_provider",
			}), nil
		}
		state, derr := s.handleCompatSetTTSProvider(map[string]any{
			"provider": args[1],
		})
		if derr != nil {
			return telegramCommandReceipt(target, fmt.Sprintf("Failed to set provider: %s", derr.Message), map[string]any{
				"type":   "tts.provider",
				"target": target,
				"error":  derr.Message,
			}), nil
		}
		return telegramCommandReceipt(target, fmt.Sprintf("TTS provider set to `%s`.", toString(state["provider"], "native")), map[string]any{
			"type":     "tts.provider",
			"target":   target,
			"provider": toString(state["provider"], "native"),
			"enabled":  toBool(state["enabled"], true),
		}), nil
	case "say":
		text := extractTTSSayText(rawCommand)
		if text == "" {
			return telegramCommandReceipt(target, "Missing text. Usage: `/tts say <text>`", map[string]any{
				"type":   "tts.say",
				"target": target,
				"error":  "missing_text",
			}), nil
		}
		converted := s.handleCompatTTSConvert(map[string]any{
			"text": text,
		})
		return telegramCommandReceipt(target, fmt.Sprintf("TTS synthesized `%d` bytes.", toInt(converted["bytes"], 0)), map[string]any{
			"type":     "tts.say",
			"target":   target,
			"text":     text,
			"audioRef": toString(converted["audioRef"], ""),
			"bytes":    toInt(converted["bytes"], 0),
			"provider": toString(converted["provider"], "native"),
		}), nil
	default:
		return telegramCommandReceipt(target, "Unknown `/tts` action. Use `/tts status|on|off|provider|say`.", map[string]any{
			"type":   "tts.invalid",
			"target": target,
			"action": action,
		}), nil
	}
}

func extractTTSSayText(rawCommand string) string {
	normalized := strings.TrimSpace(rawCommand)
	if normalized == "" {
		return ""
	}
	idx := strings.Index(strings.ToLower(normalized), "tts")
	if idx == -1 {
		return ""
	}
	after := strings.TrimSpace(normalized[idx+3:])
	after = strings.TrimPrefix(after, " ")
	if !strings.HasPrefix(strings.ToLower(after), "say") {
		return ""
	}
	return strings.TrimSpace(after[3:])
}

func telegramCommandReceipt(target string, message string, metadata map[string]any) channels.SendReceipt {
	return channels.SendReceipt{
		ID:        fmt.Sprintf("tgcmd-%d", time.Now().UTC().UnixNano()),
		Provider:  "telegram",
		Channel:   "telegram",
		To:        target,
		Message:   message,
		Status:    "command",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Metadata:  metadata,
	}
}

func authExpiresInSeconds(expiresAt string) int {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(expiresAt))
	if err != nil {
		return 0
	}
	remaining := time.Until(parsed).Seconds()
	if remaining <= 0 {
		return 0
	}
	return int(remaining)
}
