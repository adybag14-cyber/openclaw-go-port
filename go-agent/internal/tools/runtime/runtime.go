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
	EndpointByProvider   map[string]string
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
	normalizedByProvider := map[string]string{}
	for rawProvider, endpoint := range out.EndpointByProvider {
		provider := normalizeBrowserProviderAlias(rawProvider)
		trimmedEndpoint := strings.TrimSpace(endpoint)
		if provider == "" || trimmedEndpoint == "" {
			continue
		}
		normalizedByProvider[provider] = trimmedEndpoint
	}
	out.EndpointByProvider = normalizedByProvider
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
	messages      []map[string]any
	messageIndex  map[string]map[string]any
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
		jobs:         map[string]map[string]any{},
		messages:     make([]map[string]any, 0, 1024),
		messageIndex: map[string]map[string]any{},
		browser:      normalizeBrowserBridgeOptions(options),
		httpClient:   &http.Client{},
	}
}

func (b *BuiltinBridgeProvider) Name() string {
	return "builtin-bridge"
}

func (b *BuiltinBridgeProvider) Supports(tool string) bool {
	switch strings.ToLower(strings.TrimSpace(tool)) {
	case "read",
		"write",
		"edit",
		"apply_patch",
		"exec",
		"process",
		"gateway",
		"sessions",
		"message",
		"browser",
		"canvas",
		"nodes",
		"wasm",
		"routines",
		"system":
		return true
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
	case "read",
		"write",
		"edit",
		"apply_patch",
		"exec",
		"process",
		"gateway",
		"sessions",
		"message",
		"browser",
		"canvas",
		"nodes",
		"wasm",
		"routines",
		"system":
		return b.invokeToolFamily(ctx, req)
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
		return b.handleMessageSend(req.Input, req.SessionID), nil
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

func (b *BuiltinBridgeProvider) invokeToolFamily(ctx context.Context, req Request) (any, error) {
	switch req.Tool {
	case "read":
		return b.Invoke(ctx, Request{Tool: "file.read", SessionID: req.SessionID, Input: req.Input})
	case "write":
		return b.Invoke(ctx, Request{Tool: "file.write", SessionID: req.SessionID, Input: req.Input})
	case "edit", "apply_patch":
		return b.Invoke(ctx, Request{Tool: "file.patch", SessionID: req.SessionID, Input: req.Input})
	case "exec":
		return b.Invoke(ctx, Request{Tool: "exec.run", SessionID: req.SessionID, Input: req.Input})
	case "process", "system":
		action := normalizeToolAction(req.Input, "run")
		switch action {
		case "run", "exec":
			return b.Invoke(ctx, Request{Tool: "exec.run", SessionID: req.SessionID, Input: req.Input})
		case "start", "background-start", "background":
			return b.Invoke(ctx, Request{Tool: "task.background.start", SessionID: req.SessionID, Input: req.Input})
		case "poll", "background-poll":
			return b.Invoke(ctx, Request{Tool: "task.background.poll", SessionID: req.SessionID, Input: req.Input})
		default:
			return nil, fmt.Errorf("unsupported process action %q", action)
		}
	case "browser":
		action := normalizeToolAction(req.Input, "request")
		switch action {
		case "open":
			return b.Invoke(ctx, Request{Tool: "browser.open", SessionID: req.SessionID, Input: req.Input})
		case "request", "send", "completion":
			return b.Invoke(ctx, Request{Tool: "browser.request", SessionID: req.SessionID, Input: req.Input})
		default:
			return nil, fmt.Errorf("unsupported browser action %q", action)
		}
	case "nodes":
		action := normalizeToolAction(req.Input, "invoke")
		switch action {
		case "invoke", "run":
			return b.Invoke(ctx, Request{Tool: "node.invoke", SessionID: req.SessionID, Input: req.Input})
		default:
			return nil, fmt.Errorf("unsupported nodes action %q", action)
		}
	case "message":
		action := normalizeToolAction(req.Input, "send")
		switch action {
		case "send", "append":
			return b.handleMessageSend(req.Input, req.SessionID), nil
		case "poll":
			return b.handleMessagePoll(req.Input), nil
		case "read":
			return b.handleMessageRead(req.Input)
		case "edit":
			return b.handleMessageEdit(req.Input)
		case "delete", "remove":
			return b.handleMessageDelete(req.Input)
		case "react", "reaction":
			return b.handleMessageReact(req.Input)
		case "reactions":
			return b.handleMessageReactions(req.Input)
		case "search":
			return b.handleMessageSearch(req.Input), nil
		default:
			return nil, fmt.Errorf("unsupported message action %q", action)
		}
	case "sessions":
		action := normalizeToolAction(req.Input, "list")
		switch action {
		case "list":
			return b.handleSessionsList(), nil
		case "history":
			return b.handleSessionsHistory(req.Input), nil
		case "reset":
			return b.handleSessionsReset(req.Input), nil
		case "usage":
			return b.handleSessionsUsage(req.Input), nil
		default:
			return nil, fmt.Errorf("unsupported sessions action %q", action)
		}
	case "gateway":
		action := normalizeToolAction(req.Input, "status")
		switch action {
		case "status", "health":
			return map[string]any{
				"status":       200,
				"ok":           true,
				"service":      "openclaw-go-runtime",
				"jobCount":     b.jobCount(),
				"messageCount": b.messageCount(),
				"time":         time.Now().UTC().Format(time.RFC3339),
			}, nil
		default:
			return nil, fmt.Errorf("unsupported gateway action %q", action)
		}
	case "canvas":
		action := normalizeToolAction(req.Input, "present")
		switch action {
		case "present", "push":
			frameRef := toString(req.Input["frameRef"], "canvas://latest")
			return map[string]any{
				"status":      200,
				"ok":          true,
				"frameRef":    frameRef,
				"presentedAt": time.Now().UTC().Format(time.RFC3339),
			}, nil
		default:
			return nil, fmt.Errorf("unsupported canvas action %q", action)
		}
	case "wasm":
		action := normalizeToolAction(req.Input, "inspect")
		switch action {
		case "inspect", "list":
			return map[string]any{
				"status":       200,
				"ok":           true,
				"runtimeMode":  "wazero",
				"capabilities": []string{"workspace.read", "workspace.write", "exec"},
				"action":       action,
			}, nil
		case "execute", "run":
			return map[string]any{
				"status":      200,
				"ok":          true,
				"action":      "execute",
				"module":      toString(req.Input["module"], ""),
				"entrypoint":  toString(req.Input["entrypoint"], toString(req.Input["export"], "run")),
				"executedAt":  time.Now().UTC().Format(time.RFC3339),
				"runtimeMode": "wazero",
			}, nil
		default:
			return nil, fmt.Errorf("unsupported wasm action %q", action)
		}
	case "routines":
		action := normalizeToolAction(req.Input, "list")
		switch action {
		case "list":
			return map[string]any{
				"status":   200,
				"ok":       true,
				"count":    0,
				"routines": []map[string]any{},
			}, nil
		case "run", "execute":
			return map[string]any{
				"status":     200,
				"ok":         true,
				"routine":    toString(req.Input["name"], toString(req.Input["routine"], "ad-hoc")),
				"state":      "completed",
				"executedAt": time.Now().UTC().Format(time.RFC3339),
			}, nil
		default:
			return nil, fmt.Errorf("unsupported routines action %q", action)
		}
	default:
		return nil, fmt.Errorf("unsupported tool family %q", req.Tool)
	}
}

func normalizeToolAction(input map[string]any, fallback string) string {
	action := strings.ToLower(strings.TrimSpace(toString(input["action"], toString(input["op"], toString(input["method"], fallback)))))
	if action == "" {
		action = strings.ToLower(strings.TrimSpace(fallback))
	}
	return strings.ReplaceAll(action, "_", "-")
}

func (b *BuiltinBridgeProvider) handleMessageSend(input map[string]any, sessionID string) map[string]any {
	channel := toString(input["channel"], "webchat")
	to := toString(input["to"], "")
	message := toString(input["message"], toString(input["text"], ""))
	stored := b.storeMessage(channel, to, message, sessionID)
	return map[string]any{
		"status":  200,
		"ok":      true,
		"channel": channel,
		"to":      to,
		"message": message,
		"entry":   stored,
	}
}

func (b *BuiltinBridgeProvider) handleMessagePoll(input map[string]any) map[string]any {
	limit := toInt(input["limit"], 50)
	if limit <= 0 {
		limit = 50
	}
	channelFilter := strings.ToLower(strings.TrimSpace(toString(input["channel"], "")))
	sessionFilter := strings.TrimSpace(toString(input["sessionId"], toString(input["session_id"], "")))

	b.mu.RLock()
	filtered := make([]map[string]any, 0, limit)
	for i := len(b.messages) - 1; i >= 0 && len(filtered) < limit; i-- {
		entry := b.messages[i]
		if channelFilter != "" && strings.ToLower(toString(entry["channel"], "")) != channelFilter {
			continue
		}
		if sessionFilter != "" && strings.TrimSpace(toString(entry["sessionId"], "")) != sessionFilter {
			continue
		}
		filtered = append(filtered, cloneMap(entry))
	}
	b.mu.RUnlock()
	reverseMapSlice(filtered)
	return map[string]any{
		"status": 200,
		"ok":     true,
		"count":  len(filtered),
		"items":  filtered,
	}
}

func (b *BuiltinBridgeProvider) handleMessageRead(input map[string]any) (any, error) {
	messageID := strings.TrimSpace(toString(input["messageId"], toString(input["id"], "")))
	if messageID == "" {
		return nil, errors.New("message.read requires messageId")
	}
	b.mu.RLock()
	entry, ok := b.messageIndex[messageID]
	b.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("message %q not found", messageID)
	}
	return map[string]any{
		"status":  200,
		"ok":      true,
		"message": cloneMap(entry),
	}, nil
}

func (b *BuiltinBridgeProvider) handleMessageEdit(input map[string]any) (any, error) {
	messageID := strings.TrimSpace(toString(input["messageId"], toString(input["id"], "")))
	if messageID == "" {
		return nil, errors.New("message.edit requires messageId")
	}
	updatedText := toString(input["message"], toString(input["text"], ""))
	if updatedText == "" {
		return nil, errors.New("message.edit requires message/text")
	}

	b.mu.Lock()
	entry, ok := b.messageIndex[messageID]
	if !ok {
		b.mu.Unlock()
		return nil, fmt.Errorf("message %q not found", messageID)
	}
	entry["message"] = updatedText
	entry["text"] = updatedText
	entry["edited"] = true
	entry["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	cloned := cloneMap(entry)
	b.mu.Unlock()
	return map[string]any{
		"status":  200,
		"ok":      true,
		"message": cloned,
	}, nil
}

func (b *BuiltinBridgeProvider) handleMessageDelete(input map[string]any) (any, error) {
	messageID := strings.TrimSpace(toString(input["messageId"], toString(input["id"], "")))
	if messageID == "" {
		return nil, errors.New("message.delete requires messageId")
	}
	b.mu.Lock()
	_, ok := b.messageIndex[messageID]
	if ok {
		delete(b.messageIndex, messageID)
		for i := len(b.messages) - 1; i >= 0; i-- {
			if toString(b.messages[i]["id"], "") == messageID {
				b.messages = append(b.messages[:i], b.messages[i+1:]...)
				break
			}
		}
	}
	b.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("message %q not found", messageID)
	}
	return map[string]any{
		"status":    200,
		"ok":        true,
		"deleted":   true,
		"messageId": messageID,
	}, nil
}

func (b *BuiltinBridgeProvider) handleMessageReact(input map[string]any) (any, error) {
	messageID := strings.TrimSpace(toString(input["messageId"], toString(input["id"], "")))
	if messageID == "" {
		return nil, errors.New("message.react requires messageId")
	}
	reaction := toString(input["reaction"], "")
	if reaction == "" {
		return nil, errors.New("message.react requires reaction")
	}

	b.mu.Lock()
	entry, ok := b.messageIndex[messageID]
	if !ok {
		b.mu.Unlock()
		return nil, fmt.Errorf("message %q not found", messageID)
	}
	reactions := toStringSlice(entry["reactions"])
	reactions = append(reactions, reaction)
	entry["reactions"] = reactions
	entry["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	cloned := cloneMap(entry)
	b.mu.Unlock()
	return map[string]any{
		"status":    200,
		"ok":        true,
		"messageId": messageID,
		"reactions": reactions,
		"message":   cloned,
	}, nil
}

func (b *BuiltinBridgeProvider) handleMessageReactions(input map[string]any) (any, error) {
	messageID := strings.TrimSpace(toString(input["messageId"], toString(input["id"], "")))
	if messageID == "" {
		return nil, errors.New("message.reactions requires messageId")
	}
	b.mu.RLock()
	entry, ok := b.messageIndex[messageID]
	b.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("message %q not found", messageID)
	}
	return map[string]any{
		"status":    200,
		"ok":        true,
		"messageId": messageID,
		"reactions": toStringSlice(entry["reactions"]),
	}, nil
}

func (b *BuiltinBridgeProvider) handleMessageSearch(input map[string]any) map[string]any {
	query := strings.ToLower(strings.TrimSpace(toString(input["query"], toString(input["text"], ""))))
	channelFilter := strings.ToLower(strings.TrimSpace(toString(input["channel"], "")))
	limit := toInt(input["limit"], 25)
	if limit <= 0 {
		limit = 25
	}

	b.mu.RLock()
	hits := make([]map[string]any, 0, limit)
	for i := len(b.messages) - 1; i >= 0 && len(hits) < limit; i-- {
		entry := b.messages[i]
		if channelFilter != "" && strings.ToLower(toString(entry["channel"], "")) != channelFilter {
			continue
		}
		message := strings.ToLower(toString(entry["message"], ""))
		if query != "" && !strings.Contains(message, query) {
			continue
		}
		hits = append(hits, cloneMap(entry))
	}
	b.mu.RUnlock()
	reverseMapSlice(hits)
	return map[string]any{
		"status": 200,
		"ok":     true,
		"query":  query,
		"count":  len(hits),
		"items":  hits,
	}
}

func (b *BuiltinBridgeProvider) handleSessionsList() map[string]any {
	b.mu.RLock()
	stats := map[string]map[string]any{}
	for _, entry := range b.messages {
		sessionID := strings.TrimSpace(toString(entry["sessionId"], ""))
		if sessionID == "" {
			continue
		}
		row, ok := stats[sessionID]
		if !ok {
			row = map[string]any{
				"sessionId": sessionID,
				"count":     0,
				"channel":   toString(entry["channel"], ""),
				"lastAt":    toString(entry["createdAt"], ""),
			}
			stats[sessionID] = row
		}
		row["count"] = toInt(row["count"], 0) + 1
		row["lastAt"] = toString(entry["createdAt"], toString(row["lastAt"], ""))
	}
	b.mu.RUnlock()

	items := make([]map[string]any, 0, len(stats))
	for _, item := range stats {
		items = append(items, cloneMap(item))
	}
	sort.Slice(items, func(i, j int) bool {
		return toString(items[i]["sessionId"], "") < toString(items[j]["sessionId"], "")
	})
	return map[string]any{
		"status": 200,
		"ok":     true,
		"count":  len(items),
		"items":  items,
	}
}

func (b *BuiltinBridgeProvider) handleSessionsHistory(input map[string]any) map[string]any {
	sessionID := strings.TrimSpace(toString(input["sessionId"], toString(input["session_id"], "")))
	limit := toInt(input["limit"], 50)
	if limit <= 0 {
		limit = 50
	}
	items := b.messagesBySession(sessionID, limit)
	return map[string]any{
		"status":    200,
		"ok":        true,
		"sessionId": sessionID,
		"count":     len(items),
		"items":     items,
	}
}

func (b *BuiltinBridgeProvider) handleSessionsReset(input map[string]any) map[string]any {
	sessionID := strings.TrimSpace(toString(input["sessionId"], toString(input["session_id"], "")))
	if sessionID == "" {
		return map[string]any{
			"status": 400,
			"ok":     false,
			"error":  "sessions.reset requires sessionId",
		}
	}

	removed := 0
	b.mu.Lock()
	if len(b.messages) > 0 {
		kept := make([]map[string]any, 0, len(b.messages))
		for _, entry := range b.messages {
			if strings.TrimSpace(toString(entry["sessionId"], "")) == sessionID {
				removed++
				delete(b.messageIndex, toString(entry["id"], ""))
				continue
			}
			kept = append(kept, entry)
		}
		b.messages = kept
	}
	b.mu.Unlock()
	return map[string]any{
		"status":         200,
		"ok":             true,
		"sessionId":      sessionID,
		"removedEntries": removed,
	}
}

func (b *BuiltinBridgeProvider) handleSessionsUsage(input map[string]any) map[string]any {
	sessionID := strings.TrimSpace(toString(input["sessionId"], toString(input["session_id"], "")))
	items := b.messagesBySession(sessionID, toInt(input["limit"], 5000))
	tokenCount := 0
	for _, item := range items {
		tokenCount += len(strings.Fields(toString(item["message"], "")))
	}
	return map[string]any{
		"status":    200,
		"ok":        true,
		"sessionId": sessionID,
		"messages":  len(items),
		"tokens":    tokenCount,
	}
}

func (b *BuiltinBridgeProvider) storeMessage(channel, to, message, sessionID string) map[string]any {
	now := time.Now().UTC().Format(time.RFC3339)
	id := fmt.Sprintf("msg-%06d", b.seq.Add(1))
	entry := map[string]any{
		"id":        id,
		"channel":   strings.ToLower(strings.TrimSpace(valueOr(channel, "webchat"))),
		"to":        strings.TrimSpace(to),
		"message":   message,
		"text":      message,
		"sessionId": strings.TrimSpace(sessionID),
		"createdAt": now,
		"updatedAt": now,
		"reactions": []string{},
	}
	b.mu.Lock()
	b.messages = append(b.messages, cloneMap(entry))
	if len(b.messages) > 5000 {
		oldest := b.messages[0]
		delete(b.messageIndex, toString(oldest["id"], ""))
		b.messages = b.messages[1:]
	}
	b.messageIndex[id] = cloneMap(entry)
	b.mu.Unlock()
	return entry
}

func (b *BuiltinBridgeProvider) messagesBySession(sessionID string, limit int) []map[string]any {
	if limit <= 0 {
		limit = 50
	}
	sid := strings.TrimSpace(sessionID)
	b.mu.RLock()
	items := make([]map[string]any, 0, limit)
	for i := len(b.messages) - 1; i >= 0 && len(items) < limit; i-- {
		entry := b.messages[i]
		if sid != "" && strings.TrimSpace(toString(entry["sessionId"], "")) != sid {
			continue
		}
		items = append(items, cloneMap(entry))
	}
	b.mu.RUnlock()
	reverseMapSlice(items)
	return items
}

func (b *BuiltinBridgeProvider) jobCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.jobs)
}

func (b *BuiltinBridgeProvider) messageCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.messages)
}

func reverseMapSlice(items []map[string]any) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}

func valueOr(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func (b *BuiltinBridgeProvider) Catalog() []ToolSpec {
	return []ToolSpec{
		{Tool: "apply_patch", Provider: b.Name(), Description: "Alias for file patch operation"},
		{Tool: "browser", Provider: b.Name(), Description: "Browser tool family with open/request actions"},
		{Tool: "browser.open", Provider: b.Name(), Description: "Open browser URL through bridge runtime"},
		{Tool: "browser.request", Provider: b.Name(), Description: "Send browser bridge request or completion payload"},
		{Tool: "canvas", Provider: b.Name(), Description: "Canvas tool family for present/push actions"},
		{Tool: "edit", Provider: b.Name(), Description: "Alias for file patch operation"},
		{Tool: "exec", Provider: b.Name(), Description: "Exec tool family alias for command execution"},
		{Tool: "exec.run", Provider: b.Name(), Description: "Execute local process command with timeout"},
		{Tool: "file.patch", Provider: b.Name(), Description: "Apply text replacement patch to file"},
		{Tool: "file.read", Provider: b.Name(), Description: "Read local file content"},
		{Tool: "file.write", Provider: b.Name(), Description: "Write local file content"},
		{Tool: "gateway", Provider: b.Name(), Description: "Gateway runtime status and control family"},
		{Tool: "message", Provider: b.Name(), Description: "Message tool family (send/poll/read/edit/delete/react/search)"},
		{Tool: "message.send", Provider: b.Name(), Description: "Send message through runtime bridge"},
		{Tool: "nodes", Provider: b.Name(), Description: "Nodes tool family alias for node operations"},
		{Tool: "node.invoke", Provider: b.Name(), Description: "Invoke node operation through runtime bridge"},
		{Tool: "process", Provider: b.Name(), Description: "Process tool family (run/start/poll)"},
		{Tool: "read", Provider: b.Name(), Description: "Alias for file.read"},
		{Tool: "routines", Provider: b.Name(), Description: "Routines family for list/run actions"},
		{Tool: "sessions", Provider: b.Name(), Description: "Sessions tool family (list/history/reset/usage)"},
		{Tool: "system", Provider: b.Name(), Description: "System tool family alias for process actions"},
		{Tool: "task.background.poll", Provider: b.Name(), Description: "Poll background task state"},
		{Tool: "task.background.start", Provider: b.Name(), Description: "Start background task execution"},
		{Tool: "tool.echo", Provider: b.Name(), Description: "Echo request payload for smoke validation"},
		{Tool: "wasm", Provider: b.Name(), Description: "WASM tool family (inspect/list/execute)"},
		{Tool: "write", Provider: b.Name(), Description: "Alias for file.write"},
	}
}

func (b *BuiltinBridgeProvider) handleBrowserRequest(ctx context.Context, input map[string]any) (any, error) {
	payload, hasCompletionPayload := toBrowserCompletionPayload(input)
	if !hasCompletionPayload {
		url := toString(input["url"], "")
		method := strings.ToUpper(toString(input["method"], "GET"))
		provider := normalizeBrowserProviderAlias(toString(input["provider"], "chatgpt"))
		return map[string]any{
			"status":   200,
			"ok":       true,
			"url":      url,
			"method":   method,
			"provider": provider,
			"response": "bridge request accepted",
		}, nil
	}
	if !b.browser.Enabled {
		return nil, errors.New("browser bridge is disabled")
	}
	return b.invokeBrowserCompletion(ctx, payload)
}

func toBrowserCompletionPayload(input map[string]any) (map[string]any, bool) {
	provider := normalizeBrowserProviderAlias(toString(input["provider"], "chatgpt"))
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
		"provider": provider,
		"model":    model,
		"messages": messages,
	}
	if value, ok := input["temperature"]; ok {
		payload["temperature"] = value
	}
	if value, ok := input["max_tokens"]; ok {
		payload["max_tokens"] = value
	}
	if loginSessionID := strings.TrimSpace(toString(input["loginSessionId"], toString(input["login_session_id"], ""))); loginSessionID != "" {
		payload["loginSessionId"] = loginSessionID
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

func normalizeBrowserProviderAlias(provider string) string {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	switch normalized {
	case "", "openai", "openai-chatgpt", "chatgpt-web", "chatgpt.com":
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
	case "z.ai", "z-ai", "zaiweb", "zai-web", "glm", "glm5", "glm-5":
		return "zai"
	case "inception-labs", "inceptionlabs", "mercury", "mercury2", "mercury-2":
		return "inception"
	default:
		return normalized
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
	provider := normalizeBrowserProviderAlias(toString(payload["provider"], "chatgpt"))
	endpoint := b.browserEndpointForProvider(provider)
	if strings.TrimSpace(endpoint) == "" {
		return nil, errors.New("browser bridge endpoint is empty")
	}
	if allowed, wait := b.allowBridgeRequest(); !allowed {
		return nil, fmt.Errorf("browser bridge circuit breaker open (retry in %s)", wait.Round(time.Millisecond).String())
	}

	lastErr := error(nil)
	maxAttempts := b.browser.Retries + 1
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		response, statusCode, err := b.postBridgeCompletion(ctx, payload, endpoint)
		if err == nil {
			assistant := extractAssistantMessage(response)
			b.recordBridgeSuccess()
			return map[string]any{
				"status":        200,
				"ok":            true,
				"provider":      provider,
				"bridge":        "browser",
				"endpoint":      endpoint,
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

func (b *BuiltinBridgeProvider) browserEndpointForProvider(provider string) string {
	canonical := normalizeBrowserProviderAlias(provider)
	if endpoint, ok := b.browser.EndpointByProvider[canonical]; ok {
		trimmed := strings.TrimSpace(endpoint)
		if trimmed != "" {
			return trimmed
		}
	}
	return strings.TrimSpace(b.browser.Endpoint)
}

func (b *BuiltinBridgeProvider) postBridgeCompletion(ctx context.Context, payload map[string]any, endpoint string) (map[string]any, int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}

	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/") + "/v1/chat/completions"
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
