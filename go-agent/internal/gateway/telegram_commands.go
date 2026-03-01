package gateway

import (
	"context"
	"fmt"
	"net/http"
	neturl "net/url"
	"strconv"
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
	case "set":
		reply, err = s.handleTelegramSetCommand(target, parts[1:])
	case "tts":
		reply, err = s.handleTelegramTTSCommand(target, command, parts[1:])
	default:
		reply = telegramCommandReceipt(target, fmt.Sprintf("Unknown command `%s`. Supported: /model, /auth, /set, /tts", root), map[string]any{
			"type":      "unknown",
			"command":   root,
			"supported": []string{"model", "auth", "set", "tts"},
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

func (s *Server) handleTelegramSetCommand(target string, args []string) (channels.SendReceipt, error) {
	if len(args) < 4 || !strings.EqualFold(args[0], "api") || !strings.EqualFold(args[1], "key") {
		return telegramCommandReceipt(target, "Usage: `/set api key <provider> <key>`", map[string]any{
			"type":   "set.invalid",
			"target": target,
		}), nil
	}
	provider := normalizeProviderAlias(args[2])
	apiKey := strings.TrimSpace(strings.Join(args[3:], " "))
	if provider == "" || apiKey == "" {
		return telegramCommandReceipt(target, "Usage: `/set api key <provider> <key>`", map[string]any{
			"type":   "set.invalid",
			"target": target,
			"error":  "missing_provider_or_key",
		}), nil
	}
	if strings.Contains(apiKey, "\n") || strings.Contains(apiKey, "\r") {
		return telegramCommandReceipt(target, "API key must be a single line.", map[string]any{
			"type":     "set.api_key",
			"target":   target,
			"provider": provider,
			"error":    "invalid_key_format",
		}), nil
	}
	if !s.compat.setProviderAPIKey(provider, apiKey) {
		return telegramCommandReceipt(target, "Failed to store API key.", map[string]any{
			"type":     "set.api_key",
			"target":   target,
			"provider": provider,
			"error":    "store_failed",
		}), nil
	}
	return telegramCommandReceipt(target, fmt.Sprintf("Provider API key saved for `%s`. You can now set a model with `/model %s/<model>`.", provider, provider), map[string]any{
		"type":      "set.api_key",
		"target":    target,
		"provider":  provider,
		"stored":    true,
		"keyMasked": maskSecret(apiKey),
	}), nil
}

func (s *Server) handleTelegramModelCommand(target string, args []string) (channels.SendReceipt, error) {
	currentProvider, currentModel := s.compat.getTelegramModelSelection(target)
	descriptors := s.compat.listModelDescriptors()
	availableProviders := s.compat.listModelProviders()

	if len(args) == 0 || strings.EqualFold(args[0], "status") {
		available := s.compat.listModelIDs()
		return telegramCommandReceipt(target, fmt.Sprintf("Current model: `%s/%s`\nAvailable providers: %s", currentProvider, currentModel, strings.Join(availableProviders, ", ")), map[string]any{
			"type":            "model.status",
			"currentModel":    currentModel,
			"currentProvider": currentProvider,
			"modelRef":        fmt.Sprintf("%s/%s", currentProvider, currentModel),
			"availableModels": available,
			"providers":       availableProviders,
			"models":          descriptors,
		}), nil
	}

	action := strings.ToLower(strings.TrimSpace(args[0]))
	switch action {
	case "list":
		if len(args) < 2 {
			return telegramCommandReceipt(target, fmt.Sprintf("Providers: %s\nUse `/model list <provider>` for full model IDs.", strings.Join(availableProviders, ", ")), map[string]any{
				"type":            "model.list",
				"providers":       availableProviders,
				"availableModels": s.compat.listModelIDs(),
				"models":          descriptors,
			}), nil
		}
		requestedProvider := normalizeProviderAlias(args[1])
		filteredDescriptors := filterModelDescriptorsByProvider(descriptors, requestedProvider)
		filteredIDs := descriptorIDs(filteredDescriptors)
		if len(filteredDescriptors) == 0 {
			return telegramCommandReceipt(target, fmt.Sprintf("No models found for provider `%s`.", requestedProvider), map[string]any{
				"type":              "model.list",
				"requestedProvider": requestedProvider,
				"providers":         availableProviders,
				"availableModels":   []string{},
				"models":            []map[string]any{},
			}), nil
		}
		return telegramCommandReceipt(target, fmt.Sprintf("Models for `%s`: %s", requestedProvider, strings.Join(filteredIDs, ", ")), map[string]any{
			"type":              "model.list",
			"requestedProvider": requestedProvider,
			"providers":         availableProviders,
			"availableModels":   filteredIDs,
			"models":            filteredDescriptors,
		}), nil
	case "next":
		selected := s.compat.nextTelegramModel(target)
		selectedProvider := s.compat.getTelegramModelProvider(target)
		return telegramCommandReceipt(target, fmt.Sprintf("Model advanced to `%s/%s` for `%s`.", selectedProvider, selected, target), map[string]any{
			"type":            "model.next",
			"provider":        selectedProvider,
			"currentProvider": selectedProvider,
			"currentModel":    selected,
			"modelRef":        fmt.Sprintf("%s/%s", selectedProvider, selected),
			"target":          target,
		}), nil
	case "reset":
		selectedProvider, selected := s.compat.setTelegramModelSelection(target, "chatgpt", "gpt-5.2")
		return telegramCommandReceipt(target, fmt.Sprintf("Model reset to `%s/%s` for `%s`.", selectedProvider, selected, target), map[string]any{
			"type":            "model.reset",
			"provider":        selectedProvider,
			"currentProvider": selectedProvider,
			"currentModel":    selected,
			"modelRef":        fmt.Sprintf("%s/%s", selectedProvider, selected),
			"target":          target,
		}), nil
	}

	requestedProvider, requestedModel, providerScoped := parseProviderScopedModelArgs(args)
	if providerScoped {
		if requestedProvider == "" {
			return telegramCommandReceipt(target, "Provider is required. Usage: `/model <provider>/<model>` or `/model <provider> <model>`.", map[string]any{
				"type":   "model.invalid",
				"target": target,
				"error":  "missing_provider",
			}), nil
		}
		if requestedModel == "" {
			defaultModel, ok := s.compat.defaultModelForProvider(requestedProvider)
			if !ok {
				return telegramCommandReceipt(target, fmt.Sprintf("Provider `%s` has no catalog models. Run `/model list` first.", requestedProvider), map[string]any{
					"type":              "model.invalid",
					"target":            target,
					"requestedProvider": requestedProvider,
					"error":             "missing_provider_model",
				}), nil
			}
			selectedProvider, selectedModel := s.compat.setTelegramModelSelection(target, requestedProvider, defaultModel)
			return telegramCommandReceipt(target, fmt.Sprintf("Model set to `%s/%s` for `%s`.", selectedProvider, selectedModel, target), map[string]any{
				"type":                "model.set",
				"target":              target,
				"requestedProvider":   requestedProvider,
				"currentProvider":     selectedProvider,
				"currentModel":        selectedModel,
				"modelRef":            fmt.Sprintf("%s/%s", selectedProvider, selectedModel),
				"matchedCatalogModel": true,
			}), nil
		}
		selectedModel, matchedCatalogModel, aliasUsed := resolveModelForProvider(descriptors, requestedProvider, requestedModel)
		if !matchedCatalogModel && !s.compat.hasModelProvider(requestedProvider) {
			return telegramCommandReceipt(target, fmt.Sprintf("Unknown provider `%s`. Available providers: %s", requestedProvider, strings.Join(availableProviders, ", ")), map[string]any{
				"type":              "model.invalid",
				"target":            target,
				"requestedProvider": requestedProvider,
				"providers":         availableProviders,
			}), nil
		}
		selectedProvider, selected := s.compat.setTelegramModelSelection(target, requestedProvider, selectedModel)
		message := fmt.Sprintf("Model set to `%s/%s` for `%s`.", selectedProvider, selected, target)
		if !matchedCatalogModel {
			message += "\nNote: custom model override applied (not found in catalog)."
		}
		return telegramCommandReceipt(target, message, map[string]any{
			"type":                "model.set",
			"target":              target,
			"requestedProvider":   requestedProvider,
			"requestedModel":      requestedModel,
			"requested":           strings.TrimSpace(strings.Join(args, " ")),
			"aliasUsed":           aliasUsed,
			"currentProvider":     selectedProvider,
			"currentModel":        selected,
			"modelRef":            fmt.Sprintf("%s/%s", selectedProvider, selected),
			"matchedCatalogModel": matchedCatalogModel,
			"customOverride":      !matchedCatalogModel,
		}), nil
	}

	resolvedModel, aliasUsed, ok := s.compat.resolveModelChoice(action)
	if !ok {
		if defaultModel, providerMatch := s.compat.defaultModelForProvider(action); providerMatch {
			selectedProvider, selectedModel := s.compat.setTelegramModelSelection(target, action, defaultModel)
			return telegramCommandReceipt(target, fmt.Sprintf("Model set to `%s/%s` for `%s`.", selectedProvider, selectedModel, target), map[string]any{
				"type":                "model.set",
				"target":              target,
				"requestedProvider":   selectedProvider,
				"currentProvider":     selectedProvider,
				"currentModel":        selectedModel,
				"modelRef":            fmt.Sprintf("%s/%s", selectedProvider, selectedModel),
				"matchedCatalogModel": true,
			}), nil
		}
		available := s.compat.listModelIDs()
		return telegramCommandReceipt(target, fmt.Sprintf("Unknown model `%s`. Available: %s", action, strings.Join(available, ", ")), map[string]any{
			"type":            "model.invalid",
			"requestedModel":  action,
			"availableModels": available,
			"providers":       availableProviders,
		}), nil
	}
	resolvedProvider := s.compat.providerForModel(resolvedModel)
	if resolvedProvider == "" {
		resolvedProvider = currentProvider
	}
	selectedProvider, selectedModel := s.compat.setTelegramModelSelection(target, resolvedProvider, resolvedModel)
	return telegramCommandReceipt(target, fmt.Sprintf("Model set to `%s/%s` for `%s`.", selectedProvider, selectedModel, target), map[string]any{
		"type":            "model.set",
		"requested":       action,
		"aliasUsed":       aliasUsed,
		"provider":        selectedProvider,
		"currentProvider": selectedProvider,
		"currentModel":    selectedModel,
		"modelRef":        fmt.Sprintf("%s/%s", selectedProvider, selectedModel),
		"target":          target,
	}), nil
}

func parseProviderScopedModelArgs(args []string) (string, string, bool) {
	if len(args) == 0 {
		return "", "", false
	}
	first := strings.TrimSpace(args[0])
	if first == "" {
		return "", "", false
	}
	if providerRaw, modelRaw, ok := strings.Cut(first, "/"); ok {
		model := strings.TrimSpace(modelRaw)
		if len(args) > 1 {
			model = strings.TrimSpace(model + " " + strings.Join(args[1:], " "))
		}
		return normalizeProviderAlias(providerRaw), model, true
	}
	if len(args) >= 2 {
		model := strings.TrimSpace(strings.Join(args[1:], " "))
		return normalizeProviderAlias(first), model, true
	}
	return "", "", false
}

func filterModelDescriptorsByProvider(descriptors []map[string]any, provider string) []map[string]any {
	normalizedProvider := normalizeProviderAlias(provider)
	if normalizedProvider == "" {
		return cloneMapList(descriptors)
	}
	out := make([]map[string]any, 0, len(descriptors))
	for _, descriptor := range descriptors {
		descriptorProvider := normalizeProviderAlias(toString(descriptor["provider"], ""))
		if descriptorProvider != normalizedProvider {
			continue
		}
		out = append(out, cloneMap(descriptor))
	}
	return out
}

func descriptorIDs(descriptors []map[string]any) []string {
	out := make([]string, 0, len(descriptors))
	for _, descriptor := range descriptors {
		id := strings.ToLower(strings.TrimSpace(toString(descriptor["id"], "")))
		if id == "" {
			continue
		}
		out = append(out, id)
	}
	return out
}

func resolveModelForProvider(descriptors []map[string]any, provider string, requestedModel string) (string, bool, string) {
	normalizedProvider := normalizeProviderAlias(provider)
	normalizedRequested := normalizeModelAlias(requestedModel)
	if normalizedRequested == "" {
		return "", false, ""
	}
	for _, descriptor := range descriptors {
		descriptorProvider := normalizeProviderAlias(toString(descriptor["provider"], ""))
		if descriptorProvider != normalizedProvider {
			continue
		}
		modelID := strings.ToLower(strings.TrimSpace(toString(descriptor["id"], "")))
		if modelID == "" {
			continue
		}
		if normalizeModelAlias(modelID) == normalizedRequested {
			return modelID, true, ""
		}
		mode := normalizeModelAlias(toString(descriptor["mode"], ""))
		if mode != "" && mode == normalizedRequested {
			return modelID, true, mode
		}
		name := normalizeModelAlias(toString(descriptor["name"], ""))
		if name != "" && name == normalizedRequested {
			return modelID, true, name
		}
		for _, alias := range descriptorAliases(descriptor["aliases"]) {
			if normalizeModelAlias(alias) == normalizedRequested {
				return modelID, true, normalizeModelAlias(alias)
			}
		}
	}
	return strings.ToLower(strings.TrimSpace(requestedModel)), false, ""
}

func descriptorAliases(raw any) []string {
	switch values := raw.(type) {
	case []string:
		out := make([]string, 0, len(values))
		for _, entry := range values {
			if normalized := strings.TrimSpace(entry); normalized != "" {
				out = append(out, normalized)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(values))
		for _, entry := range values {
			if text, ok := entry.(string); ok {
				if normalized := strings.TrimSpace(text); normalized != "" {
					out = append(out, normalized)
				}
			}
		}
		return out
	default:
		return []string{}
	}
}

func (s *Server) handleTelegramAuthCommand(target string, args []string) (channels.SendReceipt, error) {
	action := "start"
	if len(args) > 0 {
		action = strings.ToLower(strings.TrimSpace(args[0]))
	}

	switch action {
	case "help":
		return telegramCommandReceipt(target, strings.Join([]string{
			"Auth command usage:",
			"`/auth providers`",
			"`/auth status`",
			"`/auth bridge`",
			"`/auth` (start)",
			"`/auth wait [session_id] [--timeout <seconds>]`",
			"`/auth complete <CODE> [session_id]`",
			"`/auth cancel`",
		}, "\n"), map[string]any{
			"type":   "auth.help",
			"target": target,
		}), nil
	case "providers":
		providers := make([]map[string]any, 0, 8)
		for _, provider := range knownAuthProviders() {
			providers = append(providers, map[string]any{
				"id":                     provider,
				"supportsBrowserSession": strings.EqualFold(provider, "chatgpt") || strings.EqualFold(provider, "codex"),
				"apiKeyConfigured":       s.compat.hasProviderAPIKey(provider),
			})
		}
		return telegramCommandReceipt(target, formatAuthProvidersMessage(providers), map[string]any{
			"type":      "auth.providers",
			"target":    target,
			"providers": providers,
		}), nil
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

		modelProvider, model := s.compat.getTelegramModelSelection(target)
		login := s.webLogin.Start(webbridge.StartOptions{
			Provider: modelProvider,
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
			"provider":                modelProvider,
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
	case "wait":
		defaultLoginID := s.compat.getTelegramAuth(target)
		loginID, waitTimeout, waitParseErr := parseAuthWaitArgs(args[1:], defaultLoginID)
		if waitParseErr != "" {
			return telegramCommandReceipt(target, waitParseErr, map[string]any{
				"type":   "auth.wait",
				"target": target,
				"error":  "invalid_wait_args",
			}), nil
		}
		login, err := s.webLogin.Wait(context.Background(), loginID, waitTimeout)
		if err != nil {
			return telegramCommandReceipt(target, fmt.Sprintf("Auth wait failed: %s", err.Error()), map[string]any{
				"type":           "auth.wait",
				"target":         target,
				"loginSessionId": loginID,
				"timeoutSeconds": int(waitTimeout.Seconds()),
				"error":          err.Error(),
			}), nil
		}
		return telegramCommandReceipt(target, fmt.Sprintf("Auth wait result: `%s` (session `%s`).", login.Status, login.ID), map[string]any{
			"type":             "auth.wait",
			"target":           target,
			"login":            login,
			"expiresInSeconds": authExpiresInSeconds(login.ExpiresAt),
			"timeoutSeconds":   int(waitTimeout.Seconds()),
		}), nil
	case "bridge":
		bridgeStatus := probeBrowserBridge(s.cfg.Runtime.BrowserBridge.Enabled, s.cfg.Runtime.BrowserBridge.Endpoint, 2*time.Second)
		message := fmt.Sprintf("Bridge `%s` (%s).", toString(bridgeStatus["status"], "unknown"), toString(bridgeStatus["endpoint"], ""))
		if probeErr := toString(bridgeStatus["error"], ""); probeErr != "" {
			message += fmt.Sprintf("\nProbe error: %s", probeErr)
		}
		return telegramCommandReceipt(target, message, map[string]any{
			"type":   "auth.bridge",
			"target": target,
			"bridge": bridgeStatus,
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
		if len(args) >= 3 && strings.TrimSpace(args[2]) != "" {
			loginID = strings.TrimSpace(args[2])
		}
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
		code := extractAuthCode(args[1])
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
		return telegramCommandReceipt(target, "Unknown `/auth` action. Use `/auth help` for full usage.", map[string]any{
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
	case "providers":
		providers := []map[string]any{
			{"id": "native", "name": "Native Bridge", "enabled": true},
			{"id": "elevenlabs", "name": "ElevenLabs", "enabled": false},
			{"id": "openai-voice", "name": "OpenAI Voice", "enabled": true},
			{"id": "edge", "name": "Edge TTS", "enabled": true},
		}
		lines := make([]string, 0, len(providers))
		for _, provider := range providers {
			lines = append(lines, fmt.Sprintf("%s (%t)", toString(provider["id"], ""), toBool(provider["enabled"], false)))
		}
		return telegramCommandReceipt(target, fmt.Sprintf("TTS providers: %s", strings.Join(lines, ", ")), map[string]any{
			"type":      "tts.providers",
			"target":    target,
			"providers": providers,
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
	case "help":
		return telegramCommandReceipt(target, strings.Join([]string{
			"TTS command usage:",
			"`/tts status`",
			"`/tts providers`",
			"`/tts provider <name>`",
			"`/tts on`",
			"`/tts off`",
			"`/tts say <text>`",
		}, "\n"), map[string]any{
			"type":   "tts.help",
			"target": target,
		}), nil
	default:
		return telegramCommandReceipt(target, "Unknown `/tts` action. Use `/tts status|providers|on|off|provider|say|help`.", map[string]any{
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

func parseAuthWaitArgs(args []string, defaultLoginID string) (string, time.Duration, string) {
	loginID := strings.TrimSpace(defaultLoginID)
	timeout := 30 * time.Second

	for idx := 0; idx < len(args); idx++ {
		token := strings.TrimSpace(args[idx])
		if token == "" {
			continue
		}
		lowered := strings.ToLower(token)
		switch {
		case lowered == "session":
			if idx+1 >= len(args) {
				return "", 0, "Usage: `/auth wait [session <ID>|<ID>] [--timeout <seconds>]`"
			}
			idx++
			loginID = strings.TrimSpace(args[idx])
		case lowered == "--timeout":
			if idx+1 >= len(args) {
				return "", 0, "Missing timeout value. Example: `/auth wait --timeout 90`"
			}
			idx++
			seconds, err := strconv.Atoi(strings.TrimSpace(args[idx]))
			if err != nil || seconds < 1 || seconds > 900 {
				return "", 0, "Timeout must be an integer between 1 and 900 seconds."
			}
			timeout = time.Duration(seconds) * time.Second
		case strings.HasPrefix(lowered, "--timeout="):
			raw := strings.TrimSpace(strings.TrimPrefix(lowered, "--timeout="))
			seconds, err := strconv.Atoi(raw)
			if err != nil || seconds < 1 || seconds > 900 {
				return "", 0, "Timeout must be an integer between 1 and 900 seconds."
			}
			timeout = time.Duration(seconds) * time.Second
		case strings.HasPrefix(lowered, "--"):
			return "", 0, fmt.Sprintf("Unknown wait option `%s`.", token)
		default:
			loginID = token
		}
	}

	if loginID == "" {
		return "", 0, "No auth session selected. Start auth first with `/auth`."
	}
	return loginID, timeout, ""
}

func extractAuthCode(input string) string {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		return raw
	}
	parsed, err := neturl.Parse(raw)
	if err != nil {
		return raw
	}
	query := parsed.Query()
	for _, key := range []string{"openclaw_code", "code", "device_code"} {
		if value := strings.TrimSpace(query.Get(key)); value != "" {
			return value
		}
	}
	return raw
}

func knownAuthProviders() []string {
	return []string{"chatgpt", "codex", "openrouter", "kimi", "qwen"}
}

func formatAuthProvidersMessage(providers []map[string]any) string {
	if len(providers) == 0 {
		return "No auth providers configured."
	}
	lines := make([]string, 0, len(providers))
	for _, provider := range providers {
		name := toString(provider["id"], "unknown")
		supportsBrowser := toBool(provider["supportsBrowserSession"], false)
		keyConfigured := toBool(provider["apiKeyConfigured"], false)
		lines = append(lines, fmt.Sprintf("%s (browser:%t, apiKey:%t)", name, supportsBrowser, keyConfigured))
	}
	return "Auth providers: " + strings.Join(lines, ", ")
}

func probeBrowserBridge(enabled bool, endpoint string, timeout time.Duration) map[string]any {
	normalizedEndpoint := strings.TrimSpace(endpoint)
	if normalizedEndpoint == "" {
		return map[string]any{
			"enabled":   enabled,
			"endpoint":  normalizedEndpoint,
			"status":    "missing-endpoint",
			"reachable": false,
		}
	}
	if !enabled {
		return map[string]any{
			"enabled":   enabled,
			"endpoint":  normalizedEndpoint,
			"status":    "disabled",
			"reachable": false,
		}
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(strings.TrimRight(normalizedEndpoint, "/") + "/health")
	if err != nil {
		return map[string]any{
			"enabled":   enabled,
			"endpoint":  normalizedEndpoint,
			"status":    "unreachable",
			"reachable": false,
			"error":     err.Error(),
		}
	}
	defer resp.Body.Close()
	reachable := resp.StatusCode >= 200 && resp.StatusCode < 500
	status := "reachable"
	if !reachable {
		status = "unhealthy"
	}
	return map[string]any{
		"enabled":    enabled,
		"endpoint":   normalizedEndpoint,
		"status":     status,
		"reachable":  reachable,
		"httpStatus": resp.StatusCode,
	}
}

func maskSecret(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) <= 6 {
		return strings.Repeat("*", len(runes))
	}
	return string(runes[:3]) + strings.Repeat("*", len(runes)-6) + string(runes[len(runes)-3:])
}
