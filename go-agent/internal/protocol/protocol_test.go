package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestClassifyMethodFamilies(t *testing.T) {
	cases := map[string]MethodFamily{
		"connect":          MethodFamilyConnect,
		"health":           MethodFamilyGateway,
		"status":           MethodFamilyGateway,
		"usage.cost":       MethodFamilyGateway,
		"agent.exec":       MethodFamilyAgent,
		"agents.list":      MethodFamilyAgent,
		"models.list":      MethodFamilyGateway,
		"skills.status":    MethodFamilyGateway,
		"update.run":       MethodFamilyGateway,
		"web.login.start":  MethodFamilyGateway,
		"wizard.start":     MethodFamilyGateway,
		"sessions.patch":   MethodFamilySessions,
		"node.invoke":      MethodFamilyNode,
		"browser.open":     MethodFamilyBrowser,
		"canvas.present":   MethodFamilyCanvas,
		"pairing.approve":  MethodFamilyPairing,
		"config.patch":     MethodFamilyConfig,
		"unknown.method.x": MethodFamilyUnknown,
	}

	for method, expected := range cases {
		if got := ClassifyMethod(method); got != expected {
			t.Fatalf("method family mismatch for %q: got=%s want=%s", method, got, expected)
		}
	}
}

func TestParseRPCRequestResponseAndError(t *testing.T) {
	req := map[string]any{
		"type":   "req",
		"id":     "r-1",
		"method": "agent.exec",
		"params": map[string]any{"command": "git status"},
	}
	reqMeta := ParseRPCRequest(req)
	if reqMeta == nil {
		t.Fatalf("expected request metadata")
	}
	if reqMeta.ID != "r-1" || reqMeta.Method != "agent.exec" {
		t.Fatalf("unexpected request metadata: %+v", reqMeta)
	}

	respOK := map[string]any{
		"type":   "resp",
		"id":     "r-1",
		"ok":     true,
		"result": map[string]any{"status": "ok"},
	}
	okMeta := ParseRPCResponse(respOK)
	if okMeta == nil {
		t.Fatalf("expected response metadata")
	}
	if okMeta.OK == nil || !*okMeta.OK {
		t.Fatalf("expected response ok=true")
	}
	if okMeta.Error != nil {
		t.Fatalf("did not expect response error: %+v", okMeta.Error)
	}

	respErr := map[string]any{
		"type": "resp",
		"id":   "r-2",
		"ok":   false,
		"error": map[string]any{
			"code":    403,
			"message": "denied",
			"details": map[string]any{"policy": "tool_deny"},
		},
	}
	errMeta := ParseRPCResponse(respErr)
	if errMeta == nil || errMeta.Error == nil {
		t.Fatalf("expected response error")
	}
	if errMeta.Error.Code == nil || *errMeta.Error.Code != 403 {
		t.Fatalf("unexpected error code: %+v", errMeta.Error)
	}
	if errMeta.Error.Message != "denied" {
		t.Fatalf("unexpected error message: %+v", errMeta.Error)
	}
}

func TestProtocolCorpusSnapshotMatchesExpectations(t *testing.T) {
	corpusPath := filepath.Join("testdata", "frame-corpus.json")
	raw, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatalf("failed reading corpus: %v", err)
	}

	var corpus protocolCorpus
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatalf("failed parsing corpus: %v", err)
	}

	for _, c := range corpus.Cases {
		if got := FrameKindOf(c.Frame); got != c.Expect.Kind {
			t.Fatalf("case %s kind mismatch: got=%s want=%s", c.Name, got, c.Expect.Kind)
		}

		if c.Expect.MethodFamily != nil {
			method := MethodName(c.Frame)
			if method == "" {
				t.Fatalf("case %s expected method, got empty", c.Name)
			}
			if got := ClassifyMethod(method); got != *c.Expect.MethodFamily {
				t.Fatalf("case %s method family mismatch: got=%s want=%s", c.Name, got, *c.Expect.MethodFamily)
			}
		}

		if c.Expect.Method != nil {
			if got := MethodName(c.Frame); got != *c.Expect.Method {
				t.Fatalf("case %s method mismatch: got=%q want=%q", c.Name, got, *c.Expect.Method)
			}
		}

		if c.Expect.RequestID != nil {
			switch FrameKindOf(c.Frame) {
			case FrameKindReq:
				req := ParseRPCRequest(c.Frame)
				if req == nil || req.ID != *c.Expect.RequestID {
					t.Fatalf("case %s req id mismatch", c.Name)
				}
			case FrameKindResp:
				resp := ParseRPCResponse(c.Frame)
				if resp == nil || resp.ID != *c.Expect.RequestID {
					t.Fatalf("case %s resp id mismatch", c.Name)
				}
			}
		}

		if c.Expect.ResponseHasError != nil {
			hasError := false
			if resp := ParseRPCResponse(c.Frame); resp != nil && resp.Error != nil {
				hasError = true
			}
			if hasError != *c.Expect.ResponseHasError {
				t.Fatalf("case %s response_has_error mismatch: got=%t want=%t", c.Name, hasError, *c.Expect.ResponseHasError)
			}
		}
	}
}

type protocolCorpus struct {
	Cases []protocolCase `json:"cases"`
}

type protocolCase struct {
	Name   string         `json:"name"`
	Frame  map[string]any `json:"frame"`
	Expect protocolExpect `json:"expect"`
}

type protocolExpect struct {
	Kind             FrameKind     `json:"kind"`
	MethodFamily     *MethodFamily `json:"method_family,omitempty"`
	RequestID        *string       `json:"request_id,omitempty"`
	Method           *string       `json:"method,omitempty"`
	ResponseHasError *bool         `json:"response_has_error,omitempty"`
}
