package bank

import (
	"sync"

	accountsv1 "github.com/garm-ai/examples/bank/gen/accounts/v1"
	cardsv1 "github.com/garm-ai/examples/bank/gen/cards/v1"
	paymentsv1 "github.com/garm-ai/examples/bank/gen/payments/v1"
)

// Fixed data, because a fixture that varies is a test that cannot assert.
//
// Every value here is chosen to be visibly redactable: an email whose domain
// survives masking, a phone whose last four survive, a birth date whose year
// survives truncation, and a national identifier that survives nothing. A
// fixture of "x" would pass the same tests and demonstrate none of it.

func s(v string) *string { return &v }
func i(v int64) *int64   { return &v }

var accounts = map[string]*accountsv1.GetBalanceResponse{
	"acct_ab12cd": {
		AccountId:           s("acct_ab12cd"),
		BalanceMinorUnits:   i(482355),
		CurrencyCode:        s("GBP"),
		AvailableMinorUnits: i(430100),
	},
}

var customers = map[string]*accountsv1.GetCustomerResponse{
	"cust_ab12cd": {
		CustomerId:  s("cust_ab12cd"),
		DisplayName: s("A. Okonkwo"),
		Email:       s("ada.okonkwo@example.com"),
		Phone:       s("+44 7700 900412"),
		DateOfBirth: s("1988-03-14"),
		NationalId:  s("QQ123456C"),
	},
}

// payments is written by concurrent handlers — NATS delivers requests on its
// own goroutines — so it is behind a mutex. go vet found the value copies that
// led here; the race was the thing underneath them, and a sequential test
// would never have shown it.
var (
	paymentsMu sync.Mutex
	payments   = map[string]*paymentsv1.InitiatePaymentResponse{}
)

func cardsFor(customerID string) []*cardsv1.Card {
	if customerID != "cust_ab12cd" {
		return nil
	}
	active := cardsv1.CardState_CARD_STATE_ACTIVE
	return []*cardsv1.Card{{
		CardId: s("card_ab12cd"),
		State:  &active,
		Pan:    s("4111111111111111"),
		Expiry: s("2029-07"),
	}}
}

func setCardState(cardID string, to cardsv1.CardState) (*cardsv1.FreezeCardResponse, error) {
	// Idempotent, as freeze_card's guidance promises: already frozen is
	// success, and an agent told otherwise would retry or escalate.
	return &cardsv1.FreezeCardResponse{CardId: &cardID, State: &to}, nil
}
