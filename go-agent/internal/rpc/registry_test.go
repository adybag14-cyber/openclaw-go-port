package rpc

import "testing"

func TestResolveKnownCanonicalAliasMethods(t *testing.T) {
	reg := DefaultRegistry()

	cases := []struct {
		input     string
		canonical string
	}{
		{input: "connect", canonical: "connect"},
		{input: "TTS.setProvider", canonical: "tts.setprovider"},
		{input: "exec.approval.waitDecision", canonical: "exec.approval.waitdecision"},
		{input: " sessions.list ", canonical: "sessions.list"},
	}

	for _, c := range cases {
		resolved := reg.Resolve(c.input)
		if !resolved.Known {
			t.Fatalf("expected method %q to be known", c.input)
		}
		if resolved.Canonical != c.canonical {
			t.Fatalf("canonical mismatch for %q: got=%q want=%q", c.input, resolved.Canonical, c.canonical)
		}
		if resolved.Spec == nil {
			t.Fatalf("expected spec for %q", c.input)
		}
	}
}

func TestResolveUnknownMethod(t *testing.T) {
	reg := DefaultRegistry()
	resolved := reg.Resolve("totally.unknown.method")
	if resolved.Known {
		t.Fatalf("unknown method unexpectedly resolved as known")
	}
	if resolved.Spec != nil {
		t.Fatalf("unknown method should not include a spec")
	}
}

func TestSupportedMethodsSnapshotMinimum(t *testing.T) {
	reg := DefaultRegistry()
	methods := reg.SupportedMethods()
	if len(methods) < 100 {
		t.Fatalf("supported method list unexpectedly small: %d", len(methods))
	}

	expectPresent := map[string]bool{
		"connect":                    false,
		"tts.setProvider":            false,
		"exec.approval.waitDecision": false,
		"session.status":             false,
	}

	for _, method := range methods {
		if _, ok := expectPresent[method]; ok {
			expectPresent[method] = true
		}
	}

	for method, present := range expectPresent {
		if !present {
			t.Fatalf("expected supported method missing: %s", method)
		}
	}
}
