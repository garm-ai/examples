# examples

**Where to start, and the proof the boundaries hold.**

## calculator

The smallest tool worth governing: no side effects, nothing secret, three
typed methods.

```
add(a, b)                → sum
divide(numerator, denom) → quotient    refuses a zero denominator
summarize(values[])      → mean, median, min, max, count
```

Typed methods rather than one `eval(expression)` on purpose. An expression in
and a number out has no schema, and the schema is what garm is about — field
annotations, projection, redaction all act on typed fields.

```console
$ mise run gen          # messages from the proto
$ mise run catalogue    # the artifact a daemon loads
$ mise run test         # including end to end over an embedded NATS
$ mise run serve        # run calcd against a local NATS
```

## Running it

```console
$ nats-server &
$ go run ./calculator/cmd/calcd
serving service=calculator version=0.0.0-dev nats=nats://127.0.0.1:4222
```

`calcd` is what a tool service's `main` looks like: connect, register, run,
drain. Only the handlers are about calculating; the rest is the same in every
tool service, which is what `garm new toolservice` will eventually write.

It reconnects forever rather than exiting when NATS blinks — a service that
dies on a transient outage turns it into a deployment event, and the daemon
already reports the tool as unreachable meanwhile. SIGTERM drains rather than
closes: a call cut off mid-flight is a call whose effect the caller cannot
determine.

## Generated code is committed

`calculator/gen/` is in the repository, not ignored. A Go module has to build
from its own source: anyone cloning this to fork it runs `go build`, not buf
plus three plugins and a network round trip.

The cost of committing generated code is that it can drift from the `.proto`.
`mise run gen-check` regenerates and fails if the tree moved, and CI runs it —
so a contract change cannot ship without the binding that matches it.

The one generated thing that IS ignored is `calculator/gen/garm/`, the output
for the vendored annotations, which nothing imports because the real one lives
in `garm/contracts`.

## Why this repository is the acceptance test

It builds against **published artifacts only** — the annotations and contracts
from [`garm`](https://github.com/garm-ai/garm), the runtime from
[`tool-go`](https://github.com/garm-ai/tool-go) — with no replace directive
and no path to the daemon. CI asserts both.

If it builds, the boundaries are real. If it needed a shortcut, they are not,
and the shortcut is the bug.

## What the layout says

```
calculator/
├── proto/                    your protos — the only thing generated from
├── third_party/proto/        the garm annotations, vendored by `garm init`
├── gen/                      generated messages
├── cmd/calcd/                the binary: connect, register, run, drain
├── gen/calc/v1/calcv1micro/  the GENERATED binding: Handler, Serve, hash
├── handlers.go               the work, and nothing else
└── calculator.binpb          the catalogue (built, not committed)
```

`handlers.go` does not authenticate, authorise, check input or redact. The
daemon did all of that before the request arrived, and doing any of it here
would be a second, unreviewed implementation of the chain in the one place
that must not have one.

Registration is **generated**, not written. `ServeCalculator` comes from
`protoc-gen-garm-go -emit=toolsdk`, and with it a typed `CalculatorHandler`
interface — so a missing method is a compile error rather than a tool that
quietly fails to appear — plus the contract version and descriptor hash the
service advertises on `$SRV.INFO`.

The binding lands in a `…micro` sibling package via `package_suffix`. Without
that it sits in the base package, references the connect Handler from the base
package's connect sibling, and that sibling imports the base package back for
message types: a two-package cycle, unavoidable for any colocated package that
both generates connect code and declares a tool.

MIT licensed.
