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
in `github.com/garm-ai/contracts`.

## The annotations are vendored, and vendored copies drift

Both trees keep the garm annotations as `.proto` files under
`third_party/proto/garm/`, so their own protos can write
`import "garm/tool/v1/tool.proto"` and have it resolve. They are copies, and a
copy of a published file goes stale the moment the publisher moves — which
happened here: the bank was carrying the annotations as they stood at garm
v0.16.0 and the calculator's were older still, from before `FieldPolicy.source`
and `Approval.material_fields` existed.

**Nothing warns you about this.** A stale vendored annotation compiles: your
protos still parse, the catalogue still builds, and the only symptom is that a
field you could have declared has no syntax in your tree to declare it with.

So `mise run vendor-annotations` refreshes both trees from the pinned contract
module and `mise run vendor-check` fails if either has drifted, in CI with
everything else. The bytes are written verbatim, so the check is a byte
comparison. **If you are adding a tool here and an annotation you expect is
missing, run those two before anything else.**

It is not `garm init --force`, which is the documented way to vendor them and
is right for a new tree. `init` writes six files and two of them are `buf.yaml`
and `buf.gen.yaml`: on these trees it would replace the bank's protovalidate
dependency, the `exclude_paths` keeping the adopted packages out of
generation, and both trees' real `contract_version`, with scaffold defaults.
Re-vendoring must move the vendored protos and nothing else.

## A platform proto that declares tools is an INPUT; one that only defines annotations is not

Two files in this repository come out of the same module,
`github.com/garm-ai/contracts`, at the same pinned version, and they live in
different trees. That is not an inconsistency to tidy up, and the rule is worth
knowing before you vendor the next one.

| the file | where it goes | why |
| --- | --- | --- |
| `garm/tool/v1/tool.proto`, `garm/agent/v1`, `garm/card/v1`, `garm/meta/v1` | `<tree>/third_party/proto/garm/…` | They **define annotations**. This tree's protos import them so `import "garm/tool/v1/tool.proto"` resolves. They declare no tools, so `third_party/proto` is deliberately **not** an input to `garm catalogue build`: feeding it in would add files to the descriptor set and nothing to the catalogue. |
| `garm/tasks/v1/tasks.proto` | `bank/proto/garm/tasks/v1/` | It **declares eight tools** — the task queue `tasksd` serves. The catalogue is built from one proto tree, so a declaration the builder cannot see is a tool the daemon cannot dispatch to. It has to be an input. |

**The rule is what the file declares, not which module it came from.** Declares
tools → the input tree. Defines annotations only → `third_party`.

Both copies carry the same drift risk and `mise run vendor-check` covers both:
`bank/third_party`, `calculator/third_party` and `bank/proto/garm` are compared
byte for byte against the module. Leaving the second one out would make it the
single uncovered copy in the repository, and the worse of the two to lose — a
drifted annotation costs a rule nobody applies, while a drifted
`garm/tasks/v1` means this catalogue advertises a shape of the task tools that
the `tasksd` answering them no longer has.

Adopted-package Go is never generated here, whichever tree the proto sits in.
`bank/buf.gen.yaml` excludes `proto/tools`, `proto/web` and `proto/garm`,
because the generated messages and bindings already arrived through the module
under the `go_package` those files name, and a second copy registering the same
descriptor file panics in `protoregistry` at init.

## Why this repository is the acceptance test

It builds against **published artifacts only** — the annotations and contracts
from [`contracts`](https://github.com/garm-ai/contracts), the runtime from
[`tool-go`](https://github.com/garm-ai/tool-go) — with no replace directive
and no path to the daemon. CI asserts both.

The contract used to come from [`garm`](https://github.com/garm-ai/garm), which
is now the CLI and the generator and nothing this module links. The two cannot
be held at once: `github.com/garm-ai/garm/contracts/garm/tool/v1` and
`github.com/garm-ai/contracts/garm/tool/v1` register the same descriptor file
paths, so a binary linking both compiles and then dies in `protoregistry` at
init. That is why `go.mod` names neither `garm` nor anything that requires it.

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

## bank

A proto tree shaped like a real bank's — four tool domains, a shared
taxonomy, five agents, two adopted packages — and the answer to the question
the calculator cannot ask: does this scale to three hundred engineers?
Per-principal projection, the mount refusal on an ungated payment tool, what a
grant binds, what a policy change looks like in review. Its catalogue is
twenty-seven tools in seven packages, eight of them `garm.tasks.v1` adopted
from `garm-ai/contracts` so the queue `tasksd` serves is reachable at all.
`bank/README.md` is the long version.

### An agent, declared like a tool

`bank/proto/bank/agents/v1/support_assistant.proto` declares an agent the same
way the other four directories declare tools: a proto service with an
annotation, linted by `garm lint`, built into the same catalogue, reviewed on
the same pull request. Its system prompt is `bank/prompts/support-assistant.md`
and the annotation pins that file's sha256 — `mise run prompt-sha` recomputes
it, `mise run prompt-sha-check` fails CI when it has drifted.

Nothing in this repository runs the agent. `garm-ai/agentd` does: its compose
file mounts this checkout, builds `bankd` from it and publishes its catalogue,
and its acceptance test checks this repository out at a tag (`v0.1.0` today).
See `bank/README.md`, "The agent demo, command by command".

MIT licensed.
