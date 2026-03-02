package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	webbridge "github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/bridge/web"
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
		{input: "glm5", expected: "zai"},
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

func TestAuthProviderCatalogMarksBrowserSessionProviders(t *testing.T) {
	entries := authProviderCatalogPayload(nil)
	if len(entries) == 0 {
		t.Fatalf("expected non-empty provider catalog")
	}
	requiresBrowserSession := map[string]bool{}
	for _, entry := range entries {
		providerID := toString(entry["id"], "")
		supports, _ := entry["supportsBrowserSession"].(bool)
		requiresBrowserSession[providerID] = supports
	}

	for _, providerID := range []string{"chatgpt", "qwen", "zai", "inception"} {
		if !requiresBrowserSession[providerID] {
			t.Fatalf("expected provider %q to advertise supportsBrowserSession=true", providerID)
		}
	}
	if requiresBrowserSession["opencode"] {
		t.Fatalf("expected provider opencode to remain api-key / bridge optional without browser-session requirement")
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
	containsQwenSmall := false
	for _, model := range qwenModels {
		if model == "qwen3-0.6b" {
			containsQwenSmall = true
			break
		}
	}
	if !containsQwenSmall {
		t.Fatalf("expected qwen3-0.6b in qwen model list, got %v", qwenModels)
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

func TestCompatModelsListRefreshesOpenRouterCatalog(t *testing.T) {
	openRouterAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"qwen/qwen3-0.6b:free","name":"Qwen 3 0.6B Free"},{"id":"qwen/qwen3-4b:free","name":"Qwen 3 4B Free"}]}`))
	}))
	defer openRouterAPI.Close()
	t.Setenv("OPENCLAW_GO_OPENROUTER_MODELS_URL", openRouterAPI.URL)

	cfg := config.Default()
	cfg.Runtime.ModelCatalogRefreshTTLSeconds = 1
	s := New(cfg, buildinfo.Default())
	defer s.Close()

	result, derr := s.handleCompatMethod(context.Background(), "models-list-openrouter-refresh", "models.list", map[string]any{
		"provider": "openrouter",
	})
	if derr != nil {
		t.Fatalf("models.list openrouter failed: %+v", *derr)
	}
	models, ok := result["models"].([]map[string]any)
	if !ok || len(models) == 0 {
		t.Fatalf("expected non-empty openrouter models list")
	}
	found := false
	for _, model := range models {
		if toString(model["id"], "") == "openrouter/qwen/qwen3-0.6b:free" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected refreshed openrouter model in models.list payload, got %v", models)
	}
}

func TestCompatModelsListRefreshesOpenCodeCatalog(t *testing.T) {
	openCodeAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"claude-opus-4-6"},{"id":"qwen3-coder-30b-a3b-instruct"}]}`))
	}))
	defer openCodeAPI.Close()
	t.Setenv("OPENCLAW_GO_OPENCODE_MODELS_URL", openCodeAPI.URL)

	cfg := config.Default()
	cfg.Runtime.ModelCatalogRefreshTTLSeconds = 1
	s := New(cfg, buildinfo.Default())
	defer s.Close()

	result, derr := s.handleCompatMethod(context.Background(), "models-list-opencode-refresh", "models.list", map[string]any{
		"provider": "opencode",
	})
	if derr != nil {
		t.Fatalf("models.list opencode failed: %+v", *derr)
	}
	models, ok := result["models"].([]map[string]any)
	if !ok || len(models) == 0 {
		t.Fatalf("expected non-empty opencode models list")
	}
	found := false
	for _, model := range models {
		if toString(model["id"], "") == "opencode/qwen3-coder-30b-a3b-instruct" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected refreshed opencode model in models.list payload, got %v", models)
	}
}

func TestModelCatalogIncludesRustProviderParitySet(t *testing.T) {
	compat := newCompatState()
	providers := compat.listModelProviders()
	if len(providers) == 0 {
		t.Fatalf("expected non-empty model provider list")
	}
	seen := map[string]bool{}
	for _, provider := range providers {
		seen[provider] = true
	}

	// Rust model registry canonical providers (with Go canonical mapping applied):
	// openai->chatgpt, openai-codex->codex, anthropic->claude, qwen-portal->qwen.
	for _, required := range []string{
		"chatgpt",
		"codex",
		"claude",
		"groq",
		"opencode",
		"zhipuai",
		"zai",
		"qwen",
		"inception",
		"openrouter",
	} {
		if !seen[required] {
			t.Fatalf("expected model provider %q in Go catalog, got %v", required, providers)
		}
	}
}

func TestCompatModelsListRejectsUnknownParams(t *testing.T) {
	s := New(config.Default(), buildinfo.Default())
	defer s.Close()

	_, derr := s.handleCompatMethod(context.Background(), "models-list-invalid", "models.list", map[string]any{
		"unknownField": true,
	})
	if derr == nil {
		t.Fatalf("expected models.list invalid params error")
	}
	if derr.Code != -32602 {
		t.Fatalf("expected -32602 for invalid models.list params, got %d", derr.Code)
	}
}

func TestCompatModelsListProviderFilterSupportsCopawAlias(t *testing.T) {
	s := New(config.Default(), buildinfo.Default())
	defer s.Close()

	result, derr := s.handleCompatMethod(context.Background(), "models-list-provider", "models.list", map[string]any{
		"provider": "copaw",
	})
	if derr != nil {
		t.Fatalf("models.list failed: %+v", *derr)
	}
	if toString(result["providerRequested"], "") != "qwen" {
		t.Fatalf("expected providerRequested=qwen, got %v", result["providerRequested"])
	}
	models, ok := result["models"].([]map[string]any)
	if !ok || len(models) == 0 {
		t.Fatalf("expected non-empty models list for qwen provider")
	}
	for _, model := range models {
		if provider := toString(model["provider"], ""); provider != "qwen" {
			t.Fatalf("expected provider qwen in filtered result, got %q", provider)
		}
	}
}

func TestCompatAuthOAuthProvidersRejectsUnknownParams(t *testing.T) {
	s := New(config.Default(), buildinfo.Default())
	defer s.Close()

	_, derr := s.handleCompatMethod(context.Background(), "oauth-providers-invalid", "auth.oauth.providers", map[string]any{
		"unknownField": true,
	})
	if derr == nil {
		t.Fatalf("expected auth.oauth.providers invalid params error")
	}
	if derr.Code != -32602 {
		t.Fatalf("expected -32602 for invalid auth.oauth.providers params, got %d", derr.Code)
	}
}

func TestCompatAuthOAuthProvidersFilterSupportsAlias(t *testing.T) {
	s := New(config.Default(), buildinfo.Default())
	defer s.Close()

	result, derr := s.handleCompatMethod(context.Background(), "oauth-providers-filter", "auth.oauth.providers", map[string]any{
		"provider": "openai-codex",
	})
	if derr != nil {
		t.Fatalf("auth.oauth.providers failed: %+v", *derr)
	}
	if toString(result["providerRequested"], "") != "codex" {
		t.Fatalf("expected providerRequested=codex, got %v", result["providerRequested"])
	}
	providers, ok := result["providers"].([]map[string]any)
	if !ok || len(providers) != 1 {
		t.Fatalf("expected single filtered provider entry, got %T len=%d", result["providers"], len(providers))
	}
	if id := toString(providers[0]["id"], ""); id != "codex" {
		t.Fatalf("expected filtered provider id codex, got %q", id)
	}
}

func TestCompatOAuthImportRejectsUnknownProvider(t *testing.T) {
	s := New(config.Default(), buildinfo.Default())
	defer s.Close()

	_, derr := s.handleCompatMethod(context.Background(), "oauth-import-invalid", "auth.oauth.import", map[string]any{
		"provider": "unknown-oauth-provider",
	})
	if derr == nil {
		t.Fatalf("expected auth.oauth.import to reject unknown provider")
	}
	if derr.Code != -32602 {
		t.Fatalf("expected -32602 for unknown provider, got %d", derr.Code)
	}
}

func TestCompatOAuthImportCanonicalizesProviderAlias(t *testing.T) {
	s := New(config.Default(), buildinfo.Default())
	defer s.Close()

	result, derr := s.handleCompatMethod(context.Background(), "oauth-import-codex", "auth.oauth.import", map[string]any{
		"provider": "openai-codex",
	})
	if derr != nil {
		t.Fatalf("auth.oauth.import failed: %+v", *derr)
	}
	if toString(result["providerId"], "") != "codex" {
		t.Fatalf("expected providerId=codex, got %v", result["providerId"])
	}
	if toString(result["providerDisplayName"], "") == "" {
		t.Fatalf("expected non-empty providerDisplayName")
	}
	login, ok := result["login"].(webbridge.Session)
	if !ok {
		t.Fatalf("expected login session object, got %T", result["login"])
	}
	if login.Provider != "codex" {
		t.Fatalf("expected imported login provider codex, got %v", login.Provider)
	}
	if login.Status != webbridge.LoginAuthorized {
		t.Fatalf("expected authorized login status, got %v", login.Status)
	}
}
