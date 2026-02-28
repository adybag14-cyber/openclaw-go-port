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
	defaultStatePath   = "memory://openclaw-go-state"
)

type Config struct {
	Gateway  GatewayConfig  `toml:"gateway"`
	Runtime  RuntimeConfig  `toml:"runtime"`
	Channels ChannelsConfig `toml:"channels"`
	Security SecurityConfig `toml:"security"`
}

type GatewayConfig struct {
	URL      string              `toml:"url"`
	Token    string              `toml:"token"`
	Password string              `toml:"password"`
	Server   GatewayServerConfig `toml:"server"`
}

type GatewayServerConfig struct {
	Bind     string `toml:"bind"`
	HTTPBind string `toml:"http_bind"`
	AuthMode string `toml:"auth_mode"`
}

type RuntimeConfig struct {
	AuditOnly bool   `toml:"audit_only"`
	StatePath string `toml:"state_path"`
}

type ChannelsConfig struct {
	Telegram TelegramChannelConfig `toml:"telegram"`
}

type TelegramChannelConfig struct {
	BotToken      string `toml:"bot_token"`
	DefaultTarget string `toml:"default_target"`
}

type SecurityConfig struct {
	PolicyBundlePath       string            `toml:"policy_bundle_path"`
	DefaultAction          string            `toml:"default_action"`
	ToolPolicies           map[string]string `toml:"tool_policies"`
	BlockedMessagePatterns []string          `toml:"blocked_message_patterns"`
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
			StatePath: defaultStatePath,
		},
		Channels: ChannelsConfig{
			Telegram: TelegramChannelConfig{
				BotToken:      "",
				DefaultTarget: "",
			},
		},
		Security: SecurityConfig{
			PolicyBundlePath: "memory://security-policy.json",
			DefaultAction:    "allow",
			ToolPolicies:     map[string]string{},
			BlockedMessagePatterns: []string{
				"rm -rf /",
				"del /f /s /q",
			},
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
	setIfPresent("OPENCLAW_GO_GATEWAY_PASSWORD", &cfg.Gateway.Password)
	setIfPresent("OPENCLAW_GO_WS_BIND", &cfg.Gateway.Server.Bind)
	setIfPresent("OPENCLAW_GO_HTTP_BIND", &cfg.Gateway.Server.HTTPBind)
	setIfPresent("OPENCLAW_GO_GATEWAY_AUTH_MODE", &cfg.Gateway.Server.AuthMode)
	setIfPresent("OPENCLAW_GO_STATE_PATH", &cfg.Runtime.StatePath)
	setIfPresent("OPENCLAW_GO_TELEGRAM_BOT_TOKEN", &cfg.Channels.Telegram.BotToken)
	setIfPresent("OPENCLAW_GO_TELEGRAM_DEFAULT_TARGET", &cfg.Channels.Telegram.DefaultTarget)
	setIfPresent("OPENCLAW_GO_POLICY_BUNDLE_PATH", &cfg.Security.PolicyBundlePath)
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
	if strings.TrimSpace(cfg.Runtime.StatePath) == "" {
		return errors.New("runtime.state_path cannot be empty")
	}
	if strings.TrimSpace(cfg.Security.DefaultAction) == "" {
		return errors.New("security.default_action cannot be empty")
	}
	return nil
}
