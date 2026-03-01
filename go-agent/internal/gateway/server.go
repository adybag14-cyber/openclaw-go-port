package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	osexec "os/exec"
	"path/filepath"
	stdruntime "runtime"
	"strings"
	"time"

	webbridge "github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/bridge/web"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/buildinfo"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/channels"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/config"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/memory"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/protocol"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/routines"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/rpc"
	agentruntime "github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/runtime"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/scheduler"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/security"
	securityaudit "github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/security/audit"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/state"
	toolruntime "github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/tools/runtime"
	wasmruntime "github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/wasm/runtime"
)

type Server struct {
	cfg       config.Config
	build     buildinfo.Info
	startedAt time.Time

	methods   *rpc.MethodRegistry
	sessions  *SessionRegistry
	scheduler *scheduler.Scheduler
	tools     *toolruntime.Runtime
	channels  *channels.Registry
	memory    *memory.Store
	runtime   *agentruntime.Runtime
	state     *state.Store
	guard     *security.Guard
	compat    *compatState
	routines  *routines.Manager
	wasm      *wasmruntime.Runtime
	webLogin  *webbridge.Manager
	edge      *edgeState
}

func New(cfg config.Config, build buildinfo.Info) *Server {
	s := &Server{
		cfg:       cfg,
		build:     build,
		startedAt: time.Now().UTC(),
		methods:   rpc.DefaultRegistry(),
		sessions:  NewSessionRegistry(),
		tools: toolruntime.NewDefaultWithOptions(toolruntime.RuntimeOptions{
			BrowserBridge: toolruntime.BrowserBridgeOptions{
				Enabled:              cfg.Runtime.BrowserBridge.Enabled,
				Endpoint:             cfg.Runtime.BrowserBridge.Endpoint,
				RequestTimeout:       time.Duration(cfg.Runtime.BrowserBridge.RequestTimeoutMs) * time.Millisecond,
				Retries:              cfg.Runtime.BrowserBridge.Retries,
				RetryBackoff:         time.Duration(cfg.Runtime.BrowserBridge.RetryBackoffMs) * time.Millisecond,
				CircuitFailThreshold: cfg.Runtime.BrowserBridge.CircuitFailThreshold,
				CircuitCooldown:      time.Duration(cfg.Runtime.BrowserBridge.CircuitCooldownMs) * time.Millisecond,
			},
		}),
		channels: channels.NewRegistry(cfg.Channels.Telegram.BotToken, cfg.Channels.Telegram.DefaultTarget),
		memory:   memory.NewStore(cfg.Runtime.StatePath, 10_000),
		runtime:  agentruntime.New(cfg.Runtime),
		state:    state.NewStore(),
		guard: security.NewGuard(security.GuardConfig{
			PolicyBundlePath:        cfg.Security.PolicyBundlePath,
			DefaultAction:           cfg.Security.DefaultAction,
			ToolPolicies:            cfg.Security.ToolPolicies,
			BlockedMessagePatterns:  cfg.Security.BlockedMessagePatterns,
			TelemetryHighRiskTags:   cfg.Security.TelemetryHighRiskTags,
			TelemetryAction:         cfg.Security.TelemetryAction,
			CredentialSensitiveKeys: cfg.Security.CredentialSensitiveKeys,
			CredentialLeakAction:    cfg.Security.CredentialLeakAction,
			LoopGuardEnabled:        cfg.Security.LoopGuardEnabled,
			LoopGuardWindowMS:       cfg.Security.LoopGuardWindowMS,
			LoopGuardMaxHits:        cfg.Security.LoopGuardMaxHits,
			RiskReviewThreshold:     cfg.Security.RiskReviewThreshold,
			RiskBlockThreshold:      cfg.Security.RiskBlockThreshold,
		}),
		compat:   newCompatState(),
		routines: routines.NewManager(),
		wasm:     wasmruntime.NewRuntime(),
		webLogin: webbridge.NewManager(10 * time.Minute),
		edge:     newEdgeState(),
	}

	s.scheduler = scheduler.New(2, 128, s.executeScheduledJob)
	return s
}

func (s *Server) Close() {
	if s.scheduler != nil {
		s.scheduler.Stop()
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/rpc", s.handleRPC)
	return mux
}

func (s *Server) Run(ctx context.Context) error {
	httpServer := &http.Server{
		Addr:              s.cfg.Gateway.Server.HTTPBind,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	listener, err := net.Listen("tcp", s.cfg.Gateway.Server.HTTPBind)
	if err != nil {
		return err
	}

	errCh := make(chan error, 1)
	go func() {
		err := httpServer.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
		s.Close()
		return <-errCh
	case err := <-errCh:
		s.Close()
		return err
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.healthPayload())
}

func (s *Server) handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var frame map[string]any
	if err := json.NewDecoder(r.Body).Decode(&frame); err != nil {
		writeJSON(w, http.StatusBadRequest, protocol.RPCErrorResponseFrame(
			"unknown",
			-32700,
			"parse error",
			nil,
		))
		return
	}
	if protocol.FrameKindOf(frame) != protocol.FrameKindReq {
		writeJSON(w, http.StatusBadRequest, protocol.RPCErrorResponseFrame(
			"unknown",
			-32600,
			"invalid request frame",
			map[string]any{
				"expectedType": "req",
			},
		))
		return
	}

	req := protocol.ParseRPCRequest(frame)
	if req == nil {
		writeJSON(w, http.StatusBadRequest, protocol.RPCErrorResponseFrame(
			"unknown",
			-32600,
			"invalid request frame",
			map[string]any{
				"reason": "missing method",
			},
		))
		return
	}

	params := asMap(req.Params)
	resolved := s.methods.Resolve(req.Method)

	result, rpcErr := s.dispatchRPC(r.Context(), req.ID, resolved.Canonical, params)
	if rpcErr != nil {
		writeJSON(w, http.StatusOK, protocol.RPCErrorResponseFrame(
			req.ID,
			rpcErr.Code,
			rpcErr.Message,
			rpcErr.Details,
		))
		return
	}
	writeJSON(w, http.StatusOK, protocol.RPCSuccessResponseFrame(req.ID, result))
}

type dispatchError struct {
	Code    int64
	Message string
	Details map[string]any
}

func (s *Server) dispatchRPC(ctx context.Context, requestID string, canonical string, params map[string]any) (map[string]any, *dispatchError) {
	if isMutatingMethod(canonical) {
		decision := s.guard.Evaluate(canonical, params)
		if decision.Action == security.ActionBlock {
			return nil, &dispatchError{
				Code:    -32050,
				Message: "blocked by security policy",
				Details: map[string]any{
					"method": canonical,
					"action": decision.Action,
					"reason": decision.Reason,
				},
			}
		}
	}

	switch canonical {
	case "health":
		return s.healthPayload(), nil
	case "status":
		return s.statusPayload(), nil
	case "config.get":
		return s.handleConfigGet(), nil
	case "security.audit":
		return s.handleSecurityAudit(params), nil
	case "connect":
		return s.handleConnect(params)
	case "sessions.list":
		return s.handleSessionsList(), nil
	case "sessions.history":
		return s.handleSessionsHistory(params), nil
	case "chat.history":
		return s.handleChatHistory(params), nil
	case "session.status":
		return s.handleSessionStatus(params)
	case "channels.status":
		return s.handleChannelsStatus(), nil
	case "channels.logout":
		return s.handleChannelsLogout(ctx, params)
	case "tools.catalog":
		return map[string]any{
			"tools": s.tools.Catalog(),
			"count": len(s.tools.Catalog()),
		}, nil
	case "web.login.start", "auth.oauth.start":
		return s.handleWebLoginStart(params), nil
	case "web.login.wait", "auth.oauth.wait":
		return s.handleWebLoginWait(ctx, params)
	case "auth.oauth.complete":
		return s.handleOAuthComplete(params)
	case "auth.oauth.logout":
		return s.handleOAuthLogout(params), nil
	case "browser.request", "browser.open", "agent", "send", "chat.send", "sessions.send":
		return s.enqueueRuntimeRequest(requestID, canonical, params)
	case "edge.wasm.marketplace.list":
		return s.handleEdgeWasmMarketplace(), nil
	case "edge.router.plan":
		return s.handleEdgeRouterPlan(params), nil
	case "edge.acceleration.status":
		return s.handleEdgeAccelerationStatus(), nil
	case "edge.swarm.plan":
		return s.handleEdgeSwarmPlan(params)
	case "edge.multimodal.inspect":
		return s.handleEdgeMultimodalInspect(params)
	case "edge.enclave.status":
		return s.handleEdgeEnclaveStatus(), nil
	case "edge.enclave.prove":
		return s.handleEdgeEnclaveProve(params), nil
	case "edge.mesh.status":
		return s.handleEdgeMeshStatus(), nil
	case "edge.homomorphic.compute":
		return s.handleEdgeHomomorphicCompute(params), nil
	case "edge.finetune.status":
		return s.handleEdgeFinetuneStatus(), nil
	case "edge.finetune.run":
		return s.handleEdgeFinetuneRun(ctx, params)
	case "edge.identity.trust.status":
		return s.handleEdgeIdentityTrustStatus(), nil
	case "edge.personality.profile":
		return s.handleEdgePersonalityProfile(params), nil
	case "edge.handoff.plan":
		return s.handleEdgeHandoffPlan(params), nil
	case "edge.marketplace.revenue.preview":
		return s.handleEdgeMarketplaceRevenuePreview(params), nil
	case "edge.finetune.cluster.plan":
		return s.handleEdgeFinetuneClusterPlan(params), nil
	case "edge.alignment.evaluate":
		return s.handleEdgeAlignmentEvaluate(params), nil
	case "edge.quantum.status":
		return s.handleEdgeQuantumStatus(params)
	case "edge.collaboration.plan":
		return s.handleEdgeCollaborationPlan(params), nil
	case "edge.voice.transcribe":
		return s.handleEdgeVoiceTranscribe(params)
	case "agent.wait":
		return s.handleAgentWait(ctx, params)
	default:
		known := s.methods.Resolve(canonical).Known
		if known {
			return s.handleCompatMethod(ctx, requestID, canonical, params)
		}
		return nil, &dispatchError{
			Code:    -32601,
			Message: "method not implemented in go phase-4 scaffold",
			Details: map[string]any{
				"requested": canonical,
				"canonical": canonical,
				"known":     known,
			},
		}
	}
}

func (s *Server) handleConfigGet() map[string]any {
	return map[string]any{
		"gateway": map[string]any{
			"url":      s.cfg.Gateway.URL,
			"authMode": s.resolveAuthMode(),
		},
		"runtime": s.runtime.Snapshot(),
		"browserBridge": map[string]any{
			"enabled":              s.cfg.Runtime.BrowserBridge.Enabled,
			"endpoint":             s.cfg.Runtime.BrowserBridge.Endpoint,
			"requestTimeoutMs":     s.cfg.Runtime.BrowserBridge.RequestTimeoutMs,
			"retries":              s.cfg.Runtime.BrowserBridge.Retries,
			"retryBackoffMs":       s.cfg.Runtime.BrowserBridge.RetryBackoffMs,
			"circuitFailThreshold": s.cfg.Runtime.BrowserBridge.CircuitFailThreshold,
			"circuitCooldownMs":    s.cfg.Runtime.BrowserBridge.CircuitCooldownMs,
		},
		"channels": map[string]any{
			"telegramConfigured": strings.TrimSpace(s.cfg.Channels.Telegram.BotToken) != "",
		},
		"memory":   s.memory.Stats(),
		"security": s.guard.Snapshot(),
		"routines": map[string]any{
			"count": len(s.routines.List()),
			"list":  s.routines.List(),
		},
		"wasm": map[string]any{
			"count":   len(s.wasm.MarketplaceList()),
			"modules": s.wasm.MarketplaceList(),
			"policy":  s.wasm.Policy(),
		},
	}
}

func (s *Server) handleSecurityAudit(params map[string]any) map[string]any {
	report := securityaudit.Run(s.cfg, securityaudit.Options{
		Deep: toBool(params["deep"], false),
	})
	return map[string]any{
		"report": report,
	}
}

func (s *Server) handleConnect(params map[string]any) (map[string]any, *dispatchError) {
	role := toString(params["role"], "client")
	channel := strings.ToLower(toString(params["channel"], "webchat"))
	scopes := toStringSlice(params["scopes"])
	clientID := toString(asMap(params["client"])["id"], "")
	token := toString(asMap(params["auth"])["token"], toString(params["token"], ""))
	password := toString(asMap(params["auth"])["password"], toString(params["password"], ""))

	mode := s.resolveAuthMode()
	authenticated, reason := s.validateAuth(mode, token, password)
	if !authenticated {
		return nil, &dispatchError{
			Code:    -32001,
			Message: "authentication failed",
			Details: map[string]any{
				"authMode": mode,
				"reason":   reason,
			},
		}
	}
	session := s.sessions.Create(clientID, channel, role, scopes, mode, authenticated)

	return map[string]any{
		"sessionId":         session.ID,
		"role":              session.Role,
		"channel":           session.Channel,
		"scopes":            session.Scopes,
		"authMode":          session.AuthMode,
		"authenticated":     session.Authenticated,
		"authFailureReason": "",
		"supportedMethods":  s.methods.SupportedMethods(),
	}, nil
}

func (s *Server) handleSessionsList() map[string]any {
	items := s.sessions.List()
	return map[string]any{
		"count": len(items),
		"items": items,
	}
}

func (s *Server) handleSessionsHistory(params map[string]any) map[string]any {
	sessionID := toString(params["sessionId"], "")
	limit := toInt(params["limit"], 50)
	items := s.memory.HistoryBySession(sessionID, limit)
	return map[string]any{
		"sessionId": sessionID,
		"count":     len(items),
		"items":     items,
	}
}

func (s *Server) handleChatHistory(params map[string]any) map[string]any {
	channel := strings.ToLower(toString(params["channel"], ""))
	limit := toInt(params["limit"], 50)
	items := s.memory.HistoryByChannel(channel, limit)
	return map[string]any{
		"channel": channel,
		"count":   len(items),
		"items":   items,
	}
}

func (s *Server) handleChannelsStatus() map[string]any {
	status := s.channels.Status()
	return map[string]any{
		"count": len(status),
		"items": status,
	}
}

func (s *Server) handleChannelsLogout(ctx context.Context, params map[string]any) (map[string]any, *dispatchError) {
	channel := strings.ToLower(toString(params["channel"], ""))
	if channel == "" {
		return nil, &dispatchError{
			Code:    -32602,
			Message: "missing channel",
		}
	}
	accountID := toString(params["accountId"], "")
	ok, err := s.channels.Logout(ctx, channel, accountID)
	if err != nil {
		return nil, &dispatchError{
			Code:    -32040,
			Message: err.Error(),
			Details: map[string]any{"channel": channel},
		}
	}
	return map[string]any{
		"ok":      ok,
		"channel": channel,
	}, nil
}

func (s *Server) handleSessionStatus(params map[string]any) (map[string]any, *dispatchError) {
	sessionID := toString(params["sessionId"], toString(params["id"], ""))
	if sessionID == "" {
		return nil, &dispatchError{
			Code:    -32602,
			Message: "missing sessionId",
		}
	}
	session, ok := s.sessions.Get(sessionID)
	if !ok {
		return nil, &dispatchError{
			Code:    -32004,
			Message: "session not found",
			Details: map[string]any{"sessionId": sessionID},
		}
	}
	s.sessions.Touch(sessionID)
	return map[string]any{
		"session": session,
	}, nil
}

func (s *Server) handleWebLoginStart(params map[string]any) map[string]any {
	session := s.webLogin.Start(webbridge.StartOptions{
		Provider: toString(params["provider"], "chatgpt"),
		Model:    toString(params["model"], "gpt-5.2"),
	})
	return map[string]any{
		"login": session,
	}
}

func (s *Server) handleWebLoginWait(ctx context.Context, params map[string]any) (map[string]any, *dispatchError) {
	loginID := toString(params["loginSessionId"], "")
	if loginID == "" {
		return nil, &dispatchError{
			Code:    -32602,
			Message: "missing loginSessionId",
		}
	}
	timeout := toDurationMs(params["timeoutMs"], 15000)
	session, err := s.webLogin.Wait(ctx, loginID, timeout)
	if err != nil {
		return nil, &dispatchError{
			Code:    -32004,
			Message: err.Error(),
			Details: map[string]any{"loginSessionId": loginID},
		}
	}
	return map[string]any{
		"login": session,
	}, nil
}

func (s *Server) handleOAuthComplete(params map[string]any) (map[string]any, *dispatchError) {
	loginID := toString(params["loginSessionId"], "")
	if loginID == "" {
		return nil, &dispatchError{
			Code:    -32602,
			Message: "missing loginSessionId",
		}
	}
	code := toString(params["code"], "")
	session, err := s.webLogin.Complete(loginID, code)
	if err != nil {
		return nil, &dispatchError{
			Code:    -32041,
			Message: err.Error(),
			Details: map[string]any{"loginSessionId": loginID},
		}
	}
	return map[string]any{
		"login": session,
	}, nil
}

func (s *Server) handleOAuthLogout(params map[string]any) map[string]any {
	loginID := toString(params["loginSessionId"], "")
	if loginID == "" {
		count := s.webLogin.LogoutAll()
		return map[string]any{
			"ok":      true,
			"revoked": count,
		}
	}
	return map[string]any{
		"ok":      s.webLogin.Logout(loginID),
		"revoked": 1,
	}
}

func (s *Server) enqueueRuntimeRequest(requestID string, canonical string, params map[string]any) (map[string]any, *dispatchError) {
	if canonical == "browser.request" || canonical == "browser.open" {
		loginID := strings.TrimSpace(toString(params["loginSessionId"], ""))
		if loginID != "" {
			if !s.webLogin.IsAuthorized(loginID) {
				return nil, &dispatchError{
					Code:    -32040,
					Message: "browser bridge requires specified loginSessionId to be authorized",
					Details: map[string]any{
						"loginSessionId": loginID,
					},
				}
			}
		} else if !s.webLogin.HasAuthorizedSession() {
			return nil, &dispatchError{
				Code:    -32040,
				Message: "browser bridge requires active authorized login session",
			}
		}
	}

	sessionID := toString(params["sessionId"], "")
	if canonical == "sessions.send" {
		canonical = "send"
	}
	if (canonical == "send" || canonical == "chat.send") && strings.TrimSpace(toString(params["channel"], "")) == "" && sessionID != "" {
		if session, ok := s.sessions.Get(sessionID); ok && strings.TrimSpace(session.Channel) != "" {
			params["channel"] = session.Channel
		}
	}

	job, err := s.scheduler.Submit(requestID, sessionID, canonical, params)
	if err != nil {
		return nil, &dispatchError{
			Code:    -32020,
			Message: err.Error(),
		}
	}

	return map[string]any{
		"accepted": true,
		"jobId":    job.ID,
		"state":    job.State,
	}, nil
}

func (s *Server) handleAgentWait(ctx context.Context, params map[string]any) (map[string]any, *dispatchError) {
	jobID := toString(params["jobId"], "")
	if jobID == "" {
		return nil, &dispatchError{
			Code:    -32602,
			Message: "missing jobId",
		}
	}
	timeout := toDurationMs(params["timeoutMs"], 15000)
	job, ok := s.scheduler.Wait(ctx, jobID, timeout)
	if !ok {
		return nil, &dispatchError{
			Code:    -32004,
			Message: "job not found",
			Details: map[string]any{"jobId": jobID},
		}
	}
	if job.State == scheduler.JobFailed {
		return nil, &dispatchError{
			Code:    -32010,
			Message: "job failed",
			Details: map[string]any{
				"jobId":  job.ID,
				"state":  job.State,
				"error":  job.Error,
				"method": job.Method,
			},
		}
	}
	return map[string]any{
		"jobId":   job.ID,
		"done":    job.State == scheduler.JobSucceeded,
		"state":   job.State,
		"result":  job.Result,
		"method":  job.Method,
		"session": job.SessionID,
	}, nil
}

func (s *Server) executeScheduledJob(ctx context.Context, job scheduler.Job) (any, error) {
	switch strings.ToLower(job.Method) {
	case "browser.request", "browser.open":
		result, err := s.tools.Invoke(ctx, toolruntime.Request{
			Tool:      job.Method,
			SessionID: job.SessionID,
			Input:     job.Params,
		})
		if err != nil {
			return nil, err
		}
		s.recordMemory(job, "assistant", fmt.Sprintf("browser bridge %s", job.Method), map[string]any{
			"provider": result.Provider,
			"output":   result.Output,
		})
		return map[string]any{
			"provider": result.Provider,
			"output":   result.Output,
		}, nil
	case "agent":
		message := toString(job.Params["message"], toString(job.Params["prompt"], ""))
		s.recordMemory(job, "user", message, job.Params)
		return map[string]any{
			"status": "accepted",
			"method": job.Method,
			"echo":   job.Params,
		}, nil
	case "send", "chat.send":
		channel := strings.ToLower(toString(job.Params["channel"], "webchat"))
		message := toString(job.Params["message"], toString(job.Params["text"], ""))
		if channel == "telegram" {
			if result, handled, err := s.handleTelegramCommand(job, message); handled {
				if err != nil {
					return nil, err
				}
				if job.SessionID != "" {
					s.sessions.UpdateChannel(job.SessionID, "telegram")
				}
				return result, nil
			}
		}
		receipt, err := s.channels.Send(ctx, channels.SendRequest{
			Channel:   channel,
			To:        toString(job.Params["to"], ""),
			Message:   message,
			SessionID: job.SessionID,
		})
		if err != nil {
			return nil, err
		}
		if job.SessionID != "" {
			s.sessions.UpdateChannel(job.SessionID, receipt.Channel)
		}
		s.recordMemory(job, "user", message, map[string]any{
			"receipt": receipt,
		})
		return map[string]any{
			"status": "accepted",
			"method": job.Method,
			"result": receipt,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported scheduled method: %s", job.Method)
	}
}

func (s *Server) handleEdgeWasmMarketplace() map[string]any {
	modules := s.wasm.MarketplaceList()
	return map[string]any{
		"count":   len(modules),
		"modules": modules,
		"sandbox": s.wasm.Policy(),
	}
}

func (s *Server) handleEdgeRouterPlan(params map[string]any) map[string]any {
	goal := toString(params["goal"], "balanced")
	return map[string]any{
		"goal": goal,
		"route": map[string]any{
			"primary":  "gpt-5.2",
			"fallback": "gpt-5.1-mini",
			"strategy": "latency-cost-balanced",
		},
	}
}

func (s *Server) handleEdgeAccelerationStatus() map[string]any {
	cpuCores := stdruntime.NumCPU()
	gpuAvailable := envTruthy("OPENCLAW_GO_GPU_AVAILABLE") || envTruthy("OPENCLAW_GO_ACCEL_GPU")
	tpuAvailable := envTruthy("OPENCLAW_GO_TPU_AVAILABLE") || envTruthy("OPENCLAW_GO_ACCEL_TPU")

	mode := strings.ToLower(normalizeOptionalText(os.Getenv("OPENCLAW_GO_ACCEL_MODE"), 48))
	if mode == "" {
		switch {
		case gpuAvailable && tpuAvailable:
			mode = "heterogeneous"
		case gpuAvailable:
			mode = "gpu-hybrid"
		case tpuAvailable:
			mode = "tpu-hybrid"
		default:
			mode = "cpu"
		}
	}

	throughputClass := "standard"
	if cpuCores <= 2 && !gpuAvailable && !tpuAvailable {
		throughputClass = "low"
	} else if cpuCores >= 8 || gpuAvailable || tpuAvailable {
		throughputClass = "high"
	}

	features := []string{
		"request-batching",
		"cache-warmup",
		"prefetch-routing",
	}
	capabilities := []string{"cpu"}
	if gpuAvailable {
		features = append(features, "gpu-offload")
		capabilities = append(capabilities, "gpu")
	}
	if tpuAvailable {
		features = append(features, "tpu-offload")
		capabilities = append(capabilities, "tpu")
	}

	return map[string]any{
		"enabled":         true,
		"mode":            mode,
		"cpuCores":        cpuCores,
		"features":        features,
		"capabilities":    capabilities,
		"throughputClass": throughputClass,
		"runtimeProfile":  runtimeProfileName(s.cfg.Runtime.Profile),
	}
}

func (s *Server) handleEdgeSwarmPlan(params map[string]any) (map[string]any, *dispatchError) {
	goal := normalizeOptionalText(firstNonEmptyValue(params, "goal", "task"), 16_000)
	tasks := extractSwarmTasks(params["tasks"])
	if len(tasks) == 0 && goal != "" {
		tasks = buildDefaultSwarmTasks(goal)
	}
	if len(tasks) == 0 {
		return nil, &dispatchError{
			Code:    -32602,
			Message: "edge.swarm.plan requires tasks or goal",
		}
	}

	const maxTasks = 32
	if len(tasks) > maxTasks {
		tasks = tasks[:maxTasks]
	}

	defaultAgents := 3
	if strings.EqualFold(runtimeProfileName(s.cfg.Runtime.Profile), "edge") {
		defaultAgents = 6
	}
	agentLimit := toInt(params["maxAgents"], defaultAgents)
	if agentLimit < 1 {
		agentLimit = 1
	}
	if agentLimit > 12 {
		agentLimit = 12
	}
	agentCount := agentLimit
	if agentCount > len(tasks) {
		agentCount = len(tasks)
	}

	planned := make([]map[string]any, 0, len(tasks))
	for idx, task := range tasks {
		dependsOn := []string{}
		if idx > 0 {
			dependsOn = append(dependsOn, fmt.Sprintf("task-%d", idx))
		}
		planned = append(planned, map[string]any{
			"id":             fmt.Sprintf("task-%d", idx+1),
			"title":          task,
			"assignedAgent":  fmt.Sprintf("swarm-agent-%d", (idx%agentCount)+1),
			"specialization": classifySwarmTask(task),
			"dependsOn":      dependsOn,
		})
	}

	agents := make([]map[string]any, 0, agentCount)
	for idx := 0; idx < agentCount; idx++ {
		role := "builder"
		switch {
		case idx == 0:
			role = "planning"
		case idx == agentCount-1:
			role = "validation"
		}
		agents = append(agents, map[string]any{
			"id":   fmt.Sprintf("swarm-agent-%d", idx+1),
			"role": role,
		})
	}

	return map[string]any{
		"planId":         fmt.Sprintf("swarm-%d", time.Now().UTC().UnixMilli()),
		"runtimeProfile": runtimeProfileName(s.cfg.Runtime.Profile),
		"goal":           valueOrNil(goal),
		"task":           valueOrNil(goal),
		"agentCount":     agentCount,
		"taskCount":      len(planned),
		"tasks":          planned,
		"agents":         agents,
	}, nil
}

func (s *Server) handleEdgeMultimodalInspect(params map[string]any) (map[string]any, *dispatchError) {
	imagePath := normalizeOptionalText(firstNonEmptyValue(params, "imagePath", "image"), 2_048)
	screenPath := normalizeOptionalText(firstNonEmptyValue(params, "screenPath", "screen"), 2_048)
	videoPath := normalizeOptionalText(firstNonEmptyValue(params, "videoPath", "video"), 2_048)
	sourcePath := normalizeOptionalText(firstNonEmptyValue(params, "source"), 2_048)
	prompt := normalizeOptionalText(firstNonEmptyValue(params, "prompt"), 8_000)
	ocrText := normalizeOptionalText(firstNonEmptyValue(params, "ocrText", "ocr"), 16_000)

	if imagePath == "" && sourcePath != "" {
		imagePath = sourcePath
	}

	if imagePath == "" && screenPath == "" && videoPath == "" && prompt == "" && ocrText == "" {
		return nil, &dispatchError{
			Code:    -32602,
			Message: "edge.multimodal.inspect requires media path, prompt, or ocrText",
		}
	}

	media := make([]map[string]any, 0, 3)
	if imagePath != "" {
		media = append(media, inspectMediaPath("image", imagePath))
	}
	if screenPath != "" {
		media = append(media, inspectMediaPath("screen", screenPath))
	}
	if videoPath != "" {
		media = append(media, inspectMediaPath("video", videoPath))
	}

	modalities := inferMultimodalModalities(media, ocrText)
	signals := append([]string(nil), modalities...)
	if len(signals) == 0 {
		signals = []string{"metadata"}
	}
	summary := summarizeMultimodalContext(prompt, ocrText, media, modalities)

	source := imagePath
	if source == "" {
		source = screenPath
	}
	if source == "" {
		source = videoPath
	}
	if source == "" {
		source = "context-only"
	}

	return map[string]any{
		"runtimeProfile":          runtimeProfileName(s.cfg.Runtime.Profile),
		"source":                  source,
		"signals":                 signals,
		"modalities":              modalities,
		"media":                   media,
		"ocrText":                 valueOrNil(ocrText),
		"summary":                 summary,
		"memoryAugmentationReady": true,
	}, nil
}

func (s *Server) handleEdgeEnclaveStatus() map[string]any {
	return s.edge.enclaveStatus()
}

func (s *Server) handleEdgeEnclaveProve(params map[string]any) map[string]any {
	challenge := toString(params["challenge"], "default-challenge")
	return s.edge.issueEnclaveProof(challenge)
}

func (s *Server) handleEdgeMeshStatus() map[string]any {
	topology := s.compat.edgeTopologySnapshot()
	approvedPairs := toInt(topology["approvedPairs"], 0)
	approvedPeers := toInt(topology["approvedPeers"], 0)
	onlineNodes := toInt(topology["onlineNodes"], 0)

	mode := "single-node-bridge"
	switch {
	case approvedPeers >= 3:
		mode = "mesh"
	case approvedPeers >= 1:
		mode = "multi-node-bridge"
	case onlineNodes > 1:
		mode = "cluster-preview"
	}
	connected := onlineNodes > 0 || approvedPairs > 0

	return map[string]any{
		"connected": connected,
		"peers":     approvedPeers,
		"mode":      mode,
		"topology":  topology,
	}
}

func (s *Server) handleEdgeHomomorphicCompute(params map[string]any) map[string]any {
	op := strings.ToLower(toString(params["operation"], "sum"))
	values := asSlice(params["values"])
	total := 0.0
	maxValue := 0.0
	minValue := 0.0
	for _, value := range values {
		total += value
		if value > maxValue || maxValue == 0 {
			maxValue = value
		}
		if value < minValue || minValue == 0 {
			minValue = value
		}
	}
	result := total
	switch op {
	case "mean", "avg", "average":
		if len(values) > 0 {
			result = total / float64(len(values))
		}
	case "max":
		result = maxValue
	case "min":
		result = minValue
	case "sum":
		// default, no-op
	default:
		op = "sum"
	}
	return map[string]any{
		"operation": op,
		"result":    result,
		"count":     len(values),
	}
}

func (s *Server) handleEdgeFinetuneStatus() map[string]any {
	return map[string]any{
		"jobs": s.edge.listFinetuneJobs(25),
	}
}

func (s *Server) handleEdgeFinetuneRun(ctx context.Context, params map[string]any) (map[string]any, *dispatchError) {
	result, err := s.routines.Run(ctx, "edge-wasm-smoke", params)
	if err != nil {
		job := s.edge.addFinetuneJob(params, "failed")
		return nil, &dispatchError{
			Code:    -32060,
			Message: fmt.Sprintf("%s (job=%s)", err.Error(), toString(job["id"], "")),
		}
	}
	job := s.edge.addFinetuneJob(params, "completed")
	job["runtime"] = result
	return map[string]any{
		"job": job,
	}, nil
}

func (s *Server) handleEdgeIdentityTrustStatus() map[string]any {
	topology := s.compat.edgeTopologySnapshot()
	pendingApprovals := toInt(topology["pendingApprovals"], 0)
	rejectedApprovals := toInt(topology["rejectedApprovals"], 0)
	pendingPairs := toInt(topology["pendingPairs"], 0)
	rejectedPairs := toInt(topology["rejectedPairs"], 0)

	guardSnapshot := s.guard.Snapshot()
	loopGuard := asMap(guardSnapshot["loopGuard"])
	loopGuardEnabled := toBool(loopGuard["enabled"], true)

	score := 0.98
	score -= float64(pendingApprovals) * 0.03
	score -= float64(rejectedApprovals) * 0.06
	score -= float64(pendingPairs) * 0.015
	score -= float64(rejectedPairs) * 0.04
	if !loopGuardEnabled {
		score -= 0.08
	}
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}

	signals := make([]string, 0, 6)
	if pendingApprovals > 0 {
		signals = append(signals, fmt.Sprintf("pending_approvals:%d", pendingApprovals))
	}
	if rejectedApprovals > 0 {
		signals = append(signals, fmt.Sprintf("rejected_approvals:%d", rejectedApprovals))
	}
	if pendingPairs > 0 {
		signals = append(signals, fmt.Sprintf("pending_pairs:%d", pendingPairs))
	}
	if rejectedPairs > 0 {
		signals = append(signals, fmt.Sprintf("rejected_pairs:%d", rejectedPairs))
	}
	if !loopGuardEnabled {
		signals = append(signals, "loop_guard:disabled")
	}
	if len(signals) == 0 {
		signals = append(signals, "steady_state")
	}

	status := "trusted"
	switch {
	case score < 0.75:
		status = "restricted"
	case score < 0.90:
		status = "review"
	}

	return map[string]any{
		"status":              status,
		"score":               score,
		"signals":             signals,
		"pendingApprovals":    pendingApprovals,
		"rejectedApprovals":   rejectedApprovals,
		"pendingPairs":        pendingPairs,
		"rejectedPairs":       rejectedPairs,
		"riskReviewThreshold": toInt(guardSnapshot["riskReviewThreshold"], 70),
		"riskBlockThreshold":  toInt(guardSnapshot["riskBlockThreshold"], 90),
	}
}

func (s *Server) handleEdgePersonalityProfile(params map[string]any) map[string]any {
	profile := toString(params["profile"], "default")
	return map[string]any{
		"profile": profile,
		"traits": []string{
			"pragmatic",
			"direct",
			"defensive",
		},
	}
}

func (s *Server) handleEdgeHandoffPlan(params map[string]any) map[string]any {
	target := toString(params["target"], "operator")
	return map[string]any{
		"target": target,
		"steps": []string{
			"summarize-context",
			"attach-artifacts",
			"transfer-session",
		},
	}
}

func (s *Server) handleEdgeMarketplaceRevenuePreview(params map[string]any) map[string]any {
	units := toInt(params["units"], 0)
	price := toFloat(params["price"], 0.0)
	return map[string]any{
		"units":   units,
		"price":   price,
		"revenue": float64(units) * price,
	}
}

func (s *Server) handleEdgeFinetuneClusterPlan(params map[string]any) map[string]any {
	size := toInt(params["workers"], 2)
	if size < 1 {
		size = 1
	}
	return map[string]any{
		"workers": size,
		"plan":    "burst",
	}
}

func (s *Server) handleEdgeAlignmentEvaluate(params map[string]any) map[string]any {
	input := normalizeOptionalText(toString(params["input"], ""), 8_000)
	evalParams := map[string]any{
		"message": input,
	}
	if tags, ok := params["telemetryTags"]; ok {
		evalParams["telemetryTags"] = tags
	}

	decision := s.guard.Evaluate("edge.alignment.evaluate", evalParams)
	score := float64(100-decision.RiskScore) / 100
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}

	status := "pass"
	switch decision.Action {
	case security.ActionBlock:
		status = "fail"
	case security.ActionReview:
		status = "review"
	}

	return map[string]any{
		"score":      score,
		"status":     status,
		"riskScore":  decision.RiskScore,
		"action":     decision.Action,
		"reason":     decision.Reason,
		"signals":    decision.Signals,
		"inputEmpty": strings.TrimSpace(input) == "",
	}
}

func (s *Server) handleEdgeQuantumStatus(params map[string]any) (map[string]any, *dispatchError) {
	if _, ok := params["strictMode"]; ok {
		// accepted for compatibility, behavior is driven by env policy flags.
	}
	pqcEnabled := envTruthy("OPENCLAW_GO_PQC_ENABLED") || envTruthy("OPENCLAW_GO_QUANTUM_SAFE")
	hybridMode := envTruthy("OPENCLAW_GO_PQC_HYBRID")
	kem := normalizeOptionalText(os.Getenv("OPENCLAW_GO_PQC_KEM"), 64)
	if kem == "" {
		kem = "kyber768"
	}
	signature := normalizeOptionalText(os.Getenv("OPENCLAW_GO_PQC_SIG"), 64)
	if signature == "" {
		signature = "dilithium3"
	}
	mode := "off"
	if pqcEnabled {
		if hybridMode {
			mode = "hybrid"
		} else {
			mode = "strict-pqc"
		}
	}
	return map[string]any{
		"feature": "quantum-safe-cryptography-mode",
		"enabled": pqcEnabled,
		"mode":    mode,
		"algorithms": map[string]any{
			"kem":       kem,
			"signature": signature,
			"hash":      "sha256",
		},
		"fallback": map[string]any{
			"classicalSignature":    "ed25519",
			"classicalKeyExchange":  "x25519",
			"activeWhenPqcDisabled": !pqcEnabled,
		},
		"available": pqcEnabled,
	}, nil
}

func (s *Server) handleEdgeCollaborationPlan(params map[string]any) map[string]any {
	team := toString(params["team"], "default")
	goal := toString(params["goal"], "delivery")
	return map[string]any{
		"team": team,
		"goal": goal,
		"plan": []string{
			"assign-lead",
			"define-slices",
			"merge-validation",
		},
		"checkpoints": []map[string]any{
			{"name": "spec-freeze", "owner": team, "status": "pending"},
			{"name": "integration-pass", "owner": "qa", "status": "pending"},
			{"name": "release-readiness", "owner": "ops", "status": "pending"},
		},
	}
}

func (s *Server) handleEdgeVoiceTranscribe(params map[string]any) (map[string]any, *dispatchError) {
	audioPath := normalizeOptionalText(firstNonEmptyValue(params, "audioPath", "audioRef"), 2_048)
	hintText := normalizeOptionalText(firstNonEmptyValue(params, "hintText", "hint", "text"), 16_000)
	language := normalizeOptionalText(firstNonEmptyValue(params, "language"), 32)
	providerRequested := strings.ToLower(normalizeOptionalText(firstNonEmptyValue(params, "provider"), 64))

	if audioPath == "" && hintText == "" {
		return nil, &dispatchError{
			Code:    -32602,
			Message: "edge.voice.transcribe requires audioPath or hintText",
		}
	}

	providerUsed := providerRequested
	if providerUsed == "" {
		providerUsed = "edge"
	}
	transcript := simulatedTranscript(audioPath, hintText)
	confidence := 0.46
	if hintText != "" {
		confidence = 0.9
	}

	return map[string]any{
		"runtimeProfile":       runtimeProfileName(s.cfg.Runtime.Profile),
		"providerRequested":    valueOrNil(providerRequested),
		"providerUsed":         providerUsed,
		"source":               "simulated",
		"audioPath":            valueOrNil(audioPath),
		"audioRef":             valueOrDefault(audioPath, "memory://audio"),
		"transcript":           transcript,
		"confidence":           confidence,
		"language":             valueOrNil(language),
		"hasTinyWhisperBinary": tinyWhisperBinaryAvailable(),
	}, nil
}

func (s *Server) healthPayload() map[string]any {
	uptime := time.Since(s.startedAt)
	return map[string]any{
		"status":    "ok",
		"service":   s.build.Service,
		"version":   s.build.Version,
		"commit":    s.build.Commit,
		"built_at":  s.build.BuiltAt,
		"uptime_ms": uptime.Milliseconds(),
		"time":      time.Now().UTC().Format(time.RFC3339),
	}
}

func (s *Server) statusPayload() map[string]any {
	methods := s.methods.SupportedMethods()
	return map[string]any{
		"status":           "ok",
		"service":          s.build.Service,
		"version":          s.build.Version,
		"phase":            "phase-4-runtime-scaffold",
		"supportedMethods": methods,
		"count":            len(methods),
		"sessions": map[string]any{
			"count": s.sessions.Count(),
		},
		"channels": map[string]any{
			"count": len(s.channels.Status()),
			"items": s.channels.Status(),
		},
		"memory": map[string]any{
			"count":     s.memory.Count(),
			"lastError": s.memory.LastError(),
			"stats":     s.memory.Stats(),
		},
		"runtime": s.runtime.Snapshot(),
		"state": map[string]any{
			"sessions": s.state.List(),
		},
		"scheduler": s.scheduler.SnapshotStats(),
		"webLogin": map[string]any{
			"authorized": s.webLogin.HasAuthorizedSession(),
		},
		"security": s.guard.Snapshot(),
	}
}

func (s *Server) recordMemory(job scheduler.Job, role string, text string, payload map[string]any) {
	channel := strings.ToLower(toString(job.Params["channel"], ""))
	if channel == "" && job.SessionID != "" {
		if session, ok := s.sessions.Get(job.SessionID); ok {
			channel = strings.ToLower(session.Channel)
		}
	}
	entry := memory.MessageEntry{
		SessionID: job.SessionID,
		Channel:   channel,
		Method:    job.Method,
		Role:      role,
		Text:      text,
		Payload:   payload,
	}
	s.memory.Append(entry)
	s.state.TouchMessage(job.SessionID, channel, job.Method, text)
}

func (s *Server) resolveAuthMode() string {
	raw := strings.ToLower(strings.TrimSpace(s.cfg.Gateway.Server.AuthMode))
	switch raw {
	case "", "auto":
		if strings.TrimSpace(s.cfg.Gateway.Token) != "" {
			return "token"
		}
		if strings.TrimSpace(s.cfg.Gateway.Password) != "" {
			return "password"
		}
		return "none"
	case "none", "token", "password":
		return raw
	default:
		return "none"
	}
}

func (s *Server) validateAuth(mode string, token string, password string) (bool, string) {
	switch mode {
	case "none":
		return true, ""
	case "token":
		expected := strings.TrimSpace(s.cfg.Gateway.Token)
		if expected == "" {
			return false, "gateway token is not configured"
		}
		if strings.TrimSpace(token) == expected {
			return true, ""
		}
		return false, "invalid token"
	case "password":
		expected := strings.TrimSpace(s.cfg.Gateway.Password)
		if expected == "" {
			return false, "gateway password is not configured"
		}
		if strings.TrimSpace(password) == expected {
			return true, ""
		}
		return false, "invalid password"
	default:
		return false, "unsupported auth mode"
	}
}

func firstNonEmptyValue(params map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := params[key]; ok {
			if normalized := normalizeOptionalText(toString(value, ""), 0); normalized != "" {
				return normalized
			}
		}
	}
	return ""
}

func normalizeOptionalText(value string, maxRunes int) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if maxRunes <= 0 {
		return trimmed
	}
	runes := []rune(trimmed)
	if len(runes) <= maxRunes {
		return trimmed
	}
	return string(runes[:maxRunes])
}

func runtimeProfileName(raw string) string {
	profile := strings.ToLower(strings.TrimSpace(raw))
	if profile == "" {
		return "core"
	}
	return profile
}

func valueOrNil(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func valueOrDefault(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func extractSwarmTasks(raw any) []string {
	items, ok := raw.([]any)
	if !ok {
		return []string{}
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		switch value := item.(type) {
		case string:
			if normalized := normalizeOptionalText(value, 256); normalized != "" {
				out = append(out, normalized)
			}
		case map[string]any:
			title := normalizeOptionalText(toString(value["title"], toString(value["task"], "")), 256)
			if title != "" {
				out = append(out, title)
			}
		}
	}
	return out
}

func buildDefaultSwarmTasks(goal string) []string {
	if goal == "" {
		return []string{}
	}
	return []string{
		fmt.Sprintf("Analyze objective: %s", goal),
		"Implement the main changes with guarded rollout",
		"Validate behavior and produce release evidence",
	}
}

func classifySwarmTask(task string) string {
	normalized := strings.ToLower(task)
	switch {
	case strings.Contains(normalized, "test"),
		strings.Contains(normalized, "validate"),
		strings.Contains(normalized, "audit"):
		return "qa"
	case strings.Contains(normalized, "deploy"),
		strings.Contains(normalized, "release"),
		strings.Contains(normalized, "infra"):
		return "ops"
	case strings.Contains(normalized, "research"),
		strings.Contains(normalized, "investigate"),
		strings.Contains(normalized, "analyze"):
		return "research"
	default:
		return "builder"
	}
}

func inspectMediaPath(kind string, path string) map[string]any {
	cleaned := strings.TrimSpace(path)
	exists := false
	sizeBytes := any(nil)
	modifiedAtMs := any(nil)
	if info, err := os.Stat(cleaned); err == nil && !info.IsDir() {
		exists = true
		sizeBytes = info.Size()
		modifiedAtMs = info.ModTime().UTC().UnixMilli()
	}
	return map[string]any{
		"kind":         kind,
		"path":         cleaned,
		"exists":       exists,
		"extension":    strings.TrimPrefix(strings.ToLower(filepath.Ext(cleaned)), "."),
		"sizeBytes":    sizeBytes,
		"modifiedAtMs": modifiedAtMs,
	}
}

func inferMultimodalModalities(media []map[string]any, ocrText string) []string {
	out := make([]string, 0, 3)
	if len(media) > 0 {
		out = append(out, "vision")
	}
	for _, entry := range media {
		if kind := toString(entry["kind"], ""); kind == "screen" || kind == "video" {
			out = append(out, "screen")
			break
		}
	}
	if normalizeOptionalText(ocrText, 16_000) != "" {
		out = append(out, "text")
	}
	seen := map[string]struct{}{}
	deduped := make([]string, 0, len(out))
	for _, entry := range out {
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		deduped = append(deduped, entry)
	}
	return deduped
}

func summarizeMultimodalContext(prompt string, ocrText string, media []map[string]any, modalities []string) string {
	promptHint := normalizeOptionalText(prompt, 256)
	if promptHint == "" {
		promptHint = "inspect incoming scene and extract actionable state"
	}
	modalitiesText := "none"
	if len(modalities) > 0 {
		modalitiesText = strings.Join(modalities, ", ")
	}
	summary := fmt.Sprintf(
		"Multimodal pipeline detected %d modality(ies): %s. Objective: %s.",
		len(modalities),
		modalitiesText,
		promptHint,
	)
	if ocr := normalizeOptionalText(ocrText, 256); ocr != "" {
		summary += fmt.Sprintf(" OCR hint: %s.", ocr)
	}
	if len(media) > 0 {
		available := 0
		for _, entry := range media {
			if toBool(entry["exists"], false) {
				available++
			}
		}
		summary += fmt.Sprintf(
			" Media sources: %d provided (%d currently available on disk).",
			len(media),
			available,
		)
	}
	return summary
}

func simulatedTranscript(audioPath string, hintText string) string {
	if hint := normalizeOptionalText(hintText, 16_000); hint != "" {
		return hint
	}
	if path := normalizeOptionalText(audioPath, 2_048); path != "" {
		base := filepath.Base(path)
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		stem = normalizeOptionalText(stem, 256)
		if stem == "" {
			stem = "audio-capture"
		}
		return fmt.Sprintf("simulated transcript from %s", stem)
	}
	return "simulated transcript"
}

func tinyWhisperBinaryAvailable() bool {
	_, err := osexec.LookPath("tinywhisper")
	return err == nil
}

func envTruthy(key string) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func asMap(v any) map[string]any {
	if v == nil {
		return map[string]any{}
	}
	value, ok := v.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return value
}

func toString(v any, fallback string) string {
	raw, ok := v.(string)
	if !ok {
		return fallback
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func toStringSlice(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return []string{}
	}
	out := make([]string, 0, len(raw))
	for _, entry := range raw {
		if str, ok := entry.(string); ok && strings.TrimSpace(str) != "" {
			out = append(out, strings.TrimSpace(str))
		}
	}
	return out
}

func toDurationMs(v any, fallbackMs int64) time.Duration {
	switch value := v.(type) {
	case float64:
		return time.Duration(int64(value)) * time.Millisecond
	case int64:
		return time.Duration(value) * time.Millisecond
	case int:
		return time.Duration(int64(value)) * time.Millisecond
	default:
		return time.Duration(fallbackMs) * time.Millisecond
	}
}

func toInt(v any, fallback int) int {
	switch value := v.(type) {
	case float64:
		return int(value)
	case int:
		return value
	case int64:
		return int(value)
	default:
		return fallback
	}
}

func toFloat(v any, fallback float64) float64 {
	switch value := v.(type) {
	case float64:
		return value
	case int:
		return float64(value)
	case int64:
		return float64(value)
	default:
		return fallback
	}
}

func toBool(v any, fallback bool) bool {
	switch value := v.(type) {
	case bool:
		return value
	case string:
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "true" || normalized == "1" || normalized == "yes" {
			return true
		}
		if normalized == "false" || normalized == "0" || normalized == "no" {
			return false
		}
	}
	return fallback
}

func asSlice(v any) []float64 {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]float64, 0, len(raw))
	for _, entry := range raw {
		switch value := entry.(type) {
		case float64:
			out = append(out, value)
		case int:
			out = append(out, float64(value))
		case int64:
			out = append(out, float64(value))
		}
	}
	return out
}

func isMutatingMethod(method string) bool {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "connect",
		"agent",
		"send",
		"chat.send",
		"sessions.send",
		"browser.request",
		"browser.open",
		"web.login.start",
		"web.login.wait",
		"auth.oauth.start",
		"auth.oauth.wait",
		"auth.oauth.complete",
		"auth.oauth.logout",
		"channels.logout",
		"edge.finetune.run":
		return true
	default:
		return false
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
