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

	reqBody := []byte(`{"jsonrpc":"2.0","id":"1","method":"models.list","params":{}}`)
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
}
