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
			"model": "gpt-5.2",
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
