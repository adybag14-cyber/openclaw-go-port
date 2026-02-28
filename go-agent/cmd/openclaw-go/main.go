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
		configPath = flag.String("config", "openclaw-go.toml", "Path to TOML config file")
		httpBind   = flag.String("http-bind", "", "Override HTTP bind for control/health server")
	)
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := app.Run(ctx, app.Options{
		ConfigPath:       *configPath,
		HTTPBindOverride: *httpBind,
	}); err != nil {
		log.Fatalf("openclaw-go bootstrap failed: %v", err)
	}
}
