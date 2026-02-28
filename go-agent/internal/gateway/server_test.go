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
	defer s.Close()

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
}

func TestConnectAuthAndSessionLifecycle(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Server.AuthMode = "token"
	cfg.Gateway.Token = "top-secret-token"
	cfg.Runtime.StatePath = "memory://test-connect"

	s := New(cfg, buildinfo.Default())
	defer s.Close()

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	failConnect := rpcCall(t, ts.URL, map[string]any{
		"type":   "req",
		"id":     "c-fail",
		"method": "connect",
		"params": map[string]any{
			"role": "client",
		},
	})
	assertRPCErrorCode(t, failConnect, -32001)

	connect := rpcCall(t, ts.URL, map[string]any{
		"type":   "req",
		"id":     "c-ok",
		"method": "connect",
		"params": map[string]any{
			"role":   "client",
			"scopes": []string{"operator.read", "operator.write"},
			"client": map[string]any{
				"id": "test-client",
			},
			"auth": map[string]any{
				"token": "top-secret-token",
			},
		},
	})
	result := assertRPCResult(t, connect)
	sessionID, _ := result["sessionId"].(string)
	if sessionID == "" {
		t.Fatalf("missing sessionId on connect response")
	}

	sessionsList := rpcCall(t, ts.URL, map[string]any{
		"type":   "req",
		"id":     "s-list",
		"method": "sessions.list",
		"params": map[string]any{},
	})
	listResult := assertRPCResult(t, sessionsList)
	if count, _ := listResult["count"].(float64); int(count) < 1 {
		t.Fatalf("expected sessions list count >=1, got %v", listResult["count"])
	}

	statusResp := rpcCall(t, ts.URL, map[string]any{
		"type":   "req",
		"id":     "s-status",
		"method": "session.status",
		"params": map[string]any{
			"sessionId": sessionID,
		},
	})
	statusResult := assertRPCResult(t, statusResp)
	sessionObj, ok := statusResult["session"].(map[string]any)
	if !ok {
		t.Fatalf("session.status should include session object")
	}
	if sessionObj["id"] != sessionID {
		t.Fatalf("session id mismatch in session.status response")
	}
}

func TestWebLoginAndBrowserBridgeFlow(t *testing.T) {
	cfg := config.Default()
	cfg.Runtime.StatePath = "memory://test-browser"
	s := New(cfg, buildinfo.Default())
	defer s.Close()

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	start := rpcCall(t, ts.URL, map[string]any{
		"type":   "req",
		"id":     "wl-start",
		"method": "web.login.start",
		"params": map[string]any{
			"provider": "chatgpt",
			"model":    "gpt-5.2",
		},
	})
	startResult := assertRPCResult(t, start)
	loginObj, ok := startResult["login"].(map[string]any)
	if !ok {
		t.Fatalf("web.login.start should include login object")
	}
	loginID, _ := loginObj["loginSessionId"].(string)
	loginCode, _ := loginObj["code"].(string)
	if loginID == "" || loginCode == "" {
		t.Fatalf("login id/code missing in start response")
	}

	waitPending := rpcCall(t, ts.URL, map[string]any{
		"type":   "req",
		"id":     "wl-wait-pending",
		"method": "web.login.wait",
		"params": map[string]any{
			"loginSessionId": loginID,
			"timeoutMs":      10,
		},
	})
	waitPendingResult := assertRPCResult(t, waitPending)
	waitPendingLogin, _ := waitPendingResult["login"].(map[string]any)
	if waitPendingLogin["status"] != "pending" {
		t.Fatalf("expected pending login status, got %v", waitPendingLogin["status"])
	}

	beforeAuth := rpcCall(t, ts.URL, map[string]any{
		"type":   "req",
		"id":     "browser-before-auth",
		"method": "browser.request",
		"params": map[string]any{"url": "https://chatgpt.com"},
	})
	assertRPCErrorCode(t, beforeAuth, -32040)

	complete := rpcCall(t, ts.URL, map[string]any{
		"type":   "req",
		"id":     "oauth-complete",
		"method": "auth.oauth.complete",
		"params": map[string]any{
			"loginSessionId": loginID,
			"code":           loginCode,
		},
	})
	completeResult := assertRPCResult(t, complete)
	completeLogin, _ := completeResult["login"].(map[string]any)
	if completeLogin["status"] != "authorized" {
		t.Fatalf("expected authorized login status, got %v", completeLogin["status"])
	}

	toolsCatalog := rpcCall(t, ts.URL, map[string]any{
		"type":   "req",
		"id":     "tools-catalog",
		"method": "tools.catalog",
		"params": map[string]any{},
	})
	catalogResult := assertRPCResult(t, toolsCatalog)
	if count, _ := catalogResult["count"].(float64); int(count) < 1 {
		t.Fatalf("tools.catalog should return entries")
	}

	browserReq := rpcCall(t, ts.URL, map[string]any{
		"type":   "req",
		"id":     "browser-req",
		"method": "browser.request",
		"params": map[string]any{
			"url":    "https://chatgpt.com",
			"method": "GET",
		},
	})
	browserResult := assertRPCResult(t, browserReq)
	jobID, _ := browserResult["jobId"].(string)
	if jobID == "" {
		t.Fatalf("browser.request should return queued jobId")
	}

	waitJob := rpcCall(t, ts.URL, map[string]any{
		"type":   "req",
		"id":     "agent-wait",
		"method": "agent.wait",
		"params": map[string]any{
			"jobId":     jobID,
			"timeoutMs": 2000,
		},
	})
	waitJobResult := assertRPCResult(t, waitJob)
	done, _ := waitJobResult["done"].(bool)
	if !done {
		t.Fatalf("expected queued browser job to complete")
	}
	result, _ := waitJobResult["result"].(map[string]any)
	outputWrap, _ := result["output"].(map[string]any)
	if outputWrap["status"] != float64(200) {
		t.Fatalf("expected browser runtime output status 200, got %v", outputWrap["status"])
	}
}

func TestChannelsSendAndHistoryFlow(t *testing.T) {
	cfg := config.Default()
	cfg.Runtime.StatePath = "memory://test-channels"
	cfg.Channels.Telegram.BotToken = "telegram-bot-token"
	cfg.Channels.Telegram.DefaultTarget = "chat-1"

	s := New(cfg, buildinfo.Default())
	defer s.Close()

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	connect := rpcCall(t, ts.URL, map[string]any{
		"type":   "req",
		"id":     "connect-msg",
		"method": "connect",
		"params": map[string]any{
			"role":    "client",
			"channel": "telegram",
		},
	})
	connectResult := assertRPCResult(t, connect)
	sessionID, _ := connectResult["sessionId"].(string)
	if sessionID == "" {
		t.Fatalf("expected session id from connect")
	}

	chStatus := rpcCall(t, ts.URL, map[string]any{
		"type":   "req",
		"id":     "channels-status",
		"method": "channels.status",
		"params": map[string]any{},
	})
	chStatusResult := assertRPCResult(t, chStatus)
	if count, _ := chStatusResult["count"].(float64); int(count) < 1 {
		t.Fatalf("expected at least one channel in status")
	}

	sendResp := rpcCall(t, ts.URL, map[string]any{
		"type":   "req",
		"id":     "send-1",
		"method": "send",
		"params": map[string]any{
			"sessionId": sessionID,
			"message":   "hello telegram",
		},
	})
	sendResult := assertRPCResult(t, sendResp)
	jobID, _ := sendResult["jobId"].(string)
	if jobID == "" {
		t.Fatalf("expected job id for send")
	}

	waitResp := rpcCall(t, ts.URL, map[string]any{
		"type":   "req",
		"id":     "wait-send",
		"method": "agent.wait",
		"params": map[string]any{
			"jobId":     jobID,
			"timeoutMs": 2000,
		},
	})
	waitResult := assertRPCResult(t, waitResp)
	done, _ := waitResult["done"].(bool)
	if !done {
		t.Fatalf("expected send job completion")
	}

	sessionHistory := rpcCall(t, ts.URL, map[string]any{
		"type":   "req",
		"id":     "sessions-history",
		"method": "sessions.history",
		"params": map[string]any{
			"sessionId": sessionID,
		},
	})
	sessionHistoryResult := assertRPCResult(t, sessionHistory)
	if count, _ := sessionHistoryResult["count"].(float64); int(count) < 1 {
		t.Fatalf("expected session history entries")
	}

	chatHistory := rpcCall(t, ts.URL, map[string]any{
		"type":   "req",
		"id":     "chat-history",
		"method": "chat.history",
		"params": map[string]any{
			"channel": "telegram",
		},
	})
	chatHistoryResult := assertRPCResult(t, chatHistory)
	if count, _ := chatHistoryResult["count"].(float64); int(count) < 1 {
		t.Fatalf("expected chat history entries")
	}

	logoutResp := rpcCall(t, ts.URL, map[string]any{
		"type":   "req",
		"id":     "channels-logout",
		"method": "channels.logout",
		"params": map[string]any{
			"channel": "telegram",
		},
	})
	logoutResult := assertRPCResult(t, logoutResp)
	ok, _ := logoutResult["ok"].(bool)
	if !ok {
		t.Fatalf("expected channels.logout ok=true")
	}
}

func TestSecurityPolicyBlocksConfiguredMethods(t *testing.T) {
	cfg := config.Default()
	cfg.Runtime.StatePath = "memory://test-security"
	cfg.Channels.Telegram.BotToken = "telegram-bot-token"
	cfg.Security.ToolPolicies = map[string]string{
		"browser.request": "block",
	}

	s := New(cfg, buildinfo.Default())
	defer s.Close()
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	start := rpcCall(t, ts.URL, map[string]any{
		"type":   "req",
		"id":     "security-login-start",
		"method": "web.login.start",
		"params": map[string]any{},
	})
	startResult := assertRPCResult(t, start)
	loginObj, _ := startResult["login"].(map[string]any)
	loginID, _ := loginObj["loginSessionId"].(string)
	loginCode, _ := loginObj["code"].(string)
	_ = rpcCall(t, ts.URL, map[string]any{
		"type":   "req",
		"id":     "security-login-complete",
		"method": "auth.oauth.complete",
		"params": map[string]any{
			"loginSessionId": loginID,
			"code":           loginCode,
		},
	})

	blocked := rpcCall(t, ts.URL, map[string]any{
		"type":   "req",
		"id":     "blocked-browser",
		"method": "browser.request",
		"params": map[string]any{
			"url": "https://chatgpt.com",
		},
	})
	assertRPCErrorCode(t, blocked, -32050)

	configResp := rpcCall(t, ts.URL, map[string]any{
		"type":   "req",
		"id":     "config-get",
		"method": "config.get",
		"params": map[string]any{},
	})
	configResult := assertRPCResult(t, configResp)
	securityObj, ok := configResult["security"].(map[string]any)
	if !ok {
		t.Fatalf("config.get should include security snapshot")
	}
	if securityObj["defaultAction"] == nil {
		t.Fatalf("security snapshot missing default action")
	}
}

func TestSecurityCredentialAndTelemetryPolicies(t *testing.T) {
	cfg := config.Default()
	cfg.Runtime.StatePath = "memory://test-security-2"
	cfg.Security.DefaultAction = "allow"
	cfg.Security.CredentialLeakAction = "block"
	cfg.Security.TelemetryAction = "review"

	s := New(cfg, buildinfo.Default())
	defer s.Close()
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	leak := rpcCall(t, ts.URL, map[string]any{
		"type":   "req",
		"id":     "security-leak",
		"method": "send",
		"params": map[string]any{
			"channel": "webchat",
			"message": "test",
			"payload": map[string]any{
				"api_key": "sk-very-secret",
			},
		},
	})
	assertRPCErrorCode(t, leak, -32050)

	reviewAllowed := rpcCall(t, ts.URL, map[string]any{
		"type":   "req",
		"id":     "security-review",
		"method": "agent",
		"params": map[string]any{
			"message":       "normal message",
			"telemetryTags": []string{"threat:critical"},
		},
	})
	_ = assertRPCResult(t, reviewAllowed)
}

func TestEdgeWasmAndRoutinesPhase7Methods(t *testing.T) {
	cfg := config.Default()
	cfg.Runtime.StatePath = "memory://test-edge-phase7"
	s := New(cfg, buildinfo.Default())
	defer s.Close()
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	wasmList := rpcCall(t, ts.URL, map[string]any{
		"type":   "req",
		"id":     "edge-wasm-list",
		"method": "edge.wasm.marketplace.list",
		"params": map[string]any{},
	})
	wasmResult := assertRPCResult(t, wasmList)
	if count, _ := wasmResult["count"].(float64); int(count) < 1 {
		t.Fatalf("expected wasm marketplace entries")
	}

	finetuneRun := rpcCall(t, ts.URL, map[string]any{
		"type":   "req",
		"id":     "edge-finetune-run",
		"method": "edge.finetune.run",
		"params": map[string]any{
			"dataset": "memory://dataset",
		},
	})
	finetuneResult := assertRPCResult(t, finetuneRun)
	job, ok := finetuneResult["job"].(map[string]any)
	if !ok {
		t.Fatalf("expected finetune run job payload")
	}
	if job["status"] != "completed" {
		t.Fatalf("expected finetune job completed, got %v", job["status"])
	}

	homomorphic := rpcCall(t, ts.URL, map[string]any{
		"type":   "req",
		"id":     "edge-homo",
		"method": "edge.homomorphic.compute",
		"params": map[string]any{
			"operation": "sum",
			"values":    []any{1, 2, 3},
		},
	})
	homoResult := assertRPCResult(t, homomorphic)
	if homoResult["result"] != float64(6) {
		t.Fatalf("unexpected homomorphic compute result: %v", homoResult["result"])
	}
}

func TestEdgePhase7MethodMatrix(t *testing.T) {
	cfg := config.Default()
	cfg.Runtime.StatePath = "memory://test-edge-phase7-matrix"
	s := New(cfg, buildinfo.Default())
	defer s.Close()
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	checks := []struct {
		method string
		params map[string]any
		assert func(t *testing.T, result map[string]any)
	}{
		{
			method: "edge.router.plan",
			params: map[string]any{"goal": "latency"},
			assert: func(t *testing.T, result map[string]any) {
				route, ok := result["route"].(map[string]any)
				if !ok {
					t.Fatalf("edge.router.plan missing route payload")
				}
				primary, _ := route["primary"].(string)
				if primary == "" {
					t.Fatalf("edge.router.plan should include primary route")
				}
			},
		},
		{
			method: "edge.acceleration.status",
			params: map[string]any{},
			assert: func(t *testing.T, result map[string]any) {
				if enabled, _ := result["enabled"].(bool); !enabled {
					t.Fatalf("edge.acceleration.status expected enabled=true")
				}
			},
		},
		{
			method: "edge.swarm.plan",
			params: map[string]any{"task": "verification"},
			assert: func(t *testing.T, result map[string]any) {
				agents, ok := result["agents"].([]any)
				if !ok || len(agents) < 1 {
					t.Fatalf("edge.swarm.plan expected agents")
				}
			},
		},
		{
			method: "edge.multimodal.inspect",
			params: map[string]any{"source": "memory://image"},
			assert: func(t *testing.T, result map[string]any) {
				if summary, _ := result["summary"].(string); summary == "" {
					t.Fatalf("edge.multimodal.inspect expected summary")
				}
			},
		},
		{
			method: "edge.enclave.status",
			params: map[string]any{},
			assert: func(t *testing.T, result map[string]any) {
				if attestation, _ := result["attestation"].(string); attestation == "" {
					t.Fatalf("edge.enclave.status expected attestation")
				}
			},
		},
		{
			method: "edge.enclave.prove",
			params: map[string]any{"challenge": "abc"},
			assert: func(t *testing.T, result map[string]any) {
				if proof, _ := result["proof"].(string); proof == "" {
					t.Fatalf("edge.enclave.prove expected proof")
				}
			},
		},
		{
			method: "edge.mesh.status",
			params: map[string]any{},
			assert: func(t *testing.T, result map[string]any) {
				if connected, _ := result["connected"].(bool); !connected {
					t.Fatalf("edge.mesh.status expected connected=true")
				}
			},
		},
		{
			method: "edge.finetune.status",
			params: map[string]any{},
			assert: func(t *testing.T, result map[string]any) {
				jobs, ok := result["jobs"].([]any)
				if !ok || len(jobs) < 1 {
					t.Fatalf("edge.finetune.status expected jobs")
				}
			},
		},
		{
			method: "edge.identity.trust.status",
			params: map[string]any{},
			assert: func(t *testing.T, result map[string]any) {
				if status, _ := result["status"].(string); status != "trusted" {
					t.Fatalf("edge.identity.trust.status expected trusted status, got %v", result["status"])
				}
			},
		},
		{
			method: "edge.personality.profile",
			params: map[string]any{"profile": "assistant"},
			assert: func(t *testing.T, result map[string]any) {
				traits, ok := result["traits"].([]any)
				if !ok || len(traits) < 1 {
					t.Fatalf("edge.personality.profile expected traits")
				}
			},
		},
		{
			method: "edge.handoff.plan",
			params: map[string]any{"target": "ops"},
			assert: func(t *testing.T, result map[string]any) {
				steps, ok := result["steps"].([]any)
				if !ok || len(steps) < 1 {
					t.Fatalf("edge.handoff.plan expected steps")
				}
			},
		},
		{
			method: "edge.marketplace.revenue.preview",
			params: map[string]any{"units": 5, "price": 2.5},
			assert: func(t *testing.T, result map[string]any) {
				if revenue, _ := result["revenue"].(float64); revenue != 12.5 {
					t.Fatalf("edge.marketplace.revenue.preview expected revenue=12.5, got %v", result["revenue"])
				}
			},
		},
		{
			method: "edge.finetune.cluster.plan",
			params: map[string]any{"workers": 3},
			assert: func(t *testing.T, result map[string]any) {
				if workers, _ := result["workers"].(float64); int(workers) != 3 {
					t.Fatalf("edge.finetune.cluster.plan expected workers=3, got %v", result["workers"])
				}
			},
		},
		{
			method: "edge.alignment.evaluate",
			params: map[string]any{"input": "safe request"},
			assert: func(t *testing.T, result map[string]any) {
				if status, _ := result["status"].(string); status != "pass" {
					t.Fatalf("edge.alignment.evaluate expected pass status, got %v", result["status"])
				}
			},
		},
		{
			method: "edge.quantum.status",
			params: map[string]any{},
			assert: func(t *testing.T, result map[string]any) {
				if mode, _ := result["mode"].(string); mode != "simulated" {
					t.Fatalf("edge.quantum.status expected mode=simulated, got %v", result["mode"])
				}
			},
		},
		{
			method: "edge.collaboration.plan",
			params: map[string]any{"team": "core"},
			assert: func(t *testing.T, result map[string]any) {
				plan, ok := result["plan"].([]any)
				if !ok || len(plan) < 1 {
					t.Fatalf("edge.collaboration.plan expected plan steps")
				}
			},
		},
		{
			method: "edge.voice.transcribe",
			params: map[string]any{"audioRef": "memory://clip"},
			assert: func(t *testing.T, result map[string]any) {
				if transcript, _ := result["transcript"].(string); transcript == "" {
					t.Fatalf("edge.voice.transcribe expected transcript")
				}
			},
		},
	}

	for _, check := range checks {
		frame := rpcCall(t, ts.URL, map[string]any{
			"type":   "req",
			"id":     "edge-matrix-" + check.method,
			"method": check.method,
			"params": check.params,
		})
		result := assertRPCResult(t, frame)
		check.assert(t, result)
	}
}

func rpcCall(t *testing.T, baseURL string, frame map[string]any) map[string]any {
	t.Helper()
	body, err := json.Marshal(frame)
	if err != nil {
		t.Fatalf("failed to marshal rpc frame: %v", err)
	}
	resp, err := http.Post(baseURL+"/rpc", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /rpc failed: %v", err)
	}
	defer resp.Body.Close()

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("failed decoding rpc response: %v", err)
	}
	return payload
}

func assertRPCResult(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	if _, hasErr := payload["error"]; hasErr {
		t.Fatalf("expected rpc success response, got error: %v", payload["error"])
	}
	result, ok := payload["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected rpc result object, got: %v", payload["result"])
	}
	return result
}

func assertRPCErrorCode(t *testing.T, payload map[string]any, code int) {
	t.Helper()
	errObj, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected rpc error object, got: %v", payload)
	}
	gotCode, ok := errObj["code"].(float64)
	if !ok {
		t.Fatalf("expected numeric rpc error code, got: %v", errObj["code"])
	}
	if int(gotCode) != code {
		t.Fatalf("unexpected rpc error code: got=%d want=%d payload=%v", int(gotCode), code, payload)
	}
}
