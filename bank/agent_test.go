package bank_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	bankagentsv1 "github.com/garm-ai/examples/bank/gen/bank/agents/v1"
	agentv1 "github.com/garm-ai/garm/contracts/garm/agent/v1"
	toolv1 "github.com/garm-ai/garm/contracts/garm/tool/v1"
)

// The pin is the whole point of putting a hash in the annotation.
//
// The prompt is published to object storage as prompts/<sha256>.md and the
// runner refuses an agent whose prompt does not hash to what the catalogue
// declares. That refusal only protects anything while the declared hash is
// the hash of the file in this tree — a pin that drifts turns the check into
// a check of nothing, and it drifts silently, because editing a prompt is the
// one change nobody thinks of as a code change.
//
// `mise run prompt-sha` recomputes and rewrites it; this test is what makes
// forgetting to run it a red build rather than a broken deploy.
func TestTheDeclaredPromptHashIsTheHashOfThePromptInThisTree(t *testing.T) {
	p := supportAssistantPolicy(t)

	sys, ok := p.GetPrompts()["system"]
	if !ok {
		t.Fatal(`the agent declares no prompt under the key "system"`)
	}
	if sys.GetPath() != "prompts/support-assistant.md" {
		t.Fatalf("prompts[system].path = %q, want %q",
			sys.GetPath(), "prompts/support-assistant.md")
	}

	body, err := os.ReadFile(sys.GetPath())
	if err != nil {
		t.Fatalf("the declared prompt does not resolve in the working tree: %v", err)
	}
	sum := sha256.Sum256(body)
	want := hex.EncodeToString(sum[:])
	if got := sys.GetSha256(); got != want {
		t.Errorf("prompts[system].sha256 = %q\nthe file hashes to        %q\n"+
			"run `mise run prompt-sha` and commit the proto", got, want)
	}
}

// The manifest's shape, asserted where a reviewer can see it.
//
// Every value here is load-bearing somewhere else: the principal is what lint
// A3 checks the allowlist against and what the STS's agents: entry must
// expand to, the tool FQNs are keys into the catalogue, and the guard is CEL
// over initiate_payment's request message.
func TestTheSupportAssistantRunsAsSomethingThatCanActuallyReachItsTools(t *testing.T) {
	p := supportAssistantPolicy(t)

	if p.GetMode() != agentv1.Mode_MODE_REACT {
		t.Errorf("mode = %v, want MODE_REACT", p.GetMode())
	}

	// A3: an agent may not list a tool it could never call. get_customer needs
	// INTERNAL and pii-contact; initiate_payment needs RESTRICTED and
	// financial. CONFIDENTIAL — the clearance the design first wrote — is one
	// grade below RESTRICTED and would make the payment tool unreachable.
	pr := p.GetPrincipal()
	if pr.GetClearance() != toolv1.Clearance_CLEARANCE_RESTRICTED {
		t.Errorf("principal.clearance = %v, want CLEARANCE_RESTRICTED: "+
			"initiate_payment declares min_clearance RESTRICTED, and an agent that "+
			"lists a tool it could never call is lint rule A3", pr.GetClearance())
	}
	wantComps := map[string]bool{"financial": true, "pii-contact": true}
	if len(pr.GetCompartments()) != len(wantComps) {
		t.Fatalf("principal.compartments = %v, want financial and pii-contact",
			pr.GetCompartments())
	}
	for _, c := range pr.GetCompartments() {
		if !wantComps[c] {
			t.Errorf("principal holds unexpected compartment %q", c)
		}
	}

	if p.GetModel().GetAlias() != "fast" {
		t.Errorf("model.alias = %q, want \"fast\": aliases map to model ids in "+
			"agentd config, never in the manifest", p.GetModel().GetAlias())
	}

	b := p.GetBounds()
	if b.GetMaxSteps() != 12 || b.GetMaxToolCalls() != 30 || b.GetMaxTokens() != 200000 {
		t.Errorf("bounds = %+v, want max_steps 12, max_tool_calls 30, max_tokens 200000", b)
	}
	if b.GetTimeout().GetSeconds() != 600 {
		t.Errorf("bounds.timeout = %v, want 600s", b.GetTimeout())
	}

	tools := p.GetTools()
	if len(tools) != 2 {
		t.Fatalf("tools = %v, want exactly two", tools)
	}
	if tools[0].GetFqn() != "accounts.v1.get_customer" || tools[0].GetGuard() != "" {
		t.Errorf("tools[0] = %+v, want accounts.v1.get_customer with no guard", tools[0])
	}
	if tools[1].GetFqn() != "payments.v1.initiate_payment" {
		t.Errorf("tools[1].fqn = %q, want payments.v1.initiate_payment", tools[1].GetFqn())
	}
	if want := "args.amount_minor_units <= 500000"; tools[1].GetGuard() != want {
		t.Errorf("tools[1].guard = %q, want %q", tools[1].GetGuard(), want)
	}
}

// A5: a caller who can start a run can read its result, and nobody else.
//
// Two RPCs on one service with different labels would mean a caller able to
// start a payment run and unable to find out what it did — or, far worse, one
// able to read another team's run because GetRun was labelled more loosely.
func TestInvokeAndGetRunAreVisibleToExactlyTheSameCallers(t *testing.T) {
	svc := bankagentsv1.File_bank_agents_v1_support_assistant_proto.
		Services().ByName("SupportAssistant")
	if svc == nil {
		t.Fatal("no SupportAssistant service in the generated descriptor")
	}
	if svc.Methods().Len() != 2 {
		t.Fatalf("SupportAssistant has %d methods; A1 allows Invoke and GetRun only",
			svc.Methods().Len())
	}

	get := func(name string) *toolv1.ToolPolicy {
		md := svc.Methods().ByName(protoreflectName(name))
		if md == nil {
			t.Fatalf("SupportAssistant has no %s method", name)
		}
		p, ok := proto.GetExtension(md.Options(), toolv1.E_Tool).(*toolv1.ToolPolicy)
		if !ok || p == nil {
			t.Fatalf("%s carries no (garm.tool.v1.tool) option", name)
		}
		return p
	}
	inv, run := get("Invoke"), get("GetRun")

	if inv.GetName() != "support_assistant" {
		t.Errorf("Invoke tool name = %q, want support_assistant", inv.GetName())
	}
	if run.GetName() != "support_assistant_run" {
		t.Errorf("GetRun tool name = %q, want support_assistant_run", run.GetName())
	}
	if inv.GetVerb() != toolv1.Verb_VERB_WRITE {
		t.Errorf("Invoke verb = %v, want VERB_WRITE", inv.GetVerb())
	}
	if run.GetVerb() != toolv1.Verb_VERB_READ {
		t.Errorf("GetRun verb = %v, want VERB_READ", run.GetVerb())
	}
	if inv.GetMinClearance() != run.GetMinClearance() {
		t.Errorf("A5: min_clearance differs — Invoke %v, GetRun %v",
			inv.GetMinClearance(), run.GetMinClearance())
	}
	if inv.GetMinClearance() != toolv1.Clearance_CLEARANCE_INTERNAL {
		t.Errorf("min_clearance = %v, want CLEARANCE_INTERNAL", inv.GetMinClearance())
	}
	if len(inv.GetCompartments()) != 0 || len(run.GetCompartments()) != 0 {
		t.Errorf("A5: compartments must be empty on both — Invoke %v, GetRun %v",
			inv.GetCompartments(), run.GetCompartments())
	}
	if len(inv.GetSets()) != 1 || inv.GetSets()[0] != "support" {
		t.Errorf("Invoke sets = %v, want [support]", inv.GetSets())
	}
	if len(run.GetSets()) != 1 || run.GetSets()[0] != "support" {
		t.Errorf("GetRun sets = %v, want [support]", run.GetSets())
	}

	// L16: "irreversible and external requires at least MODE_NOTIFY", and
	// MODE_NOTIFY cannot mount in this MVP. Invoke therefore claims only what
	// is certainly true — starting a run is not idempotent — and leaves
	// reversibility and external unset. The external effect this run may cause
	// is initiate_payment's, which declares and gates it on its own account.
	e := inv.GetEffects()
	if e.GetIdempotent() {
		t.Error("Invoke declares idempotent: true; two Invokes are two runs")
	}
	if e.GetExternal() {
		t.Error("Invoke declares external: true, which trips lint L16 without an " +
			"approval mode. Starting a run leaves nothing; the payment does")
	}
	if e.GetReversibility() != toolv1.Reversibility_REVERSIBILITY_UNSPECIFIED {
		t.Errorf("Invoke declares reversibility %v; leave it unset (L16)",
			e.GetReversibility())
	}
}

func supportAssistantPolicy(t *testing.T) *agentv1.AgentPolicy {
	t.Helper()
	svc := bankagentsv1.File_bank_agents_v1_support_assistant_proto.
		Services().ByName("SupportAssistant")
	if svc == nil {
		t.Fatal("no SupportAssistant service in the generated descriptor")
	}
	p, ok := proto.GetExtension(svc.Options(), agentv1.E_Agent).(*agentv1.AgentPolicy)
	if !ok || p == nil {
		t.Fatal("SupportAssistant carries no (garm.agent.v1.agent) option")
	}
	return p
}

func protoreflectName(s string) protoreflect.Name { return protoreflect.Name(s) }
