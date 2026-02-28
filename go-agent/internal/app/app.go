package app

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"sort"
	"time"

	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/buildinfo"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/config"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/gateway"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/rpc"
	securityaudit "github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/security/audit"
)

type Options struct {
	ConfigPath       string
	HTTPBindOverride string

	Doctor        bool
	SecurityAudit bool
	ListMethods   bool
	Deep          bool
	Output        io.Writer
}

func Run(ctx context.Context, opts Options) error {
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return err
	}

	if opts.HTTPBindOverride != "" {
		cfg.Gateway.Server.HTTPBind = opts.HTTPBindOverride
	}

	if opts.Doctor || opts.SecurityAudit || opts.ListMethods {
		return runDiagnostics(cfg, opts)
	}

	server := gateway.New(cfg, buildinfo.Default())
	return server.Run(ctx)
}

func runDiagnostics(cfg config.Config, opts Options) error {
	out := opts.Output
	if out == nil {
		out = os.Stdout
	}

	payload := map[string]any{
		"ts":      time.Now().UTC().Format(time.RFC3339),
		"service": buildinfo.Default().Service,
		"version": buildinfo.Default().Version,
		"config": map[string]any{
			"gatewayURL":     cfg.Gateway.URL,
			"httpBind":       cfg.Gateway.Server.HTTPBind,
			"authMode":       cfg.Gateway.Server.AuthMode,
			"runtimeProfile": cfg.Runtime.Profile,
			"auditOnly":      cfg.Runtime.AuditOnly,
		},
	}

	if opts.ListMethods {
		methods := rpc.DefaultRegistry().SupportedMethods()
		sort.Strings(methods)
		payload["methods"] = map[string]any{
			"count": len(methods),
			"items": methods,
		}
	}

	if opts.Doctor || opts.SecurityAudit {
		report := securityaudit.Run(cfg, securityaudit.Options{
			Deep: opts.Deep,
		})
		payload["securityAudit"] = report
		payload["doctor"] = map[string]any{
			"healthy": report.Summary.Critical == 0,
			"summary": report.Summary,
		}
	}

	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(payload)
}
