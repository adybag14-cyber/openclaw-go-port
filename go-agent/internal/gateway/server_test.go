package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/buildinfo"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/config"
)

func TestHealthEndpoint(t *testing.T) {
	s := New(config.Default(), buildinfo.Info{
		Service: "openclaw-go",
		Version: "test",
		Commit:  "abc123",
		BuiltAt: "now",
	})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode health response failed: %v", err)
	}

	if payload["status"] != "ok" {
		t.Fatalf("unexpected health status: %v", payload["status"])
	}
	if payload["version"] != "test" {
		t.Fatalf("unexpected version: %v", payload["version"])
	}
}

func TestRPCStubReturnsMethodNotFound(t *testing.T) {
	s := New(config.Default(), buildinfo.Default())
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	reqBody := []byte(`{"type":"req","id":"1","method":"models.list","params":{}}`)
	resp, err := http.Post(ts.URL+"/rpc", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /rpc failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode rpc response failed: %v", err)
	}

	errObj, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("error object missing in rpc response")
	}
	if got := int(errObj["code"].(float64)); got != -32601 {
		t.Fatalf("unexpected error code: %d", got)
	}

	details, ok := errObj["details"].(map[string]any)
	if !ok {
		t.Fatalf("expected details in rpc error")
	}
	if details["canonical"] != "models.list" {
		t.Fatalf("unexpected canonical value: %v", details["canonical"])
	}
}

func TestRPCHealthMethodReturnsSuccessEnvelope(t *testing.T) {
	s := New(config.Default(), buildinfo.Default())
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	reqBody := []byte(`{"type":"req","id":"2","method":"health","params":{}}`)
	resp, err := http.Post(ts.URL+"/rpc", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /rpc failed: %v", err)
	}
	defer resp.Body.Close()

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode rpc response failed: %v", err)
	}

	if payload["type"] != "resp" {
		t.Fatalf("unexpected frame type: %v", payload["type"])
	}
	okRaw, ok := payload["ok"].(bool)
	if !ok || !okRaw {
		t.Fatalf("expected ok=true in response envelope")
	}
	result, ok := payload["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result object in response")
	}
	if result["status"] != "ok" {
		t.Fatalf("expected health result status ok, got %v", result["status"])
	}
}

func TestRPCAliasCanonicalizationInErrorDetails(t *testing.T) {
	s := New(config.Default(), buildinfo.Default())
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	reqBody := []byte(`{"type":"req","id":"3","method":"exec.approval.waitDecision","params":{}}`)
	resp, err := http.Post(ts.URL+"/rpc", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /rpc failed: %v", err)
	}
	defer resp.Body.Close()

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode rpc response failed: %v", err)
	}
	errObj, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object in rpc response")
	}
	details, ok := errObj["details"].(map[string]any)
	if !ok {
		t.Fatalf("expected details in rpc error")
	}
	if details["canonical"] != "exec.approval.waitdecision" {
		t.Fatalf("unexpected canonical alias mapping: %v", details["canonical"])
	}
	if details["known"] != true {
		t.Fatalf("expected known=true for aliased supported method")
	}
}
