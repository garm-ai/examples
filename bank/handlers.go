// Package bank implements the bank's tools.
//
// Handler bodies and nothing else. Authentication, authorisation, input
// checking and response redaction all happened in the daemon before any of
// this ran, and repeating any of it here would be a second implementation of
// the chain in the one place that must not have one.
//
// So these handlers return everything they know, always. GetCustomer hands
// back a national identifier to every caller; whether the caller ever sees it
// is decided by the field policy in the proto and applied by the chain on the
// way out. A handler that tried to be careful here would be guessing at a
// decision it cannot make — it does not know the caller's clearance — and
// would produce a second, weaker answer that nobody reviews.
//
// The data is fixed. A fixture that varies is a test that cannot assert.
package bank

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	accountsv1 "github.com/garm-ai/examples/bank/gen/accounts/v1"
	cardsv1 "github.com/garm-ai/examples/bank/gen/cards/v1"
	paymentsv1 "github.com/garm-ai/examples/bank/gen/payments/v1"
	screeningv1 "github.com/garm-ai/examples/bank/gen/screening/v1"
)

// Accounts ---------------------------------------------------------------

type Accounts struct{}

func (Accounts) GetBalance(_ context.Context, r *accountsv1.GetBalanceRequest) (*accountsv1.GetBalanceResponse, error) {
	acct, ok := accounts[r.GetAccountId()]
	if !ok {
		return nil, fmt.Errorf("no account %s", r.GetAccountId())
	}
	// Cloned, not shared. The runtime marshals what a handler returns on its
	// own goroutine, and handing out one pointer to fixture data makes every
	// concurrent call share a message.
	return proto.Clone(acct).(*accountsv1.GetBalanceResponse), nil
}

func (Accounts) GetCustomer(_ context.Context, r *accountsv1.GetCustomerRequest) (*accountsv1.GetCustomerResponse, error) {
	cust, ok := customers[r.GetCustomerId()]
	if !ok {
		// Not found is not "you may not see them". The chain answers that,
		// identically, for a customer this caller is not cleared for — which
		// is why the guidance tells an agent never to report to a customer
		// that no record exists.
		return nil, fmt.Errorf("no customer %s", r.GetCustomerId())
	}
	return proto.Clone(cust).(*accountsv1.GetCustomerResponse), nil
}

// Cards ------------------------------------------------------------------

type Cards struct{}

func (Cards) ListCards(_ context.Context, r *cardsv1.ListCardsRequest) (*cardsv1.ListCardsResponse, error) {
	return &cardsv1.ListCardsResponse{Cards: cardsFor(r.GetCustomerId())}, nil
}

func (Cards) FreezeCard(_ context.Context, r *cardsv1.FreezeCardRequest) (*cardsv1.FreezeCardResponse, error) {
	return setCardState(r.GetCardId(), cardsv1.CardState_CARD_STATE_FROZEN)
}

func (Cards) UnfreezeCard(_ context.Context, r *cardsv1.FreezeCardRequest) (*cardsv1.FreezeCardResponse, error) {
	return setCardState(r.GetCardId(), cardsv1.CardState_CARD_STATE_ACTIVE)
}

// Payments ---------------------------------------------------------------

type Payments struct{}

func (Payments) InitiatePayment(_ context.Context, r *paymentsv1.InitiatePaymentRequest) (*paymentsv1.InitiatePaymentResponse, error) {
	// The idempotency key is honoured, because the annotation promises it is.
	// A tool declaring idempotent: true and then moving the money twice has
	// told the chain something false, and the chain has no way to check.
	paymentsMu.Lock()
	defer paymentsMu.Unlock()
	if prior, seen := payments[r.GetIdempotencyKey()]; seen {
		return proto.Clone(prior).(*paymentsv1.InitiatePaymentResponse), nil
	}
	id := "pay_" + strings.ToLower(strings.ReplaceAll(r.GetIdempotencyKey(), "-", ""))
	if len(id) > 36 {
		id = id[:36]
	}
	status := paymentsv1.PaymentStatus_PAYMENT_STATUS_PENDING_APPROVAL
	out := &paymentsv1.InitiatePaymentResponse{PaymentId: &id, Status: &status}
	payments[r.GetIdempotencyKey()] = out
	return proto.Clone(out).(*paymentsv1.InitiatePaymentResponse), nil
}

func (Payments) GetPaymentStatus(_ context.Context, r *paymentsv1.GetPaymentStatusRequest) (*paymentsv1.GetPaymentStatusResponse, error) {
	status := paymentsv1.PaymentStatus_PAYMENT_STATUS_SETTLED
	amount := int64(125000)
	id := r.GetPaymentId()
	return &paymentsv1.GetPaymentStatusResponse{
		PaymentId: &id, Status: &status, AmountMinorUnits: &amount,
	}, nil
}

// Screening --------------------------------------------------------------

type Screening struct{}

func (Screening) ScreenParty(_ context.Context, r *screeningv1.ScreenPartyRequest) (*screeningv1.ScreenPartyResponse, error) {
	if r.GetFullName() == "" {
		return nil, errors.New("a name is required to screen")
	}
	// One fixed hit, so a test can assert what a cleared caller sees and an
	// uncleared one does not.
	if !strings.EqualFold(r.GetFullName(), "Ivan Petrov") {
		review := false
		return &screeningv1.ScreenPartyResponse{RequiresReview: &review}, nil
	}
	review := true
	list, matched := "OFAC SDN", "PETROV, Ivan"
	strength := screeningv1.MatchStrength_MATCH_STRENGTH_POSSIBLE
	notes := "same surname and birth year; passport number does not match"
	return &screeningv1.ScreenPartyResponse{
		RequiresReview: &review,
		Matches: []*screeningv1.Match{{
			ListName: &list, MatchedName: &matched, Strength: &strength, Notes: &notes,
		}},
	}, nil
}
