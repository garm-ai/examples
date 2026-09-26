// Command bankd serves the bank's tools over NATS.
//
// FOUR micro services in one process, not one service with four domains'
// endpoints, and that is a contract requirement rather than tidiness. A
// service advertises ONE descriptor hash on $SRV.INFO, the catalogue records
// one per proto package, and the daemon quarantines a tool whose service
// disagrees. Four packages therefore need four advertised identities, and a
// single service could only ever tell the truth about one of them.
//
// One process is a convenience for running the example. A bank would deploy
// these separately — different teams, different scaling, and payments in a
// zone the others are not in.
//
// Nothing here authenticates, authorises, checks input or redacts a response.
// The daemon did all of that before the request arrived.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"golang.org/x/sync/errgroup"

	"github.com/garm-ai/examples/bank"
	accountsv1 "github.com/garm-ai/examples/bank/gen/accounts/v1"
	cardsv1 "github.com/garm-ai/examples/bank/gen/cards/v1"
	paymentsv1 "github.com/garm-ai/examples/bank/gen/payments/v1"
	screeningv1 "github.com/garm-ai/examples/bank/gen/screening/v1"
	"github.com/garm-ai/tool-go/garmtool"
)

func main() {
	if err := run(); err != nil {
		slog.Error("bankd stopped", "err", err)
		os.Exit(1)
	}
}

// Register is one domain's registration, named so the list below reads as the
// set of services this process runs.
type domain struct {
	name     string
	register func(*garmtool.Service) error
}

func domains() []domain {
	return []domain{
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
	}
}

func run() error {
	url := flag.String("nats", nats.DefaultURL, "NATS server URL")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Reconnect forever rather than exit. A tool service that dies because
	// NATS blinked turns a transient outage into a deployment event, and the
	// daemon already reports a tool as unreachable while this is away.
	nc, err := nats.Connect(*url,
		nats.Name("bankd"),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(time.Second),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			log.Warn("disconnected from nats", "err", err)
		}),
	)
	if err != nil {
		return fmt.Errorf("connecting to nats at %s: %w", *url, err)
	}
	defer nc.Close()

	// Registration happens for every domain BEFORE any of them starts
	// serving. A process that came up half-registered would advertise some
	// tools and not others, and the daemon would quarantine the missing ones
	// as unreachable rather than report a startup failure — a deployment
	// problem wearing an availability problem's clothes.
	svcs := make([]*garmtool.Service, 0, len(domains()))
	for _, d := range domains() {
		s := garmtool.New(d.name, version())
		if err := d.register(s); err != nil {
			return fmt.Errorf("registering %s: %w", d.name, err)
		}
		svcs = append(svcs, s)
	}

	// SIGTERM cancels the context, which makes Run drain rather than close:
	// NATS stops delivering new requests while in-flight ones finish. A call
	// cut off mid-flight is a call whose effect the caller cannot determine,
	// which for a payment is the worst answer there is.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	g, gctx := errgroup.WithContext(ctx)
	for _, s := range svcs {
		g.Go(func() error {
			if err := s.Run(gctx, nc); err != nil && !errors.Is(err, context.Canceled) {
				return err
			}
			return nil
		})
	}

	log.Info("serving", "domains", len(svcs), "version", version(), "nats", nc.ConnectedUrl())
	if err := g.Wait(); err != nil {
		return err
	}
	log.Info("drained")
	return nil
}

// version is the module version the toolchain stamped. The daemon reads it
// back from $SRV.INFO to decide whether this process implements the contract
// it holds, so it comes from the build rather than a literal someone forgets.
func version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "0.0.0-dev"
}
