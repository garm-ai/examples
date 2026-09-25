# bank

A proto tree shaped like a real bank's: four domains, eight tools, one shared
taxonomy, and a deliberate demonstration of what happens when you put all of
that in one catalogue.

It exists to answer a question the calculator cannot: **does this scale to
three hundred engineers?**

```
proto/
  bank/v1/        the taxonomy — which compartments and tool sets exist at all
  accounts/v1/    balances and customer details
  cards/v1/       card state, in PCI scope
  payments/v1/    moving money
  screening/v1/   sanctions and PEP screening
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

## The refusal is asserted, in both directions

`mise run check-bank` builds the catalogue and runs `garmd check` against it
twice — once on a bare deployment, once on one with grants and a seven-year
audit sink. **The bare run must fail.**

Asserting only that the catalogue mounts somewhere would pass equally well if
the refusal had quietly stopped working, and a refusal that has stopped
working is a payment tool served with no grant and no audit trail. Same
catalogue, opposite capability flags, opposite outcomes — that is what makes
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

## Running it

```bash
mise run lint-bank         # the governance linter
mise run gen-bank          # messages and tool bindings
mise run catalogue-bank    # the artifact a daemon loads
```
