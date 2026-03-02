package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	defaultOpenRouterModelsURL = "https://openrouter.ai/api/v1/models"
	defaultOpenCodeModelsURL   = "https://opencode.ai/zen/v1/models"
)

func (s *Server) refreshCompatModelCatalog(ctx context.Context, requestedProvider string) map[string]any {
	provider := normalizeProviderAlias(requestedProvider)
	if provider != "" {
		status := s.refreshCompatModelCatalogProvider(ctx, provider)
		return map[string]any{
			"providers": []map[string]any{status},
		}
	}

	statuses := make([]map[string]any, 0, 3)
	for _, candidate := range []string{"qwen", "openrouter", "opencode"} {
		statuses = append(statuses, s.refreshCompatModelCatalogProvider(ctx, candidate))
	}
	return map[string]any{
		"providers": statuses,
	}
}

func (s *Server) refreshCompatModelCatalogProvider(ctx context.Context, provider string) map[string]any {
	canonical := normalizeProviderAlias(provider)
	status := s.compat.modelCatalogRefreshStatus(canonical)
	status["provider"] = canonical
	status["supported"] = true

	switch canonical {
	case "qwen", "openrouter", "opencode":
	default:
		status["supported"] = false
		status["skipped"] = true
		status["reason"] = "provider-not-supported"
		return status
	}

	refreshTTL := time.Duration(s.cfg.Runtime.ModelCatalogRefreshTTLSeconds) * time.Second
	if refreshTTL <= 0 {
		refreshTTL = 5 * time.Minute
	}
	if !s.compat.shouldRefreshModelCatalog(canonical, refreshTTL) {
		status["skipped"] = true
		status["reason"] = "ttl-not-expired"
		status["ttlSeconds"] = int(refreshTTL.Seconds())
		return status
	}

	refreshCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	var (
		models []map[string]any
		err    error
	)
	switch canonical {
	case "qwen":
		models = qwenSmallModelCatalogEntries()
	case "openrouter":
		models, err = fetchOpenRouterModelCatalog(refreshCtx, resolveModelCatalogAPIKey("openrouter", s.compat.getProviderAPIKey("openrouter")))
	case "opencode":
		models, err = fetchOpenCodeModelCatalog(refreshCtx, resolveModelCatalogAPIKey("opencode", s.compat.getProviderAPIKey("opencode")))
	}

	added := 0
	if err == nil {
		added = s.compat.upsertModelDescriptors(models)
	}
	total := len(s.compat.listModelIDsForProvider(canonical))
	status = s.compat.markModelCatalogRefresh(canonical, total, err)
	status["provider"] = canonical
	status["supported"] = true
	status["fetched"] = len(models)
	status["added"] = added
	status["ttlSeconds"] = int(refreshTTL.Seconds())
	return status
}

func (c *compatState) shouldRefreshModelCatalog(provider string, ttl time.Duration) bool {
	canonical := normalizeProviderAlias(provider)
	if canonical == "" {
		return false
	}
	c.mu.RLock()
	last := c.modelCatalogRefreshedAt[canonical]
	c.mu.RUnlock()
	if last.IsZero() {
		return true
	}
	if ttl <= 0 {
		return true
	}
	return time.Since(last) >= ttl
}

func (c *compatState) modelCatalogRefreshStatus(provider string) map[string]any {
	canonical := normalizeProviderAlias(provider)
	if canonical == "" {
		return map[string]any{}
	}
	c.mu.RLock()
	last := c.modelCatalogRefreshedAt[canonical]
	lastError := strings.TrimSpace(c.modelCatalogLastError[canonical])
	count := c.modelCatalogModelCounts[canonical]
	c.mu.RUnlock()
	refreshedAt := ""
	if !last.IsZero() {
		refreshedAt = last.UTC().Format(time.RFC3339)
	}
	return map[string]any{
		"provider":    canonical,
		"refreshedAt": valueOrNil(refreshedAt),
		"lastError":   valueOrNil(lastError),
		"count":       count,
	}
}

func (c *compatState) markModelCatalogRefresh(provider string, count int, err error) map[string]any {
	canonical := normalizeProviderAlias(provider)
	if canonical == "" {
		return map[string]any{}
	}
	now := time.Now().UTC()
	errText := ""
	if err != nil {
		errText = strings.TrimSpace(err.Error())
	}
	c.mu.Lock()
	c.modelCatalogRefreshedAt[canonical] = now
	c.modelCatalogModelCounts[canonical] = count
	if errText == "" {
		delete(c.modelCatalogLastError, canonical)
	} else {
		c.modelCatalogLastError[canonical] = errText
	}
	c.mu.Unlock()
	return map[string]any{
		"provider":    canonical,
		"refreshedAt": now.Format(time.RFC3339),
		"lastError":   valueOrNil(errText),
		"count":       count,
		"ok":          errText == "",
	}
}

func (c *compatState) upsertModelDescriptors(models []map[string]any) int {
	if len(models) == 0 {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	index := make(map[string]int, len(c.models))
	for i, descriptor := range c.models {
		key := modelCatalogDescriptorKey(
			normalizeProviderAlias(toString(descriptor["provider"], "")),
			strings.ToLower(strings.TrimSpace(toString(descriptor["id"], ""))),
		)
		if key == "" {
			continue
		}
		index[key] = i
	}

	added := 0
	for _, descriptor := range models {
		normalized := normalizeModelCatalogDescriptor(descriptor)
		key := modelCatalogDescriptorKey(
			normalizeProviderAlias(toString(normalized["provider"], "")),
			strings.ToLower(strings.TrimSpace(toString(normalized["id"], ""))),
		)
		if key == "" {
			continue
		}
		if i, ok := index[key]; ok {
			c.models[i] = normalized
			continue
		}
		c.models = append(c.models, normalized)
		index[key] = len(c.models) - 1
		added++
	}
	return added
}

func modelCatalogDescriptorKey(provider string, id string) string {
	p := normalizeProviderAlias(provider)
	m := strings.ToLower(strings.TrimSpace(id))
	if p == "" || m == "" {
		return ""
	}
	return p + "|" + m
}

func normalizeModelCatalogDescriptor(raw map[string]any) map[string]any {
	item := cloneMap(raw)
	provider := normalizeProviderAlias(toString(item["provider"], ""))
	modelID := strings.ToLower(strings.TrimSpace(toString(item["id"], "")))
	if provider == "" || modelID == "" {
		return map[string]any{}
	}
	item["provider"] = provider
	item["id"] = modelID
	if strings.TrimSpace(toString(item["name"], "")) == "" {
		item["name"] = modelID
	}
	if strings.TrimSpace(toString(item["mode"], "")) == "" {
		item["mode"] = inferModelModeFromID(modelID)
	}
	if strings.TrimSpace(toString(item["capability"], "")) == "" {
		item["capability"] = inferModelCapabilityFromID(modelID)
	}
	item["aliases"] = normalizeModelAliases(modelID, descriptorAliases(item["aliases"]))
	return item
}

func normalizeModelAliases(modelID string, aliases []string) []string {
	seen := map[string]struct{}{}
	add := func(value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		normalized := strings.ToLower(trimmed)
		if _, ok := seen[normalized]; ok {
			return
		}
		seen[normalized] = struct{}{}
	}

	add(modelID)
	add(normalizeModelAlias(modelID))
	if idx := strings.LastIndex(modelID, "/"); idx >= 0 && idx+1 < len(modelID) {
		leaf := strings.TrimSpace(modelID[idx+1:])
		add(leaf)
		add(normalizeModelAlias(leaf))
	}
	add(strings.ReplaceAll(modelID, ":", "-"))
	for _, alias := range aliases {
		add(alias)
		add(normalizeModelAlias(alias))
	}

	out := make([]string, 0, len(seen))
	for alias := range seen {
		if alias != "" {
			out = append(out, alias)
		}
	}
	sort.Strings(out)
	return out
}

func inferModelModeFromID(modelID string) string {
	id := strings.ToLower(strings.TrimSpace(modelID))
	switch {
	case strings.Contains(id, "flash"),
		strings.Contains(id, "mini"),
		strings.Contains(id, "small"),
		strings.Contains(id, "0.6b"),
		strings.Contains(id, "1.7b"),
		strings.Contains(id, "4b"),
		strings.Contains(id, ":free"),
		strings.Contains(id, "-free"):
		return "instant"
	case strings.Contains(id, "pro"),
		strings.Contains(id, "plus"):
		return "pro"
	default:
		return "thinking"
	}
}

func inferModelCapabilityFromID(modelID string) string {
	id := strings.ToLower(strings.TrimSpace(modelID))
	switch {
	case strings.Contains(id, "coder"), strings.Contains(id, "code"):
		return "coding"
	case strings.Contains(id, "vl"), strings.Contains(id, "vision"), strings.Contains(id, "image"), strings.Contains(id, "video"), strings.Contains(id, "omni"):
		return "multimodal"
	case strings.Contains(id, "embed"):
		return "embedding"
	case strings.Contains(id, "free"), strings.Contains(id, "flash"), strings.Contains(id, "mini"), strings.Contains(id, "small"):
		return "fast-response"
	default:
		return "reasoning"
	}
}

func resolveModelCatalogAPIKey(provider string, inMemoryValue string) string {
	if value := strings.TrimSpace(inMemoryValue); value != "" {
		return value
	}
	lookup := map[string][]string{
		"openrouter": {"OPENROUTER_API_KEY", "OPENROUTER_KEY"},
		"opencode":   {"OPENCODE_API_KEY", "OPENCODE_ZEN_API_KEY"},
	}
	for _, envName := range lookup[normalizeProviderAlias(provider)] {
		if value := strings.TrimSpace(os.Getenv(envName)); value != "" {
			return value
		}
	}
	return ""
}

func fetchOpenRouterModelCatalog(ctx context.Context, apiKey string) ([]map[string]any, error) {
	endpoint := strings.TrimSpace(os.Getenv("OPENCLAW_GO_OPENROUTER_MODELS_URL"))
	if endpoint == "" {
		endpoint = defaultOpenRouterModelsURL
	}
	var payload struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := fetchModelCatalog(ctx, endpoint, apiKey, &payload); err != nil {
		return nil, err
	}

	models := make([]map[string]any, 0, len(payload.Data))
	for _, entry := range payload.Data {
		rawID := strings.ToLower(strings.TrimSpace(entry.ID))
		if rawID == "" {
			continue
		}
		fullID := strings.ToLower("openrouter/" + rawID)
		models = append(models, map[string]any{
			"id":         fullID,
			"name":       valueOrDefault(strings.TrimSpace(entry.Name), "OpenRouter "+rawID),
			"mode":       inferModelModeFromID(fullID),
			"provider":   "openrouter",
			"capability": inferModelCapabilityFromID(fullID),
			"aliases": normalizeModelAliases(fullID, []string{
				rawID,
				strings.ReplaceAll(rawID, ":", "-"),
			}),
		})
	}
	sort.Slice(models, func(i, j int) bool {
		return toString(models[i]["id"], "") < toString(models[j]["id"], "")
	})
	return models, nil
}

func fetchOpenCodeModelCatalog(ctx context.Context, apiKey string) ([]map[string]any, error) {
	endpoint := strings.TrimSpace(os.Getenv("OPENCLAW_GO_OPENCODE_MODELS_URL"))
	if endpoint == "" {
		endpoint = defaultOpenCodeModelsURL
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := fetchModelCatalog(ctx, endpoint, apiKey, &payload); err != nil {
		return nil, err
	}

	models := make([]map[string]any, 0, len(payload.Data))
	for _, entry := range payload.Data {
		rawID := strings.ToLower(strings.TrimSpace(entry.ID))
		if rawID == "" {
			continue
		}
		fullID := strings.ToLower("opencode/" + rawID)
		models = append(models, map[string]any{
			"id":         fullID,
			"name":       "OpenCode " + strings.ToUpper(rawID),
			"mode":       inferModelModeFromID(fullID),
			"provider":   "opencode",
			"capability": inferModelCapabilityFromID(fullID),
			"aliases": normalizeModelAliases(fullID, []string{
				rawID,
				strings.ReplaceAll(rawID, ":", "-"),
			}),
		})
	}
	sort.Slice(models, func(i, j int) bool {
		return toString(models[i]["id"], "") < toString(models[j]["id"], "")
	})
	return models, nil
}

func fetchModelCatalog(ctx context.Context, endpoint string, apiKey string, out any) error {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("model catalog request failed: HTTP %d: %s", resp.StatusCode, truncateModelCatalogBody(string(body), 400))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("model catalog returned invalid JSON: %w", err)
	}
	return nil
}

func qwenSmallModelCatalogEntries() []map[string]any {
	return []map[string]any{
		{
			"id":         "qwen3-0.6b",
			"name":       "Qwen 3 0.6B",
			"mode":       "instant",
			"provider":   "qwen",
			"capability": "small-fast",
			"aliases":    []string{"qwen-0.6b", "qwen3-0.6b-instruct"},
		},
		{
			"id":         "qwen3-1.7b",
			"name":       "Qwen 3 1.7B",
			"mode":       "instant",
			"provider":   "qwen",
			"capability": "small-fast",
			"aliases":    []string{"qwen-1.7b", "qwen3-1.7b-instruct"},
		},
		{
			"id":         "qwen3-4b",
			"name":       "Qwen 3 4B",
			"mode":       "instant",
			"provider":   "qwen",
			"capability": "small-balanced",
			"aliases":    []string{"qwen-4b", "qwen3-4b-instruct"},
		},
		{
			"id":         "qwen3-8b",
			"name":       "Qwen 3 8B",
			"mode":       "thinking",
			"provider":   "qwen",
			"capability": "balanced",
			"aliases":    []string{"qwen-8b", "qwen3-8b-instruct"},
		},
	}
}

func truncateModelCatalogBody(raw string, maxLen int) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "<empty>"
	}
	if maxLen <= 0 || len(trimmed) <= maxLen {
		return trimmed
	}
	return trimmed[:maxLen] + "..."
}
