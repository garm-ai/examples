package bank_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"

	"github.com/garm-ai/examples/bank"
	accountsv1 "github.com/garm-ai/examples/bank/gen/accounts/v1"
	"github.com/garm-ai/tool-go/garmtool"
)

// The claim, end to end: one call, three callers, three different SHAPES.
//
// Everything else in this repository tests a part. This runs the whole thing —
// a real broker, the real tool service, a real catalogue artifact, the real
// garmd binary as a separate process, and a real signed token — and asks the
// only question the product exists to answer.
//
// garmd is a subprocess rather than a library because examples must not depend
// on it. That boundary is asserted in CI, and honouring it here is what makes
// this an acceptance test rather than a unit test wearing one's clothes: if
// the published binary and the published contracts cannot do this together,
// nothing else that passes matters.
//
// The catalogue is accounts-only, which is not a convenience. payments
// declares an approval gate and an audit stream, and this test starts `garmd
// serve` with neither a grant verifier nor an audit sink — so the full bank
// catalogue would refuse to mount here, which is the refusal `mise run
// check-bank` asserts. An accounts-only artifact is what a support cluster
// would actually run, and building one here is what catalogue slicing would do
// if it existed. The full catalogue, both supervisions supplied, is what
// garm-ai/agentd's compose serves.

const (
	e2eIssuer   = "https://bank-e2e.invalid/idp"
	e2eAudience = "garm"
)

type plane struct {
	baseURL string
	idp     *idp
}

func startPlane(t *testing.T) *plane {
	t.Helper()
	garmBin := mustFind(t, "garm")
	garmdBin := mustFind(t, "garmd")

	natsURL := startBroker(t)
	serveAccounts(t, natsURL)
	i := newIDP(t)
	cat := buildSupportCatalogue(t, garmBin)
	key := writeHashKey(t)
	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, garmdBin, "serve",
		"--catalogue", cat,
		"--nats", natsURL,
		"--listen", addr,
		"--jwks", i.URL+"/jwks.json",
		"--issuer", e2eIssuer,
		"--audience", e2eAudience,
		"--hash-key-file", key,
	)
	var logs strings.Builder
	cmd.Stdout, cmd.Stderr = &logs, &logs
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting garmd: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
		if t.Failed() {
			t.Logf("garmd output:\n%s", logs.String())
		}
	})

	base := "http://" + addr
	waitForPlane(t, base, cmd, &logs)
	return &plane{baseURL: base, idp: i}
}

// mustFind refuses rather than skips.
//
// A test that skips when a binary is absent reports ok while covering nothing,
// and this is the only test in the repository that exercises the daemon at
// all. `mise run test` installs both; a bare `go test` is told what to run.
func mustFind(t *testing.T, bin string) string {
	t.Helper()
	path, err := exec.LookPath(bin)
	if err != nil {
		t.Fatalf("%s is not on PATH. This test runs the real binaries rather than "+
			"importing them, because examples must not depend on garmd. Run "+
			"`mise run test`, which installs them at the pinned versions.", bin)
	}
	return path
}

func startBroker(t *testing.T) string {
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
	return srv.ClientURL()
}

func serveAccounts(t *testing.T, natsURL string) {
	t.Helper()
	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)

	svc := garmtool.New("accounts", "v0.1.0")
	if err := accountsv1.ServeAccountsService(svc, bank.Accounts{}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- svc.Run(ctx, nc) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("accounts did not drain")
		}
	})
	waitFor(t, nc, "/accounts.v1.AccountsService/GetBalance")
}

// buildSupportCatalogue is what slicing would produce: the taxonomy plus one
// domain. payments cannot be in it, because no garmd can serve an
// approval-gated tool yet.
func buildSupportCatalogue(t *testing.T, garmBin string) string {
	t.Helper()
	dir := t.TempDir()
	for _, d := range []string{"bank", "accounts"} {
		if err := os.MkdirAll(filepath.Join(dir, "proto", d), 0o755); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command("cp", "-r",
			filepath.Join("proto", d, "v1"), filepath.Join(dir, "proto", d)).CombinedOutput(); err != nil {
			t.Fatalf("copying %s: %v %s", d, err, out)
		}
	}
	out := filepath.Join(dir, "support.binpb")
	cmd := exec.Command(garmBin, "catalogue", "build",
		"--proto", filepath.Join(dir, "proto"), "-o", out)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the catalogue: %v\n%s", err, b)
	}
	return out
}

func writeHashKey(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hash.key")
	if err := os.WriteFile(path, []byte("an e2e hash key, comfortably long"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// freePort asks the kernel for one and lets it go. Racy in principle; the
// alternative is a hardcoded port, which is racy in practice.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func waitForPlane(t *testing.T, base string, cmd *exec.Cmd, logs *strings.Builder) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if cmd.ProcessState != nil {
			t.Fatalf("garmd exited before it served:\n%s", logs.String())
		}
		// Any answer means it is listening. An unauthenticated POST gets a
		// 401, which is a perfectly good sign of life.
		resp, err := http.Post(base+"/accounts.v1.AccountsService/GetBalance",
			"application/json", strings.NewReader("{}"))
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("garmd never started listening:\n%s", logs.String())
}

// ---------------------------------------------------------------- the IdP

type idp struct {
	*httptest.Server
	key *ecdsa.PrivateKey
	kid string
}

func newIDP(t *testing.T) *idp {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	const kid = "bank-e2e"
	set := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
		Key: key.Public(), KeyID: kid, Algorithm: string(jose.ES256), Use: "sig",
	}}}
	raw, err := json.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(raw)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &idp{Server: srv, key: key, kid: kid}
}

// persona is who is calling, in the shape the token carries.
type persona struct {
	name         string
	subject      string
	clearance    string
	compartments []string
}

func (i *idp) token(t *testing.T, p persona) string {
	t.Helper()
	now := time.Now()
	claim := map[string]any{"clearance": p.clearance, "kind": "USER", "verbs": []string{"VERB_READ"}}
	if len(p.compartments) > 0 {
		claim["compartments"] = p.compartments
	}
	body, err := json.Marshal(map[string]any{
		"iss": e2eIssuer, "sub": p.subject, "aud": e2eAudience,
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
		"garm": claim,
	})
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.ES256, Key: i.key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", i.kid))
	if err != nil {
		t.Fatal(err)
	}
	obj, err := signer.Sign(body)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := obj.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// getCustomer calls through the plane and returns the status and the decoded
// body. JSON rather than binary proto, because the point is which FIELDS came
// back and a map shows absence directly.
func (pl *plane) getCustomer(t *testing.T, p persona) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost,
		pl.baseURL+"/accounts.v1.AccountsService/GetCustomer",
		strings.NewReader(`{"customerId":"cust_ab12cd"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+pl.idp.token(t, p))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s: %v", p.name, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return resp.StatusCode, out
}

var (
	support = persona{"support agent", "user:support", "CLEARANCE_INTERNAL",
		[]string{"pii-contact"}}
	analyst = persona{"fraud analyst", "user:analyst", "CLEARANCE_RESTRICTED",
		[]string{"pii-contact", "pii-identity"}}
	outsider = persona{"cleared but need-to-know fails", "user:outsider",
		"CLEARANCE_RESTRICTED", nil}
)

// One call, three callers, three shapes.
func TestTheSameCallReturnsADifferentShapePerCaller(t *testing.T) {
	pl := startPlane(t)

	t.Run("a support agent gets contact details redacted, not withheld", func(t *testing.T) {
		code, got := pl.getCustomer(t, support)
		if code != http.StatusOK {
			t.Fatalf("status = %d, body %v", code, got)
		}
		t.Logf("support sees: %v", got)

		// The exact forms, because they are the contract. Each one is a
		// deliberate middle between disclosure and absence: enough to confirm,
		// never enough to use, and never "no data" — which is the answer that
		// escalates a call this one closes.
		for _, f := range []struct{ key, want, why string }{
			{"displayName", "A. Okonkwo",
				"readable at the tool's own floor"},
			{"email", "***@example.com",
				"the domain survives so an agent can say an address is on file"},
			{"phone", "***********0412",
				"the last four confirm a number without disclosing it"},
			{"dateOfBirth", "1988…",
				"the birth year answers eligibility without the birthday"},
		} {
			if got[f.key] != f.want {
				t.Errorf("%s = %v, want %q — %s", f.key, got[f.key], f.want, f.why)
			}
		}
		// Identity is a different compartment from contact, on purpose:
		// support reaches customers constantly and proves who they are rarely.
		// Absent, not masked — there is no useful fraction of a national
		// identifier.
		if _, present := got["nationalId"]; present {
			t.Errorf("nationalId present for a caller without pii-identity: %v", got["nationalId"])
		}
	})

	t.Run("a fraud analyst gets everything", func(t *testing.T) {
		code, got := pl.getCustomer(t, analyst)
		if code != http.StatusOK {
			t.Fatalf("status = %d, body %v", code, got)
		}
		t.Logf("analyst sees: %v", got)

		for _, f := range []struct{ key, want string }{
			{"email", "ada.okonkwo@example.com"},
			{"phone", "+44 7700 900412"},
			{"dateOfBirth", "1988-03-14"},
			{"nationalId", "QQ123456C"},
		} {
			if got[f.key] != f.want {
				t.Errorf("%s = %v, want %q in full", f.key, got[f.key], f.want)
			}
		}
	})

	// Clearance is not need-to-know. This caller outranks the support agent
	// and sees less, because the tool requires a compartment they do not hold
	// — and the answer is not-found rather than forbidden, because the
	// existence of a tool is itself information.
	t.Run("clearance without the compartment reaches nothing", func(t *testing.T) {
		code, got := pl.getCustomer(t, outsider)
		if code == http.StatusOK {
			t.Fatalf("a RESTRICTED caller with no compartments read a customer: %v", got)
		}
		if code != http.StatusNotFound {
			t.Errorf("status = %d, want 404: a tool the caller may not see must answer "+
				"exactly as one that does not exist", code)
		}
	})
}
