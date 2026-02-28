package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"

	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/app"
)

func main() {
	var (
		configPath  = flag.String("config", "openclaw-go.toml", "Path to TOML config file")
		httpBind    = flag.String("http-bind", "", "Override HTTP bind for control/health server")
		doctor      = flag.Bool("doctor", false, "Run doctor diagnostics and exit")
		audit       = flag.Bool("security-audit", false, "Run security audit diagnostics and exit")
		fix         = flag.Bool("fix", false, "Apply safe remediations in security-audit mode")
		listMethods = flag.Bool("list-methods", false, "Print supported RPC methods and exit")
		deep        = flag.Bool("deep", false, "Enable deep probes in doctor/security-audit mode")
	)
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := app.Run(ctx, app.Options{
		ConfigPath:       *configPath,
		HTTPBindOverride: *httpBind,
		Doctor:           *doctor,
		SecurityAudit:    *audit,
		Fix:              *fix,
		ListMethods:      *listMethods,
		Deep:             *deep,
	}); err != nil {
		log.Fatalf("openclaw-go bootstrap failed: %v", err)
	}
}
