package config

import (
	"errors"
	"os"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const (
	defaultGatewayURL                    = "ws://127.0.0.1:8080/gateway"
	defaultGatewayBind                   = "127.0.0.1:8765"
	defaultHTTPBind                      = "127.0.0.1:8766"
	defaultAuthMode                      = "auto"
	defaultStatePath                     = ".openclaw-go/state/memory.json"
	defaultProfile                       = "core"
	defaultMemoryMaxEntries              = 10_000
	defaultWebLoginTTLMinutes            = 1440
	defaultModelCatalogRefreshTTLSeconds = 300
	defaultTelegramLiveStreaming         = true
	defaultTelegramStreamChunkChars      = 700
	defaultTelegramStreamChunkDelayMs    = 250
	defaultTelegramTypingIndicators      = true
	defaultTelegramTypingIntervalMs      = 3500
	defaultBrowserBridgeEndpoint         = "http://127.0.0.1:43010"
	defaultBrowserBridgeRequestTimeoutMs = 180000
	defaultBrowserBridgeRetries          = 2
	defaultBrowserBridgeRetryBackoffMs   = 750
	defaultBrowserBridgeCircuitFailures  = 3
	defaultBrowserBridgeCircuitCooldown  = 10000
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
	AuditOnly                     bool                `toml:"audit_only"`
	StatePath                     string              `toml:"state_path"`
	Profile                       string              `toml:"profile"`
	MemoryMaxEntries              int                 `toml:"memory_max_entries"`
	WebLoginTTLMinutes            int                 `toml:"web_login_ttl_minutes"`
	ModelCatalogRefreshTTLSeconds int                 `toml:"model_catalog_refresh_ttl_seconds"`
	TelegramLiveStreaming         bool                `toml:"telegram_live_streaming"`
	TelegramStreamChunkChars      int                 `toml:"telegram_stream_chunk_chars"`
	TelegramStreamChunkDelayMs    int                 `toml:"telegram_stream_chunk_delay_ms"`
	TelegramTypingIndicators      bool                `toml:"telegram_typing_indicators"`
	TelegramTypingIntervalMs      int                 `toml:"telegram_typing_interval_ms"`
	BrowserBridge                 BrowserBridgeConfig `toml:"browser_bridge"`
}

type BrowserBridgeConfig struct {
	Enabled              bool   `toml:"enabled"`
	Endpoint             string `toml:"endpoint"`
	RequestTimeoutMs     int    `toml:"request_timeout_ms"`
	Retries              int    `toml:"retries"`
	RetryBackoffMs       int    `toml:"retry_backoff_ms"`
	CircuitFailThreshold int    `toml:"circuit_fail_threshold"`
	CircuitCooldownMs    int    `toml:"circuit_cooldown_ms"`
}

type ChannelsConfig struct {
	Telegram   TelegramChannelConfig `toml:"telegram"`
	WhatsApp   ChannelAdapterConfig  `toml:"whatsapp"`
	Discord    ChannelAdapterConfig  `toml:"discord"`
	Slack      ChannelAdapterConfig  `toml:"slack"`
	Feishu     ChannelAdapterConfig  `toml:"feishu"`
	QQ         ChannelAdapterConfig  `toml:"qq"`
	WeWork     ChannelAdapterConfig  `toml:"wework"`
	DingTalk   ChannelAdapterConfig  `toml:"dingtalk"`
	Infoflow   ChannelAdapterConfig  `toml:"infoflow"`
	GoogleChat ChannelAdapterConfig  `toml:"googlechat"`
	Teams      ChannelAdapterConfig  `toml:"teams"`
	Matrix     ChannelAdapterConfig  `toml:"matrix"`
	Signal     ChannelAdapterConfig  `toml:"signal"`
	Line       ChannelAdapterConfig  `toml:"line"`
	Mattermost ChannelAdapterConfig  `toml:"mattermost"`
	IMessage   ChannelAdapterConfig  `toml:"imessage"`
}

type TelegramChannelConfig struct {
	BotToken      string `toml:"bot_token"`
	DefaultTarget string `toml:"default_target"`
}

type ChannelAdapterConfig struct {
	Enabled       bool              `toml:"enabled"`
	Token         string            `toml:"token"`
	DefaultTarget string            `toml:"default_target"`
	WebhookURL    string            `toml:"webhook_url"`
	AuthHeader    string            `toml:"auth_header"`
	AuthPrefix    string            `toml:"auth_prefix"`
	Headers       map[string]string `toml:"headers"`
}

type SecurityConfig struct {
	PolicyBundlePath        string            `toml:"policy_bundle_path"`
	DefaultAction           string            `toml:"default_action"`
	ToolPolicies            map[string]string `toml:"tool_policies"`
	BlockedMessagePatterns  []string          `toml:"blocked_message_patterns"`
	TelemetryHighRiskTags   []string          `toml:"telemetry_high_risk_tags"`
	TelemetryAction         string            `toml:"telemetry_action"`
	EDRTelemetryPath        string            `toml:"edr_telemetry_path"`
	EDRTelemetryMaxAgeSecs  int               `toml:"edr_telemetry_max_age_secs"`
	EDRTelemetryRiskBonus   int               `toml:"edr_telemetry_risk_bonus"`
	CredentialSensitiveKeys []string          `toml:"credential_sensitive_keys"`
	CredentialLeakAction    string            `toml:"credential_leak_action"`
	AttestationExpectedSHA  string            `toml:"attestation_expected_sha256"`
	AttestationReportPath   string            `toml:"attestation_report_path"`
	AttestationMismatchRisk int               `toml:"attestation_mismatch_risk_bonus"`
	LoopGuardEnabled        bool              `toml:"loop_guard_enabled"`
	LoopGuardWindowMS       int               `toml:"loop_guard_window_ms"`
	LoopGuardMaxHits        int               `toml:"loop_guard_max_hits"`
	RiskReviewThreshold     int               `toml:"risk_review_threshold"`
	RiskBlockThreshold      int               `toml:"risk_block_threshold"`
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
			AuditOnly:                     false,
			StatePath:                     defaultStatePath,
			Profile:                       defaultProfile,
			MemoryMaxEntries:              defaultMemoryMaxEntries,
			WebLoginTTLMinutes:            defaultWebLoginTTLMinutes,
			ModelCatalogRefreshTTLSeconds: defaultModelCatalogRefreshTTLSeconds,
			TelegramLiveStreaming:         defaultTelegramLiveStreaming,
			TelegramStreamChunkChars:      defaultTelegramStreamChunkChars,
			TelegramStreamChunkDelayMs:    defaultTelegramStreamChunkDelayMs,
			TelegramTypingIndicators:      defaultTelegramTypingIndicators,
			TelegramTypingIntervalMs:      defaultTelegramTypingIntervalMs,
			BrowserBridge: BrowserBridgeConfig{
				Enabled:              true,
				Endpoint:             defaultBrowserBridgeEndpoint,
				RequestTimeoutMs:     defaultBrowserBridgeRequestTimeoutMs,
				Retries:              defaultBrowserBridgeRetries,
				RetryBackoffMs:       defaultBrowserBridgeRetryBackoffMs,
				CircuitFailThreshold: defaultBrowserBridgeCircuitFailures,
				CircuitCooldownMs:    defaultBrowserBridgeCircuitCooldown,
			},
		},
		Channels: ChannelsConfig{
			Telegram: TelegramChannelConfig{
				BotToken:      "",
				DefaultTarget: "",
			},
			WhatsApp:   defaultChannelAdapterConfig(),
			Discord:    defaultChannelAdapterConfig(),
			Slack:      defaultChannelAdapterConfig(),
			Feishu:     defaultChannelAdapterConfig(),
			QQ:         defaultChannelAdapterConfig(),
			WeWork:     defaultChannelAdapterConfig(),
			DingTalk:   defaultChannelAdapterConfig(),
			Infoflow:   defaultChannelAdapterConfig(),
			GoogleChat: defaultChannelAdapterConfig(),
			Teams:      defaultChannelAdapterConfig(),
			Matrix:     defaultChannelAdapterConfig(),
			Signal:     defaultChannelAdapterConfig(),
			Line:       defaultChannelAdapterConfig(),
			Mattermost: defaultChannelAdapterConfig(),
			IMessage:   defaultChannelAdapterConfig(),
		},
		Security: SecurityConfig{
			PolicyBundlePath: "memory://security-policy.json",
			DefaultAction:    "allow",
			ToolPolicies:     map[string]string{},
			BlockedMessagePatterns: []string{
				"rm -rf /",
				"del /f /s /q",
			},
			TelemetryHighRiskTags: []string{
				"edr:high-risk",
				"behavior:ransomware",
				"threat:critical",
			},
			TelemetryAction:        "review",
			EDRTelemetryPath:       "",
			EDRTelemetryMaxAgeSecs: 300,
			EDRTelemetryRiskBonus:  45,
			CredentialSensitiveKeys: []string{
				"apiKey",
				"api_key",
				"token",
				"password",
				"secret",
				"authorization",
			},
			CredentialLeakAction:    "block",
			AttestationExpectedSHA:  "",
			AttestationReportPath:   "",
			AttestationMismatchRisk: 55,
			LoopGuardEnabled:        true,
			LoopGuardWindowMS:       5000,
			LoopGuardMaxHits:        8,
			RiskReviewThreshold:     70,
			RiskBlockThreshold:      90,
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
	setIfPresent("OPENCLAW_GO_RUNTIME_PROFILE", &cfg.Runtime.Profile)
	setIntIfPresent("OPENCLAW_GO_MEMORY_MAX_ENTRIES", &cfg.Runtime.MemoryMaxEntries)
	setIntIfPresent("OPENCLAW_GO_WEB_LOGIN_TTL_MINUTES", &cfg.Runtime.WebLoginTTLMinutes)
	setIntIfPresent("OPENCLAW_GO_MODEL_CATALOG_REFRESH_TTL_SECONDS", &cfg.Runtime.ModelCatalogRefreshTTLSeconds)
	setBoolIfPresent("OPENCLAW_GO_TELEGRAM_LIVE_STREAMING", &cfg.Runtime.TelegramLiveStreaming)
	setIntIfPresent("OPENCLAW_GO_TELEGRAM_STREAM_CHUNK_CHARS", &cfg.Runtime.TelegramStreamChunkChars)
	setIntIfPresent("OPENCLAW_GO_TELEGRAM_STREAM_CHUNK_DELAY_MS", &cfg.Runtime.TelegramStreamChunkDelayMs)
	setBoolIfPresent("OPENCLAW_GO_TELEGRAM_TYPING_INDICATORS", &cfg.Runtime.TelegramTypingIndicators)
	setIntIfPresent("OPENCLAW_GO_TELEGRAM_TYPING_INTERVAL_MS", &cfg.Runtime.TelegramTypingIntervalMs)
	setBoolIfPresent("OPENCLAW_GO_BROWSER_BRIDGE_ENABLED", &cfg.Runtime.BrowserBridge.Enabled)
	setIfPresent("OPENCLAW_GO_BROWSER_BRIDGE_ENDPOINT", &cfg.Runtime.BrowserBridge.Endpoint)
	setIntIfPresent("OPENCLAW_GO_BROWSER_BRIDGE_REQUEST_TIMEOUT_MS", &cfg.Runtime.BrowserBridge.RequestTimeoutMs)
	setIntIfPresent("OPENCLAW_GO_BROWSER_BRIDGE_RETRIES", &cfg.Runtime.BrowserBridge.Retries)
	setIntIfPresent("OPENCLAW_GO_BROWSER_BRIDGE_RETRY_BACKOFF_MS", &cfg.Runtime.BrowserBridge.RetryBackoffMs)
	setIntIfPresent("OPENCLAW_GO_BROWSER_BRIDGE_CIRCUIT_FAIL_THRESHOLD", &cfg.Runtime.BrowserBridge.CircuitFailThreshold)
	setIntIfPresent("OPENCLAW_GO_BROWSER_BRIDGE_CIRCUIT_COOLDOWN_MS", &cfg.Runtime.BrowserBridge.CircuitCooldownMs)
	setIfPresent("OPENCLAW_GO_TELEGRAM_BOT_TOKEN", &cfg.Channels.Telegram.BotToken)
	setIfPresent("OPENCLAW_GO_TELEGRAM_DEFAULT_TARGET", &cfg.Channels.Telegram.DefaultTarget)
	setIfPresent("OPENCLAW_GO_POLICY_BUNDLE_PATH", &cfg.Security.PolicyBundlePath)
	setIfPresent("OPENCLAW_GO_EDR_TELEMETRY_PATH", &cfg.Security.EDRTelemetryPath)
	setIntIfPresent("OPENCLAW_GO_EDR_TELEMETRY_MAX_AGE_SECS", &cfg.Security.EDRTelemetryMaxAgeSecs)
	setIntIfPresent("OPENCLAW_GO_EDR_TELEMETRY_RISK_BONUS", &cfg.Security.EDRTelemetryRiskBonus)
	setIfPresent("OPENCLAW_GO_ATTESTATION_EXPECTED_SHA256", &cfg.Security.AttestationExpectedSHA)
	setIfPresent("OPENCLAW_GO_ATTESTATION_REPORT_PATH", &cfg.Security.AttestationReportPath)
	setIntIfPresent("OPENCLAW_GO_ATTESTATION_MISMATCH_RISK_BONUS", &cfg.Security.AttestationMismatchRisk)
}

func setIfPresent(env string, dest *string) {
	if v, ok := os.LookupEnv(env); ok {
		trimmed := strings.TrimSpace(v)
		if trimmed != "" {
			*dest = trimmed
		}
	}
}

func setBoolIfPresent(env string, dest *bool) {
	if v, ok := os.LookupEnv(env); ok {
		normalized := strings.ToLower(strings.TrimSpace(v))
		switch normalized {
		case "1", "true", "yes", "y", "on":
			*dest = true
		case "0", "false", "no", "n", "off":
			*dest = false
		}
	}
}

func setIntIfPresent(env string, dest *int) {
	if v, ok := os.LookupEnv(env); ok {
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return
		}
		parsed, err := strconv.Atoi(trimmed)
		if err != nil {
			return
		}
		*dest = parsed
	}
}

func defaultChannelAdapterConfig() ChannelAdapterConfig {
	return ChannelAdapterConfig{
		Enabled:       false,
		Token:         "",
		DefaultTarget: "",
		WebhookURL:    "",
		AuthHeader:    "Authorization",
		AuthPrefix:    "Bearer",
		Headers:       map[string]string{},
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
	if strings.TrimSpace(cfg.Runtime.Profile) == "" {
		return errors.New("runtime.profile cannot be empty")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Runtime.Profile)) {
	case "core", "edge":
	default:
		return errors.New("runtime.profile must be one of: core, edge")
	}
	if cfg.Runtime.MemoryMaxEntries < -1 {
		return errors.New("runtime.memory_max_entries must be -1 (unlimited), 0 (unlimited), or a positive integer")
	}
	if cfg.Runtime.WebLoginTTLMinutes <= 0 {
		return errors.New("runtime.web_login_ttl_minutes must be > 0")
	}
	if cfg.Runtime.ModelCatalogRefreshTTLSeconds <= 0 {
		return errors.New("runtime.model_catalog_refresh_ttl_seconds must be > 0")
	}
	if cfg.Runtime.TelegramStreamChunkChars <= 0 {
		return errors.New("runtime.telegram_stream_chunk_chars must be > 0")
	}
	if cfg.Runtime.TelegramStreamChunkDelayMs < 0 {
		return errors.New("runtime.telegram_stream_chunk_delay_ms cannot be negative")
	}
	if cfg.Runtime.TelegramTypingIntervalMs <= 0 {
		return errors.New("runtime.telegram_typing_interval_ms must be > 0")
	}
	if cfg.Runtime.BrowserBridge.Enabled && strings.TrimSpace(cfg.Runtime.BrowserBridge.Endpoint) == "" {
		return errors.New("runtime.browser_bridge.endpoint cannot be empty when browser bridge is enabled")
	}
	if cfg.Runtime.BrowserBridge.RequestTimeoutMs <= 0 {
		return errors.New("runtime.browser_bridge.request_timeout_ms must be > 0")
	}
	if cfg.Runtime.BrowserBridge.Retries < 0 {
		return errors.New("runtime.browser_bridge.retries cannot be negative")
	}
	if cfg.Runtime.BrowserBridge.RetryBackoffMs < 0 {
		return errors.New("runtime.browser_bridge.retry_backoff_ms cannot be negative")
	}
	if cfg.Runtime.BrowserBridge.CircuitFailThreshold < 1 {
		return errors.New("runtime.browser_bridge.circuit_fail_threshold must be >= 1")
	}
	if cfg.Runtime.BrowserBridge.CircuitCooldownMs <= 0 {
		return errors.New("runtime.browser_bridge.circuit_cooldown_ms must be > 0")
	}
	if strings.TrimSpace(cfg.Security.DefaultAction) == "" {
		return errors.New("security.default_action cannot be empty")
	}
	if strings.TrimSpace(cfg.Security.TelemetryAction) == "" {
		return errors.New("security.telemetry_action cannot be empty")
	}
	if strings.TrimSpace(cfg.Security.CredentialLeakAction) == "" {
		return errors.New("security.credential_leak_action cannot be empty")
	}
	if cfg.Security.EDRTelemetryMaxAgeSecs <= 0 {
		return errors.New("security.edr_telemetry_max_age_secs must be > 0")
	}
	if cfg.Security.EDRTelemetryRiskBonus <= 0 || cfg.Security.EDRTelemetryRiskBonus > 100 {
		return errors.New("security.edr_telemetry_risk_bonus must be between 1 and 100")
	}
	if cfg.Security.AttestationMismatchRisk <= 0 || cfg.Security.AttestationMismatchRisk > 100 {
		return errors.New("security.attestation_mismatch_risk_bonus must be between 1 and 100")
	}
	expectedSHA := strings.TrimSpace(cfg.Security.AttestationExpectedSHA)
	if expectedSHA != "" {
		normalized := strings.ToLower(expectedSHA)
		if len(normalized) != 64 {
			return errors.New("security.attestation_expected_sha256 must be a 64-character hex digest")
		}
		for _, ch := range normalized {
			if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
				return errors.New("security.attestation_expected_sha256 must be a 64-character hex digest")
			}
		}
	}
	if cfg.Security.LoopGuardWindowMS < 0 {
		return errors.New("security.loop_guard_window_ms cannot be negative")
	}
	if cfg.Security.LoopGuardMaxHits < 0 {
		return errors.New("security.loop_guard_max_hits cannot be negative")
	}
	if cfg.Security.RiskReviewThreshold < 0 || cfg.Security.RiskReviewThreshold > 100 {
		return errors.New("security.risk_review_threshold must be between 0 and 100")
	}
	if cfg.Security.RiskBlockThreshold < 0 || cfg.Security.RiskBlockThreshold > 100 {
		return errors.New("security.risk_block_threshold must be between 0 and 100")
	}
	if cfg.Security.RiskBlockThreshold < cfg.Security.RiskReviewThreshold {
		return errors.New("security.risk_block_threshold must be >= security.risk_review_threshold")
	}
	channelAdapters := map[string]ChannelAdapterConfig{
		"whatsapp":   cfg.Channels.WhatsApp,
		"discord":    cfg.Channels.Discord,
		"slack":      cfg.Channels.Slack,
		"feishu":     cfg.Channels.Feishu,
		"qq":         cfg.Channels.QQ,
		"wework":     cfg.Channels.WeWork,
		"dingtalk":   cfg.Channels.DingTalk,
		"infoflow":   cfg.Channels.Infoflow,
		"googlechat": cfg.Channels.GoogleChat,
		"teams":      cfg.Channels.Teams,
		"matrix":     cfg.Channels.Matrix,
		"signal":     cfg.Channels.Signal,
		"line":       cfg.Channels.Line,
		"mattermost": cfg.Channels.Mattermost,
		"imessage":   cfg.Channels.IMessage,
	}
	for name, adapter := range channelAdapters {
		if !adapter.Enabled {
			continue
		}
		if strings.TrimSpace(adapter.Token) == "" && strings.TrimSpace(adapter.WebhookURL) == "" {
			return errors.New("channels." + name + " requires token or webhook_url when enabled")
		}
	}
	return nil
}
