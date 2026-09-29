package bank_test

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	paymentsv1 "github.com/garm-ai/examples/bank/gen/payments/v1"
	toolv1 "github.com/garm-ai/contracts/garm/tool/v1"
)

// What a human is actually approving.
//
// Without material_fields a grant binds only (tool, subject, time), so a
// fifteen-minute approval authorises any call to initiate_payment in the
// window — approve ten pounds, send ten thousand. These three names are the
// difference, and they are asserted here rather than read off the proto
// because they travel: ListTools returns them, agentd extracts them from the
// request it is about to send, the STS digests them, and garmd re-extracts
// and compares. A rename that misses one of those four places is a grant that
// silently covers less than the approver believed.
func TestTheGrantOnAPaymentBindsTheThreeValuesAHumanWouldRead(t *testing.T) {
	p := initiatePaymentPolicy(t)

	want := []string{"amount_minor_units", "beneficiary_iban", "currency_code"}
	got := p.GetApproval().GetMaterialFields()
	if len(got) != len(want) {
		t.Fatalf("material_fields = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("material_fields[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	// Every path must resolve to a scalar leaf of the REQUEST message. A path
	// naming a message, a repeated field or a map cannot be digested without a
	// canonical encoding every language agrees on, and a field a human cannot
	// read in a sentence is one they cannot meaningfully approve.
	md := (&paymentsv1.InitiatePaymentRequest{}).ProtoReflect().Descriptor()
	for _, path := range got {
		fd := md.Fields().ByName(protoreflect.Name(path))
		if fd == nil {
			t.Errorf("material field %q is not a field of %s", path, md.FullName())
			continue
		}
		switch {
		case fd.IsMap(), fd.IsList():
			t.Errorf("material field %q is repeated or a map", path)
		case fd.Kind() == protoreflect.MessageKind, fd.Kind() == protoreflect.GroupKind:
			t.Errorf("material field %q is a message; name a scalar inside it", path)
		case fd.Kind() == protoreflect.FloatKind, fd.Kind() == protoreflect.DoubleKind:
			t.Errorf("material field %q is floating point; its text form differs "+
				"between runtimes, so a grant would match in one and not another", path)
		}
	}
}

// The approver predicate has two axes, and both must be load-bearing.
//
// Clearance alone would let any sufficiently senior employee approve a
// payment; the compartment says whose business it is. amir is cleared to
// CONFIDENTIAL and holds `financial`, and must not be able to approve — which
// is only true while approver_min_clearance is RESTRICTED. sam holds both.
func TestAPaymentApproverNeedsBothClearanceAndTheFinancialCompartment(t *testing.T) {
	a := initiatePaymentPolicy(t).GetApproval()

	if a.GetMode() != toolv1.Approval_MODE_GRANT {
		t.Fatalf("approval.mode = %v, want MODE_GRANT", a.GetMode())
	}
	if a.GetApproverMinClearance() != toolv1.Clearance_CLEARANCE_RESTRICTED {
		t.Errorf("approver_min_clearance = %v, want CLEARANCE_RESTRICTED",
			a.GetApproverMinClearance())
	}
	comps := a.GetApproverCompartments()
	if len(comps) != 1 || comps[0] != "financial" {
		t.Errorf("approver_compartments = %v, want [financial]: without a "+
			"compartment the predicate has one axis and seniority alone approves "+
			"a payment", comps)
	}
	if a.GetMaxGrantAgeSeconds() != 900 {
		t.Errorf("max_grant_age_seconds = %d, want 900", a.GetMaxGrantAgeSeconds())
	}
}

func initiatePaymentPolicy(t *testing.T) *toolv1.ToolPolicy {
	t.Helper()
	md := paymentsv1.File_payments_v1_payments_proto.
		Services().ByName("PaymentsService").
		Methods().ByName("InitiatePayment")
	if md == nil {
		t.Fatal("PaymentsService has no InitiatePayment method")
	}
	p, ok := proto.GetExtension(md.Options(), toolv1.E_Tool).(*toolv1.ToolPolicy)
	if !ok || p == nil {
		t.Fatal("InitiatePayment carries no (garm.tool.v1.tool) option")
	}
	return p
}
