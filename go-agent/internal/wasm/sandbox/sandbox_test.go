package sandbox

import "testing"

func TestPolicyDeniesDisallowedCapabilities(t *testing.T) {
	policy := DefaultPolicy()
	decision := policy.EvaluateCapabilities([]string{"network"})
	if decision.Allowed {
		t.Fatalf("expected network capability to be denied by default")
	}
}

func TestPolicyAllowsSafeCapabilities(t *testing.T) {
	policy := DefaultPolicy()
	decision := policy.EvaluateCapabilities([]string{"compute"})
	if !decision.Allowed {
		t.Fatalf("expected compute capability to be allowed")
	}
}
