package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDefaultCatalogAndInvoke(t *testing.T) {
	rt := NewDefault()
	catalog := rt.Catalog()
	if len(catalog) < 3 {
		t.Fatalf("expected default catalog entries, got %d", len(catalog))
	}

	result, err := rt.Invoke(context.Background(), Request{
		Tool: "browser.request",
		Input: map[string]any{
			"url":    "https://example.com",
			"method": "post",
		},
	})
	if err != nil {
		t.Fatalf("invoke failed: %v", err)
	}
	if result.Provider != "builtin-bridge" {
		t.Fatalf("unexpected provider: %s", result.Provider)
	}

	output, ok := result.Output.(map[string]any)
	if !ok {
		t.Fatalf("unexpected output type: %T", result.Output)
	}
	if output["status"] != 200 {
		t.Fatalf("unexpected status output: %v", output["status"])
	}
	if output["provider"] != "chatgpt" {
		t.Fatalf("expected default provider chatgpt, got %v", output["provider"])
	}
}

func TestInvokeUnknownTool(t *testing.T) {
	rt := NewDefault()
	_, err := rt.Invoke(context.Background(), Request{
		Tool: "does.not.exist",
	})
	if err == nil {
		t.Fatalf("expected error for unknown tool")
	}
}

func TestExecRunTool(t *testing.T) {
	rt := NewDefault()
	result, err := rt.Invoke(context.Background(), Request{
		Tool: "exec.run",
		Input: map[string]any{
			"command": shellCommand(),
			"args":    shellArgs("echo openclaw-runtime"),
		},
	})
	if err != nil {
		t.Fatalf("exec.run invoke failed: %v", err)
	}
	output, ok := result.Output.(map[string]any)
	if !ok {
		t.Fatalf("unexpected output type: %T", result.Output)
	}
	if okValue, _ := output["ok"].(bool); !okValue {
		t.Fatalf("exec.run expected ok=true, got %v", output)
	}
	stdout, _ := output["stdout"].(string)
	if !strings.Contains(stdout, "openclaw-runtime") {
		t.Fatalf("exec.run stdout mismatch: %q", stdout)
	}
}

func TestFileWriteReadPatchTools(t *testing.T) {
	rt := NewDefault()
	path := filepath.Join(t.TempDir(), "runtime-tools.txt")

	_, err := rt.Invoke(context.Background(), Request{
		Tool: "file.write",
		Input: map[string]any{
			"path":    path,
			"content": "alpha beta gamma",
		},
	})
	if err != nil {
		t.Fatalf("file.write failed: %v", err)
	}

	readResult, err := rt.Invoke(context.Background(), Request{
		Tool:  "file.read",
		Input: map[string]any{"path": path},
	})
	if err != nil {
		t.Fatalf("file.read failed: %v", err)
	}
	readOutput, _ := readResult.Output.(map[string]any)
	if readOutput["content"] != "alpha beta gamma" {
		t.Fatalf("file.read content mismatch: %v", readOutput["content"])
	}

	_, err = rt.Invoke(context.Background(), Request{
		Tool: "file.patch",
		Input: map[string]any{
			"path":    path,
			"oldText": "beta",
			"newText": "delta",
		},
	})
	if err != nil {
		t.Fatalf("file.patch failed: %v", err)
	}

	afterPatch, err := rt.Invoke(context.Background(), Request{
		Tool:  "file.read",
		Input: map[string]any{"path": path},
	})
	if err != nil {
		t.Fatalf("file.read after patch failed: %v", err)
	}
	afterPatchOutput, _ := afterPatch.Output.(map[string]any)
	if afterPatchOutput["content"] != "alpha delta gamma" {
		t.Fatalf("file.patch content mismatch: %v", afterPatchOutput["content"])
	}
}

func TestBackgroundTaskStartAndPoll(t *testing.T) {
	rt := NewDefault()
	startResult, err := rt.Invoke(context.Background(), Request{
		Tool: "task.background.start",
		Input: map[string]any{
			"command": shellCommand(),
			"args":    shellArgs("echo bg-runtime"),
		},
	})
	if err != nil {
		t.Fatalf("task.background.start failed: %v", err)
	}
	startOutput, _ := startResult.Output.(map[string]any)
	jobID, _ := startOutput["jobId"].(string)
	if strings.TrimSpace(jobID) == "" {
		t.Fatalf("missing jobId from background start: %v", startOutput)
	}

	var pollOutput map[string]any
	for i := 0; i < 20; i++ {
		pollResult, pollErr := rt.Invoke(context.Background(), Request{
			Tool: "task.background.poll",
			Input: map[string]any{
				"jobId": jobID,
			},
		})
		if pollErr != nil {
			t.Fatalf("task.background.poll failed: %v", pollErr)
		}
		pollOutput, _ = pollResult.Output.(map[string]any)
		state, _ := pollOutput["state"].(string)
		if state == "completed" || state == "failed" || state == "timeout" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	state, _ := pollOutput["state"].(string)
	if state == "running" || state == "" {
		t.Fatalf("background task did not complete in expected window: %v", pollOutput)
	}
}

func TestBrowserRequestCompletionUsesBridgeEndpoint(t *testing.T) {
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request payload failed: %v", err)
		}
		if payload["model"] != "gpt-5.2" {
			t.Fatalf("unexpected model: %v", payload["model"])
		}
		if payload["provider"] != "qwen" {
			t.Fatalf("unexpected provider: %v", payload["provider"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cmpl-1","model":"gpt-5.2","choices":[{"message":{"role":"assistant","content":"bridge-response"}}]}`))
	}))
	defer bridge.Close()

	rt := NewDefaultWithOptions(RuntimeOptions{
		BrowserBridge: BrowserBridgeOptions{
			Enabled:              true,
			Endpoint:             bridge.URL,
			RequestTimeout:       3 * time.Second,
			Retries:              0,
			RetryBackoff:         0,
			CircuitFailThreshold: 3,
			CircuitCooldown:      3 * time.Second,
		},
	})

	result, err := rt.Invoke(context.Background(), Request{
		Tool: "browser.request",
		Input: map[string]any{
			"provider": "copaw",
			"model":    "gpt-5.2",
			"messages": []map[string]any{
				{"role": "user", "content": "hello"},
			},
		},
	})
	if err != nil {
		t.Fatalf("browser.request completion invoke failed: %v", err)
	}

	output, ok := result.Output.(map[string]any)
	if !ok {
		t.Fatalf("unexpected output type: %T", result.Output)
	}
	if output["status"] != 200 {
		t.Fatalf("unexpected status output: %v", output["status"])
	}
	if output["assistantText"] != "bridge-response" {
		t.Fatalf("unexpected assistant text: %v", output["assistantText"])
	}
	if output["provider"] != "qwen" {
		t.Fatalf("expected output provider qwen, got %v", output["provider"])
	}
}

func TestBrowserRequestCompletionForwardsAPIKey(t *testing.T) {
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request payload failed: %v", err)
		}
		if payload["provider"] != "openrouter" {
			t.Fatalf("unexpected provider: %v", payload["provider"])
		}
		if payload["apiKey"] != "sk-or-test" {
			t.Fatalf("expected apiKey to be forwarded, got %v", payload["apiKey"])
		}
		if payload["api_key"] != "sk-or-test" {
			t.Fatalf("expected api_key to be forwarded, got %v", payload["api_key"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cmpl-key","model":"openrouter/auto","choices":[{"message":{"role":"assistant","content":"key-ok"}}]}`))
	}))
	defer bridge.Close()

	rt := NewDefaultWithOptions(RuntimeOptions{
		BrowserBridge: BrowserBridgeOptions{
			Enabled:              true,
			Endpoint:             bridge.URL,
			RequestTimeout:       3 * time.Second,
			Retries:              0,
			RetryBackoff:         0,
			CircuitFailThreshold: 3,
			CircuitCooldown:      3 * time.Second,
		},
	})

	result, err := rt.Invoke(context.Background(), Request{
		Tool: "browser.request",
		Input: map[string]any{
			"provider": "openrouter",
			"model":    "openrouter/auto",
			"apiKey":   "sk-or-test",
			"messages": []map[string]any{
				{"role": "user", "content": "hello"},
			},
		},
	})
	if err != nil {
		t.Fatalf("browser.request invoke failed: %v", err)
	}
	output, _ := result.Output.(map[string]any)
	if output["assistantText"] != "key-ok" {
		t.Fatalf("unexpected assistant text: %v", output["assistantText"])
	}
}

func TestBrowserRequestCompletionRetriesThenSucceeds(t *testing.T) {
	var attempts atomic.Int32
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := attempts.Add(1)
		if count < 3 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"upstream unavailable"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cmpl-2","model":"gpt-5.2","choices":[{"message":{"role":"assistant","content":"recovered"}}]}`))
	}))
	defer bridge.Close()

	rt := NewDefaultWithOptions(RuntimeOptions{
		BrowserBridge: BrowserBridgeOptions{
			Enabled:              true,
			Endpoint:             bridge.URL,
			RequestTimeout:       2 * time.Second,
			Retries:              2,
			RetryBackoff:         10 * time.Millisecond,
			CircuitFailThreshold: 3,
			CircuitCooldown:      2 * time.Second,
		},
	})

	result, err := rt.Invoke(context.Background(), Request{
		Tool: "browser.request",
		Input: map[string]any{
			"model": "gpt-5.2",
			"messages": []map[string]any{
				{"role": "user", "content": "retry please"},
			},
		},
	})
	if err != nil {
		t.Fatalf("browser.request retry invoke failed: %v", err)
	}
	output, _ := result.Output.(map[string]any)
	if output["assistantText"] != "recovered" {
		t.Fatalf("unexpected assistant text after retries: %v", output["assistantText"])
	}
	if output["attempt"] != 3 {
		t.Fatalf("expected attempt=3 after retries, got %v", output["attempt"])
	}
	if attempts.Load() != 3 {
		t.Fatalf("expected bridge to receive 3 requests, got %d", attempts.Load())
	}
}

func TestBrowserRequestCompletionUsesProviderSpecificEndpoint(t *testing.T) {
	var defaultCalls atomic.Int32
	defaultBridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defaultCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cmpl-default","model":"gpt-5.2","choices":[{"message":{"role":"assistant","content":"default"}}]}`))
	}))
	defer defaultBridge.Close()

	var qwenCalls atomic.Int32
	qwenBridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		qwenCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cmpl-qwen","model":"qwen3.5-plus","choices":[{"message":{"role":"assistant","content":"qwen-endpoint"}}]}`))
	}))
	defer qwenBridge.Close()

	rt := NewDefaultWithOptions(RuntimeOptions{
		BrowserBridge: BrowserBridgeOptions{
			Enabled:            true,
			Endpoint:           defaultBridge.URL,
			EndpointByProvider: map[string]string{"qwen": qwenBridge.URL},
			RequestTimeout:     3 * time.Second,
			Retries:            0,
			RetryBackoff:       0,
			CircuitCooldown:    3 * time.Second,
		},
	})

	result, err := rt.Invoke(context.Background(), Request{
		Tool: "browser.request",
		Input: map[string]any{
			"provider": "copaw",
			"model":    "qwen3.5-plus",
			"messages": []map[string]any{
				{"role": "user", "content": "hello"},
			},
		},
	})
	if err != nil {
		t.Fatalf("browser.request provider endpoint invoke failed: %v", err)
	}

	output, ok := result.Output.(map[string]any)
	if !ok {
		t.Fatalf("unexpected output type: %T", result.Output)
	}
	if output["provider"] != "qwen" {
		t.Fatalf("expected output provider qwen, got %v", output["provider"])
	}
	if output["assistantText"] != "qwen-endpoint" {
		t.Fatalf("expected provider-specific endpoint response, got %v", output["assistantText"])
	}
	if qwenCalls.Load() != 1 {
		t.Fatalf("expected qwen endpoint to be called once, got %d", qwenCalls.Load())
	}
	if defaultCalls.Load() != 0 {
		t.Fatalf("expected default endpoint to be unused, got %d calls", defaultCalls.Load())
	}
}

func TestBrowserRequestCircuitBreakerOpens(t *testing.T) {
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"busy"}`))
	}))
	defer bridge.Close()

	rt := NewDefaultWithOptions(RuntimeOptions{
		BrowserBridge: BrowserBridgeOptions{
			Enabled:              true,
			Endpoint:             bridge.URL,
			RequestTimeout:       2 * time.Second,
			Retries:              0,
			RetryBackoff:         0,
			CircuitFailThreshold: 2,
			CircuitCooldown:      30 * time.Second,
		},
	})

	input := map[string]any{
		"messages": []map[string]any{
			{"role": "user", "content": "hello"},
		},
	}

	_, err := rt.Invoke(context.Background(), Request{Tool: "browser.request", Input: input})
	if err == nil {
		t.Fatalf("expected first bridge call to fail")
	}
	_, err = rt.Invoke(context.Background(), Request{Tool: "browser.request", Input: input})
	if err == nil {
		t.Fatalf("expected second bridge call to fail")
	}
	_, err = rt.Invoke(context.Background(), Request{Tool: "browser.request", Input: input})
	if err == nil {
		t.Fatalf("expected third bridge call to fail with open circuit")
	}
	if !strings.Contains(err.Error(), "circuit breaker open") {
		t.Fatalf("expected circuit breaker error, got: %v", err)
	}
}

func TestBrowserRequestDisabledBridgeFailsCompletionPayload(t *testing.T) {
	rt := NewDefaultWithOptions(RuntimeOptions{
		BrowserBridge: BrowserBridgeOptions{
			Enabled:              false,
			Endpoint:             "http://127.0.0.1:1",
			RequestTimeout:       2 * time.Second,
			Retries:              0,
			RetryBackoff:         0,
			CircuitFailThreshold: 2,
			CircuitCooldown:      2 * time.Second,
		},
	})
	_, err := rt.Invoke(context.Background(), Request{
		Tool: "browser.request",
		Input: map[string]any{
			"messages": []map[string]any{
				{"role": "user", "content": "hi"},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "browser bridge is disabled") {
		t.Fatalf("expected disabled bridge error, got: %v", err)
	}
}

func TestToolFamilyAliasesReadWriteEditAndBrowser(t *testing.T) {
	rt := NewDefault()
	path := filepath.Join(t.TempDir(), "tool-family.txt")

	_, err := rt.Invoke(context.Background(), Request{
		Tool: "write",
		Input: map[string]any{
			"path":    path,
			"content": "hello beta",
		},
	})
	if err != nil {
		t.Fatalf("write alias failed: %v", err)
	}
	_, err = rt.Invoke(context.Background(), Request{
		Tool: "edit",
		Input: map[string]any{
			"path":    path,
			"oldText": "beta",
			"newText": "gamma",
		},
	})
	if err != nil {
		t.Fatalf("edit alias failed: %v", err)
	}
	readRes, err := rt.Invoke(context.Background(), Request{
		Tool: "read",
		Input: map[string]any{
			"path": path,
		},
	})
	if err != nil {
		t.Fatalf("read alias failed: %v", err)
	}
	readOut, _ := readRes.Output.(map[string]any)
	if got := toString(readOut["content"], ""); got != "hello gamma" {
		t.Fatalf("unexpected read alias content: %q", got)
	}

	browserRes, err := rt.Invoke(context.Background(), Request{
		Tool: "browser",
		Input: map[string]any{
			"action": "open",
			"url":    "https://example.com",
		},
	})
	if err != nil {
		t.Fatalf("browser family open failed: %v", err)
	}
	browserOut, _ := browserRes.Output.(map[string]any)
	if ok, _ := browserOut["opened"].(bool); !ok {
		t.Fatalf("expected browser open result, got %v", browserOut)
	}
}

func TestMessageAndSessionsFamiliesLifecycle(t *testing.T) {
	rt := NewDefault()

	first, err := rt.Invoke(context.Background(), Request{
		Tool:      "message",
		SessionID: "sess-family",
		Input: map[string]any{
			"action":  "send",
			"channel": "telegram",
			"message": "hello runtime",
		},
	})
	if err != nil {
		t.Fatalf("message send failed: %v", err)
	}
	firstOut, _ := first.Output.(map[string]any)
	entry, _ := firstOut["entry"].(map[string]any)
	messageID := toString(entry["id"], "")
	if messageID == "" {
		t.Fatalf("expected message id in entry: %v", firstOut)
	}

	_, err = rt.Invoke(context.Background(), Request{
		Tool:      "message",
		SessionID: "sess-family",
		Input: map[string]any{
			"action":  "send",
			"channel": "telegram",
			"message": "second payload",
		},
	})
	if err != nil {
		t.Fatalf("second message send failed: %v", err)
	}

	_, err = rt.Invoke(context.Background(), Request{
		Tool: "message",
		Input: map[string]any{
			"action":    "react",
			"messageId": messageID,
			"reaction":  "thumbs_up",
		},
	})
	if err != nil {
		t.Fatalf("message react failed: %v", err)
	}

	readRes, err := rt.Invoke(context.Background(), Request{
		Tool: "message",
		Input: map[string]any{
			"action":    "read",
			"messageId": messageID,
		},
	})
	if err != nil {
		t.Fatalf("message read failed: %v", err)
	}
	readOut, _ := readRes.Output.(map[string]any)
	msgObj, _ := readOut["message"].(map[string]any)
	if got := toString(msgObj["message"], ""); got != "hello runtime" {
		t.Fatalf("unexpected message read payload: %q", got)
	}

	searchRes, err := rt.Invoke(context.Background(), Request{
		Tool: "message",
		Input: map[string]any{
			"action": "search",
			"query":  "second",
		},
	})
	if err != nil {
		t.Fatalf("message search failed: %v", err)
	}
	searchOut, _ := searchRes.Output.(map[string]any)
	if count := toInt(searchOut["count"], 0); count != 1 {
		t.Fatalf("expected 1 search hit, got %v", searchOut["count"])
	}

	usageRes, err := rt.Invoke(context.Background(), Request{
		Tool: "sessions",
		Input: map[string]any{
			"action":    "usage",
			"sessionId": "sess-family",
		},
	})
	if err != nil {
		t.Fatalf("sessions usage failed: %v", err)
	}
	usageOut, _ := usageRes.Output.(map[string]any)
	if messages := toInt(usageOut["messages"], 0); messages != 2 {
		t.Fatalf("expected 2 session messages, got %v", usageOut["messages"])
	}

	resetRes, err := rt.Invoke(context.Background(), Request{
		Tool: "sessions",
		Input: map[string]any{
			"action":    "reset",
			"sessionId": "sess-family",
		},
	})
	if err != nil {
		t.Fatalf("sessions reset failed: %v", err)
	}
	resetOut, _ := resetRes.Output.(map[string]any)
	if removed := toInt(resetOut["removedEntries"], 0); removed != 2 {
		t.Fatalf("expected removedEntries=2, got %v", resetOut["removedEntries"])
	}

	_, err = rt.Invoke(context.Background(), Request{
		Tool: "message",
		Input: map[string]any{
			"action":    "delete",
			"messageId": messageID,
		},
	})
	if err == nil {
		t.Fatalf("expected delete to fail after session reset removed message")
	}
}

func TestGatewayCanvasWasmRoutinesFamilies(t *testing.T) {
	rt := NewDefault()

	gatewayRes, err := rt.Invoke(context.Background(), Request{
		Tool: "gateway",
		Input: map[string]any{
			"action": "status",
		},
	})
	if err != nil {
		t.Fatalf("gateway status failed: %v", err)
	}
	gatewayOut, _ := gatewayRes.Output.(map[string]any)
	if ok, _ := gatewayOut["ok"].(bool); !ok {
		t.Fatalf("gateway status expected ok=true: %v", gatewayOut)
	}

	canvasRes, err := rt.Invoke(context.Background(), Request{
		Tool: "canvas",
		Input: map[string]any{
			"action":   "present",
			"frameRef": "canvas://one",
		},
	})
	if err != nil {
		t.Fatalf("canvas present failed: %v", err)
	}
	canvasOut, _ := canvasRes.Output.(map[string]any)
	if frame := toString(canvasOut["frameRef"], ""); frame != "canvas://one" {
		t.Fatalf("unexpected canvas frameRef: %q", frame)
	}

	wasmRes, err := rt.Invoke(context.Background(), Request{
		Tool: "wasm",
		Input: map[string]any{
			"action": "inspect",
			"module": "sample.wasm",
		},
	})
	if err != nil {
		t.Fatalf("wasm inspect failed: %v", err)
	}
	wasmOut, _ := wasmRes.Output.(map[string]any)
	if mode := toString(wasmOut["runtimeMode"], ""); mode != "wazero" {
		t.Fatalf("unexpected wasm runtime mode: %q", mode)
	}

	routineRes, err := rt.Invoke(context.Background(), Request{
		Tool: "routines",
		Input: map[string]any{
			"action": "run",
			"name":   "nightly-validate",
		},
	})
	if err != nil {
		t.Fatalf("routines run failed: %v", err)
	}
	routineOut, _ := routineRes.Output.(map[string]any)
	if state := toString(routineOut["state"], ""); state != "completed" {
		t.Fatalf("unexpected routines state: %q", state)
	}
}

func TestNormalizeBrowserProviderAliasIncludesCopaw(t *testing.T) {
	if got := normalizeBrowserProviderAlias("copaw"); got != "qwen" {
		t.Fatalf("expected copaw -> qwen, got %q", got)
	}
	if got := normalizeBrowserProviderAlias("glm5"); got != "zai" {
		t.Fatalf("expected glm5 -> zai, got %q", got)
	}
	if got := normalizeBrowserProviderAlias("mercury2"); got != "inception" {
		t.Fatalf("expected mercury2 -> inception, got %q", got)
	}
	if got := normalizeBrowserProviderAlias("openai-codex"); got != "codex" {
		t.Fatalf("expected openai-codex -> codex, got %q", got)
	}
}

func shellCommand() string {
	if runtime.GOOS == "windows" {
		return "cmd"
	}
	return "/bin/sh"
}

func shellArgs(script string) []any {
	if runtime.GOOS == "windows" {
		return []any{"/C", script}
	}
	return []any{"-lc", script}
}
