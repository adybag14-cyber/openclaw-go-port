package gateway

import (
	"runtime"
	"strings"
	"testing"

	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/buildinfo"
	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/config"
)

func TestCompatTTSProvidersIncludeCoreCatalog(t *testing.T) {
	s := New(config.Default(), buildinfo.Default())
	defer s.Close()

	payload := s.handleCompatTTSProviders()
	rawProviders, ok := payload["providers"].([]map[string]any)
	if !ok || len(rawProviders) == 0 {
		t.Fatalf("expected providers catalog in tts.providers payload")
	}

	seen := map[string]bool{}
	for _, provider := range rawProviders {
		id := normalizeTTSProviderID(toString(provider["id"], ""))
		if id != "" {
			seen[id] = true
		}
	}
	for _, required := range []string{"native", "openai-voice", "kittentts"} {
		if !seen[required] {
			t.Fatalf("expected provider %q in catalog", required)
		}
	}
}

func TestCompatTTSConvertFallbackWhenProviderUnavailable(t *testing.T) {
	s := New(config.Default(), buildinfo.Default())
	defer s.Close()

	result, derr := s.handleCompatTTSConvert(map[string]any{
		"text":     "hello from fallback",
		"provider": "openai-voice",
	})
	if derr != nil {
		t.Fatalf("expected fallback conversion success, got error: %v", derr)
	}
	if toString(result["provider"], "") != "native" {
		t.Fatalf("expected provider fallback to native, got %v", result["provider"])
	}
	if !toBool(result["fallback"], false) {
		t.Fatalf("expected fallback=true for unavailable provider")
	}
	if toBool(result["realAudio"], true) {
		t.Fatalf("expected fallback conversion to report realAudio=false")
	}
	if toInt(result["bytes"], 0) <= 0 {
		t.Fatalf("expected positive synthesized bytes, got %v", result["bytes"])
	}
	if strings.TrimSpace(toString(result["audioBase64"], "")) == "" {
		t.Fatalf("expected audioBase64 payload in tts.convert result")
	}
}

func TestCompatTTSConvertRequireRealAudioFailsWithoutKittenBinary(t *testing.T) {
	t.Setenv("OPENCLAW_GO_KITTENTTS_BIN", "__missing_kittentts__")
	t.Setenv("OPENCLAW_GO_TTS_KITTENTTS_BIN", "__missing_kittentts_alt__")
	t.Setenv("PATH", "")

	s := New(config.Default(), buildinfo.Default())
	defer s.Close()

	_, derr := s.handleCompatTTSConvert(map[string]any{
		"text":             "hello",
		"provider":         "kittentts",
		"requireRealAudio": true,
	})
	if derr == nil {
		t.Fatalf("expected real-audio failure when kittentts is unavailable")
	}
	if derr.Code != -32050 {
		t.Fatalf("expected -32050 code for real-audio failure, got %d", derr.Code)
	}
}

func TestCompatTTSConvertKittenTTSSuccessWithCommandAdapter(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Setenv("OPENCLAW_GO_KITTENTTS_BIN", "cmd")
		t.Setenv("OPENCLAW_GO_KITTENTTS_ARGS", "/c echo RIFFTESTAUDIO")
	} else {
		t.Setenv("PATH", "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
		t.Setenv("OPENCLAW_GO_KITTENTTS_BIN", "cat")
		t.Setenv("OPENCLAW_GO_KITTENTTS_ARGS", "")
	}

	s := New(config.Default(), buildinfo.Default())
	defer s.Close()

	result, derr := s.handleCompatTTSConvert(map[string]any{
		"text":             "hello from kitten",
		"provider":         "kittentts",
		"requireRealAudio": true,
	})
	if derr != nil {
		t.Fatalf("expected kittentts conversion success, got error: %v", derr)
	}
	if toString(result["provider"], "") != "kittentts" {
		t.Fatalf("expected provider kittentts, got %v", result["provider"])
	}
	if !toBool(result["realAudio"], false) {
		t.Fatalf("expected realAudio=true for kittentts command adapter path")
	}
	if toInt(result["bytes"], 0) <= 0 {
		t.Fatalf("expected non-zero bytes from kittentts output")
	}
}
