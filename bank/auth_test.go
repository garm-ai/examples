package bank_test

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"

	toolv1 "github.com/garm-ai/garm/contracts/garm/tool/v1"
)

// personaFile is only as much of personas.yaml as this test reasons about.
// Deliberately not KnownFields(true): devkit owns that file's full shape, and
// a test here that refused an unrelated key devkit added would be asserting
// something it has no standing to assert.
type personaFile struct {
	Tenant string `yaml:"tenant"`
	Roles  map[string]struct {
		Clearance    string   `yaml:"clearance"`
		Compartments []string `yaml:"compartments"`
		Verbs        []string `yaml:"verbs"`
		ToolSets     []string `yaml:"tool_sets"`
	} `yaml:"roles"`
	Users map[string]struct {
		Subject string   `yaml:"subject"`
		Roles   []string `yaml:"roles"`
	} `yaml:"users"`
	Agents map[string]struct {
		Subject   string   `yaml:"subject"`
		Roles     []string `yaml:"roles"`
		MayActFor []string `yaml:"may_act_for"`
	} `yaml:"agents"`
}

// authority is what a set of roles expands to: the UNION, because holding two
// roles gives you more. Delegation is the opposite operation and happens in
// garmd, never here.
type authority struct {
	clearance    toolv1.Clearance
	compartments map[string]bool
	verbs        map[string]bool
	toolSets     map[string]bool
}

func clearanceOf(s string) toolv1.Clearance {
	switch s {
	case "PUBLIC", "CLEARANCE_PUBLIC":
		return toolv1.Clearance_CLEARANCE_PUBLIC
	case "INTERNAL", "CLEARANCE_INTERNAL":
		return toolv1.Clearance_CLEARANCE_INTERNAL
	case "CONFIDENTIAL", "CLEARANCE_CONFIDENTIAL":
		return toolv1.Clearance_CLEARANCE_CONFIDENTIAL
	case "RESTRICTED", "CLEARANCE_RESTRICTED":
		return toolv1.Clearance_CLEARANCE_RESTRICTED
	}
	return toolv1.Clearance_CLEARANCE_UNSPECIFIED
}

func (f personaFile) expand(roles []string) authority {
	a := authority{
		compartments: map[string]bool{},
		verbs:        map[string]bool{},
		toolSets:     map[string]bool{},
	}
	for _, name := range roles {
		r := f.Roles[name]
		if c := clearanceOf(r.Clearance); int32(c) > int32(a.clearance) {
			a.clearance = c
		}
		for _, c := range r.Compartments {
			a.compartments[c] = true
		}
		for _, v := range r.Verbs {
			a.verbs[v] = true
		}
		for _, s := range r.ToolSets {
			a.toolSets[s] = true
		}
	}
	return a
}

// fold is garmd's rule, reimplemented here on purpose rather than imported:
// examples must not depend on garmd, and the arithmetic this test is about is
// four lines. Minimum clearance, AND of compartments, AND of verbs, and an
// intersection of tool sets in which an empty set on either side means "no
// constraint from this side" rather than "nothing".
func fold(human, agent authority) authority {
	out := authority{
		clearance:    human.clearance,
		compartments: map[string]bool{},
		verbs:        map[string]bool{},
		toolSets:     map[string]bool{},
	}
	if int32(agent.clearance) < int32(out.clearance) {
		out.clearance = agent.clearance
	}
	for c := range human.compartments {
		if agent.compartments[c] {
			out.compartments[c] = true
		}
	}
	for v := range human.verbs {
		if agent.verbs[v] {
			out.verbs[v] = true
		}
	}
	switch {
	case len(human.toolSets) == 0:
		out.toolSets = agent.toolSets
	case len(agent.toolSets) == 0:
		out.toolSets = human.toolSets
	default:
		for s := range human.toolSets {
			if agent.toolSets[s] {
				out.toolSets[s] = true
			}
		}
	}
	return out
}

func loadPersonas(t *testing.T) personaFile {
	t.Helper()
	raw, err := os.ReadFile("auth/personas.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var f personaFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

// The run has to be able to see the payment, or the whole demo is a no-op.
//
// This is the arithmetic that decides whether spec §5.1 can happen at all.
// The fold takes the MINIMUM of the human and the agent, so it is not enough
// for the agent's manifest to request RESTRICTED: the human on the other end
// must hold it too, or the payment tool is invisible at step 2 and the run
// never reaches the grant. Getting this wrong produces no error anywhere —
// the model simply never sees the tool, and the run answers "I cannot do
// that", which reads exactly like a working refusal.
func TestAJdoeRunActingAsTheAssistantCanReachBothOfItsTools(t *testing.T) {
	f := loadPersonas(t)

	human := f.expand(f.Users["jdoe"].Roles)
	agent := f.expand(f.Agents["support-assistant"].Roles)
	run := fold(human, agent)

	// get_customer: INTERNAL, pii-contact, READ, set support.
	// initiate_payment: RESTRICTED, financial, DESTRUCTIVE, set payments.
	if int32(run.clearance) < int32(toolv1.Clearance_CLEARANCE_RESTRICTED) {
		t.Errorf("folded clearance = %v; initiate_payment needs RESTRICTED. "+
			"jdoe has %v and the agent has %v, and the fold takes the minimum",
			run.clearance, human.clearance, agent.clearance)
	}
	for _, c := range []string{"financial", "pii-contact"} {
		if !run.compartments[c] {
			t.Errorf("folded compartments lack %q: %v", c, run.compartments)
		}
	}
	for _, v := range []string{"READ", "DESTRUCTIVE"} {
		if !run.verbs[v] {
			t.Errorf("folded verbs lack %q: %v", v, run.verbs)
		}
	}
	for _, s := range []string{"support", "payments"} {
		if !run.toolSets[s] {
			t.Errorf("folded tool sets lack %q: %v — a set narrows and never "+
				"widens, so a session missing it reaches no tool in it", s, run.toolSets)
		}
	}
}

// Exactly one persona may approve a payment, and it is not the obvious one.
//
// The predicate is spec §3.5: clearance >= approver_min_clearance AND
// compartments >= approver_compartments, with the run's own subject excluded.
// amir is the near miss the design assumed would qualify — RESTRICTED in
// nobody's file, CONFIDENTIAL in this one, holding `financial` all the same —
// and a near miss on one axis has to be a denial, or the axis is decoration.
func TestOnlySamSatisfiesTheApproverPredicateForAPayment(t *testing.T) {
	f := loadPersonas(t)
	a := initiatePaymentPolicy(t).GetApproval()

	satisfies := func(user string) bool {
		au := f.expand(f.Users[user].Roles)
		if int32(au.clearance) < int32(a.GetApproverMinClearance()) {
			return false
		}
		for _, c := range a.GetApproverCompartments() {
			if !au.compartments[c] {
				return false
			}
		}
		return true
	}

	for user, want := range map[string]bool{
		"sam":   true,  // payments-ops: RESTRICTED, financial
		"jdoe":  true,  // after this task, also payments-ops — excluded by SoD, not by claims
		"amir":  false, // fraud-analyst: financial, but only CONFIDENTIAL
		"priya": false, // compliance-officer: CONFIDENTIAL and kyc, neither axis
		"ada":   false, // a customer
	} {
		if got := satisfies(user); got != want {
			t.Errorf("%s satisfies the approver predicate = %v, want %v", user, got, want)
		}
	}

	// And the one that matters for the demo: jdoe qualifies arithmetically and
	// must still be refused, because the run's own subject is excluded. That
	// exclusion is behaviour, asserted end to end in the agent repository's
	// TestTheRequesterCannotApproveTheirOwnTask; here we only record that the
	// claims alone would let it through, which is exactly why the exclusion
	// exists.
	if f.Users["jdoe"].Subject != "employee:jdoe" {
		t.Errorf("jdoe subject = %q, want employee:jdoe", f.Users["jdoe"].Subject)
	}
	if f.Users["sam"].Subject != "employee:sam" {
		t.Errorf("sam subject = %q, want employee:sam", f.Users["sam"].Subject)
	}
}

// The agent may act for the requester, and the IdP is what asserts it.
// Without a tenant on the file, every exchange fails and neither door works.
// It is one line and it is the kind of one line that is added, removed in a
// cleanup, and then costs an afternoon.
func TestThePersonasFileDeclaresATenant(t *testing.T) {
	if got := loadPersonas(t).Tenant; got != "bank" {
		t.Errorf("personas.yaml tenant = %q, want bank — the STS refuses to "+
			"exchange a subject token whose tenant is empty", got)
	}
}

func TestTheAssistantMayActForTheRequester(t *testing.T) {
	f := loadPersonas(t)
	a := f.Agents["support-assistant"]
	if a.Subject != "agent:support-assistant" {
		t.Errorf("agent subject = %q, want agent:support-assistant", a.Subject)
	}
	found := false
	for _, who := range a.MayActFor {
		if who == "jdoe" {
			found = true
		}
	}
	if !found {
		t.Errorf("may_act_for = %v, want it to include jdoe", a.MayActFor)
	}
}

// can_run is the fact exchange 2 checks before it will mint for a runner.
type tupleFile struct {
	CanRun []struct {
		Runner string `yaml:"runner"`
		Agent  string `yaml:"agent"`
	} `yaml:"can_run"`
	CanInvoke []struct {
		Principal string `yaml:"principal"`
		Agent     string `yaml:"agent"`
	} `yaml:"can_invoke"`
	InSegment []struct {
		Principal string `yaml:"principal"`
		Segment   string `yaml:"segment"`
	} `yaml:"in_segment"`
}

func TestTheRunnerIsEntitledToExecuteTheAssistant(t *testing.T) {
	raw, err := os.ReadFile("auth/tuples.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var f tupleFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}

	// can_run(runner, agent): may THIS runner execute this agent? Distinct
	// from can_invoke, which asks whether a human may reach it — a compromised
	// runner holding a human's assertion must not be able to run an agent it
	// was never deployed to run.
	want := [2]string{"runner:agentd", "agent:support-assistant"}
	if !hasPair(t, len(f.CanRun), func(i int) [2]string {
		return [2]string{f.CanRun[i].Runner, f.CanRun[i].Agent}
	}, want) {
		t.Errorf("tuples.yaml has no can_run %v; exchange 2 refuses without it", want)
	}

	if !hasPair(t, len(f.CanInvoke), func(i int) [2]string {
		return [2]string{f.CanInvoke[i].Principal, f.CanInvoke[i].Agent}
	}, [2]string{"employee:jdoe", "agent:support-assistant"}) {
		t.Error("tuples.yaml has no can_invoke for employee:jdoe")
	}

	// jdoe's payments-team membership is what turns the personas change into
	// the same authority on the real STS path: claims.yaml resolves roles from
	// segments, and segments come from here.
	if !hasPair(t, len(f.InSegment), func(i int) [2]string {
		return [2]string{f.InSegment[i].Principal, f.InSegment[i].Segment}
	}, [2]string{"employee:jdoe", "payments-team"}) {
		t.Error("tuples.yaml does not put employee:jdoe in payments-team; the STS " +
			"path would then resolve less authority than the devkit path, and the " +
			"governed door would behave differently from the direct one")
	}
}

func hasPair(t *testing.T, n int, at func(int) [2]string, want [2]string) bool {
	t.Helper()
	for i := 0; i < n; i++ {
		if at(i) == want {
			return true
		}
	}
	return false
}
