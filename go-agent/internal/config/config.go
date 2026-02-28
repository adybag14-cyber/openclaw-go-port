package config

import (
	"errors"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const (
	defaultGatewayURL  = "ws://127.0.0.1:8080/gateway"
	defaultGatewayBind = "127.0.0.1:8765"
	defaultHTTPBind    = "127.0.0.1:8766"
	defaultAuthMode    = "auto"
)

type Config struct {
	Gateway GatewayConfig `toml:"gateway"`
	Runtime RuntimeConfig `toml:"runtime"`
}

type GatewayConfig struct {
	URL    string              `toml:"url"`
	Token  string              `toml:"token"`
	Server GatewayServerConfig `toml:"server"`
}

type GatewayServerConfig struct {
	Bind     string `toml:"bind"`
	HTTPBind string `toml:"http_bind"`
	AuthMode string `toml:"auth_mode"`
}

type RuntimeConfig struct {
	AuditOnly bool `toml:"audit_only"`
}

func Default() Config {
	return Config{
		Gateway: GatewayConfig{
			URL: defaultGatewayURL,
			Server: GatewayServerConfig{
				Bind:     defaultGatewayBind,
				HTTPBind: defaultHTTPBind,
				AuthMode: defaultAuthMode,
			},
		},
		Runtime: RuntimeConfig{
			AuditOnly: false,
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()

	if path != "" {
		if _, err := os.Stat(path); err == nil {
			raw, err := os.ReadFile(path)
			if err != nil {
				return Config{}, err
			}
			if err := toml.Unmarshal(raw, &cfg); err != nil {
				return Config{}, err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return Config{}, err
		}
	}

	applyEnvOverrides(&cfg)
	if err := validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	setIfPresent("OPENCLAW_GO_GATEWAY_URL", &cfg.Gateway.URL)
	setIfPresent("OPENCLAW_GO_GATEWAY_TOKEN", &cfg.Gateway.Token)
	setIfPresent("OPENCLAW_GO_WS_BIND", &cfg.Gateway.Server.Bind)
	setIfPresent("OPENCLAW_GO_HTTP_BIND", &cfg.Gateway.Server.HTTPBind)
	setIfPresent("OPENCLAW_GO_GATEWAY_AUTH_MODE", &cfg.Gateway.Server.AuthMode)
}

func setIfPresent(env string, dest *string) {
	if v, ok := os.LookupEnv(env); ok {
		trimmed := strings.TrimSpace(v)
		if trimmed != "" {
			*dest = trimmed
		}
	}
}

func validate(cfg Config) error {
	if strings.TrimSpace(cfg.Gateway.URL) == "" {
		return errors.New("gateway.url cannot be empty")
	}
	if strings.TrimSpace(cfg.Gateway.Server.Bind) == "" {
		return errors.New("gateway.server.bind cannot be empty")
	}
	if strings.TrimSpace(cfg.Gateway.Server.HTTPBind) == "" {
		return errors.New("gateway.server.http_bind cannot be empty")
	}
	if strings.TrimSpace(cfg.Gateway.Server.AuthMode) == "" {
		return errors.New("gateway.server.auth_mode cannot be empty")
	}
	return nil
}
