package gateway

import "testing"

func TestExtractAuthCodeSupportsQueryFragmentAndPathForms(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{input: "OC-123456", expected: "OC-123456"},
		{input: "https://chatgpt.com/?openclaw_code=OC-AAA111", expected: "OC-AAA111"},
		{input: "https://chatgpt.com/callback?code=OC-BBB222", expected: "OC-BBB222"},
		{input: "https://chatgpt.com/callback#code=OC-CCC333", expected: "OC-CCC333"},
		{input: "https://chatgpt.com/callback#token=OC-DDD444", expected: "OC-DDD444"},
		{input: "https://chatgpt.com/oauth/complete/OC-EEE555", expected: "OC-EEE555"},
		{input: "https://chatgpt.com/oauth/callback#OC-FFF666", expected: "OC-FFF666"},
	}
	for _, tc := range cases {
		if got := extractAuthCode(tc.input); got != tc.expected {
			t.Fatalf("extractAuthCode(%q)=%q want %q", tc.input, got, tc.expected)
		}
	}
}

func TestParseAuthCompleteScopeSupportsCopawAlias(t *testing.T) {
	scope, parseErr := parseAuthCompleteScope([]string{
		"copaw",
		"https://chat.qwen.ai/callback?code=OC-QWEN999",
		"web-login-000777",
		"mobile",
	}, "chatgpt")
	if parseErr != "" {
		t.Fatalf("unexpected parse error: %s", parseErr)
	}
	if scope.Provider != "qwen" {
		t.Fatalf("expected provider qwen, got %q", scope.Provider)
	}
	if scope.SessionID != "web-login-000777" {
		t.Fatalf("expected session id web-login-000777, got %q", scope.SessionID)
	}
	if scope.Account != "mobile" {
		t.Fatalf("expected account mobile, got %q", scope.Account)
	}
	if scope.Code != "https://chat.qwen.ai/callback?code=OC-QWEN999" {
		t.Fatalf("unexpected scope code payload: %q", scope.Code)
	}
	if extracted := extractAuthCode(scope.Code); extracted != "OC-QWEN999" {
		t.Fatalf("expected extracted code OC-QWEN999, got %q", extracted)
	}
}
