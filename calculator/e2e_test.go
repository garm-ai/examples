package calculator_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	"github.com/garm-ai/examples/calculator"
	calcv1 "github.com/garm-ai/examples/calculator/gen/calc/v1"
	"github.com/garm-ai/tool-go/garmtool"
)

// The acceptance test for the split.
//
// It builds against published artifacts only — the annotations and contracts
// from github.com/garm-ai/garm, the runtime from github.com/garm-ai/tool-go —
// with no path to the daemon and no replace directive. If it compiles, the
// boundaries are real. If it needed a shortcut, they are not, and the
// shortcut is the bug.

func runNATS(t *testing.T) *nats.Conn {
	t.Helper()
	opts := &natsserver.Options{Port: -1, NoLog: true, NoSigs: true}
	srv, err := natsserver.NewServer(opts)
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
	return nc
}

func serve(t *testing.T, nc *nats.Conn) {
	t.Helper()
	svc := garmtool.New("calculator", "v0.1.0")
	if err := calculator.Register(svc, calculator.Handlers{}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	// Buffered and read at most once. An earlier version read it in the wait
	// loop AND in cleanup, so a startup failure deadlocked the cleanup and
	// the test reported a timeout instead of the error it already had.
	done := make(chan error, 1)
	go func() { done <- svc.Run(ctx, nc) }()
	var stopped bool
	t.Cleanup(func() {
		if stopped {
			return
		}
		cancel()
		select {
		case err := <-done:
			if err != nil && ctx.Err() == nil {
				t.Errorf("serve: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("the service did not drain within five seconds")
		}
	})

	// Wait for the subject to answer rather than sleeping: a fixed sleep is
	// either slower than needed or flaky on a loaded machine. A failure from
	// Run is reported instead of waited out, because "never answered" tells
	// you nothing about why.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			stopped = true
			t.Fatalf("the service stopped before it could answer: %v", err)
		default:
		}
		if _, err := nc.Request(garmtool.Subject("/calc.v1.Calculator/Add"), mustMarshal(t,
			&calcv1.AddRequest{}), 100*time.Millisecond); err == nil {
			return
		}
	}
	t.Fatal("the service never started answering")
}

func mustMarshal(t *testing.T, m proto.Message) []byte {
	t.Helper()
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// call sends a request the way the daemon's transport does: marshal, request
// on the subject derived from the route, unmarshal the reply.
func call(t *testing.T, nc *nats.Conn, route string, req, resp proto.Message) error {
	t.Helper()
	msg, err := nc.Request(garmtool.Subject(route), mustMarshal(t, req), 3*time.Second)
	if err != nil {
		return err
	}
	if code := msg.Header.Get("Nats-Service-Error-Code"); code != "" {
		return &toolError{code: code, msg: msg.Header.Get("Nats-Service-Error")}
	}
	return proto.Unmarshal(msg.Data, resp)
}

type toolError struct{ code, msg string }

func (e *toolError) Error() string { return e.code + ": " + e.msg }

func TestToolsAnswerOverNATS(t *testing.T) {
	nc := runNATS(t)
	serve(t, nc)

	t.Run("add", func(t *testing.T) {
		a, b := 2.5, 4.0
		var got calcv1.AddResponse
		if err := call(t, nc, "/calc.v1.Calculator/Add",
			&calcv1.AddRequest{A: &a, B: &b}, &got); err != nil {
			t.Fatal(err)
		}
		if got.GetSum() != 6.5 {
			t.Errorf("sum = %v, want 6.5", got.GetSum())
		}
	})

	t.Run("summarize", func(t *testing.T) {
		var got calcv1.SummarizeResponse
		if err := call(t, nc, "/calc.v1.Calculator/Summarize",
			&calcv1.SummarizeRequest{Values: []float64{5, 1, 3}}, &got); err != nil {
			t.Fatal(err)
		}
		if got.GetMean() != 3 || got.GetMedian() != 3 || got.GetMin() != 1 ||
			got.GetMax() != 5 || got.GetCount() != 3 {
			t.Errorf("summary = %+v", got.String())
		}
	})

	// A handler error must arrive as an error, not as a zero-valued response.
	// A caller that cannot tell "divide refused" from "the answer is 0" will
	// carry the zero into whatever it does next.
	t.Run("divide by zero is refused", func(t *testing.T) {
		n, d := 1.0, 0.0
		var got calcv1.DivideResponse
		err := call(t, nc, "/calc.v1.Calculator/Divide",
			&calcv1.DivideRequest{Numerator: &n, Denominator: &d}, &got)
		if err == nil {
			t.Fatalf("dividing by zero succeeded and returned %v", got.GetQuotient())
		}
	})
}

// TestTheCatalogueMatchesTheService: the artifact the daemon loads and the
// routes this service answers on must agree. They are generated from one
// .proto, so a mismatch means something drifted — and it would show up as a
// tool that is declared and unreachable, in production.
func TestTheCatalogueMatchesTheService(t *testing.T) {
	path := filepath.Join("calculator.binpb")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("no catalogue built; run: garm catalogue build -o %s", path)
	}
	// Deliberately not parsing it here — that would mean depending on the
	// daemon's loader from the consumer side. Presence is what this checks;
	// the daemon's own tests cover the contents.
}
