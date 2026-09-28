# auth/

`proto/` answers "what kind of data is this" — compiler-checked, resolved at
build time, and a change means a rebuild of `bank.binpb`. This directory
answers a different question, "who may do what," and it changes without one.
That is why `auth/` sits beside `proto/` as a sibling rather than underneath
it: the two are governed by different clocks, and putting them in the same
tree with different rules would hide that. A compartment typo is a build
failure. A segment gaining a role is a merged pull request, served the moment
the next token is minted.

Two kinds of file live here, and they are not interchangeable.

## Policy — reviewable, version-controlled, argued about in a pull request

- **`claims.yaml`** — the STS's claims policy. `roles:` bundles a clearance,
  a set of compartments, verbs and (optionally) one or more tool sets into a
  name; `segments:` says which roles a segment of customers or employees
  holds; `agents:` says what an agent may bring to a delegation on its own
  account. The compartments and tool sets named here are bank's own —
  `pii-contact`, `pii-identity`, `financial`, `kyc`, `card-data`, and the
  `support`, `payments` and `compliance` tool sets — exactly as
  `proto/bank/v1/taxonomy.proto` declares them, and no others. That agreement
  is the whole point of the file sitting here, and `garm claims check` is what
  keeps it true.

- **`personas.yaml`** — the same policy shape, read by the dev identity
  provider (`garmdev idp --personas auth/personas.yaml`) instead of a real
  IdP and a real OpenFGA store. Same role *names* as `claims.yaml` on
  purpose: `users:` and `agents:` here are the dev-mode stand-in for what
  `tuples.yaml` plus `claims.yaml` do together in a real deployment, so the
  two files are meant to be read side by side, not as independent designs
  that happen to overlap.

- **`model.fga`** — the OpenFGA authorization model: the platform types
  every garm deployment ships (`customer`, `employee`, `segment`, `agent`)
  plus bank's own instance type, `account`. `account.can_read` is `owner or
  servicer or handled_by from owner` — the last clause is why `handled_by`
  lives on `customer` at all: the employee assigned to a customer reads
  every one of that customer's accounts, present and future, from one tuple,
  never one per account.

## World data — not version-controlled in a real deployment

- **`tuples.yaml`** — seed and test data, stated on its own first line. In
  production, the application writes `can_invoke`, `handled_by` and
  `in_segment` facts itself as customers sign up, employees get assigned,
  and accounts open. Nothing here should be reconciled from a file into a
  live store: a GitOps pipeline doing that will, on an ordinary bad merge,
  revoke a live customer's access to their own records. This file exists
  only so the static-authorizer path — a laptop, a CI job — has something
  concrete to check `model.fga` against.

Put differently: if you would open a pull request to change it under normal
operation, it is policy and belongs above. If a customer support call or a
sign-up flow would change it, it is world data, and `tuples.yaml` is a
fixture standing in for a store, not a source of truth for one.

## The agent, and why jdoe gained a role

`claims.yaml`'s `agents:` block is the assistant's own authority, and it names
two roles because the assistant reaches two catalogues: `support-desk` for
`get_customer` and `payments-ops` for `initiate_payment`. It must stay at least
as wide as the `principal` block in
`proto/bank/agents/v1/support_assistant.proto`, or lint rule A3 refuses the
build — an agent may not list a tool it could never call.

`personas.yaml`'s `jdoe` gained `payments-ops` for a different reason, and the
distinction is worth keeping straight. Delegation **intersects**: garmd folds
the chain and takes the minimum of every identity in it, so an assistant acting
for jdoe can reach `initiate_payment` only if **both** sides can. Widening the
agent alone does nothing. An agent that could reach further than the human it
acts for would be the confused deputy the fold exists to prevent, so the
widening has to be on the human too, and it has to be visible — which is what
this paragraph is for.

What stops jdoe paying unsupervised is not this file. It is
`initiate_payment`'s `MODE_GRANT`, its `approver_min_clearance: RESTRICTED` and
`approver_compartments: [financial]`, and spec §3.5's rule that the run's own
subject is excluded from the approver predicate. jdoe satisfies that predicate
arithmetically and is refused anyway: that exclusion is the four eyes.

`tuples.yaml` gains `employee:jdoe in_segment payments-team` so the STS path
resolves the same authority the devkit path does — the two files are one policy
written for two readers — and a `can_run` relation, which is what STS exchange 2
checks before minting for a runner. `can_invoke` asks whether a human may reach
an agent; `can_run` asks whether the process presenting that human's assertion
is one this deployment deployed to run it.

## Verifying the vocabulary agrees

`proto/` declares compartments and tool sets; `claims.yaml` and
`personas.yaml` name them. Nothing enforces that they match except this:

```bash
garm claims check auth/claims.yaml   --against bank.binpb
garm claims check auth/personas.yaml --against bank.binpb
```

Both must pass. A name either file references that the catalogue does not
declare is the bug this gate exists to catch — a `finance`/`financial` typo
does not fail loudly on its own; a caller just silently loses access, and it
shows up as a ledger anomaly rather than an error. Run the check after any
edit to either file, or to `proto/bank/v1/taxonomy.proto`.

## What is deliberately not enforced here

`retail-customer` in `claims.yaml` is granted `financial` and `pii-contact`
at `INTERNAL`, with no per-account narrowing — that narrowing is
`account.can_read` in `model.fga`, a relation evaluated per record, and this
example does not wire anything up to call it. A `garmd` doing so would
confine a customer's token to their own accounts; one that is not still
accepts the token, so the clearance above is correct once that check runs and
reckless while it does not. That gap is named rather than hidden because a
worked example that quietly assumed enforcement it does not have would teach
the wrong lesson.
