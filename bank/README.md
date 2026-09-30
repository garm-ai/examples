# bank

A proto tree shaped like a real bank's: four domains, eight tools, one shared
taxonomy, five agents, two adopted packages — one public tool and one platform
service — and a deliberate demonstration of what happens when you put all of
that in one catalogue.

It exists to answer a question the calculator cannot: **does this scale to
three hundred engineers?**

```
proto/
  bank/v1/          the taxonomy — which compartments and tool sets exist at all
  bank/agents/v1/   the agents — the support assistant and four more
  accounts/v1/      balances and customer details
  cards/v1/         card state, in PCI scope
  payments/v1/      moving money
  screening/v1/     sanctions and PEP screening

  tools/taxonomy/v1/  ADOPTED and still a copy, alone now — the compartments
                    and sets github.com/garm-ai/tools names. It is here because
                    the protoc plugin sees ONE DIRECTORY and refuses a tool
                    naming a set it cannot see; "The one copy left" says why

../catalogue.yaml   COMPOSED, not copied: garm.tasks.v1 out of
                    github.com/garm-ai/contracts and web.v1 out of
                    github.com/garm-ai/tools/web, each at the version this
                    tree's go.mod resolves
adopted.go          the blank import that keeps tools/web a real requirement
```

Two directories that used to sit in `proto/` — `web/v1` and `garm/tasks/v1` —
are gone. They were verbatim copies out of a module cache, held out of Go
generation by `exclude_paths`, because `garm catalogue build --proto proto` read
one directory and anything the catalogue declared had to be in it. The builder
now reads them out of the module cache at the version `go.mod` pins. See
"Adopting a tool is an entry in the manifest".

## What each domain is here to teach

**accounts** — per-principal projection. One `get_customer` call returns a
different *shape* to a support agent and to a fraud analyst: email masked to
its domain, phone to its last four, a birth date truncated to the year,
national identifier absent entirely. Not the same shape with nulls in it.

**cards** — that destructive is a grade, not a category. `freeze_card` changes
state and is `VERB_WRITE`, because the verb grades the consequence of being
wrong and a freeze is undone by the next call. The linter refused
`VERB_DESTRUCTIVE` here and it was right to: that would compel a human grant
on something reversible, and a team taught to click through a grant on a
reversible action will click through one on an irreversible one.

**screening** — that clearance and compartments are different questions. A
senior engineer cleared to RESTRICTED has no business in `kyc`; a compliance
analyst two grades below them does. Seniority is not need-to-know.

**payments** — the refusal. `initiate_payment` is destructive, irreversible,
external, gated on a human grant, and written to an audit stream that must
succeed before the money moves. A `garmd` with no GrantVerifier and no audit
Sink **will not start** rather than serve it ungated. Do not weaken the
annotations to make a build go green: a payment tool that mounts with less
supervision than its schema claims is worse than one that will not mount,
because the declaration reads as protection to everyone who reviews it.

**bank/agents** — that a manifest is a schema. `SupportAssistant` is a proto
service with a `(garm.agent.v1.agent)` annotation: the authority it runs as,
the model alias, the bounds, the prompt it is pinned to by hash, and the two
tools it may reach. It sits beside those tools, so widening what an agent may
do is a proto diff on the same pull request as the tools it widens toward, and
`mise run diff-bank` reports it like any other policy change.

Two annotations on one service, answering two questions.
`(garm.agent.v1.agent)` is what the agent **runs as**.
`(garm.tool.v1.tool)` on `Invoke` is what a **caller** needs to start it — and
it is `CLEARANCE_INTERNAL` with no compartments, which is much lower than the
agent's own `RESTRICTED`. That is the point: starting a run does not require
the authority the run will use, because the run is confined by the fold to the
intersection of the agent's authority and the caller's, and every destructive
thing inside it is separately gated. Conflating the two would mean only
someone already able to move money could ask an assistant to draft one.

## One buf module, directories per domain

Per-domain buf *modules* were the first attempt. Buf refuses them for a good
reason: a module root must contain the full package path, so `accounts.v1`
under a module rooted at `proto/accounts` would have to live in
`proto/accounts/accounts/v1`. A doubled directory for nothing.

Nothing is lost. CODEOWNERS routes on paths, `buf breaking` reports per file,
and a catalogue is sliced by directory rather than by module. The domain
boundary is the directory; buf does not have to agree for it to be real.

## Every domain imports the taxonomy, and must

`bank/v1/taxonomy.proto` declares the compartments. A compartment naming
anything undeclared is refused at mount, which makes that file the bank's
allowlist rather than documentation of one. Adoption has added two more
declaring files. `tools/taxonomy/v1/taxonomy.proto`, adopted from
`garm-ai/tools` with the names its tools use (`internet`,
`generated-artefacts`; `research`, `documents`). And
`garm/tasks/v1/tasks.proto`, composed in from `garm-ai/contracts`, which
declares no compartment at all and exactly one tool set, `triage` — the set an
agent's manifest lists if it may work a queue without deciding anything on it.
The catalogue merges identical declarations of one name and refuses different
ones (L29), and today the three files share none: the compartment count is still
seven and the tool sets six.

Each domain imports it even though it references no symbol from it. That is
not tidiness — without the import the taxonomy is not in the domain's
dependency graph, and any tool working on a subset of files cannot see the
declarations. `buf generate` fails exactly that way. A domain that uses a
compartment name *depends on* the file declaring that name, and the import
graph should say so.

**And that sentence is why `proto/tools/` is the one adopted directory still
copied into this tree.** `garm catalogue build` composes the whole catalogue and
would read the taxonomy out of `github.com/garm-ai/tools/taxonomy` perfectly
happily. The protoc plugin is the problem: buf runs it over one directory, lint
rule L20 refuses a tool that names an undeclared tool set as an **error** rather
than degrading to a warning on a partial set the way A3 does, and
`research_assistant.proto` names `research`. So the file declaring `research` has
to be inside the tree buf compiles. Proved by deleting it: `garm catalogue build`
was content and `mise run gen` failed on two L20 errors.

It goes away by either route — a deployment declaring its own vocabulary in
`catalogue.yaml`, which also removes the import that reaches it, or L20 joining
A3 in the group the plugin warns on instead of refusing. Until then it is a copy
nothing gates, which is worth knowing rather than inferring; `mise.toml` says so
where `vendor-check` is defined.

## The claim, demonstrated

`mise run test` runs the whole thing: a real broker, the real tool service, a
real catalogue artifact, the real `garmd` **binary as a separate process**, and
a real signed token. One call to `get_customer`, three callers:

```
support:  email ***@example.com          phone ***********0412   dateOfBirth 1988…
analyst:  email ada.okonkwo@example.com  phone +44 7700 900412   dateOfBirth 1988-03-14
                                                                 nationalId  QQ123456C
outsider: 404
```

Three things in there are the whole design.

**The support agent's fields are redacted, not withheld.** A domain, a last
four, a birth year. Each is a deliberate middle: enough to confirm, never
enough to use, and never "no data" — which is the answer that escalates a call
this one closes.

**`nationalId` is absent rather than masked**, because there is no useful
fraction of a national identifier — and it is absent from the *schema*, so a
model never learns the field exists and never retries for it.

**The outsider is `CLEARANCE_RESTRICTED` and sees less than the support agent**,
who is two grades below them. Clearance is not need-to-know. The answer is 404
rather than 403, because the existence of a tool is itself information.

garmd is a subprocess rather than an import, because examples must not depend
on it and CI asserts that. Honouring the boundary here is what makes this an
acceptance test: if the published binary and the published contracts cannot do
this together, nothing else that passes matters.

The catalogue this test serves is accounts-only, and that is not a
convenience. `payments` declares an approval gate and an audit stream, and the
test starts `garmd serve` with neither a grant verifier nor an audit sink, so
the full bank catalogue would refuse to mount here — the refusal `mise run
check-bank` asserts. An accounts-only artifact is what a support cluster would
run, and building one here is what catalogue slicing would do if it existed.
The full ten-tool catalogue, grants and audit stream included, is served by
the compose in `garm-ai/agentd`; see "The agent demo, command by command"
below.

## What a payment grant actually binds

Approval on `initiate_payment` has two axes and a digest, and all three are
asserted in `bank/material_test.go` rather than only read off the proto,
because they travel:

- **`approver_compartments: ["financial"]`**, alongside
  `approver_min_clearance: CLEARANCE_RESTRICTED`. Clearance alone would let any
  sufficiently senior employee approve a payment; the compartment says whose
  business it is. Both axes must hold — a `CLEARANCE_CONFIDENTIAL` approver
  with no `financial` compartment must not be able to approve, and neither
  should a `financial` approver who has not cleared `RESTRICTED`.
- **`material_fields: ["amount_minor_units", "beneficiary_iban",
  "currency_code"]`**. Without these a grant binds only `(tool, subject,
  time)`, so a fifteen-minute approval would authorise *any* call to
  `initiate_payment` in the window — approve ten pounds, send ten thousand.
  With them, the grant carries a digest over these three values and garmd
  refuses a request that does not match, so the human approved *this* payment
  rather than *a* payment. `ListTools` returns these same names, agentd
  extracts them from the request it is about to send, the STS digests them,
  and garmd re-extracts and compares — a rename that misses one of those four
  places is a grant that silently covers less than the approver believed.

  Three, not six: `source_account_id` and `idempotency_key` are real and
  material to the *system*, but an approver reading a sentence decides on the
  amount, the destination and the currency — adding fields nobody reads makes
  the digest stricter without making the approval better informed, and
  `reference` is free text a caller chooses, which would let a tampering
  caller invalidate its own grant.

- **`idempotency_key` is the runner's, never the model's.** Its field policy
  carries `source: SOURCE_RUNNER` (garm v0.16.0). agentd strips it from the
  schema the model sees and fills it with `<run_id>-<dispatch seq>` — the
  same key on the granted retry of a parked payment, a different one for a
  second payment in the same run — so a model that retries after a timeout
  it never saw the answer to cannot pay twice by inventing a fresh key. Lint
  L34 holds the mark to that one rule. garmd's half — projecting the field
  out of `ListTools` and refusing a caller that sets it without a runner
  identity — is garmd v0.2.2; the v0.2.0 daemon this tree pins ignores the
  mark, which is why `mise run check-bank` still mounts on it.

## The refusal is asserted, in both directions

`initiate_payment` declares two independent forms of supervision — `MODE_GRANT`
and `LEVEL_AUDIT` with `fail_closed` and seven years of retention — and each is
independently sufficient to refuse an unsupervised deployment. `garm.tasks.v1`'s
`decide_task` declares the same audit block, for the same reason: the decision
that releases the money is as irreversible as the payment. `mise run
check-bank` builds the catalogue and runs `garmd check` against it four times,
and the three failing runs are the point:

1. a bare deployment (no grant verifier, no audit sink) — **must fail**.
2. a deployment with an audit sink but **no** grant verifier — **must also
   fail**, because a durable record of an ungated payment is not a substitute
   for a grant a person signed.
3. a deployment with a grant verifier but **no** audit sink — **must also
   fail**, because a grant alone does not buy a durable record before the
   money moves.
4. a deployment with both, and an audit sink keeping at least the seven years
   `initiate_payment` declares — **must mount**.

Asserting only that the catalogue mounts somewhere would pass equally well if
one of the two refusals had quietly stopped working, and a refusal that has
stopped working is a payment tool served with no grant or no audit trail. Same
catalogue, four capability combinations, four outcomes — that is what makes it
a test rather than a formality.

**Run 2 is new, and adopting `garm.tasks.v1` is what required it.** `garmd
check` names the **first** tool it would refuse over, `decide_task` sorts ahead
of `initiate_payment`, and `decide_task` declares audit and no approval mode —
so the bare run's refusal is now about the missing audit sink even though the
grant verifier is missing too. A broken `GrantVerifier` check would have left
every one of the old three runs green. Isolating the grant leg on a deployment
that already has the audit sink puts that assertion back where a reader can
see it.

Two layers catch it, which is worth knowing when you are tempted to weaken an
annotation to get a build green:

```
# remove the supervision and drop the verb to WRITE:
error: L16: InitiatePayment: irreversible and external requires at least MODE_NOTIFY
```

The linter refuses to *build* a catalogue serving an irreversible external
tool with no supervision at all. The mount check then refuses to *serve* one
whose supervision this deployment cannot apply. Getting a payment tool past
both, ungated, takes a deliberate lie about its effects.

## The agent demo, command by command

`garm-ai/agentd` holds the runner and a compose file that brings the whole
thing up: NATS with JetStream, Postgres, SeaweedFS as the S3, the dev IdP, the
STS, `garmd`, `bankd` built from this tree, and `agentd`. From a checkout of
`agentd` beside this one (`deploy/README.md` there lists the ports and what
`deploy/.env` may move):

```console
$ cd ../agentd
$ mise run up
== storage, identity and the broker
createbucket: bucket garm created
provision: GARM_LEDGER: created
provision: GARM_AUDIT: created
provision: GARM_SINK_DEAD: created
== publishing the bank catalogue
ok — 16 file(s), no errors
wrote bank.binpb
  10 tool(s) in 5 package(s), 16 file(s), 18 documented field(s), schema v1
  digest sha256:fd6be733…
wrote s3://garm/catalogue/prompts/0654986860461c19d2cb12f84af3915f82efc846c79659962696de224abbd5a4.md
wrote s3://garm/catalogue/catalogue.binpb
published s3://garm/catalogue/catalogue.binpb
== the tool plane
== waiting for garmd and agentd
ready: garmd on 7440, agentd on 7460, idp on 7451, sts on 8080
```

`mise run publish` is the interesting step and runs on its own too. It lints,
builds and uploads — **prompts first, catalogue last**, because a catalogue
visible before the prompt it pins is a runner that refuses an agent by name
for a file that is about to exist. The prompt object is named by the sha256
the annotation declares, which is why `mise run prompt-sha-check` is in CI:
`garmd` and `agentd` both poll the catalogue object and reload on a changed
ETag, so editing `prompts/support-assistant.md`, running `mise run
prompt-sha` and `mise run publish` is the whole deploy.

The compose has no real model in it: `agentd` is pointed at the acceptance
test's scripted model, so a run started by hand with nothing else running
fails at its first generation. `deploy/README.md` in `agentd` says which two
values in `deploy/.env` point it at a real one; the commands below assume
that, or the scripted model that `mise run e2e` starts.

### Start a run as jdoe

```console
$ JDOE=$(curl -s '127.0.0.1:7451/token?user=jdoe')
$ curl -s -X POST 127.0.0.1:7460/runs -H "Authorization: Bearer $JDOE" \
    -H 'Content-Type: application/json' -d '{
      "agent": "support-assistant",
      "input": {
        "customerId": "cust_ab12cd",
        "request": "Please send GBP 1250.00 to the beneficiary at GB29NWBK60161331926819 for this customer, ref demo-0000000000001"
      }
    }'
{"run_id":"01jb8xk2m0q7v3ry5f9d2h4n6p"}
```

`201`, and a run id. The run looks the customer up, proposes the payment, and
stops:

```console
$ curl -s "127.0.0.1:7460/runs/01jb8xk2m0q7v3ry5f9d2h4n6p" \
    -H "Authorization: Bearer $JDOE" | python3 -m json.tool | head -5
{
    "status": {
        "runId": "01jb8xk2m0q7v3ry5f9d2h4n6p",
        "state": "RUN_STATE_WAITING_APPROVAL",
        "startedAt": "…"
```

Under `status` is `garm.agent.v1.RunStatus` in protojson — the same message
`GetRun` answers through `garmd`. Beside it are `steps`, the transcript index
(a `seq`, a `kind`, the `tool_fqn` and the `ledger_event_id` of every
dispatch — digests, never bodies), and `tasks`.

### The task is addressed to whoever qualifies, and jdoe is not it

`initiate_payment` declares `approver_min_clearance: CLEARANCE_RESTRICTED` and
`approver_compartments: ["financial"]`. Of the personas, **`sam`**
(`payments-ops`) satisfies both. `amir` holds `financial` and is cleared only
to `CONFIDENTIAL`, so the task is not in amir's queue and amir cannot approve
it. `jdoe` satisfies the predicate arithmetically — jdoe holds `payments-ops`
too — and is excluded anyway, because the run's own subject never approves its
own task. That exclusion is the four eyes.

```console
$ SAM=$(curl -s '127.0.0.1:7451/token?user=sam')
$ curl -s '127.0.0.1:7460/tasks?view=queue' -H "Authorization: Bearer $SAM" \
    | python3 -m json.tool
{
    "tasks": [
        {
            "id": "01jb8xk3…",
            "run_id": "01jb8xk2m0q7v3ry5f9d2h4n6p",
            "tool_fqn": "payments.v1.initiate_payment",
            "subject": "employee:jdoe",
            "material": {
                "amount_minor_units": "125000",
                "beneficiary_iban": "GB29NWBK60161331926819",
                "currency_code": "GBP"
            },
            "state": "open",
            …
        }
    ],
    "truncated": false
}

$ curl -s '127.0.0.1:7460/tasks?view=queue' -H "Authorization: Bearer $JDOE"
{"tasks":[],"truncated":false}
```

`material` is what the grant binds to. Approving this authorises **this**
payment: garmd re-extracts those three values from the request the runner
actually sends and refuses a digest that does not match, so a runner that
showed sam one amount and sent another is caught without the STS ever seeing
the bank's catalogue.

What sam actually reads is a card, and the card is declared in the proto too:

```console
$ TASK=tsk_…
$ curl -s "127.0.0.1:7460/tasks/$TASK/card" -H "Authorization: Bearer $SAM" \
    | python3 -c 'import sys,json; c=json.load(sys.stdin); print(c["title"]); print(json.dumps(c["body"][:2], indent=2))'
Payment: GBP 125000 (minor units)
[
  { "facts": { "facts": [
      { "label": "Owned by", "value": "payments-platform" },
      { "label": "Contact",  "value": "#payments-oncall" } ] } },
  { "facts": { "facts": [
      { "label": "To",                   "value": "GB29NWBK60161331926819" },
      { "label": "Amount (minor units)", "value": "125000" },
      { "label": "Currency",             "value": "GBP" } ] } }
]
```

The title and the three labelled facts come from the
`(garm.card.v1.task_card)` on `InitiatePayment`; the two facts above them
come from the `(garm.meta.v1.owner)` on `PaymentsService` — see "Owners and
cards" above. The rest of the card is the runner's: who requested it, the
run it belongs to, when it expires, the audit trail, a `reason` input and the
`approve`/`decline` actions (`POST /tasks/{id}/card` is the card's form of
the two routes below). Lint rule C1 is why the template can show only those
three values: they are the `material_fields`, the task stores nothing else of
the request, and so what the approver reads is exactly what the grant digest
binds. The inbox at `http://127.0.0.1:7460/` renders the same card.

Try the two who do not qualify. Neither sees the task, and neither can act on
it — `404`, not `403`, because a task a caller may not act on is a task they
may not learn exists:

```console
$ AMIR=$(curl -s '127.0.0.1:7451/token?user=amir')
$ curl -s -o /dev/null -w '%{http_code}\n' -X POST "127.0.0.1:7460/tasks/$TASK/approve" \
    -H "Authorization: Bearer $AMIR" -H 'Content-Type: application/json' -d '{"reason":"looks fine"}'
404
$ curl -s -o /dev/null -w '%{http_code}\n' -X POST "127.0.0.1:7460/tasks/$TASK/approve" \
    -H "Authorization: Bearer $JDOE" -H 'Content-Type: application/json' -d '{"reason":"it is my own"}'
404
```

### Approve, and watch it execute once

```console
$ curl -s -o /dev/null -w '%{http_code}\n' -X POST "127.0.0.1:7460/tasks/$TASK/claim" \
    -H "Authorization: Bearer $SAM"
204
$ curl -s -o /dev/null -w '%{http_code}\n' -X POST "127.0.0.1:7460/tasks/$TASK/approve" \
    -H "Authorization: Bearer $SAM" -H 'Content-Type: application/json' \
    -d '{"reason":"verified the beneficiary by phone"}'
204
$ curl -s "127.0.0.1:7460/runs/01jb8xk2m0q7v3ry5f9d2h4n6p" \
    -H "Authorization: Bearer $JDOE" | python3 -m json.tool | head -4
{
    "status": {
        "runId": "01jb8xk2m0q7v3ry5f9d2h4n6p",
        "state": "RUN_STATE_COMPLETED",
```

The task routes answer `204`: the decision is recorded, and `GET /tasks/{id}`
says the rest — `"state": "approved"`, `"decided_by": "employee:sam"`, the
reason, and a `grant_jti`. The identifier and never the grant: the task API
exposes what a ledger row can be joined on, not the credential itself. A
second approve on the same task is `409`.

The grant is spent. Presenting it a second time — the same `Garm-Grant` on the
same request — is refused with `403 {"code":"permission_denied"}` and its own
`Garm-Event-Id`: an approval authorises one call, and a grant that could be
spent twice would be a fifteen-minute licence to repeat a payment. A grant
presented with one material value changed is refused the same way, and the
approval is not burned by the attempt.

### The other door

The same agent is a governed tool, so `garmd` serves it like any other:

```console
$ curl -s -X POST 127.0.0.1:7440/bank.agents.v1.SupportAssistant/Invoke \
    -H "Authorization: Bearer $JDOE" -H 'Content-Type: application/json' \
    -d '{"customerId":"cust_ab12cd","request":"… ref demo-0000000000002"}'
{"runId":"01jb8xm5…"}

$ curl -s -X POST 127.0.0.1:7440/bank.agents.v1.SupportAssistant/GetRun \
    -H "Authorization: Bearer $JDOE" -H 'Content-Type: application/json' \
    -d '{"runId":"01jb8xm5…"}'
{"runId":"01jb8xm5…","state":"RUN_STATE_RUNNING"}
```

Both answers carry `Garm-Catalogue-Digest` and `Garm-Event-Id`, because they
are governed calls: `garmd` never reads the agent annotation. It sees two
tools, `support_assistant` and `support_assistant_run`, and applies the same
ten steps it applies to `get_balance`. What differs is on the ledger. Every
tool row a run produces is at chain depth 2 — `employee:jdoe` acted for by
`agent:support-assistant` — through either door. A run started this way also
carries an `exec` claim naming `runner:agentd`, so each of its tool rows
records which runner executed it; a run started through the direct door has
no `exec`, because the human's own bearer, not a governed hop, is what
reached the runner.

### Tear it down

```console
$ mise run down
```

The volumes go with it, deliberately: Postgres holds run rows, JetStream holds
a spent-grant cache, and "the grant was already spent" is a confusing way to
learn that a previous demo is still lying around.

### The same thing, as a test

`agentd/e2e` is the acceptance test: eleven scenarios over exactly these
routes, with a scripted model deciding each turn, from `mise run up && mise
run e2e && mise run down` in `agentd`. It runs nightly and on demand from
`agentd`'s `e2e` workflow (`.github/workflows/e2e.yml`), which checks this
repository out at the tag its `examples_ref` input names — `v0.1.0` by
default — because the catalogue it publishes and the `bankd` it builds must
come from one tree.

## What this tree proves is still missing

**One catalogue is one blast radius.** Build this whole tree into a single
artifact and `payments` refuses the mount — so `accounts`, `cards` and
`screening` do not serve either. One team's annotation stops the bank. At
three hundred engineers and a hundred deploys a day, that is not a thought
experiment.

**And slicing does not work yet.** The obvious fix is a catalogue per domain,
but a manifest whose one `path:` entry is `bank/proto/accounts` fails the same
way `--proto proto/accounts` always did: the taxonomy lives in `proto/bank` and a
directory slice loses it, so every compartment reads as undeclared.

The fix is to slice the **artifact** rather than the compile — build the whole
tree so imports and declarations always resolve, and emit only the named
packages. That flag does not exist. Until it does, a bank runs one catalogue
and accepts the blast radius, which is not an acceptable answer. The manifest
does not change this: it composes more inputs into one catalogue, which is the
opposite axis.

**~~Policy changes are invisible in review.~~** Fixed — see below.

## What a policy change looks like in review

The annotations *are* the policy, so lowering a `min_clearance` is a security
decision that arrives as an ordinary proto diff. CODEOWNERS routes it to a
domain owner, who is not a security reviewer, and at this scale nobody reads
every diff.

`mise run diff-bank` builds the catalogue on both sides of a branch and
reports what moved. CI runs it on every pull request:

```
WIDENING — more callers, or less recorded (2)
  screening.v1.ScreenPartyResponse.requires_review
      read CLEARANCE_CONFIDENTIAL → CLEARANCE_INTERNAL
      every caller cleared below the old bar now reads the value in full
  screening.v1.ScreenPartyResponse.requires_review
      compartment "kyc" no longer required to read
      a wider audience reads the value in full
```

That is a plausible pull request — *"support should be able to see whether a
screening needs review"* — and a reasonable domain owner would approve the
diff without noticing it takes a sanctions signal out of `kyc`.

Three things about the output are deliberate.

**It reports direction, not fields.** `git diff` already lists the fields. What
a reviewer cannot compute in their head is whether the change means more
callers or fewer.

**Inherited defaults are resolved.** A field carrying no annotation of its own
still moves when its message default moves, so a diff reading only explicit
annotations would report nothing. That is also the edit most likely to be made
carelessly, because it is made in one place and lands on every field.

**Verb and redaction changes are `UNCLEAR`, not `WIDENING`.** A verb going
READ → WRITE moves a tool between caller sets rather than up or down. Filing
those under widening would train people to skim the widening list, which is
worse than not having one.

It reports rather than blocks. A widening is frequently correct, and failing
every one of them teaches people to route around the check — what it must never
be is invisible. `--fail-on-widening` is there for where you want the gate,
with an override a named human applies.

## The first agent: `support_assistant`

`proto/bank/agents/v1/support_assistant.proto` declares the bank's first
agent, `bank.agents.v1.SupportAssistant`, as a proto service beside the tools
it uses — the same review, the same `mise run diff-bank`, as any other tool
change.

Two annotations answer two different questions. `(garm.agent.v1.agent)` on
the service is what the agent **runs as**: the authority it requests, the
tools it may reach, its model alias and its bounds. `(garm.tool.v1.tool)` on
each RPC is what a **caller** needs in order to see and start it. A caller who
can see `support_assistant` in the catalogue is not thereby granted anything
the agent itself runs as — those are two separate principals folded together
at run time.

**What it is allowed to call.** The agent's `principal` is
`CLEARANCE_RESTRICTED` with the `financial` and `pii-contact` compartments —
the floor at which both of its tools are visible, and nothing wider. Its
allowlist, by catalogue FQN, is exactly two tools:

- `accounts.v1.get_customer`, with no guard.
- `payments.v1.initiate_payment`, guarded by
  `args.amount_minor_units <= 500000` — a ceiling *below* the tool's own
  limit (`1_000_000_000`), evaluated in the runner before the call leaves.
  The guard narrows and never widens: the tool's own validation, clearance
  and grant all still apply on top, so a run cannot propose more than
  5,000.00 in the account currency even though the tool itself would accept
  it, and any amount it does propose still needs the human grant
  `initiate_payment` already requires.

Lint rule A3 ("an agent may not list a tool it could never call") is why the
principal is `RESTRICTED` and not `CONFIDENTIAL`: `initiate_payment` declares
`min_clearance: CLEARANCE_RESTRICTED`, one grade above `CONFIDENTIAL`, and a
narrower principal would make the payment tool unreachable while still being
listed.

**The two RPCs, and A5.** `Invoke` (tool name `support_assistant`) starts a
run and returns a `RunRef` immediately; `GetRun` (tool name
`support_assistant_run`) reads its state and, once finished, its result. Both
declare identical `min_clearance: CLEARANCE_INTERNAL`, empty `compartments`,
and `sets: ["support"]` — so a caller who can start a run can read its result,
and no one else can. That equality is asserted in
`bank/agent_test.go`'s `TestInvokeAndGetRunAreVisibleToExactlyTheSameCallers`.
Lint rule A5 checks this on the CLI too (garm v0.14.1 and later; this repo
pins v0.20.0), so `mise run lint-bank` and `bank/agent_test.go` enforce the
same equality from two sides.

`Invoke` declares only `effects: { idempotent: false }` and leaves
`reversibility` and `external` unset. An irreversible external tool with no
approval mode is lint rule L16, and `MODE_NOTIFY` — the weakest mode that
would satisfy it — cannot mount on any deployment in this MVP. Starting a run
is not idempotent (two `Invoke`s are two runs) but leaves nothing external by
itself; the external, irreversible effect a run may cause is
`payments.v1.initiate_payment`'s, which declares and gates that on its own
account.

**Where the prompt lives and how it is pinned.** The system prompt is
`bank/prompts/support-assistant.md`, published verbatim — it is the product,
not documentation of one. The proto's `prompts["system"]` entry names its
path and pins a lowercase-hex SHA-256 of its bytes with no prefix. `garm
catalogue publish` uploads the file as `prompts/<sha256>.md`, and a runner
fetching that object refuses to start the agent if the prompt it fetched does
not hash to the value the catalogue declares. Nothing in this repository runs
the publish — `garm-ai/agentd`'s `mise run publish` does; CI here builds the
catalogue and checks it mounts.

That pin only protects anything while it is the hash of the file actually in
this tree, and editing a prompt is the change nobody thinks of as a code
change — so two things keep it honest:

- `mise run prompt-sha` recomputes the hash from
  `prompts/support-assistant.md` and rewrites the `sha256:` field in
  `proto/bank/agents/v1/support_assistant.proto` in place.
- `mise run prompt-sha-check` (part of `mise run ci`) reruns `prompt-sha` and
  fails if that rewrite changes anything committed — a stale pin is a red
  build, not a runner that refuses the agent at deploy time.
- `bank/agent_test.go`'s
  `TestTheDeclaredPromptHashIsTheHashOfThePromptInThisTree` reads the file the
  proto names, hashes it, and fails if that does not match the declared
  `sha256`, so the same drift is caught by `go test` even outside `mise run
  ci`.

**What mounting proves, and what it does not.** `garmd` mounts `Invoke` and
`GetRun` as two ordinary governed tools and never reads the
`(garm.agent.v1.agent)` annotation at all — it is agent-blind by design, so a
change to what an agent runs as can never affect what garmd itself decides to
serve. `mise run check-bank` builds the full twenty-seven-tool catalogue (the
eight tools the bank already had, `web.v1.fetch_page` adopted from
`garm-ai/tools`, the eight of `garm.tasks.v1` adopted from
`garm-ai/contracts`, plus `Invoke` and `GetRun` for each of the five agents)
and asserts only that adding the agents does not make the bank unmountable. It says nothing about whether an agent runner exists to
serve `SupportAssistant` — nothing in this repository implements it, and the
generated `SupportAssistantHandler` interface in
`bank/gen/bank/agents/v1/bankagentsv1_micro.pb.go` is deliberately
unimplemented here: `agentd` serves this service dynamically from the
catalogue, not from a Go binding compiled into the bank.

## Four more agents, and one adopted tool

Four agents beside the assistant, each reached by one persona and no
other, so a demo can show the same runner confining four different shapes
of authority. Each is a proto file under `proto/bank/agents/v1/`, a prompt
under `prompts/` pinned by hash, an `agents:` entry in `auth/claims.yaml` and
`auth/personas.yaml`, and a `can_invoke` plus a `can_run` tuple in
`auth/tuples.yaml`. `mise run prompt-sha` rewrites every agent's hash.

**`card-guardian`** (`CardGuardian`, owner `card-services`). jdoe may invoke
it. It calls `get_customer`, `list_cards` and `freeze_card`, the last one
guarded by `args.reason.size() >= 10` so a freeze always lands in the ledger
with a reason a person can act on. It **cannot unfreeze**: `unfreeze_card`
is not in its allowlist, so a card it froze is unfrozen by a person. Its
principal is `CONFIDENTIAL` with `card-data` and `pii-contact` — the floor
for its three tools and nothing wider — and its door (`Invoke`, `GetRun`)
carries the same labels in the `support` set, so the caller who can start it
already holds what the run needs.

**`concierge`** (`Concierge`, owner `retail-digital`). The customer
`customer:cust_ab12cd` (persona `ada`) may invoke it, for herself. It calls
`get_customer`, `get_balance` and `get_payment_status`, all reads. It
**cannot move money**, and not because the prompt says so: `initiate_payment`
needs `RESTRICTED` and `DESTRUCTIVE`, the agent's principal is the
`retail-customer` ceiling (`INTERNAL`, `financial`, `pii-contact`, `READ`),
and lint rule A3 refuses an agent that lists a tool it could never call. Its
door is `VERB_READ` — a customer's token holds no other verb — and declares
no sets, which is what makes it reachable by a customer's unscoped session
and by no staff catalogue.

**`compliance-screen`** (`ComplianceScreen`, owner `financial-crime`). priya
may invoke it. It calls `get_customer` to confirm which customer a screening
is about and `screen_party` once, with the party name exactly as given, and
reports `requires_review` and any matches exactly as returned. It **does not
decide outcomes** and holds nothing it could decide with. Its principal is
`CONFIDENTIAL` with `kyc` and `pii-contact`; the second compartment is why
`auth/claims.yaml` gains one minimal role, `compliance-contact` (`INTERNAL`,
`pii-contact`, `READ`, `compliance`), held by the agent and by priya — the
fold intersects, so an officer who could not identify a customer would have
an agent that could not either. Because the principal is `CONFIDENTIAL` and
the matches themselves read at `RESTRICTED`, a run sees `requires_review`
and not the names behind it; the prompt tells the model to report exactly
that rather than guess.

**`web.v1.fetch_page`, adopted.** The fourth agent needs a tool the bank did
not write, and adopting it is now two lines. `go get
github.com/garm-ai/tools/web@v0.2.0` in the module, and an entry in
`../catalogue.yaml`:

```yaml
  - module: github.com/garm-ai/tools/web
    version: v0.2.0
    packages: [web.v1]
```

`garm catalogue build` resolves that module with `go list -m`, reads `web.v1`
out of the module cache and composes it in: `web.v1.fetch_page` (`INTERNAL`,
compartment `internet`, set `research`, `VERB_READ`, `external: true`) plus the
two synthesised card endpoints. The package's taxonomy and the bank's own do not
clash — `internet` and `generated-artefacts`, `research` and `documents` are
names `bank/v1/taxonomy.proto` never declared, so there is nothing to merge and
nothing to rename (L29 refuses two *different* declarations of one name).

It used to be a copy: `proto/web/v1/web.proto` verbatim out of the module cache
with the `chmod -R u+w` a read-only cache makes necessary, plus an
`exclude_paths` line keeping its Go out of generation, plus nothing watching the
copy for drift. **The entry is better than the copy for one reason above all
others: `go.mod` decides the version.** A copy can be a tag behind what the tree
compiles against, and the artifact says nothing about it; a module entry cannot,
because the builder refuses a module the tree does not require and checks the
`version:` key against the module graph.

**The one warning this catalogue carries is still `O1` on `web.v1.WebService`**,
which names no owner. It is reported by `garm catalogue build` now rather than by
`garm lint` — the tree the linter would be pointed at no longer contains the
adopted package — and it is the package's to fix, not this tree's.

The service that answers `fetch_page` is not in this tree: it is the package's
own `webd`, run beside `bankd` by the deployment, with the host allowlist in its
policy file.

**A module this tree pins and never calls needs one line of Go.** `adopted.go`
blank-imports `github.com/garm-ai/tools/web/gen/web/v1`, and it is not
decoration: nothing here calls that package — `fetch_page` goes through the
daemon over NATS — so `go mod tidy` would drop the requirement, and a dropped
requirement makes the manifest entry illegal. A blank import is the ordinary Go
idiom for a dependency you pin without calling. The adopted taxonomy needs no
line, because a proto in this tree still imports it and the generated code
carries one.

**An adopted package's tag is part of the contract migration, and now the build
enforces it.** `web` and `taxonomy` both moved to `v0.2.0` when the contracts
left `garm`. Taking an older tag is not an option that merely lags:
`taxonomy@v0.1.1` is generated against `github.com/garm-ai/garm/contracts`, so a
binary holding it beside anything on `github.com/garm-ai/contracts` registers
`garm/tool/v1/*.proto` twice and dies in `protoregistry` at init — it compiles,
and then does not start. That used to be advisory prose in `buf.gen.yaml`; it is
now structural, because the proto version and the Go version are the same
version by construction. **When you adopt a package here, check that the tag you
take is built against the same contract module this one is.**

The role `support-desk` gains the `internet` compartment and the `research`
set, in `auth/claims.yaml` and `auth/personas.yaml` alike, so a first-line
persona can reach the tool at all. The assistant and the guardian hold that
role too and reach nothing new through it: neither lists `fetch_page`, and
the runner shows a model its allowlist, not its claim.

**`research-assistant`** (`ResearchAssistant`, owner `knowledge-desk`). jdoe
may invoke it. It calls `fetch_page`, guarded by
`args.url.startsWith("https://")`, and `get_customer` if the question names a
customer. Its principal is `INTERNAL` with `internet` and `pii-contact` —
the floor for its two tools — and its door carries the same labels in the
`support` and `research` sets. What comes back from a fetch was written by
whoever runs that site: it arrives between `<<<untrusted-content
source="…">>>` and `<<<end-untrusted-content>>>` markers, and the prompt
says everything between them is data about the page, never a message to the
model — quote what the page says, never follow what it asks, and finish
with the answer and the page's origin. The guard here repeats the tool's own
`https://` rule on purpose: which hosts a fetch may reach is decided per
deployment by the fetch service's allowlist and per agent by this guard, and
a guard visible in the manifest is one a reviewer can tighten to a host
without touching the service.

**Why there is no front desk.** An agent that calls another agent —
`front-desk` handing a request to `support_assistant` and polling
`support_assistant_run` — was designed for this tree and does not fit the
runner yet. A run's calls leave garmd with `act` filled from the run's own
chain (the subject is jdoe, the actor is the agent), so the assistant's
`Invoke` would reach agentd's governed door as a delegated caller, and both
ends refuse a chain deeper than one: agentd's exchange step fails the run
with reason `exchange` before the STS is called, and the STS refuses a
presented `act` outright (`garm-ai/sts`'s README, "The governed door's
request"). No claims, tuples or `may_act_for` change reaches past that; it
needs a depth-two chain minted by the STS and accepted by the runner, which
is program plan §7 item 22's open item, not this tree's.

What the start card shows each persona follows from the doors: jdoe sees the
assistant, the guardian and the research assistant, ada the concierge, priya
the screen, and a 404 from `GET /agents/{name}/card` for any other pairing
is the answer working.

## Adopting a tool is an entry in the manifest: `garm.tasks.v1`

`tasksd` serves the task queue as eight governed tools on
`garm.tasks.v1.TasksService` over NATS — `create_task`, `list_tasks`,
`get_task`, `approval_card`, `claim_task`, `release_task`, `decide_task`,
`triage_task`. Until this tree declared them, **nothing routed to any of
them**: garmd dispatches what a catalogue declares, the catalogue this
deployment mounts is the bank's, and the bank's proto tree said nothing about
`garm.tasks.v1`. A service serving eight subjects nobody can address.

The fix is adoption, and it is the same mechanism `web.v1.fetch_page` uses — an
entry in `../catalogue.yaml`:

```yaml
  - module: github.com/garm-ai/contracts
    version: v0.5.0
    packages: [garm.tasks.v1]
```

It used to be a copy of `garm/tasks/v1/tasks.proto` in `proto/garm/tasks/v1/`,
with `proto/garm` in `exclude_paths` so its Go was not generated twice, and a
byte-comparison gate watching it. All three are gone, because the builder reads
the package out of the module cache at the version `go.mod` resolves.

**The asymmetry worth understanding is still there, and it is now between a
COPY and an ENTRY rather than between two copies.** The annotations and this
file come out of *the same module* at *the same version* and are handled
differently:

| | how | why |
| --- | --- | --- |
| `garm/tool/v1`, `garm/agent/v1`, `garm/card/v1`, `garm/meta/v1` | **copied**, into `third_party/proto/garm/…` | They **define annotations**. This tree's own protos `import "garm/tool/v1/tool.proto"` and no toolchain resolves an import out of a Go module for you. They declare no tools, so they are not an input to the catalogue either, and the manifest cannot replace them. `mise run vendor-check` is the only thing watching them. |
| `garm/tasks/v1/tasks.proto` | **composed**, by a `catalogue.yaml` entry | It **declares eight tools**, so it is an input, and an input is exactly what the manifest names. Nothing imports it, so nothing needs it on disk. |

**The rule is what the file declares, not which module it came from.** Declares
tools → an input, which now means a manifest entry. Defines annotations only →
`third_party`, still vendored, still gated.

`mise run vendor-annotations` writes two destinations now rather than three, and
`mise run vendor-check` compares those two. **The gate went with the copy it was
watching**, which is the point: there is nothing left to drift when the builder
reads the bytes out of the module cache at the version `go.mod` resolved. The
concern the gate existed for — that a stale `garm/tasks/v1` would make this
catalogue advertise a shape of the task tools that the `tasksd` answering them no
longer has — is now structurally impossible rather than checked.

**The artifact says which version it got, which is new.** `Provenance` records
each resolved input, so `bank.binpb` now carries:

```
local  bank/proto  source=garm-ai/examples/bank
       packages=[accounts.v1 bank.agents.v1 bank.v1 cards.v1 payments.v1
                 screening.v1 tools.taxonomy.v1]
module github.com/garm-ai/contracts@v0.5.0   packages=[garm.tasks.v1]
module github.com/garm-ai/tools/web@v0.2.0   packages=[web.v1]
```

"Which version of the task contract is this bank running" is answerable from the
catalogue now. It was previously knowable only from a commit, which for the one
artefact a reviewer is asked to trust was a serious omission.

**One caveat, and it is the reason this tree requires `contracts v0.5.0` rather
than `v0.3.0`.** The compiler resolves a proto file from the Go registry before
the filesystem, deliberately — that is what makes the linked annotations
authoritative — so for a package the CLI itself **links**, the bytes compiled are
the CLI's and the version recorded is the tree's requirement. `garm v0.20.0`
links `contracts v0.5.0`. A tree requiring `v0.3.0` would therefore publish
`v0.5.0`'s descriptors under a provenance entry saying `v0.3.0`, and nothing
would warn: a bill of materials whose first line is wrong, in the feature whose
whole purpose is a truthful bill of materials. The two are held equal here by
hand until the CLI warns about the mismatch.

**What it costs the catalogue, and what it does not.** Eight tools and the
fourteen card endpoints synthesised for them, so the bank goes from nineteen
tools in six packages to **twenty-seven in seven**, and from twenty-nine
synthesised card endpoints to forty-three. The documented-field count does not
move (thirty-six): `tasks.proto`'s field policies are flat and `CLEARANCE_PUBLIC`
by message default, with the two `RESTRICTED` exceptions (`Task.answer`, the
grant on `DecideTaskRequest`) carrying no documentation string. Lint adds no
finding — the one `O1` warning is still `web.v1.WebService`'s missing owner and
nothing else. The compartment count is unchanged at seven, because the file
declares no compartment; it declares one tool set, `triage`.

**Moving to the manifest moved none of that.** Twenty-seven tools in seven
packages, twenty-four files, thirty-six documented fields, forty-three card
endpoints, seven compartments, six tool sets, one `O1` warning, and `garm
catalogue diff` reports no policy change across it. What moved is the digest,
and it had to: provenance gained its inputs and `Provenance.Producer` went from
`garm/v0.19.0` to `garm/v0.20.0`.

**Every method is `MODE_NONE` and `CLEARANCE_PUBLIC` at the method gate**, which
is what makes the catalogue mountable rather than a daemon that will not boot:
garmd refuses a catalogue declaring a mode it cannot enforce, and a task tool
asking for something this plane has no step for would stop `garmd` starting
rather than degrade. Which task a viewer may see is decided per task by the
label on its card, projected by the daemon, not per method — a method-level gate
would have to be the lowest predicate any tool in the deployment declares, which
is no gate at all.

**A persona sees a subset, and the tool set is what decides it.** At `contracts
v0.5.0` **seven** of the eight declare `sets: ["triage"]`: the five that always
did — `list_tasks`, `get_task`, `claim_task`, `release_task`, `triage_task` — and
`decide_task` and `approval_card`, which `v0.5.0` added. Only `create_task`
(`AUDIENCE_RUNNER`) declares no set. A token scoped to tool sets reaches only
tools in one of them, so a tool with no set is reachable by an **unscoped**
session and by no scoped one — the same rule that makes the concierge reachable
by a customer and by no staff catalogue.

**That is the one behaviour this tree's move to `contracts v0.5.0` changes, and
it is a fix rather than a widening anyone chose.** Declaring no set was the
original reading of `decide_task` and it was wrong: every role that names a set
was refused the method, which in this tree is every role but the customer's, so
`task-triage` bought sam and priya a queue they could look at and not decide
anything on. `sam` and `priya` now reach `decide_task` and `approval_card`;
`jdoe` and `amir` still get garmd's `not_found` for both, because neither holds
`triage`; `ada` is unchanged, because an unscoped session was never narrowed by a
set. Verified against the running plane rather than reasoned about.

Against this plane (`ListTools`, dev IdP personas):

| persona | roles | what it sees of `garm.tasks.v1` |
| --- | --- | --- |
| `ada` | `retail-customer` — `INTERNAL`, `READ`, **no `tool_sets`** | `list_tasks`, `get_task`. `approval_card` is reachable too but not listed: garmd keeps card endpoints out of `ListTools`. |
| `sam`, `priya` | `task-triage` — `PUBLIC`, `READ`+`WRITE`, `tool_sets: [triage]`, beside their own | `list_tasks`, `get_task`, `claim_task`, `release_task`, `triage_task`. `decide_task` and `approval_card` are reachable and not listed: both are `AUDIENCE_PERSON` card-and-decision endpoints garmd keeps out of `ListTools`. |
| `jdoe`, `amir` | staff roles scoped to `support` / `research` / `compliance`, none with `triage` | **nothing**. Every tool in the package but `create_task` is in `triage`, and `create_task` needs an unscoped token. |

Verbs do the rest of the narrowing: `ada` holds `READ`, so `claim_task`,
`release_task`, `triage_task`, `decide_task` and `create_task` answer garmd's
own `not_found` to her — the daemon does not admit a tool you cannot reach
exists.

**Reachability is proved, and the queue being empty is not the same thing as
nothing routing.** `POST /garm.tasks.v1.TasksService/ListTasks` as `ada`
answers `200 {"cursor":""}`, and `GetTask` for an id that does not exist answers
`404 tool_refused` — *the tool* found nothing, which is tasksd's own answer
arriving back over NATS through the daemon. Before adoption both were garmd's
`not_found`: a declared-nowhere subject. `DecideTask` as `sam` answers `400
invalid_argument` on an empty request and as `jdoe` answers garmd's `not_found`,
which is the set doing its work on either side.

## Owners and cards

Every service in this tree names who is answerable for it, and the payment
tool says how its approval should read. Both are annotations from garm
v0.15.0, vendored beside the others by `mise run vendor-annotations`
(`third_party/proto/garm/{card,meta}/v1/`; the pin is now `contracts v0.5.0`,
whose annotations are byte-identical to `v0.3.0`'s — only
`garm/tasks/v1/tasks.proto` moved), and neither is policy: `garmd` never reads them, its catalogue
version check ignores both namespaces, and `mise run check-bank` still
mounts all twenty-seven tools on the pinned daemon.
The agent runner reads them from the catalogue and puts them on every card
the inbox renders.

**`(garm.meta.v1.owner)` on every service.** `PaymentsService` is
`payments-platform` (contact `#payments-oncall`), `AccountsService` is
`retail-accounts`, `CardsService` is `card-services`, `ScreeningService` is
`financial-crime`, and the `SupportAssistant` agent is `agent-platform`. A
card for an open payment approval therefore names payments-platform, and the
run card that proposed it names agent-platform: the tool is one team's and
the run is another's. Lint rule O1 warns on any tool or agent service without
an owner; every service this tree writes has one. The one warning `mise run
lint-bank` prints is for `web.v1.WebService`, the package composed in from
`github.com/garm-ai/tools/web` (see "Four more agents"): its owner is the
package's to add, not this tree's to patch in.

**`(garm.card.v1.task_card)` on `InitiatePayment`.** The one declared template
in the bank. Its title is `Payment: {currency_code} {amount_minor_units} (minor units)`
and its body is three facts under human labels: `beneficiary_iban` as "To",
`amount_minor_units` as "Amount (minor units)", `currency_code` as
"Currency". Those three are exactly the tool's `material_fields`, and that is
not a coincidence: the task stores the material fields and nothing else of
the request, so lint rule C1 refuses a template that references anything
else. `reference` is free text the caller chose, deliberately not material,
and a template naming it fails `garm lint` with a message that says so. What
the approver reads is what the grant digest binds.

## Running it

```bash
mise run lint-bank         # the governance linter, over the whole composed catalogue
mise run gen-bank          # messages and tool bindings
mise run catalogue-bank    # the artifact a daemon loads
mise run prompt-sha        # rewrite an agent's prompt hash after editing its prompt
mise run prompt-sha-check  # fail if a committed prompt hash has drifted from its file
```
