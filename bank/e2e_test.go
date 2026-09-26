package bank_test

import (
	"context"
	"sync"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	"github.com/garm-ai/examples/bank"
	accountsv1 "github.com/garm-ai/examples/bank/gen/accounts/v1"
	cardsv1 "github.com/garm-ai/examples/bank/gen/cards/v1"
	paymentsv1 "github.com/garm-ai/examples/bank/gen/payments/v1"
	screeningv1 "github.com/garm-ai/examples/bank/gen/screening/v1"
	"github.com/garm-ai/tool-go/garmtool"
)

// The bank's tool side, end to end over a real broker.
//
// What it asserts is deliberately narrow: that four domains register, answer
// on the subjects the contract derives, and each advertise their OWN
// descriptor hash. It says nothing about who may call them or what they are
// allowed to see of the answer — that happens in the daemon, which is not in
// this picture and must not be.
//
// The most important assertion here is the one that looks wrong: these
// handlers return a national identifier and a full card number to anybody who
// asks. That is correct. A handler that withheld them would be guessing at a
// decision it cannot make — it does not know the caller's clearance — and
// would produce a second, weaker, unreviewed answer beside the one the chain
// gives.

func runBank(t *testing.T) *nats.Conn {
	t.Helper()
	srv, err := natsserver.NewServer(&natsserver.Options{Port: -1, NoLog: true, NoSigs: true})
	if err != nil {
		t.Fatal(err)
	}
	go srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
		t.Fatal("nats did not start")
	}
	t.Cleanup(srv.Shutdown)

	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)

	ctx, cancel := context.WithCancel(context.Background())

	// One cleanup that cancels and THEN waits, rather than a cancel cleanup
	// plus a wait cleanup per service. t.Cleanup is LIFO, so registering them
	// separately runs every wait before the cancel that would end it, and
	// each service reports "did not drain" after its full timeout.
	var running []struct {
		name string
		done <-chan error
	}
	t.Cleanup(func() {
		cancel()
		for _, r := range running {
			select {
			case err := <-r.done:
				if err != nil && ctx.Err() == nil {
					t.Errorf("%s: %v", r.name, err)
				}
			case <-time.After(5 * time.Second):
				t.Errorf("%s did not drain", r.name)
			}
		}
	})

	for _, d := range []struct {
		name     string
		register func(*garmtool.Service) error
	}{
		{"accounts", func(s *garmtool.Service) error {
			return accountsv1.ServeAccountsService(s, bank.Accounts{})
		}},
		{"cards", func(s *garmtool.Service) error {
			return cardsv1.ServeCardsService(s, bank.Cards{})
		}},
		{"payments", func(s *garmtool.Service) error {
			return paymentsv1.ServePaymentsService(s, bank.Payments{})
		}},
		{"screening", func(s *garmtool.Service) error {
			return screeningv1.ServeScreeningService(s, bank.Screening{})
		}},
	} {
		svc := garmtool.New(d.name, "v0.1.0")
		if err := d.register(svc); err != nil {
			t.Fatalf("registering %s: %v", d.name, err)
		}
		done := make(chan error, 1)
		go func() { done <- svc.Run(ctx, nc) }()
		running = append(running, struct {
			name string
			done <-chan error
		}{d.name, done})
	}
	waitFor(t, nc, "/accounts.v1.AccountsService/GetBalance")
	waitFor(t, nc, "/screening.v1.ScreeningService/ScreenParty")
	return nc
}

// waitFor polls rather than sleeping: a fixed sleep is either slower than
// needed or flaky on a loaded machine.
func waitFor(t *testing.T, nc *nats.Conn, route string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := nc.Request(garmtool.Subject(route), nil, 100*time.Millisecond); err == nil {
			return
		}
	}
	t.Fatalf("%s never started answering", route)
}

func callTool(t *testing.T, nc *nats.Conn, route string, req, resp proto.Message) {
	t.Helper()
	body, err := proto.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := nc.Request(garmtool.Subject(route), body, 3*time.Second)
	if err != nil {
		t.Fatalf("%s: %v", route, err)
	}
	if code := msg.Header.Get("Nats-Service-Error-Code"); code != "" {
		t.Fatalf("%s: %s %s", route, code, msg.Header.Get("Nats-Service-Error"))
	}
	if err := proto.Unmarshal(msg.Data, resp); err != nil {
		t.Fatalf("%s: %v", route, err)
	}
}

func TestEveryDomainAnswersOnItsContractSubject(t *testing.T) {
	nc := runBank(t)

	var bal accountsv1.GetBalanceResponse
	callTool(t, nc, "/accounts.v1.AccountsService/GetBalance",
		&accountsv1.GetBalanceRequest{AccountId: "acct_ab12cd"}, &bal)
	if bal.GetBalanceMinorUnits() != 482355 {
		t.Errorf("balance = %d, want 482355", bal.GetBalanceMinorUnits())
	}

	var cards cardsv1.ListCardsResponse
	callTool(t, nc, "/cards.v1.CardsService/ListCards",
		&cardsv1.ListCardsRequest{CustomerId: "cust_ab12cd"}, &cards)
	if len(cards.GetCards()) != 1 {
		t.Fatalf("got %d cards, want 1", len(cards.GetCards()))
	}

	var pay paymentsv1.GetPaymentStatusResponse
	callTool(t, nc, "/payments.v1.PaymentsService/GetPaymentStatus",
		&paymentsv1.GetPaymentStatusRequest{PaymentId: "pay_ab12cd"}, &pay)
	if pay.GetStatus() != paymentsv1.PaymentStatus_PAYMENT_STATUS_SETTLED {
		t.Errorf("status = %v", pay.GetStatus())
	}

	var scr screeningv1.ScreenPartyResponse
	callTool(t, nc, "/screening.v1.ScreeningService/ScreenParty",
		&screeningv1.ScreenPartyRequest{FullName: "Ivan Petrov"}, &scr)
	if !scr.GetRequiresReview() {
		t.Error("a known match did not require review")
	}
}

// The tool side does no redaction, and that is the design rather than an
// omission.
//
// If a handler withheld anything, there would be two answers to "what may this
// caller see" — the chain's, which is reviewed and per-principal, and the
// handler's, which is neither. The second would win silently wherever it was
// stricter, and no test in the daemon could detect it.
func TestHandlersReturnEverythingAndRedactNothing(t *testing.T) {
	nc := runBank(t)

	var cust accountsv1.GetCustomerResponse
	callTool(t, nc, "/accounts.v1.AccountsService/GetCustomer",
		&accountsv1.GetCustomerRequest{CustomerId: "cust_ab12cd"}, &cust)

	for _, f := range []struct{ name, got, want string }{
		{"email", cust.GetEmail(), "ada.okonkwo@example.com"},
		{"phone", cust.GetPhone(), "+44 7700 900412"},
		{"date_of_birth", cust.GetDateOfBirth(), "1988-03-14"},
		{"national_id", cust.GetNationalId(), "QQ123456C"},
	} {
		if f.got != f.want {
			t.Errorf("%s = %q, want %q in full — the handler must not redact, or "+
				"there are two answers to what a caller may see and only one of "+
				"them is reviewed", f.name, f.got, f.want)
		}
	}

	var cards cardsv1.ListCardsResponse
	callTool(t, nc, "/cards.v1.CardsService/ListCards",
		&cardsv1.ListCardsRequest{CustomerId: "cust_ab12cd"}, &cards)
	if got := cards.GetCards()[0].GetPan(); got != "4111111111111111" {
		t.Errorf("pan = %q, want the full number; the chain turns it into BIN and "+
			"last four on the way out", got)
	}
}

// initiate_payment declares idempotent: true, and the chain has no way to
// check that. Nothing verifies this promise except a test on the tool itself —
// and a payment tool that lies about it duplicates money on an agent retry,
// which is the exact failure its own guidance warns about.
func TestAPaymentIsIdempotentOnItsKeyAsTheAnnotationPromises(t *testing.T) {
	nc := runBank(t)

	req := &paymentsv1.InitiatePaymentRequest{
		SourceAccountId:  "acct_ab12cd",
		BeneficiaryIban:  "GB29NWBK60161331926819",
		AmountMinorUnits: proto.Int64(125000),
		CurrencyCode:     "GBP",
		IdempotencyKey:   "retry-me-0123456789abcdef",
	}

	var first, second paymentsv1.InitiatePaymentResponse
	callTool(t, nc, "/payments.v1.PaymentsService/InitiatePayment", req, &first)
	callTool(t, nc, "/payments.v1.PaymentsService/InitiatePayment", req, &second)

	if first.GetPaymentId() == "" {
		t.Fatal("no payment id")
	}
	if first.GetPaymentId() != second.GetPaymentId() {
		t.Errorf("two payments for one key: %q then %q. An agent retrying a call it "+
			"never saw the answer to has just paid twice",
			first.GetPaymentId(), second.GetPaymentId())
	}
}

// Four proto packages, four descriptor hashes.
//
// The daemon records one hash per package and quarantines a tool whose service
// advertises a different one. A process that served all four domains under a
// single micro service could advertise only one identity, so three of the four
// would be permanently mismatched — a failure that looks like a contract drift
// and is really a deployment shape.
func TestEachDomainAdvertisesItsOwnDescriptorHash(t *testing.T) {
	seen := map[string]string{}
	for name, hash := range map[string]string{
		"accounts":  accountsv1.DescriptorHash,
		"cards":     cardsv1.DescriptorHash,
		"payments":  paymentsv1.DescriptorHash,
		"screening": screeningv1.DescriptorHash,
	} {
		if hash == "" {
			t.Errorf("%s advertises no descriptor hash; the daemon cannot tell "+
				"whether it implements the contract the catalogue holds", name)
			continue
		}
		if other, dup := seen[hash]; dup {
			t.Errorf("%s and %s advertise the same hash %s; one of them is lying "+
				"about which contract it serves", name, other, hash)
		}
		seen[hash] = name
	}
}

// The same key, arriving at once.
//
// Two retries of a timed-out payment racing each other is the realistic shape:
// an agent that saw no answer retries, and so does the human watching it.
//
// Measured while writing this: tool-go currently handles ONE request at a time
// per endpoint — eight concurrent calls to a 150ms handler peaked at a
// concurrency of one, because a NATS async subscription delivers sequentially.
// So this test does not presently exercise the lock in InitiatePayment, and
// removing that lock does not fail it.
//
// The lock stays and the test stays, for the same reason: neither is a
// statement about today's runtime. A worker pool in tool-go, a second service
// instance, or a second endpoint touching the same map all make the race real,
// and none of them would announce themselves by failing a test that had been
// deleted for being redundant.
func TestConcurrentRetriesOfOnePaymentStillMakeOnePayment(t *testing.T) {
	nc := runBank(t)

	const n = 16
	ids := make(chan string, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // release together, so they actually contend
			body, err := proto.Marshal(&paymentsv1.InitiatePaymentRequest{
				SourceAccountId:  "acct_ab12cd",
				BeneficiaryIban:  "GB29NWBK60161331926819",
				AmountMinorUnits: proto.Int64(125000),
				CurrencyCode:     "GBP",
				IdempotencyKey:   "concurrent-retry-0123456789ab",
			})
			if err != nil {
				return
			}
			msg, err := nc.Request(
				garmtool.Subject("/payments.v1.PaymentsService/InitiatePayment"),
				body, 5*time.Second)
			if err != nil {
				return
			}
			var resp paymentsv1.InitiatePaymentResponse
			if proto.Unmarshal(msg.Data, &resp) == nil {
				ids <- resp.GetPaymentId()
			}
		}()
	}
	close(start)
	wg.Wait()
	close(ids)

	distinct := map[string]bool{}
	got := 0
	for id := range ids {
		distinct[id] = true
		got++
	}
	if got != n {
		t.Fatalf("%d of %d calls answered", got, n)
	}
	if len(distinct) != 1 {
		t.Errorf("%d distinct payment ids for one idempotency key: %v. Every extra "+
			"one is money moved twice", len(distinct), distinct)
	}
}
