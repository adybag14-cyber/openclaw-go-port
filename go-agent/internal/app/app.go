package app

import (
	"context"

	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/buildinfo"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/config"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/gateway"
)

type Options struct {
	ConfigPath       string
	HTTPBindOverride string
}

func Run(ctx context.Context, opts Options) error {
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return err
	}

	if opts.HTTPBindOverride != "" {
		cfg.Gateway.Server.HTTPBind = opts.HTTPBindOverride
	}

	server := gateway.New(cfg, buildinfo.Default())
	return server.Run(ctx)
}
