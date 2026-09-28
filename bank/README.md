# bank

A proto tree shaped like a real bank's: four domains, eight tools, one shared
taxonomy, and a deliberate demonstration of what happens when you put all of
that in one catalogue.

It exists to answer a question the calculator cannot: **does this scale to
three hundred engineers?**

```
proto/
  bank/v1/          the taxonomy — which compartments and tool sets exist at all
  bank/agents/v1/   the support assistant — the bank's first agent
  accounts/v1/      balances and customer details
  cards/v1/         card state, in PCI scope
  payments/v1/      moving money
  screening/v1/     sanctions and PEP screening
```

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
allowlist rather than documentation of one.

Each domain imports it even though it references no symbol from it. That is
not tidiness — without the import the taxonomy is not in the domain's
dependency graph, and any tool working on a subset of files cannot see the
declarations. `buf generate` fails exactly that way. A domain that uses a
compartment name *depends on* the file declaring that name, and the import
graph should say so.

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

The catalogue is accounts-only, and that is not a convenience. `payments`
declares an approval gate and an audit stream and `garmd serve` has no flag to
supply either, so the full bank catalogue cannot be served by any garmd that
exists today. An accounts-only artifact is what a support cluster would run —
and building one here is what catalogue slicing would do if it existed.

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

## The refusal is asserted, in both directions

`initiate_payment` declares two independent forms of supervision — `MODE_GRANT`
and `LEVEL_AUDIT` with `fail_closed` and seven years of retention — and each is
independently sufficient to refuse an unsupervised deployment. `mise run
check-bank` builds the catalogue and runs `garmd check` against it three times,
and the two failing runs are the point:

1. a bare deployment (no grant verifier, no audit sink) — **must fail**.
2. a deployment with a grant verifier but **no** audit sink — **must also
   fail**, because a grant alone does not buy a durable record before the
   money moves.
3. a deployment with both, and an audit sink keeping at least the seven years
   `initiate_payment` declares — **must mount**.

Asserting only that the catalogue mounts somewhere would pass equally well if
one of the two refusals had quietly stopped working, and a refusal that has
stopped working is a payment tool served with no grant or no audit trail. Same
catalogue, three capability combinations, three outcomes — that is what makes
it a test rather than a formality.

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

## What this tree proves is still missing

**One catalogue is one blast radius.** Build this whole tree into a single
artifact and `payments` refuses the mount — so `accounts`, `cards` and
`screening` do not serve either. One team's annotation stops the bank. At
three hundred engineers and a hundred deploys a day, that is not a thought
experiment.

**And slicing does not work yet.** The obvious fix is a catalogue per domain,
but `garm catalogue build --proto proto/accounts` fails: the taxonomy lives in
`proto/bank` and a directory slice loses it, so every compartment reads as
undeclared.

The fix is to slice the **artifact** rather than the compile — build the whole
tree so imports and declarations always resolve, and emit only the named
packages. That flag does not exist. Until it does, a bank runs one catalogue
and accepts the blast radius, which is not an acceptable answer.

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
  £5,000.00 in the account currency even though the tool itself would accept
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
Lint rule A5, which checks this on the CLI, ships in garm v0.14.1; this repo
is pinned to v0.14.0, so today `mise run lint-bank` does not check it and the
equality above is enforced only by `bank/agent_test.go`, until the pin moves.

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
catalogue publish` — a CLI subcommand still in progress upstream, not present
in the pinned v0.14.0 `garm` this repo builds with — is designed to upload the
file as `prompts/<sha256>.md`; once it ships, a runner fetching that object is
meant to refuse to start the agent if the prompt it fetched does not hash to
the value the catalogue declares. Nothing here uploads to object storage yet.

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
serve. `mise run check-bank` builds the full ten-tool catalogue (the eight
tools the bank already had, plus `support_assistant` and
`support_assistant_run`) and asserts only that adding the agent does not make
the bank unmountable. It says nothing about whether an agent runner exists to
serve `SupportAssistant` — nothing in this repository implements it, and the
generated `SupportAssistantHandler` interface in
`bank/gen/bank/agents/v1/bankagentsv1_micro.pb.go` is deliberately
unimplemented here: `agentd` serves this service dynamically from the
catalogue, not from a Go binding compiled into the bank.

## Running it

```bash
mise run lint-bank         # the governance linter
mise run gen-bank          # messages and tool bindings
mise run catalogue-bank    # the artifact a daemon loads
mise run prompt-sha        # rewrite an agent's prompt hash after editing its prompt
mise run prompt-sha-check  # fail if a committed prompt hash has drifted from its file
```
