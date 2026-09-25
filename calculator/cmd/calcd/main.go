// Command calcd serves the calculator tools over NATS.
//
// What a tool service's main looks like: connect, register, run, drain. The
// handlers are the only part that is about calculating, and the rest is the
// same in every tool service — which is the point, and is what
// `garm new toolservice` will eventually write.
//
// It does not authenticate, authorise, check input or redact the response.
// The daemon did all of that before the request arrived. Anything here that
// re-checked would be a second, unreviewed implementation of the chain in the
// one place that must not have one.
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

	"github.com/garm-ai/examples/calculator"
	calcv1 "github.com/garm-ai/examples/calculator/gen/calc/v1"
	"github.com/garm-ai/tool-go/garmtool"
)

func main() {
	if err := run(); err != nil {
		slog.Error("calcd stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	url := flag.String("nats", nats.DefaultURL, "NATS server URL")
	name := flag.String("name", "calculator", "Service name, as it appears in $SRV.INFO")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Reconnect forever rather than exit. A tool service that dies because
	// NATS blinked turns a transient outage into a deployment event, and the
	// daemon already reports a tool as unreachable while this is away.
	nc, err := nats.Connect(*url,
		nats.Name(*name),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(time.Second),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			log.Warn("disconnected from nats", "err", err)
		}),
		nats.ReconnectHandler(func(c *nats.Conn) {
			log.Info("reconnected to nats", "url", c.ConnectedUrl())
		}),
	)
	if err != nil {
		return fmt.Errorf("connecting to nats at %s: %w", *url, err)
	}
	defer nc.Close()

	svc := garmtool.New(*name, version())
	// Generated from the .proto: the routes, the request types, the contract
	// version and the descriptor hash all come from the contract rather than
	// from anything written here. A handler missing from Handlers is a
	// compile error, not a tool that quietly fails to appear.
	if err := calcv1.ServeCalculator(svc, calculator.Handlers{}); err != nil {
		return fmt.Errorf("registering the calculator tools: %w", err)
	}

	// SIGTERM cancels the context, which makes Run drain rather than close:
	// NATS stops delivering new requests here while in-flight ones finish. A
	// call cut off mid-flight is a call whose effect the caller cannot
	// determine, which for anything non-idempotent is the worst answer there
	// is — and it is worth getting right even for a calculator, because this
	// file is what people will copy.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Info("serving", "service", *name, "version", version(), "nats", nc.ConnectedUrl())
	if err := svc.Run(ctx, nc); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	log.Info("drained")
	return nil
}

// version is the module version the toolchain stamped.
//
// It is what a daemon reads back from $SRV.INFO to decide whether this process
// implements the contract it holds, so it comes from the build rather than
// from a literal someone forgets to bump.
func version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "0.0.0-dev"
}
