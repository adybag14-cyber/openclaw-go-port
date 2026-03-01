package gateway

import (
	"context"
	"testing"

	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/buildinfo"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/config"
)

func TestNormalizeProviderAliasSupportsExpandedAliases(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{input: "openai", expected: "chatgpt"},
		{input: "codex-cli", expected: "codex"},
		{input: "qwen-portal", expected: "qwen"},
		{input: "copaw", expected: "qwen"},
		{input: "mercury2", expected: "inception"},
		{input: "z.ai", expected: "zai"},
	}

	for _, tc := range cases {
		if got := normalizeProviderAlias(tc.input); got != tc.expected {
			t.Fatalf("normalizeProviderAlias(%q)=%q want %q", tc.input, got, tc.expected)
		}
	}
}

func TestAuthProviderCatalogPayloadMarksConfiguredAliases(t *testing.T) {
	compat := newCompatState()
	if !compat.setProviderAPIKey("copaw", "qwen-secret") {
		t.Fatalf("expected setProviderAPIKey to report change")
	}

	providers := authProviderCatalogPayload(func(provider string) bool {
		return compat.hasProviderAPIKey(provider)
	})
	if len(providers) == 0 {
		t.Fatalf("expected non-empty provider payload")
	}

	var qwen map[string]any
	for _, entry := range providers {
		if toString(entry["id"], "") == "qwen" {
			qwen = entry
			break
		}
	}
	if len(qwen) == 0 {
		t.Fatalf("expected qwen provider entry in payload")
	}
	if toString(qwen["verificationUrl"], "") != "https://chat.qwen.ai/" {
		t.Fatalf("unexpected qwen verification url: %v", qwen["verificationUrl"])
	}
	if configured, _ := qwen["apiKeyConfigured"].(bool); !configured {
		t.Fatalf("expected qwen apiKeyConfigured=true when copaw alias key is set")
	}
	aliases, ok := qwen["aliases"].([]string)
	if !ok {
		t.Fatalf("expected qwen aliases as []string, got %T", qwen["aliases"])
	}
	foundCopaw := false
	for _, alias := range aliases {
		if alias == "copaw" {
			foundCopaw = true
			break
		}
	}
	if !foundCopaw {
		t.Fatalf("expected qwen aliases to include copaw")
	}
}

func TestHandleCompatAuthOAuthProvidersReturnsCatalog(t *testing.T) {
	s := New(config.Default(), buildinfo.Default())
	defer s.Close()

	result, derr := s.handleCompatMethod(context.Background(), "oauth-providers", "auth.oauth.providers", map[string]any{})
	if derr != nil {
		t.Fatalf("auth.oauth.providers failed: %+v", *derr)
	}
	rawProviders, ok := result["providers"].([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any providers payload, got %T", result["providers"])
	}
	if len(rawProviders) < 6 {
		t.Fatalf("expected expanded auth provider catalog, got %d entries", len(rawProviders))
	}

	seen := map[string]bool{}
	for _, entry := range rawProviders {
		seen[toString(entry["id"], "")] = true
	}
	for _, required := range []string{"chatgpt", "codex", "qwen", "openrouter"} {
		if !seen[required] {
			t.Fatalf("expected provider %q in auth.oauth.providers payload", required)
		}
	}
}

func TestParseAuthStartScopeAcceptsCopawAlias(t *testing.T) {
	scope, parseErr := parseAuthStartScope([]string{"copaw", "mobile"}, "chatgpt")
	if parseErr != "" {
		t.Fatalf("unexpected parse error: %s", parseErr)
	}
	if scope.Provider != "qwen" {
		t.Fatalf("expected provider qwen for copaw alias, got %q", scope.Provider)
	}
	if scope.Account != "mobile" {
		t.Fatalf("expected account mobile, got %q", scope.Account)
	}
}

func TestResolveModelChoiceSupportsSlashScopedCatalogIDs(t *testing.T) {
	compat := newCompatState()

	modelID, alias, ok := compat.resolveModelChoice("openrouter/qwen/qwen3-coder:free")
	if !ok {
		t.Fatalf("expected slash-scoped model id to resolve")
	}
	if modelID != "openrouter/qwen/qwen3-coder:free" {
		t.Fatalf("unexpected model id: %s", modelID)
	}
	if alias != "" {
		t.Fatalf("expected direct id match alias to be empty, got %q", alias)
	}

	modelID, alias, ok = compat.resolveModelChoice("qwen3-coder-free")
	if !ok {
		t.Fatalf("expected alias to resolve slash-scoped model id")
	}
	if modelID != "openrouter/qwen/qwen3-coder:free" {
		t.Fatalf("unexpected model id from alias: %s", modelID)
	}
	if alias != "qwen3-coder-free" {
		t.Fatalf("unexpected alias key: %q", alias)
	}
}

func TestExpandedModelCatalogListsProviderSpecificModels(t *testing.T) {
	compat := newCompatState()

	qwenModels := compat.listModelIDsForProvider("copaw")
	if len(qwenModels) == 0 {
		t.Fatalf("expected qwen models for copaw alias provider")
	}
	containsQwenPrimary := false
	for _, model := range qwenModels {
		if model == "qwen3.5-397b-a17b" {
			containsQwenPrimary = true
			break
		}
	}
	if !containsQwenPrimary {
		t.Fatalf("expected qwen3.5-397b-a17b in qwen model list, got %v", qwenModels)
	}

	openRouterModels := compat.listModelIDsForProvider("openrouter")
	containsSlashScoped := false
	for _, model := range openRouterModels {
		if model == "openrouter/qwen/qwen3-coder:free" {
			containsSlashScoped = true
			break
		}
	}
	if !containsSlashScoped {
		t.Fatalf("expected slash-scoped openrouter model in provider list, got %v", openRouterModels)
	}
}
