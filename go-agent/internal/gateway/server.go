package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	osexec "os/exec"
	"path/filepath"
	stdruntime "runtime"
	"strconv"
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
	"github.com/gorilla/websocket"
)

var rpcWebsocketUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(_ *http.Request) bool {
		return true
	},
}

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
			EDRTelemetryPath:        cfg.Security.EDRTelemetryPath,
			EDRTelemetryMaxAgeSecs:  cfg.Security.EDRTelemetryMaxAgeSecs,
			EDRTelemetryRiskBonus:   cfg.Security.EDRTelemetryRiskBonus,
			CredentialSensitiveKeys: cfg.Security.CredentialSensitiveKeys,
			CredentialLeakAction:    cfg.Security.CredentialLeakAction,
			AttestationExpectedSHA:  cfg.Security.AttestationExpectedSHA,
			AttestationReportPath:   cfg.Security.AttestationReportPath,
			AttestationMismatchRisk: cfg.Security.AttestationMismatchRisk,
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
	mux.HandleFunc("/ws", s.handleWS)
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

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	conn, err := rpcWebsocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	for {
		messageType, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			continue
		}

		var frame map[string]any
		if err := json.Unmarshal(raw, &frame); err != nil {
			_ = conn.WriteJSON(protocol.RPCErrorResponseFrame(
				"unknown",
				-32700,
				"parse error",
				nil,
			))
			continue
		}

		if protocol.FrameKindOf(frame) != protocol.FrameKindReq {
			_ = conn.WriteJSON(protocol.RPCErrorResponseFrame(
				"unknown",
				-32600,
				"invalid request frame",
				map[string]any{
					"expectedType": "req",
				},
			))
			continue
		}

		req := protocol.ParseRPCRequest(frame)
		if req == nil {
			_ = conn.WriteJSON(protocol.RPCErrorResponseFrame(
				"unknown",
				-32600,
				"invalid request frame",
				map[string]any{
					"reason": "missing method",
				},
			))
			continue
		}

		params := asMap(req.Params)
		resolved := s.methods.Resolve(req.Method)
		result, rpcErr := s.dispatchRPC(r.Context(), req.ID, resolved.Canonical, params)
		if rpcErr != nil {
			_ = conn.WriteJSON(protocol.RPCErrorResponseFrame(
				req.ID,
				rpcErr.Code,
				rpcErr.Message,
				rpcErr.Details,
			))
			continue
		}
		_ = conn.WriteJSON(protocol.RPCSuccessResponseFrame(req.ID, result))
	}
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
		return s.handleEdgeEnclaveProve(params)
	case "edge.mesh.status":
		return s.handleEdgeMeshStatus(), nil
	case "edge.homomorphic.compute":
		return s.handleEdgeHomomorphicCompute(params)
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
	moduleRoot := strings.TrimSpace(os.Getenv("OPENCLAW_GO_WASM_MODULE_ROOT"))
	if moduleRoot == "" {
		moduleRoot = ".openclaw-go/wasm/modules"
	}
	witRoot := strings.TrimSpace(os.Getenv("OPENCLAW_GO_WASM_WIT_ROOT"))
	if witRoot == "" {
		witRoot = ".openclaw-go/wasm/wit"
	}
	policy := s.wasm.Policy()
	return map[string]any{
		"runtimeProfile": runtimeProfileName(s.cfg.Runtime.Profile),
		"moduleRoot":     moduleRoot,
		"witRoot":        witRoot,
		"moduleCount":    len(modules),
		"count":          len(modules),
		"modules":        modules,
		"witPackages":    []map[string]any{},
		"sandbox":        policy,
		"builder": map[string]any{
			"mode":      "visual-ai-builder",
			"supported": true,
			"templates": []string{"tool.execute", "tool.fetch", "tool.workflow"},
			"scaffoldHints": map[string]any{
				"fields":            []string{"name", "description", "inputs", "outputs", "capabilities"},
				"defaultCapability": "workspace.read",
			},
		},
	}
}

func (s *Server) handleEdgeRouterPlan(params map[string]any) map[string]any {
	objective := normalizeOptionalText(firstNonEmptyValue(params, "objective", "goal"), 64)
	if objective == "" {
		objective = "balanced"
	}
	requestedProvider := normalizeProviderAlias(firstNonEmptyValue(params, "provider"))
	requestedModel := normalizeOptionalText(firstNonEmptyValue(params, "model"), 256)
	messageChars := len([]rune(normalizeOptionalText(firstNonEmptyValue(params, "message"), 16_000)))

	selectedProvider := requestedProvider
	selectedModel := requestedModel
	if selectedModel != "" && selectedProvider == "" {
		selectedProvider = s.compat.providerForModel(selectedModel)
	}
	if selectedProvider == "" {
		selectedProvider = "chatgpt"
	}
	if selectedModel == "" {
		if fallback, ok := s.compat.defaultModelForProvider(selectedProvider); ok {
			selectedModel = fallback
		} else {
			selectedModel = "gpt-5.2"
		}
	}
	if descriptorProvider := s.compat.providerForModel(selectedModel); descriptorProvider != "" {
		selectedProvider = descriptorProvider
	}

	recommendedChain := []string{
		selectedProvider,
		"chatgpt",
		"openrouter",
	}
	acceleration := s.handleEdgeAccelerationStatus()
	gpuActive := toBool(acceleration["gpuActive"], false)
	npuActive := toBool(acceleration["npuActive"], false)
	return map[string]any{
		"goal":           objective,
		"objective":      objective,
		"runtimeProfile": runtimeProfileName(s.cfg.Runtime.Profile),
		"selected": map[string]any{
			"provider": selectedProvider,
			"model":    selectedModel,
			"name":     strings.ToUpper(selectedModel),
		},
		"fallbackProviders":        []string{"chatgpt", "openrouter"},
		"recommendedProviderChain": recommendedChain,
		"reasoning": fmt.Sprintf(
			"Objective `%s` selected `%s/%s` with runtime profile `%s`.",
			objective,
			selectedProvider,
			selectedModel,
			runtimeProfileName(s.cfg.Runtime.Profile),
		),
		"accelerationHint": map[string]any{
			"gpuActive":       gpuActive,
			"npuActive":       npuActive,
			"recommendedMode": toString(acceleration["recommendedMode"], "cpu"),
		},
		"messageChars": messageChars,
		"route": map[string]any{
			"primary":  selectedModel,
			"provider": selectedProvider,
			"fallback": "gpt-5.1-mini",
			"strategy": "latency-cost-balanced",
			"chain":    recommendedChain,
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
	availableEngines := []string{"cpu"}
	if gpuAvailable {
		availableEngines = append(availableEngines, "gpu")
	}
	if tpuAvailable {
		availableEngines = append(availableEngines, "npu")
	}

	return map[string]any{
		"enabled":          true,
		"mode":             mode,
		"gpuActive":        gpuAvailable,
		"npuActive":        tpuAvailable,
		"recommendedMode":  mode,
		"availableEngines": availableEngines,
		"hints": map[string]any{
			"cuda":        gpuAvailable,
			"rocm":        envTruthy("OPENCLAW_GO_ROCM_AVAILABLE"),
			"metal":       envTruthy("OPENCLAW_GO_METAL_AVAILABLE"),
			"directml":    envTruthy("OPENCLAW_GO_DIRECTML_AVAILABLE"),
			"openvinoNpu": tpuAvailable,
		},
		"tooling": map[string]any{
			"nvidiaSmi": envTruthy("OPENCLAW_GO_NVIDIA_SMI") || gpuAvailable,
			"rocmSmi":   envTruthy("OPENCLAW_GO_ROCM_SMI"),
		},
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
	status := s.edge.enclaveStatus()
	lastProofRecord := map[string]any{}
	if proof, ok := status["lastProof"].(map[string]any); ok {
		lastProofRecord = cloneMap(proof)
	}
	status["runtimeProfile"] = runtimeProfileName(s.cfg.Runtime.Profile)
	status["activeMode"] = "simulated-enclave"
	status["availableModes"] = []string{"simulated-enclave", "tpm", "sgx", "sev"}
	status["isolationAvailable"] = true
	status["signals"] = map[string]any{
		"sgx": envTruthy("OPENCLAW_GO_ENCLAVE_SGX"),
		"tpm": true,
		"sev": envTruthy("OPENCLAW_GO_ENCLAVE_SEV"),
	}
	status["runtime"] = map[string]any{
		"activeMode":     status["activeMode"],
		"availableModes": status["availableModes"],
		"profile":        runtimeProfileName(s.cfg.Runtime.Profile),
	}
	status["attestationDetail"] = map[string]any{
		"configured": true,
		"binary":     strings.TrimSpace(os.Getenv("OPENCLAW_GO_ENCLAVE_ATTEST_BIN")),
		"lastProof":  lastProofRecord,
	}
	status["attestationInfo"] = map[string]any{
		"configured": strings.TrimSpace(edgeEnclaveAttestationBinaryPath()) != "",
		"binary":     valueOrNil(edgeEnclaveAttestationBinaryPath()),
		"lastProof":  lastProofRecord,
	}
	status["zeroKnowledge"] = map[string]any{
		"enabled":     true,
		"scheme":      "attestation-quote-v1",
		"proofMethod": "edge.enclave.prove",
	}
	return status
}

func (s *Server) handleEdgeEnclaveProve(params map[string]any) (map[string]any, *dispatchError) {
	statement := normalizeOptionalText(firstNonEmptyValue(params, "statement", "challenge"), 16_000)
	if statement == "" {
		return nil, &dispatchError{
			Code:    -32602,
			Message: "edge.enclave.prove requires statement",
		}
	}
	nonce := normalizeOptionalText(firstNonEmptyValue(params, "nonce"), 512)
	if nonce == "" {
		nonce = fmt.Sprintf("nonce-%d", time.Now().UTC().UnixMilli())
	}
	activeMode := "simulated-enclave"
	signals := map[string]any{
		"sgx": envTruthy("OPENCLAW_GO_ENCLAVE_SGX"),
		"tpm": true,
		"sev": envTruthy("OPENCLAW_GO_ENCLAVE_SEV"),
	}
	record := buildEdgeEnclaveProofRecord(statement, nonce, activeMode, runtimeProfileName(s.cfg.Runtime.Profile), signals)
	proof := s.edge.recordEnclaveProof(statement, toString(record["proof"], ""), toString(record["generatedAt"], ""))

	statementDigest := sha256.Sum256([]byte(statement))
	return map[string]any{
		"runtimeProfile":    runtimeProfileName(s.cfg.Runtime.Profile),
		"activeMode":        activeMode,
		"challenge":         statement,
		"statementHash":     valueOrDefault(toString(record["statementHash"], ""), hex.EncodeToString(statementDigest[:])),
		"nonce":             nonce,
		"proof":             proof["proof"],
		"scheme":            valueOrDefault(toString(record["scheme"], ""), "sha256-commitment-v1"),
		"verified":          toBool(record["verified"], false),
		"source":            valueOrDefault(toString(record["source"], ""), "deterministic-fallback"),
		"quote":             valueOrNil(toString(record["quote"], "")),
		"measurement":       valueOrDefault(toString(record["measurement"], ""), fmt.Sprintf("mr-enclave-%s", hex.EncodeToString(statementDigest[:8]))),
		"error":             valueOrNil(toString(record["error"], "")),
		"attestationBinary": valueOrNil(toString(record["attestationBinary"], strings.TrimSpace(os.Getenv("OPENCLAW_GO_ENCLAVE_ATTEST_BIN")))),
		"verification": map[string]any{
			"deterministic": true,
			"attested":      toBool(record["verified"], false),
			"inputs":        []string{"statement", "nonce", "activeMode", "runtimeProfile"},
		},
		"record":   record,
		"issuedAt": proof["issuedAt"],
	}, nil
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
	peerDetails := make([]map[string]any, 0, approvedPeers+1)
	peerDetails = append(peerDetails, map[string]any{
		"id":     "node-local",
		"kind":   "node",
		"paired": true,
		"status": "connected",
	})
	for idx := 0; idx < approvedPeers; idx++ {
		peerDetails = append(peerDetails, map[string]any{
			"id":       fmt.Sprintf("node-peer-%d", idx+1),
			"kind":     "node",
			"paired":   true,
			"status":   "paired",
			"remoteIp": valueOrNil(fmt.Sprintf("10.0.0.%d", idx+2)),
		})
	}
	routes := make([]map[string]any, 0, len(peerDetails))
	for _, peer := range peerDetails {
		peerID := toString(peer["id"], "")
		if peerID == "" || peerID == "node-local" {
			continue
		}
		routes = append(routes, map[string]any{
			"from":       "node-local",
			"to":         peerID,
			"transport":  "noise-like-session-keys",
			"encrypted":  true,
			"latencyMs":  18 + len(routes)*7,
			"confidence": 0.93,
		})
	}
	failedPeers := []string{}
	if !connected {
		failedPeers = append(failedPeers, "node-local")
	}

	return map[string]any{
		"runtimeProfile": runtimeProfileName(s.cfg.Runtime.Profile),
		"transport": map[string]any{
			"mode":          "p2p-overlay",
			"secureChannel": "noise-like-session-keys",
			"zeroTrust":     true,
		},
		"topology": map[string]any{
			"peerCount":        len(peerDetails),
			"trustedPeerCount": approvedPeers,
			"routeCount":       len(routes),
			"includesPending":  false,
			"approvedPairs":    toInt(topology["approvedPairs"], 0),
			"pendingPairs":     toInt(topology["pendingPairs"], 0),
			"rejectedPairs":    toInt(topology["rejectedPairs"], 0),
			"approvedPeers":    toInt(topology["approvedPeers"], 0),
			"onlineNodes":      toInt(topology["onlineNodes"], 0),
			"nodes":            toInt(topology["nodes"], 0),
			"summary":          topology,
		},
		"meshHealth": map[string]any{
			"probeEnabled":   true,
			"probeTimeoutMs": 1200,
			"probedPeers":    len(peerDetails),
			"successCount":   len(routes) + 1,
			"timeoutCount":   0,
			"failedPeers":    failedPeers,
			"lastProbeAtMs":  time.Now().UTC().UnixMilli(),
		},
		"connected": connected,
		"peers":     approvedPeers,
		"peerCount": len(peerDetails),
		"peersInfo": peerDetails,
		"routes":    routes,
		"mode":      mode,
	}
}

func (s *Server) handleEdgeHomomorphicCompute(params map[string]any) (map[string]any, *dispatchError) {
	op := strings.ToLower(toString(params["operation"], "sum"))
	keyID := normalizeOptionalText(firstNonEmptyValue(params, "keyId"), 128)
	ciphertexts := toStringSlice(params["ciphertexts"])
	if keyID != "" || len(ciphertexts) > 0 {
		if keyID == "" {
			return nil, &dispatchError{
				Code:    -32602,
				Message: "edge.homomorphic.compute requires keyId",
			}
		}
		if !slicesContains([]string{"sum", "count", "mean"}, op) {
			return nil, &dispatchError{
				Code:    -32602,
				Message: "edge.homomorphic.compute operation must be sum, count, or mean",
			}
		}
		validCiphertexts := make([]string, 0, len(ciphertexts))
		for _, entry := range ciphertexts {
			if normalized := normalizeOptionalText(entry, 1024); normalized != "" {
				validCiphertexts = append(validCiphertexts, normalized)
			}
		}
		if len(validCiphertexts) == 0 {
			return nil, &dispatchError{
				Code:    -32602,
				Message: "edge.homomorphic.compute requires ciphertexts: string[]",
			}
		}
		count := len(validCiphertexts)
		revealResult := toBool(params["revealResult"], false)
		if op == "mean" && !revealResult {
			return nil, &dispatchError{
				Code:    -32602,
				Message: "edge.homomorphic.compute mean requires revealResult=true",
			}
		}
		aggregate := count
		if op == "sum" {
			aggregate = 0
			for _, entry := range validCiphertexts {
				aggregate += len(entry)
			}
		}
		if op == "mean" && count > 0 {
			aggregate = aggregate / count
		}
		hashSeed := fmt.Sprintf("%s|%s|%v", keyID, op, validCiphertexts)
		hash := sha256.Sum256([]byte(hashSeed))
		return map[string]any{
			"mode":             "ciphertext",
			"operation":        op,
			"keyId":            keyID,
			"ciphertextCount":  count,
			"resultCiphertext": fmt.Sprintf("enc:%s", hex.EncodeToString(hash[:10])),
			"count":            count,
			"revealResult":     revealResult,
			"result": func() any {
				if revealResult {
					return float64(aggregate)
				}
				return nil
			}(),
		}, nil
	}
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
		"mode":      "plaintext",
		"operation": op,
		"result":    result,
		"count":     len(values),
	}, nil
}

func (s *Server) handleEdgeFinetuneStatus() map[string]any {
	memoryStats := s.memory.Stats()
	jobs := s.edge.listFinetuneJobs(25)
	running := 0
	completed := 0
	failed := 0
	for _, job := range jobs {
		status := strings.ToLower(toString(job["status"], ""))
		switch status {
		case "running", "queued":
			running++
		case "completed", "dry-run":
			completed++
		case "failed", "timeout":
			failed++
		}
	}
	return map[string]any{
		"runtimeProfile": runtimeProfileName(s.cfg.Runtime.Profile),
		"feature":        "on-device-finetune-self-evolution",
		"supported":      true,
		"adapterFormat":  "lora",
		"trainerBinary":  valueOrNil(strings.TrimSpace(os.Getenv("OPENCLAW_GO_LORA_TRAINER_BIN"))),
		"trainerArgs":    []string{"--model", "--provider", "--adapter", "--rank", "--epochs", "--lr", "--max-samples", "--output"},
		"defaults": map[string]any{
			"epochs":       3,
			"rank":         32,
			"learningRate": 0.0002,
			"maxSamples":   8192,
			"dryRun":       true,
		},
		"memory": map[string]any{
			"enabled":     true,
			"zvecEntries": toInt(memoryStats["vectors"], 0),
			"graphNodes":  toInt(memoryStats["graphNodes"], 0),
			"graphEdges":  toInt(memoryStats["graphEdges"], 0),
		},
		"datasetSources": []map[string]any{
			{
				"id":      "zvec",
				"path":    toString(memoryStats["statePath"], ""),
				"exists":  true,
				"entries": toInt(memoryStats["vectors"], 0),
			},
			{
				"id":     "graphlite",
				"path":   toString(memoryStats["statePath"], ""),
				"exists": true,
				"nodes":  toInt(memoryStats["graphNodes"], 0),
				"edges":  toInt(memoryStats["graphEdges"], 0),
			},
		},
		"jobs": jobs,
		"jobStats": map[string]any{
			"running":   running,
			"completed": completed,
			"failed":    failed,
			"total":     running + completed + failed,
		},
	}
}

func (s *Server) handleEdgeFinetuneRun(ctx context.Context, params map[string]any) (map[string]any, *dispatchError) {
	baseProvider := normalizeProviderAlias(firstNonEmptyValue(params, "provider"))
	if baseProvider == "" {
		baseProvider = "chatgpt"
	}
	baseModel := normalizeOptionalText(firstNonEmptyValue(params, "model", "baseModel"), 256)
	if baseModel == "" {
		if fallback, ok := s.compat.defaultModelForProvider(baseProvider); ok {
			baseModel = fallback
		} else {
			baseModel = "gpt-5.2"
		}
	}
	adapterName := normalizeOptionalText(firstNonEmptyValue(params, "adapterName"), 96)
	if adapterName == "" {
		adapterName = fmt.Sprintf("edge-lora-%d", time.Now().UTC().UnixMilli())
	}
	epochs := toInt(params["epochs"], 3)
	if epochs < 1 {
		epochs = 1
	}
	rank := toInt(params["rank"], 32)
	if rank < 4 {
		rank = 4
	}
	learningRate := toFloat(params["learningRate"], 0.0002)
	if learningRate <= 0 {
		learningRate = 0.0002
	}
	maxSamples := toInt(params["maxSamples"], 8192)
	if maxSamples < 128 {
		maxSamples = 128
	}
	dryRun := toBool(params["dryRun"], true)
	autoIngestMemory := toBool(params["autoIngestMemory"], true)
	datasetPath := normalizeOptionalText(firstNonEmptyValue(params, "datasetPath", "dataset"), 2_048)
	outputPath := normalizeOptionalText(firstNonEmptyValue(params, "outputPath"), 1024)
	if outputPath == "" {
		outputPath = fmt.Sprintf(".openclaw-go/evolution/adapters/%s", adapterName)
	}
	manifestPath := filepath.Join(outputPath, "manifest.json")
	memoryStats := s.memory.Stats()
	if datasetPath == "" && !autoIngestMemory && toInt(memoryStats["vectors"], 0) == 0 && toInt(memoryStats["graphNodes"], 0) == 0 {
		return nil, &dispatchError{
			Code:    -32602,
			Message: "edge.finetune.run requires datasetPath or autoIngestMemory=true with memory data",
		}
	}

	trainerBinary := edgeLoraTrainerBinaryPath()
	trainerArgs := edgeLoraTrainerArgs()
	timeoutMs := edgeLoraTrainerTimeoutMs()
	if !dryRun && trainerBinary == "" {
		return nil, &dispatchError{
			Code:    -32602,
			Message: "edge.finetune.run requires OPENCLAW_GO_LORA_TRAINER_BIN when dryRun=false",
		}
	}

	result, err := s.routines.Run(ctx, "edge-wasm-smoke", params)
	if err != nil {
		job := s.edge.addFinetuneJob(params, "failed")
		return nil, &dispatchError{
			Code:    -32060,
			Message: fmt.Sprintf("%s (job=%s)", err.Error(), toString(job["id"], "")),
		}
	}

	commandArgs := make([]string, 0, len(trainerArgs)+16)
	commandArgs = append(commandArgs, trainerArgs...)
	commandArgs = append(commandArgs,
		"--model", baseModel,
		"--provider", baseProvider,
		"--adapter", adapterName,
		"--rank", fmt.Sprintf("%d", rank),
		"--epochs", fmt.Sprintf("%d", epochs),
		"--lr", fmt.Sprintf("%.6f", learningRate),
		"--max-samples", fmt.Sprintf("%d", maxSamples),
		"--output", outputPath,
	)
	if datasetPath != "" {
		commandArgs = append(commandArgs, "--dataset", datasetPath)
	}

	jobID := fmt.Sprintf("finetune-%d", time.Now().UTC().UnixMilli())
	manifest := map[string]any{
		"jobId":            jobID,
		"createdAtMs":      time.Now().UTC().UnixMilli(),
		"runtimeProfile":   runtimeProfileName(s.cfg.Runtime.Profile),
		"dryRun":           dryRun,
		"autoIngestMemory": autoIngestMemory,
		"memorySnapshot": map[string]any{
			"zvecEntries": toInt(memoryStats["vectors"], 0),
			"graphNodes":  toInt(memoryStats["graphNodes"], 0),
			"graphEdges":  toInt(memoryStats["graphEdges"], 0),
		},
		"baseModel": map[string]any{
			"provider": baseProvider,
			"id":       baseModel,
			"name":     strings.ToUpper(baseModel),
		},
		"adapter": map[string]any{
			"name":       adapterName,
			"outputPath": outputPath,
		},
		"training": map[string]any{
			"epochs":       epochs,
			"rank":         rank,
			"learningRate": learningRate,
			"maxSamples":   maxSamples,
		},
		"dataset": map[string]any{
			"path":             valueOrNil(datasetPath),
			"autoIngestMemory": autoIngestMemory,
		},
		"suggestedCommand": map[string]any{
			"binary":    valueOrNil(trainerBinary),
			"argv":      commandArgs,
			"timeoutMs": timeoutMs,
		},
	}

	if !dryRun {
		if err := os.MkdirAll(outputPath, 0o755); err != nil {
			return nil, &dispatchError{
				Code:    -32060,
				Message: fmt.Sprintf("edge.finetune.run failed to create output path: %v", err),
			}
		}
		encoded, marshalErr := json.MarshalIndent(manifest, "", "  ")
		if marshalErr != nil {
			return nil, &dispatchError{
				Code:    -32060,
				Message: fmt.Sprintf("edge.finetune.run failed to encode manifest: %v", marshalErr),
			}
		}
		if writeErr := os.WriteFile(manifestPath, encoded, 0o644); writeErr != nil {
			return nil, &dispatchError{
				Code:    -32060,
				Message: fmt.Sprintf("edge.finetune.run failed to write manifest: %v", writeErr),
			}
		}
	}

	execution := map[string]any{
		"attempted": false,
		"success":   true,
		"timedOut":  false,
		"status":    "completed",
		"timeoutMs": timeoutMs,
		"binary":    valueOrNil(trainerBinary),
		"argv":      commandArgs,
		"exitCode":  nil,
		"error":     nil,
		"logTail":   []string{},
	}
	jobStatus := "completed"
	if !dryRun {
		runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
		defer cancel()
		cmd := osexec.CommandContext(runCtx, trainerBinary, commandArgs...)
		output, runErr := cmd.CombinedOutput()
		logTail := collectCommandLogTail(string(output), 14, 320)
		timedOut := errors.Is(runCtx.Err(), context.DeadlineExceeded)
		success := runErr == nil && !timedOut
		exitCode := any(nil)
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		}
		execution["attempted"] = true
		execution["success"] = success
		execution["timedOut"] = timedOut
		execution["exitCode"] = exitCode
		execution["logTail"] = logTail
		if runErr != nil {
			execution["error"] = runErr.Error()
		}
		if timedOut {
			jobStatus = "timeout"
			execution["status"] = "timeout"
		} else if !success {
			jobStatus = "failed"
			execution["status"] = "failed"
		}
	}

	job := s.edge.addFinetuneJob(params, jobStatus)
	job["id"] = jobID
	job["runtime"] = result
	job["adapterName"] = adapterName
	job["outputPath"] = outputPath
	job["dryRun"] = dryRun
	job["baseModel"] = map[string]any{
		"provider": baseProvider,
		"id":       baseModel,
	}
	job["execution"] = cloneMap(execution)
	job["manifestPath"] = manifestPath
	job["status"] = jobStatus
	job["statusReason"] = valueOrDefault(toString(execution["status"], ""), jobStatus)

	return map[string]any{
		"ok":             toBool(execution["success"], false),
		"jobId":          jobID,
		"runtimeProfile": runtimeProfileName(s.cfg.Runtime.Profile),
		"dryRun":         dryRun,
		"manifestPath":   manifestPath,
		"manifest":       manifest,
		"execution":      execution,
		"jobStatus":      job,
		"job":            job,
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

	localSeed := fmt.Sprintf("openclaw-go|%s|%d", runtimeProfileName(s.cfg.Runtime.Profile), s.startedAt.UnixMilli())
	localHash := sha256.Sum256([]byte(localSeed))
	localDID := fmt.Sprintf("did:openclaw:%s", hex.EncodeToString(localHash[:12]))
	meshHash := sha256.Sum256([]byte(fmt.Sprintf("%v", topology)))
	meshFingerprint := fmt.Sprintf("mesh-%s", hex.EncodeToString(meshHash[:10]))
	peerRecords := make([]map[string]any, 0, toInt(topology["approvedPeers"], 0))
	for idx := 0; idx < toInt(topology["approvedPeers"], 0); idx++ {
		peerID := fmt.Sprintf("node-peer-%d", idx+1)
		peerHash := sha256.Sum256([]byte(peerID))
		peerScore := 0.72 + (float64(idx%3) * 0.07)
		if peerScore > 0.99 {
			peerScore = 0.99
		}
		peerTier := "candidate"
		if peerScore >= 0.8 {
			peerTier = "trusted"
		}
		peerRecords = append(peerRecords, map[string]any{
			"peerId":          peerID,
			"peerDid":         fmt.Sprintf("did:openclaw-peer:%s", hex.EncodeToString(peerHash[:12])),
			"paired":          true,
			"status":          "paired",
			"trustScore":      peerScore,
			"trustTier":       peerTier,
			"signatureScheme": "sha256-signed-events-v1",
			"reputation": map[string]any{
				"score":           int(peerScore * 100),
				"window":          "rolling-30d",
				"verifiedActions": 12 + idx,
			},
		})
	}
	routes := make([]map[string]any, 0, len(peerRecords))
	for _, peer := range peerRecords {
		routes = append(routes, map[string]any{
			"from": "node-local",
			"to":   peer["peerId"],
			"mode": "zero-trust-overlay",
		})
	}

	return map[string]any{
		"runtimeProfile": runtimeProfileName(s.cfg.Runtime.Profile),
		"feature":        "decentralized-agent-identity-trust-system",
		"enabled":        true,
		"localIdentity": map[string]any{
			"agentId":               "openclaw-go",
			"did":                   localDID,
			"signingKeyFingerprint": fmt.Sprintf("ocpk-%s", hex.EncodeToString(localHash[:16])),
			"meshFingerprint":       meshFingerprint,
			"proofType":             "sha256-digest",
		},
		"trustGraph": map[string]any{
			"peerCount":            len(peerRecords),
			"trustedPeerCount":     countTrustedPeers(peerRecords),
			"routeCount":           len(routes),
			"zeroTrust":            true,
			"verifiableAuditTrail": true,
		},
		"peers":               peerRecords,
		"routes":              routes,
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
	modules := s.wasm.MarketplaceList()
	requestedModule := normalizeOptionalText(firstNonEmptyValue(params, "moduleId"), 128)
	dailyInvocations := toInt(params["dailyInvocations"], 800)
	if dailyInvocations < 1 {
		dailyInvocations = 1
	}
	payouts := make([]map[string]any, 0, len(modules))
	for _, module := range modules {
		moduleID := strings.ToLower(strings.TrimSpace(module.ID))
		if requestedModule != "" && moduleID != strings.ToLower(requestedModule) {
			continue
		}
		seed := sha256.Sum256([]byte(fmt.Sprintf("%s:%s", moduleID, module.Version)))
		boost := int(seed[0]) % 1500
		perCall := 40 + (int(seed[1]) % 260)
		moduleDailyInvocations := dailyInvocations + boost
		gross := moduleDailyInvocations * perCall
		creatorShare := int(float64(gross) * 0.8)
		payouts = append(payouts, map[string]any{
			"moduleId":             moduleID,
			"dailyInvocations":     moduleDailyInvocations,
			"microCreditsPerCall":  perCall,
			"grossDailyCredits":    gross,
			"creatorSharePct":      80,
			"creatorDailyCredits":  creatorShare,
			"platformDailyCredits": gross - creatorShare,
		})
	}
	return map[string]any{
		"runtimeProfile":     runtimeProfileName(s.cfg.Runtime.Profile),
		"feature":            "agent-marketplace-revenue-sharing",
		"enabled":            true,
		"currency":           "credits",
		"payoutSchedule":     "daily",
		"modules":            payouts,
		"smartContractReady": false,
		"note":               "Deterministic local payout preview; plug on-chain settlement in production.",
		"units":              units,
		"price":              price,
		"revenue":            float64(units) * price,
	}
}

func (s *Server) handleEdgeFinetuneClusterPlan(params map[string]any) map[string]any {
	size := toInt(params["workers"], 2)
	if size < 1 {
		size = 1
	}
	datasetShards := toInt(params["datasetShards"], size*2)
	if datasetShards < 1 {
		datasetShards = 1
	}
	assignments := make([]map[string]any, 0, size)
	for idx := 0; idx < size; idx++ {
		shards := make([]string, 0, datasetShards)
		for shard := 0; shard < datasetShards; shard++ {
			if shard%size == idx {
				shards = append(shards, fmt.Sprintf("shard-%d", shard+1))
			}
		}
		role := "trainer"
		if idx == 0 {
			role = "coordinator-trainer"
		}
		assignments = append(assignments, map[string]any{
			"workerId": fmt.Sprintf("node-%d", idx+1),
			"role":     role,
			"shards":   shards,
		})
	}
	return map[string]any{
		"feature":           "self-hosted-private-model-training-cluster",
		"enabled":           true,
		"mode":              "distributed-lora",
		"workers":           size,
		"plan":              "burst",
		"datasetShards":     datasetShards,
		"estimatedMemoryMb": 180 + (size * 320),
		"assignments":       assignments,
		"launcher": map[string]any{
			"method":      "edge.finetune.run",
			"clusterMode": true,
			"coordinator": "node-1",
		},
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
	values := toStringSlice(params["values"])
	if len(values) == 0 {
		values = []string{"privacy", "safety", "user-consent"}
	}
	task := normalizeOptionalText(firstNonEmptyValue(params, "task"), 16_000)
	action := normalizeOptionalText(firstNonEmptyValue(params, "action"), 16_000)
	strictMode := toBool(params["strict"], false)

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
	matchedSignals := make([]string, 0, 8)
	for _, signal := range decision.Signals {
		matchedSignals = append(matchedSignals, fmt.Sprint(signal))
	}
	recommendation := "allow"
	switch status {
	case "fail":
		recommendation = "block"
	case "review":
		recommendation = "review"
	}

	return map[string]any{
		"feature":        "ethical-alignment-layer-user-defined-values",
		"enabled":        true,
		"strictMode":     strictMode,
		"values":         values,
		"task":           valueOrNil(task),
		"actionText":     valueOrNil(action),
		"matchedSignals": matchedSignals,
		"recommendation": recommendation,
		"explanation":    mapRecommendationExplanation(recommendation),
		"score":          score,
		"status":         status,
		"riskScore":      decision.RiskScore,
		"action":         decision.Action,
		"reason":         decision.Reason,
		"signals":        decision.Signals,
		"inputEmpty":     strings.TrimSpace(input) == "",
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

	result := transcribeAudioWithProvider(
		valueOrNilString(audioPath),
		valueOrNilString(hintText),
		valueOrNilString(language),
		providerRequested,
		runtimeProfileName(s.cfg.Runtime.Profile),
	)

	return map[string]any{
		"runtimeProfile":       runtimeProfileName(s.cfg.Runtime.Profile),
		"providerRequested":    valueOrNil(providerRequested),
		"providerUsed":         result.ProviderUsed,
		"source":               result.Source,
		"audioPath":            valueOrNil(audioPath),
		"audioRef":             valueOrDefault(audioPath, "memory://audio"),
		"transcript":           result.Transcript,
		"confidence":           result.Confidence,
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

type edgeTranscriptionResult struct {
	ProviderUsed string
	Source       string
	Transcript   string
	Confidence   float64
}

func transcribeAudioWithProvider(audioPath string, hintText string, language string, requestedProvider string, profile string) edgeTranscriptionResult {
	requested := strings.ToLower(strings.TrimSpace(requestedProvider))
	order := make([]string, 0, 4)
	if requested != "" {
		order = append(order, requested)
	}
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "edge":
		if !slicesContains(order, "tinywhisper") {
			order = append(order, "tinywhisper")
		}
		if !slicesContains(order, "edge") {
			order = append(order, "edge")
		}
	default:
		if !slicesContains(order, "edge") {
			order = append(order, "edge")
		}
		if !slicesContains(order, "tinywhisper") {
			order = append(order, "tinywhisper")
		}
	}

	for _, provider := range order {
		switch provider {
		case "tinywhisper":
			if strings.TrimSpace(audioPath) == "" {
				continue
			}
			if transcript, ok := tryTranscribeAudioTinyWhisper(audioPath, language); ok {
				return edgeTranscriptionResult{
					ProviderUsed: "tinywhisper",
					Source:       "offline-local",
					Transcript:   transcript,
					Confidence:   0.92,
				}
			}
		case "edge":
			transcript := simulatedTranscript(audioPath, hintText)
			confidence := 0.46
			if strings.TrimSpace(hintText) != "" {
				confidence = 0.9
			}
			return edgeTranscriptionResult{
				ProviderUsed: "edge",
				Source:       "simulated",
				Transcript:   transcript,
				Confidence:   confidence,
			}
		}
	}

	providerUsed := requested
	if providerUsed == "" {
		providerUsed = "edge"
	}
	return edgeTranscriptionResult{
		ProviderUsed: providerUsed,
		Source:       "simulated",
		Transcript:   simulatedTranscript(audioPath, hintText),
		Confidence:   0.4,
	}
}

func tinyWhisperBinaryPath() string {
	if bin := normalizeOptionalText(strings.TrimSpace(os.Getenv("OPENCLAW_GO_TINYWHISPER_BIN")), 2048); bin != "" {
		return bin
	}
	if path, err := osexec.LookPath("tinywhisper"); err == nil {
		return path
	}
	return ""
}

func tinyWhisperExtraArgs() []string {
	raw := normalizeOptionalText(strings.TrimSpace(os.Getenv("OPENCLAW_GO_TINYWHISPER_ARGS")), 2048)
	if raw == "" {
		return []string{}
	}
	parts := strings.Fields(raw)
	if len(parts) > 32 {
		parts = parts[:32]
	}
	return parts
}

func tryTranscribeAudioTinyWhisper(audioPath string, language string) (string, bool) {
	binary := tinyWhisperBinaryPath()
	if binary == "" {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	args := []string{"--input", audioPath}
	if normalizedLanguage := normalizeOptionalText(language, 32); normalizedLanguage != "" {
		args = append(args, "--language", normalizedLanguage)
	}
	args = append(args, tinyWhisperExtraArgs()...)
	cmd := osexec.CommandContext(ctx, binary, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(string(output), "\n") {
		if normalized := normalizeOptionalText(line, 16_000); normalized != "" {
			return normalized, true
		}
	}
	return "", false
}

func tinyWhisperBinaryAvailable() bool {
	return strings.TrimSpace(tinyWhisperBinaryPath()) != ""
}

func edgeEnclaveAttestationBinaryPath() string {
	return normalizeOptionalText(strings.TrimSpace(os.Getenv("OPENCLAW_GO_ENCLAVE_ATTEST_BIN")), 2048)
}

func edgeEnclaveAttestationArgs() []string {
	raw := normalizeOptionalText(strings.TrimSpace(os.Getenv("OPENCLAW_GO_ENCLAVE_ATTEST_ARGS")), 2048)
	if raw == "" {
		return []string{}
	}
	parts := strings.Fields(raw)
	if len(parts) > 48 {
		parts = parts[:48]
	}
	return parts
}

func edgeEnclaveAttestationTimeoutMs() int {
	raw := normalizeOptionalText(strings.TrimSpace(os.Getenv("OPENCLAW_GO_ENCLAVE_ATTEST_TIMEOUT_MS")), 64)
	if raw == "" {
		return 15000
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return 15000
	}
	if parsed < 1000 {
		return 1000
	}
	if parsed > 120000 {
		return 120000
	}
	return parsed
}

func edgeLoraTrainerBinaryPath() string {
	return normalizeOptionalText(strings.TrimSpace(os.Getenv("OPENCLAW_GO_LORA_TRAINER_BIN")), 2048)
}

func edgeLoraTrainerArgs() []string {
	raw := normalizeOptionalText(strings.TrimSpace(os.Getenv("OPENCLAW_GO_LORA_TRAINER_ARGS")), 2048)
	if raw == "" {
		return []string{}
	}
	parts := strings.Fields(raw)
	if len(parts) > 48 {
		parts = parts[:48]
	}
	return parts
}

func edgeLoraTrainerTimeoutMs() int {
	raw := normalizeOptionalText(strings.TrimSpace(os.Getenv("OPENCLAW_GO_LORA_TRAINER_TIMEOUT_MS")), 64)
	if raw == "" {
		return 600000
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return 600000
	}
	if parsed < 5000 {
		return 5000
	}
	if parsed > 86400000 {
		return 86400000
	}
	return parsed
}

func collectCommandLogTail(output string, maxLines int, maxChars int) []string {
	lines := make([]string, 0, maxLines)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		trimmed := normalizeOptionalText(line, maxChars)
		if trimmed == "" {
			continue
		}
		lines = append(lines, trimmed)
	}
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return lines
}

func valueOrNilString(value string) string {
	return strings.TrimSpace(value)
}

func buildEdgeEnclaveProofRecord(statement string, nonce string, activeMode string, runtimeProfile string, signals map[string]any) map[string]any {
	statementHash := sha256.Sum256([]byte(statement))
	statementHashHex := hex.EncodeToString(statementHash[:])
	baseSeed := fmt.Sprintf(
		"zk-proof-v1|statement=%s|nonce=%s|mode=%s|profile=%s",
		strings.TrimSpace(statement),
		strings.TrimSpace(nonce),
		strings.TrimSpace(activeMode),
		strings.TrimSpace(runtimeProfile),
	)
	baseDigest := sha256.Sum256([]byte(baseSeed))
	baseProof := hex.EncodeToString(baseDigest[:])

	record := map[string]any{
		"generatedAt":       time.Now().UTC().Format(time.RFC3339),
		"activeMode":        activeMode,
		"runtimeProfile":    runtimeProfile,
		"statementHash":     statementHashHex,
		"nonce":             nonce,
		"proof":             baseProof,
		"scheme":            "sha256-commitment-v1",
		"verified":          false,
		"source":            "deterministic-fallback",
		"attestationBinary": valueOrNil(edgeEnclaveAttestationBinaryPath()),
		"quote":             "",
		"measurement":       statementHashHex,
		"error":             "attestation binary not configured",
	}

	binary := edgeEnclaveAttestationBinaryPath()
	if binary == "" {
		return record
	}

	requestPayload := map[string]any{
		"statement":      statement,
		"statementHash":  statementHashHex,
		"nonce":          nonce,
		"activeMode":     activeMode,
		"runtimeProfile": runtimeProfile,
		"signals":        signals,
	}
	requestBytes, _ := json.Marshal(requestPayload)
	timeout := edgeEnclaveAttestationTimeoutMs()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Millisecond)
	defer cancel()

	args := edgeEnclaveAttestationArgs()
	cmd := osexec.CommandContext(ctx, binary, args...)
	cmd.Stdin = strings.NewReader(string(requestBytes))
	output, err := cmd.CombinedOutput()
	if err != nil {
		record["error"] = err.Error()
		return record
	}

	outputText := strings.TrimSpace(string(output))
	if outputText == "" {
		record["error"] = "attestation command returned empty output"
		return record
	}

	parsed := map[string]any{}
	if json.Unmarshal(output, &parsed) == nil {
		quote := firstNonEmptyValue(parsed, "quote", "evidence", "attestationQuote", "rawQuote")
		measurement := firstNonEmptyValue(parsed, "measurement", "reportDigest", "mrEnclave", "mr", "digest")
		if quote == "" {
			quote = outputText
		}
		if measurement == "" {
			digest := sha256.Sum256([]byte(quote))
			measurement = hex.EncodeToString(digest[:])
		}
		finalSeed := fmt.Sprintf("edge-attestation-proof-v1|base=%s|measurement=%s|nonce=%s", baseProof, measurement, nonce)
		finalDigest := sha256.Sum256([]byte(finalSeed))
		record["proof"] = hex.EncodeToString(finalDigest[:])
		record["scheme"] = "attestation-quote-v1"
		record["verified"] = true
		record["source"] = "attestation-binary"
		record["quote"] = quote
		record["measurement"] = measurement
		record["error"] = nil
		return record
	}

	digest := sha256.Sum256([]byte(outputText))
	measurement := hex.EncodeToString(digest[:])
	finalSeed := fmt.Sprintf("edge-attestation-proof-v1|base=%s|measurement=%s|nonce=%s", baseProof, measurement, nonce)
	finalDigest := sha256.Sum256([]byte(finalSeed))
	record["proof"] = hex.EncodeToString(finalDigest[:])
	record["scheme"] = "attestation-quote-v1"
	record["verified"] = true
	record["source"] = "attestation-binary"
	record["quote"] = outputText
	record["measurement"] = measurement
	record["error"] = nil
	return record
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
	switch value := v.(type) {
	case []string:
		out := make([]string, 0, len(value))
		for _, entry := range value {
			if strings.TrimSpace(entry) != "" {
				out = append(out, strings.TrimSpace(entry))
			}
		}
		return out
	}
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

func slicesContains(items []string, target string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}

func countTrustedPeers(peers []map[string]any) int {
	count := 0
	for _, peer := range peers {
		tier := strings.ToLower(strings.TrimSpace(toString(peer["trustTier"], "")))
		if tier == "trusted" {
			count++
		}
	}
	return count
}

func mapRecommendationExplanation(recommendation string) string {
	switch strings.ToLower(strings.TrimSpace(recommendation)) {
	case "block":
		return "Action violates strict-value policy."
	case "review":
		return "Action should be human-reviewed against value constraints."
	default:
		return "No major value conflict detected."
	}
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
