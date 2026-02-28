package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	webbridge "github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/bridge/web"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/buildinfo"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/channels"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/config"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/memory"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/protocol"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/rpc"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/scheduler"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/security"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/state"
	toolruntime "github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/tools/runtime"
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
	state     *state.Store
	guard     *security.Guard
	webLogin  *webbridge.Manager
}

func New(cfg config.Config, build buildinfo.Info) *Server {
	s := &Server{
		cfg:       cfg,
		build:     build,
		startedAt: time.Now().UTC(),
		methods:   rpc.DefaultRegistry(),
		sessions:  NewSessionRegistry(),
		tools:     toolruntime.NewDefault(),
		channels:  channels.NewRegistry(cfg.Channels.Telegram.BotToken, cfg.Channels.Telegram.DefaultTarget),
		memory:    memory.NewStore(cfg.Runtime.StatePath, 10_000),
		state:     state.NewStore(),
		guard:     security.NewGuard(cfg.Security),
		webLogin:  webbridge.NewManager(10 * time.Minute),
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
	case "agent.wait":
		return s.handleAgentWait(ctx, params)
	default:
		known := s.methods.Resolve(canonical).Known
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
		"runtime": map[string]any{
			"statePath": s.cfg.Runtime.StatePath,
		},
		"channels": map[string]any{
			"telegramConfigured": strings.TrimSpace(s.cfg.Channels.Telegram.BotToken) != "",
		},
		"security": s.guard.Snapshot(),
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
	if (canonical == "browser.request" || canonical == "browser.open") && !s.webLogin.HasAuthorizedSession() {
		return nil, &dispatchError{
			Code:    -32040,
			Message: "browser bridge requires active authorized login session",
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
		},
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
		"channels.logout":
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
