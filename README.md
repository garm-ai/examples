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
```

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
├── handlers.go               the work, and nothing else
├── bind.go                   registration — hand-written, and temporary
└── calculator.binpb          the catalogue (built, not committed)
```

`handlers.go` does not authenticate, authorise, check input or redact. The
daemon did all of that before the request arrived, and doing any of it here
would be a second, unreviewed implementation of the chain in the one place
that must not have one.

`bind.go` is what `protoc-gen-garm-go` should emit. It is written by hand so
the runtime could be exercised before the tool-side generator was wired up,
and it is the first thing to delete when it is.

MIT licensed.
