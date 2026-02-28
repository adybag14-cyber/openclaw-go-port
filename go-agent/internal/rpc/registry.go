package rpc

import (
	"strings"

	"github.com/adybag14-cyber/openclaw-go-port/go-agent/internal/protocol"
)

type MethodSpec struct {
	Name         string
	Family       protocol.MethodFamily
	RequiresAuth bool
	MinRole      string
}

type ResolvedMethod struct {
	Requested string
	Canonical string
	Known     bool
	Spec      *MethodSpec
}

type MethodRegistry struct {
	known     map[string]MethodSpec
	supported []string
}

func DefaultRegistry() *MethodRegistry {
	known := make(map[string]MethodSpec, len(defaultSupportedRPCMethods))
	for _, method := range defaultSupportedRPCMethods {
		canonical := normalize(method)
		if _, exists := known[canonical]; exists {
			continue
		}
		known[canonical] = MethodSpec{
			Name:         canonical,
			Family:       protocol.ClassifyMethod(canonical),
			RequiresAuth: true,
			MinRole:      defaultMinRole(canonical),
		}
	}

	return &MethodRegistry{
		known:     known,
		supported: append([]string(nil), defaultSupportedRPCMethods...),
	}
}

func (r *MethodRegistry) Resolve(method string) ResolvedMethod {
	canonical := normalize(method)
	spec, ok := r.known[canonical]
	if !ok {
		return ResolvedMethod{
			Requested: method,
			Canonical: canonical,
			Known:     false,
			Spec:      nil,
		}
	}

	specCopy := spec
	return ResolvedMethod{
		Requested: method,
		Canonical: canonical,
		Known:     true,
		Spec:      &specCopy,
	}
}

func (r *MethodRegistry) SupportedMethods() []string {
	return append([]string(nil), r.supported...)
}

func normalize(method string) string {
	return strings.ToLower(strings.TrimSpace(method))
}

func defaultMinRole(method string) string {
	switch method {
	case "pairing.approve", "exec.approvals.set", "exec.approvals.node.set", "exec.approvals.node.get":
		return "owner"
	default:
		return "client"
	}
}

var defaultSupportedRPCMethods = []string{
	"connect",
	"health",
	"status",
	"doctor.memory.status",
	"security.audit",
	"usage.status",
	"usage.cost",
	"last-heartbeat",
	"set-heartbeats",
	"system-presence",
	"system-event",
	"wake",
	"talk.config",
	"talk.mode",
	"tts.status",
	"tts.enable",
	"tts.disable",
	"tts.convert",
	"tts.setProvider",
	"tts.providers",
	"edge.voice.transcribe",
	"edge.router.plan",
	"edge.acceleration.status",
	"edge.wasm.marketplace.list",
	"edge.swarm.plan",
	"edge.multimodal.inspect",
	"edge.enclave.status",
	"edge.enclave.prove",
	"edge.mesh.status",
	"edge.homomorphic.compute",
	"edge.finetune.status",
	"edge.finetune.run",
	"edge.identity.trust.status",
	"edge.personality.profile",
	"edge.handoff.plan",
	"edge.marketplace.revenue.preview",
	"edge.finetune.cluster.plan",
	"edge.alignment.evaluate",
	"edge.quantum.status",
	"edge.collaboration.plan",
	"voicewake.get",
	"voicewake.set",
	"models.list",
	"tools.catalog",
	"agents.list",
	"agents.create",
	"agents.update",
	"agents.delete",
	"agents.files.list",
	"agents.files.get",
	"agents.files.set",
	"agent",
	"agent.identity.get",
	"agent.wait",
	"skills.status",
	"skills.bins",
	"skills.install",
	"skills.update",
	"secrets.reload",
	"cron.list",
	"cron.status",
	"cron.add",
	"cron.update",
	"cron.remove",
	"cron.run",
	"cron.runs",
	"channels.status",
	"channels.logout",
	"update.run",
	"web.login.start",
	"web.login.wait",
	"auth.oauth.providers",
	"auth.oauth.start",
	"auth.oauth.wait",
	"auth.oauth.complete",
	"auth.oauth.logout",
	"auth.oauth.import",
	"wizard.start",
	"wizard.next",
	"wizard.cancel",
	"wizard.status",
	"device.pair.list",
	"device.pair.approve",
	"device.pair.reject",
	"device.pair.remove",
	"device.token.rotate",
	"device.token.revoke",
	"node.pair.request",
	"node.pair.list",
	"node.pair.approve",
	"node.pair.reject",
	"node.pair.verify",
	"node.rename",
	"node.list",
	"node.describe",
	"node.invoke",
	"node.invoke.result",
	"node.event",
	"push.test",
	"browser.request",
	"browser.open",
	"canvas.present",
	"exec.approvals.get",
	"exec.approvals.set",
	"exec.approvals.node.get",
	"exec.approvals.node.set",
	"exec.approval.request",
	"exec.approval.waitDecision",
	"exec.approval.resolve",
	"chat.history",
	"send",
	"poll",
	"chat.send",
	"chat.abort",
	"chat.inject",
	"config.get",
	"config.set",
	"config.patch",
	"config.apply",
	"config.schema",
	"logs.tail",
	"sessions.list",
	"sessions.preview",
	"sessions.patch",
	"sessions.resolve",
	"sessions.reset",
	"sessions.delete",
	"sessions.compact",
	"sessions.usage",
	"sessions.usage.timeseries",
	"sessions.usage.logs",
	"sessions.history",
	"sessions.send",
	"session.status",
}
