package sandbox

import "testing"

func TestPolicyDeniesDisallowedCapabilities(t *testing.T) {
	policy := DefaultPolicy()
	decision := policy.EvaluateCapabilities([]string{"network"})
	if decision.Allowed {
		t.Fatalf("expected network capability to be denied by default")
	}
	if len(decision.DeniedCapabilities) != 1 || decision.DeniedCapabilities[0] != "network" {
		t.Fatalf("expected deniedCapabilities to include network, got %v", decision.DeniedCapabilities)
	}
}

func TestPolicyAllowsSafeCapabilities(t *testing.T) {
	policy := DefaultPolicy()
	decision := policy.EvaluateCapabilities([]string{"compute"})
	if !decision.Allowed {
		t.Fatalf("expected compute capability to be allowed")
	}
}

func TestPolicyCollectsMultipleDeniedCapabilities(t *testing.T) {
	policy := DefaultPolicy()
	decision := policy.EvaluateCapabilities([]string{"network", "filesystem"})
	if decision.Allowed {
		t.Fatalf("expected decision to deny multiple disallowed capabilities")
	}
	if len(decision.DeniedCapabilities) != 2 {
		t.Fatalf("expected two denied capabilities, got %v", decision.DeniedCapabilities)
	}
}
