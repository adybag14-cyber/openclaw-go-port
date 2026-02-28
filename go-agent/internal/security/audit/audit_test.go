package audit

import (
	"net"
	"testing"

	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/config"
)

func TestRunReportsCriticalWhenAuthNone(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Server.AuthMode = "none"

	report := Run(cfg, Options{})
	if report.Summary.Critical < 1 {
		t.Fatalf("expected at least one critical finding, got %+v", report.Summary)
	}
}

func TestRunDeepGatewayProbe(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind local listener: %v", err)
	}
	defer listener.Close()

	cfg := config.Default()
	cfg.Gateway.URL = "ws://" + listener.Addr().String() + "/gateway"

	report := Run(cfg, Options{Deep: true})
	if report.Deep == nil {
		t.Fatalf("expected deep report")
	}
	if !report.Deep.Gateway.OK {
		t.Fatalf("expected deep gateway probe to pass, got error=%s", report.Deep.Gateway.Error)
	}
}

func TestRunWarnsWhenCredentialPolicyKeysMissing(t *testing.T) {
	cfg := config.Default()
	cfg.Security.CredentialSensitiveKeys = []string{}

	report := Run(cfg, Options{})
	if report.Summary.Critical < 1 {
		t.Fatalf("expected critical finding for missing credential keys")
	}
}
