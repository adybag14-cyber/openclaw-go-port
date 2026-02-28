package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/buildinfo"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/config"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/protocol"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/rpc"
)

type Server struct {
	cfg       config.Config
	build     buildinfo.Info
	startedAt time.Time
	methods   *rpc.MethodRegistry
}

func New(cfg config.Config, build buildinfo.Info) *Server {
	return &Server{
		cfg:       cfg,
		build:     build,
		startedAt: time.Now().UTC(),
		methods:   rpc.DefaultRegistry(),
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
		return <-errCh
	case err := <-errCh:
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

	resolved := s.methods.Resolve(req.Method)
	switch strings.ToLower(resolved.Canonical) {
	case "health":
		writeJSON(w, http.StatusOK, protocol.RPCSuccessResponseFrame(req.ID, s.healthPayload()))
	case "status":
		writeJSON(w, http.StatusOK, protocol.RPCSuccessResponseFrame(req.ID, s.statusPayload()))
	default:
		writeJSON(w, http.StatusOK, protocol.RPCErrorResponseFrame(
			req.ID,
			-32601,
			"method not implemented in go phase-2 scaffold",
			map[string]any{
				"requested": req.Method,
				"canonical": resolved.Canonical,
				"known":     resolved.Known,
			},
		))
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
		"phase":            "phase-2-scaffold",
		"supportedMethods": methods,
		"count":            len(methods),
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
