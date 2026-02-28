package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Request struct {
	Tool      string         `json:"tool"`
	SessionID string         `json:"session_id,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
}

type InvokeResult struct {
	Provider string `json:"provider"`
	Output   any    `json:"output"`
}

type Provider interface {
	Name() string
	Supports(tool string) bool
	Invoke(ctx context.Context, req Request) (any, error)
	Catalog() []ToolSpec
}

type ToolSpec struct {
	Tool        string `json:"tool"`
	Provider    string `json:"provider"`
	Description string `json:"description"`
}

type RuntimeOptions struct {
	BrowserBridge BrowserBridgeOptions
}

type BrowserBridgeOptions struct {
	Enabled              bool
	Endpoint             string
	RequestTimeout       time.Duration
	Retries              int
	RetryBackoff         time.Duration
	CircuitFailThreshold int
	CircuitCooldown      time.Duration
}

type Runtime struct {
	providers []Provider
}

func New() *Runtime {
	return &Runtime{
		providers: make([]Provider, 0, 4),
	}
}

func NewDefault() *Runtime {
	return NewDefaultWithOptions(DefaultRuntimeOptions())
}

func NewDefaultWithOptions(options RuntimeOptions) *Runtime {
	rt := New()
	rt.RegisterProvider(NewBuiltinBridgeProviderWithOptions(options.BrowserBridge))
	return rt
}

func DefaultRuntimeOptions() RuntimeOptions {
	return RuntimeOptions{
		BrowserBridge: DefaultBrowserBridgeOptions(),
	}
}

func DefaultBrowserBridgeOptions() BrowserBridgeOptions {
	return BrowserBridgeOptions{
		Enabled:              true,
		Endpoint:             "http://127.0.0.1:43010",
		RequestTimeout:       180 * time.Second,
		Retries:              2,
		RetryBackoff:         750 * time.Millisecond,
		CircuitFailThreshold: 3,
		CircuitCooldown:      10 * time.Second,
	}
}

func normalizeBrowserBridgeOptions(input BrowserBridgeOptions) BrowserBridgeOptions {
	defaults := DefaultBrowserBridgeOptions()

	out := input
	if strings.TrimSpace(out.Endpoint) == "" {
		out.Endpoint = defaults.Endpoint
	}
	if out.RequestTimeout <= 0 {
		out.RequestTimeout = defaults.RequestTimeout
	}
	if out.Retries < 0 {
		out.Retries = defaults.Retries
	}
	if out.RetryBackoff < 0 {
		out.RetryBackoff = defaults.RetryBackoff
	}
	if out.CircuitFailThreshold < 1 {
		out.CircuitFailThreshold = defaults.CircuitFailThreshold
	}
	if out.CircuitCooldown <= 0 {
		out.CircuitCooldown = defaults.CircuitCooldown
	}
	return out
}

func (r *Runtime) RegisterProvider(provider Provider) {
	r.providers = append(r.providers, provider)
}

func (r *Runtime) Invoke(ctx context.Context, req Request) (InvokeResult, error) {
	normalizedTool := strings.ToLower(strings.TrimSpace(req.Tool))
	if normalizedTool == "" {
		return InvokeResult{}, errors.New("tool name is required")
	}
	req.Tool = normalizedTool
	for _, provider := range r.providers {
		if !provider.Supports(req.Tool) {
			continue
		}
		output, err := provider.Invoke(ctx, req)
		if err != nil {
			return InvokeResult{}, err
		}
		return InvokeResult{
			Provider: provider.Name(),
			Output:   output,
		}, nil
	}
	return InvokeResult{}, fmt.Errorf("no provider for tool %q", req.Tool)
}

func (r *Runtime) Catalog() []ToolSpec {
	var out []ToolSpec
	for _, provider := range r.providers {
		out = append(out, provider.Catalog()...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Tool == out[j].Tool {
			return out[i].Provider < out[j].Provider
		}
		return out[i].Tool < out[j].Tool
	})
	return out
}

type BuiltinBridgeProvider struct {
	mu            sync.RWMutex
	seq           atomic.Uint64
	jobs          map[string]map[string]any
	browser       BrowserBridgeOptions
	httpClient    *http.Client
	circuitLocker sync.Mutex
	failures      int
	openUntil     time.Time
}

func NewBuiltinBridgeProvider() *BuiltinBridgeProvider {
	return NewBuiltinBridgeProviderWithOptions(DefaultBrowserBridgeOptions())
}

func NewBuiltinBridgeProviderWithOptions(options BrowserBridgeOptions) *BuiltinBridgeProvider {
	return &BuiltinBridgeProvider{
		jobs:       map[string]map[string]any{},
		browser:    normalizeBrowserBridgeOptions(options),
		httpClient: &http.Client{},
	}
}

func (b *BuiltinBridgeProvider) Name() string {
	return "builtin-bridge"
}

func (b *BuiltinBridgeProvider) Supports(tool string) bool {
	switch strings.ToLower(strings.TrimSpace(tool)) {
	case "browser.request",
		"browser.open",
		"tool.echo",
		"exec.run",
		"file.read",
		"file.write",
		"file.patch",
		"message.send",
		"node.invoke",
		"task.background.start",
		"task.background.poll":
		return true
	default:
		return false
	}
}

func (b *BuiltinBridgeProvider) Invoke(ctx context.Context, req Request) (any, error) {
	switch req.Tool {
	case "browser.request":
		return b.handleBrowserRequest(ctx, req.Input)
	case "browser.open":
		url := toString(req.Input["url"], "")
		return map[string]any{
			"status": 200,
			"ok":     true,
			"url":    url,
			"opened": true,
		}, nil
	case "tool.echo":
		return map[string]any{
			"status": 200,
			"ok":     true,
			"echo":   req.Input,
		}, nil
	case "exec.run":
		return runCommand(ctx, req.Input)
	case "file.read":
		path := toString(req.Input["path"], "")
		if path == "" {
			return nil, errors.New("file.read requires path")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"status":  200,
			"ok":      true,
			"path":    path,
			"content": string(raw),
			"bytes":   len(raw),
		}, nil
	case "file.write":
		path := toString(req.Input["path"], "")
		if path == "" {
			return nil, errors.New("file.write requires path")
		}
		content := toString(req.Input["content"], "")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return nil, err
		}
		return map[string]any{
			"status": 200,
			"ok":     true,
			"path":   path,
			"bytes":  len(content),
		}, nil
	case "file.patch":
		path := toString(req.Input["path"], "")
		if path == "" {
			return nil, errors.New("file.patch requires path")
		}
		oldText := toString(req.Input["oldText"], "")
		newText := toString(req.Input["newText"], "")
		if oldText == "" {
			return nil, errors.New("file.patch requires oldText")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		current := string(raw)
		replaceAll := toBool(req.Input["replaceAll"], false)
		count := 0
		updated := current
		if replaceAll {
			count = strings.Count(current, oldText)
			updated = strings.ReplaceAll(current, oldText, newText)
		} else if strings.Contains(current, oldText) {
			count = 1
			updated = strings.Replace(current, oldText, newText, 1)
		}
		if count == 0 {
			return nil, errors.New("file.patch oldText not found")
		}
		if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
			return nil, err
		}
		return map[string]any{
			"status":       200,
			"ok":           true,
			"path":         path,
			"replacements": count,
		}, nil
	case "message.send":
		return map[string]any{
			"status":  200,
			"ok":      true,
			"channel": toString(req.Input["channel"], "webchat"),
			"to":      toString(req.Input["to"], ""),
			"message": toString(req.Input["message"], toString(req.Input["text"], "")),
		}, nil
	case "node.invoke":
		return map[string]any{
			"status":  200,
			"ok":      true,
			"nodeId":  toString(req.Input["nodeId"], "node-local"),
			"action":  toString(req.Input["action"], "run"),
			"payload": req.Input,
		}, nil
	case "task.background.start":
		jobID := fmt.Sprintf("bg-%06d", b.seq.Add(1))
		job := map[string]any{
			"jobId":     jobID,
			"status":    "running",
			"startedAt": time.Now().UTC().Format(time.RFC3339),
			"tool":      "exec.run",
		}
		b.mu.Lock()
		b.jobs[jobID] = cloneMap(job)
		b.mu.Unlock()

		input := cloneMap(req.Input)
		go func() {
			result, err := runCommand(context.Background(), input)
			terminal := map[string]any{
				"jobId":     jobID,
				"status":    "failed",
				"startedAt": job["startedAt"],
				"endedAt":   time.Now().UTC().Format(time.RFC3339),
				"tool":      "exec.run",
			}
			if err == nil {
				if output, ok := result.(map[string]any); ok {
					if toBool(output["ok"], false) {
						terminal["status"] = "completed"
					}
					for key, value := range output {
						terminal[key] = value
					}
				}
			} else {
				terminal["error"] = err.Error()
			}

			b.mu.Lock()
			b.jobs[jobID] = terminal
			b.mu.Unlock()
		}()

		return map[string]any{
			"status":   200,
			"ok":       true,
			"accepted": true,
			"jobId":    jobID,
			"state":    "running",
		}, nil
	case "task.background.poll":
		jobID := toString(req.Input["jobId"], "")
		if jobID == "" {
			return nil, errors.New("task.background.poll requires jobId")
		}
		b.mu.RLock()
		job, ok := b.jobs[jobID]
		b.mu.RUnlock()
		if !ok {
			return nil, fmt.Errorf("background job %q not found", jobID)
		}
		return cloneMap(job), nil
	default:
		return nil, fmt.Errorf("unsupported builtin tool %q", req.Tool)
	}
}

func (b *BuiltinBridgeProvider) Catalog() []ToolSpec {
	return []ToolSpec{
		{Tool: "browser.open", Provider: b.Name(), Description: "Open browser URL through bridge runtime"},
		{Tool: "browser.request", Provider: b.Name(), Description: "Send browser bridge request or completion payload"},
		{Tool: "exec.run", Provider: b.Name(), Description: "Execute local process command with timeout"},
		{Tool: "file.patch", Provider: b.Name(), Description: "Apply text replacement patch to file"},
		{Tool: "file.read", Provider: b.Name(), Description: "Read local file content"},
		{Tool: "file.write", Provider: b.Name(), Description: "Write local file content"},
		{Tool: "message.send", Provider: b.Name(), Description: "Send message through runtime bridge"},
		{Tool: "node.invoke", Provider: b.Name(), Description: "Invoke node operation through runtime bridge"},
		{Tool: "task.background.poll", Provider: b.Name(), Description: "Poll background task state"},
		{Tool: "task.background.start", Provider: b.Name(), Description: "Start background task execution"},
		{Tool: "tool.echo", Provider: b.Name(), Description: "Echo request payload for smoke validation"},
	}
}

func (b *BuiltinBridgeProvider) handleBrowserRequest(ctx context.Context, input map[string]any) (any, error) {
	payload, hasCompletionPayload := toBrowserCompletionPayload(input)
	if !hasCompletionPayload {
		url := toString(input["url"], "")
		method := strings.ToUpper(toString(input["method"], "GET"))
		return map[string]any{
			"status":   200,
			"ok":       true,
			"url":      url,
			"method":   method,
			"response": "bridge request accepted",
		}, nil
	}
	if !b.browser.Enabled {
		return nil, errors.New("browser bridge is disabled")
	}
	return b.invokeBrowserCompletion(ctx, payload)
}

func toBrowserCompletionPayload(input map[string]any) (map[string]any, bool) {
	model := toString(input["model"], "gpt-5.2")
	messages := normalizeCompletionMessages(input["messages"])
	if len(messages) == 0 {
		prompt := toString(input["prompt"], toString(input["message"], toString(input["text"], "")))
		if prompt != "" {
			messages = []map[string]any{
				{"role": "user", "content": prompt},
			}
		}
	}
	if len(messages) == 0 {
		return nil, false
	}

	payload := map[string]any{
		"model":    model,
		"messages": messages,
	}
	if value, ok := input["temperature"]; ok {
		payload["temperature"] = value
	}
	if value, ok := input["max_tokens"]; ok {
		payload["max_tokens"] = value
	}
	return payload, true
}

func normalizeCompletionMessages(raw any) []map[string]any {
	switch value := raw.(type) {
	case []map[string]any:
		out := make([]map[string]any, 0, len(value))
		for _, entry := range value {
			role := strings.ToLower(toString(entry["role"], ""))
			if role == "" {
				continue
			}
			content := toString(entry["content"], "")
			if content == "" {
				continue
			}
			out = append(out, map[string]any{
				"role":    role,
				"content": content,
			})
		}
		return out
	case []any:
		out := make([]map[string]any, 0, len(value))
		for _, item := range value {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			role := strings.ToLower(toString(entry["role"], ""))
			if role == "" {
				continue
			}
			content := toString(entry["content"], "")
			if content == "" {
				continue
			}
			out = append(out, map[string]any{
				"role":    role,
				"content": content,
			})
		}
		return out
	default:
		return []map[string]any{}
	}
}

type bridgeHTTPError struct {
	StatusCode int
	Body       string
}

func (e *bridgeHTTPError) Error() string {
	return fmt.Sprintf("bridge HTTP %d: %s", e.StatusCode, e.Body)
}

func (e *bridgeHTTPError) Retryable() bool {
	return e.StatusCode == http.StatusTooManyRequests || e.StatusCode == http.StatusRequestTimeout || e.StatusCode >= 500
}

func (b *BuiltinBridgeProvider) invokeBrowserCompletion(ctx context.Context, payload map[string]any) (any, error) {
	if strings.TrimSpace(b.browser.Endpoint) == "" {
		return nil, errors.New("browser bridge endpoint is empty")
	}
	if allowed, wait := b.allowBridgeRequest(); !allowed {
		return nil, fmt.Errorf("browser bridge circuit breaker open (retry in %s)", wait.Round(time.Millisecond).String())
	}

	lastErr := error(nil)
	maxAttempts := b.browser.Retries + 1
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		response, statusCode, err := b.postBridgeCompletion(ctx, payload)
		if err == nil {
			assistant := extractAssistantMessage(response)
			b.recordBridgeSuccess()
			return map[string]any{
				"status":        200,
				"ok":            true,
				"provider":      "chatgpt-browser-bridge",
				"bridgeStatus":  statusCode,
				"attempt":       attempt,
				"model":         toString(response["model"], toString(payload["model"], "gpt-5.2")),
				"assistantText": assistant,
				"response":      response,
			}, nil
		}

		lastErr = err
		retryable := true
		var httpErr *bridgeHTTPError
		if errors.As(err, &httpErr) {
			retryable = httpErr.Retryable()
		}
		if !retryable || attempt >= maxAttempts {
			break
		}

		backoff := b.browser.RetryBackoff * time.Duration(attempt)
		if backoff <= 0 {
			continue
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}

	b.recordBridgeFailure()
	return nil, fmt.Errorf("browser bridge completion failed after %d attempts: %w", maxAttempts, lastErr)
}

func (b *BuiltinBridgeProvider) postBridgeCompletion(ctx context.Context, payload map[string]any) (map[string]any, int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}

	endpoint := strings.TrimRight(strings.TrimSpace(b.browser.Endpoint), "/") + "/v1/chat/completions"
	reqCtx, cancel := context.WithTimeout(ctx, b.browser.RequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, resp.StatusCode, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode, &bridgeHTTPError{
			StatusCode: resp.StatusCode,
			Body:       truncateBridgeBody(string(raw), 2048),
		}
	}

	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("bridge returned invalid JSON: %w", err)
	}
	return parsed, resp.StatusCode, nil
}

func extractAssistantMessage(response map[string]any) string {
	rawChoices, ok := response["choices"].([]any)
	if !ok || len(rawChoices) == 0 {
		return ""
	}
	first, ok := rawChoices[0].(map[string]any)
	if !ok {
		return ""
	}
	if message, ok := first["message"].(map[string]any); ok {
		text := toString(message["content"], "")
		if text != "" {
			return text
		}
	}
	return toString(first["text"], "")
}

func truncateBridgeBody(raw string, maxLen int) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "<empty>"
	}
	if len(trimmed) <= maxLen {
		return trimmed
	}
	return trimmed[:maxLen] + "..."
}

func (b *BuiltinBridgeProvider) allowBridgeRequest() (bool, time.Duration) {
	now := time.Now().UTC()
	b.circuitLocker.Lock()
	defer b.circuitLocker.Unlock()
	if now.Before(b.openUntil) {
		return false, b.openUntil.Sub(now)
	}
	return true, 0
}

func (b *BuiltinBridgeProvider) recordBridgeSuccess() {
	b.circuitLocker.Lock()
	b.failures = 0
	b.openUntil = time.Time{}
	b.circuitLocker.Unlock()
}

func (b *BuiltinBridgeProvider) recordBridgeFailure() {
	now := time.Now().UTC()
	b.circuitLocker.Lock()
	defer b.circuitLocker.Unlock()
	b.failures++
	if b.failures >= b.browser.CircuitFailThreshold {
		b.openUntil = now.Add(b.browser.CircuitCooldown)
		b.failures = 0
	}
}

func runCommand(ctx context.Context, input map[string]any) (any, error) {
	command := toString(input["command"], "")
	if command == "" {
		return nil, errors.New("exec.run requires command")
	}
	args := toStringSlice(input["args"])
	timeout := time.Duration(toInt(input["timeoutMs"], 5000)) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	started := time.Now().UTC()
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, command, args...)
	cwd := toString(input["cwd"], "")
	if cwd != "" {
		cmd.Dir = cwd
	}
	if envMap, ok := input["env"].(map[string]any); ok {
		env := os.Environ()
		for key, value := range envMap {
			k := strings.TrimSpace(key)
			if k == "" {
				continue
			}
			env = append(env, fmt.Sprintf("%s=%s", k, toString(value, "")))
		}
		cmd.Env = env
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	timedOut := errors.Is(cmdCtx.Err(), context.DeadlineExceeded)
	ok := err == nil
	status := "completed"
	if timedOut {
		status = "timeout"
		ok = false
	}
	if err != nil && !timedOut {
		status = "failed"
	}

	output := map[string]any{
		"status":     200,
		"ok":         ok,
		"state":      status,
		"command":    command,
		"args":       args,
		"cwd":        cwd,
		"exitCode":   exitCode,
		"stdout":     strings.TrimSpace(stdout.String()),
		"stderr":     strings.TrimSpace(stderr.String()),
		"timedOut":   timedOut,
		"durationMs": time.Since(started).Milliseconds(),
	}
	if err != nil {
		output["error"] = err.Error()
	}
	return output, nil
}

func toString(value any, fallback string) string {
	switch raw := value.(type) {
	case string:
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return fallback
		}
		return trimmed
	case fmt.Stringer:
		trimmed := strings.TrimSpace(raw.String())
		if trimmed == "" {
			return fallback
		}
		return trimmed
	case int:
		return strconv.Itoa(raw)
	case int64:
		return strconv.FormatInt(raw, 10)
	case float64:
		return strconv.Itoa(int(raw))
	default:
		return fallback
	}
}

func toStringSlice(value any) []string {
	switch raw := value.(type) {
	case []string:
		out := make([]string, 0, len(raw))
		for _, entry := range raw {
			trimmed := strings.TrimSpace(entry)
			if trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(raw))
		for _, entry := range raw {
			converted := toString(entry, "")
			if converted != "" {
				out = append(out, converted)
			}
		}
		return out
	default:
		return []string{}
	}
}

func toInt(value any, fallback int) int {
	switch raw := value.(type) {
	case int:
		return raw
	case int64:
		return int(raw)
	case float64:
		return int(raw)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			return fallback
		}
		return parsed
	default:
		return fallback
	}
}

func toBool(value any, fallback bool) bool {
	switch raw := value.(type) {
	case bool:
		return raw
	case string:
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "true", "1", "yes", "y":
			return true
		case "false", "0", "no", "n":
			return false
		default:
			return fallback
		}
	default:
		return fallback
	}
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
