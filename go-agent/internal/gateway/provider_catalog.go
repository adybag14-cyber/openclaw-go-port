package gateway

import "strings"

type oauthProviderCatalogEntry struct {
	ID                     string
	DisplayName            string
	Aliases                []string
	VerificationURL        string
	SupportsBrowserSession bool
}

var oauthProviderCatalog = []oauthProviderCatalogEntry{
	{
		ID:                     "chatgpt",
		DisplayName:            "ChatGPT",
		Aliases:                []string{"openai", "openai-chatgpt", "chatgpt-web", "chatgpt.com"},
		VerificationURL:        "https://chatgpt.com/",
		SupportsBrowserSession: true,
	},
	{
		ID:                     "codex",
		DisplayName:            "Codex",
		Aliases:                []string{"openai-codex", "codex-cli", "openai-codex-cli"},
		VerificationURL:        "https://chatgpt.com/",
		SupportsBrowserSession: true,
	},
	{
		ID:                     "claude",
		DisplayName:            "Claude",
		Aliases:                []string{"anthropic", "claude-cli", "claude-code", "claude-desktop"},
		VerificationURL:        "https://claude.ai/",
		SupportsBrowserSession: false,
	},
	{
		ID:                     "gemini",
		DisplayName:            "Gemini",
		Aliases:                []string{"google", "google-gemini", "google-gemini-cli", "gemini-cli"},
		VerificationURL:        "https://aistudio.google.com/",
		SupportsBrowserSession: false,
	},
	{
		ID:                     "qwen",
		DisplayName:            "Qwen",
		Aliases:                []string{"qwen-portal", "qwen-cli", "qwen-chat", "qwen35", "qwen3.5", "qwen-3.5", "copaw", "qwen-copaw", "qwen-agent"},
		VerificationURL:        "https://chat.qwen.ai/",
		SupportsBrowserSession: true,
	},
	{
		ID:                     "minimax",
		DisplayName:            "MiniMax",
		Aliases:                []string{"minimax-portal", "minimax-cli"},
		VerificationURL:        "https://chat.minimax.io/",
		SupportsBrowserSession: false,
	},
	{
		ID:                     "kimi",
		DisplayName:            "Kimi",
		Aliases:                []string{"kimi-code", "kimi-coding", "kimi-for-coding"},
		VerificationURL:        "https://www.kimi.com/",
		SupportsBrowserSession: true,
	},
	{
		ID:                     "opencode",
		DisplayName:            "OpenCode",
		Aliases:                []string{"opencode-zen", "opencode-ai", "opencode-go", "opencode_free", "opencodefree"},
		VerificationURL:        "https://opencode.ai/",
		SupportsBrowserSession: false,
	},
	{
		ID:                     "zhipuai",
		DisplayName:            "Zhipu AI",
		Aliases:                []string{"zhipu", "zhipu-ai", "bigmodel", "bigmodel-cn", "zhipuai-coding", "zhipu-coding"},
		VerificationURL:        "https://open.bigmodel.cn/",
		SupportsBrowserSession: false,
	},
	{
		ID:                     "openrouter",
		DisplayName:            "OpenRouter",
		Aliases:                []string{"openrouter-ai"},
		VerificationURL:        "https://openrouter.ai/",
		SupportsBrowserSession: true,
	},
	{
		ID:                     "zai",
		DisplayName:            "Z.ai",
		Aliases:                []string{"z.ai", "z-ai", "zaiweb", "zai-web"},
		VerificationURL:        "https://chat.z.ai/",
		SupportsBrowserSession: false,
	},
	{
		ID:                     "inception",
		DisplayName:            "Inception",
		Aliases:                []string{"inception-labs", "inceptionlabs", "mercury", "mercury2", "mercury-2"},
		VerificationURL:        "https://chat.inceptionlabs.ai/",
		SupportsBrowserSession: false,
	},
}

func oauthProviderCatalogEntries() []oauthProviderCatalogEntry {
	entries := make([]oauthProviderCatalogEntry, 0, len(oauthProviderCatalog))
	for _, entry := range oauthProviderCatalog {
		entries = append(entries, oauthProviderCatalogEntry{
			ID:                     entry.ID,
			DisplayName:            entry.DisplayName,
			Aliases:                append([]string{}, entry.Aliases...),
			VerificationURL:        entry.VerificationURL,
			SupportsBrowserSession: entry.SupportsBrowserSession,
		})
	}
	return entries
}

func resolveOAuthProviderCatalogEntry(provider string) (oauthProviderCatalogEntry, bool) {
	canonical := normalizeProviderID(provider)
	for _, entry := range oauthProviderCatalog {
		if entry.ID == canonical {
			return oauthProviderCatalogEntry{
				ID:                     entry.ID,
				DisplayName:            entry.DisplayName,
				Aliases:                append([]string{}, entry.Aliases...),
				VerificationURL:        entry.VerificationURL,
				SupportsBrowserSession: entry.SupportsBrowserSession,
			}, true
		}
	}
	return oauthProviderCatalogEntry{}, false
}

func authProviderCatalogPayload(apiKeyConfigured func(provider string) bool) []map[string]any {
	entries := oauthProviderCatalogEntries()
	out := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		configured := false
		if apiKeyConfigured != nil {
			configured = apiKeyConfigured(entry.ID)
		}
		out = append(out, map[string]any{
			"id":                     entry.ID,
			"providerId":             entry.ID,
			"name":                   entry.DisplayName,
			"displayName":            entry.DisplayName,
			"aliases":                append([]string{}, entry.Aliases...),
			"verificationUrl":        entry.VerificationURL,
			"verificationUri":        entry.VerificationURL,
			"supportsBrowserSession": entry.SupportsBrowserSession,
			"apiKeyConfigured":       configured,
		})
	}
	return out
}

func normalizeProviderID(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "openai", "openai-chatgpt", "chatgpt-web", "chatgpt.com":
		return "chatgpt"
	case "openai-codex", "codex-cli", "openai-codex-cli":
		return "codex"
	case "anthropic", "claude-cli", "claude-code", "claude-desktop":
		return "claude"
	case "google", "google-gemini", "google-gemini-cli", "gemini-cli":
		return "gemini"
	case "qwen-portal", "qwen-cli", "qwen-chat", "qwen35", "qwen3.5", "qwen-3.5", "copaw", "qwen-copaw", "qwen-agent":
		return "qwen"
	case "minimax-portal", "minimax-cli":
		return "minimax"
	case "kimi-code", "kimi-coding", "kimi-for-coding":
		return "kimi"
	case "opencode-zen", "opencode-ai", "opencode-go", "opencode_free", "opencodefree":
		return "opencode"
	case "zhipu", "zhipu-ai", "bigmodel", "bigmodel-cn", "zhipuai-coding", "zhipu-coding":
		return "zhipuai"
	case "z.ai", "z-ai", "zaiweb", "zai-web":
		return "zai"
	case "inception-labs", "inceptionlabs", "mercury", "mercury2", "mercury-2":
		return "inception"
	default:
		return normalized
	}
}
