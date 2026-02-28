package runtime

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
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

func shellCommand() string {
	if runtime.GOOS == "windows" {
		return "cmd"
	}
	return "sh"
}

func shellArgs(script string) []any {
	if runtime.GOOS == "windows" {
		return []any{"/C", script}
	}
	return []any{"-lc", script}
}
