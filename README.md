# examples

**Where to start, and the proof the boundaries hold.**

## What is here

| | |
|---|---|
| `banking/` | A worked catalogue — accounts, compliance, identity, payments — as standalone tool services. |
| `catalogue/` | How a catalogue is built, versioned, pinned and checked. |
| `deploy/` | Reference compose and Kubernetes. |

## Two jobs, and the second one matters more

The obvious one: this is what you fork to start. A working set of governed
tools, with the annotations already declared, so the first thing you write is a
handler body rather than a build.

The less obvious one: **this repository is the acceptance test for the whole
split.** It builds against published artifacts only — the annotations, a
pinned generator, a released runtime — with no path back into
[`garm`](../garm) and no replace directive.

If it builds, the boundaries are real. If it needs a shortcut, they are not,
and the shortcut is the bug.

## The demo is pointed at, not built in

A garm binary does not contain these tools. It loads a catalogue that declares
them, and that catalogue is built here. Adding a tool to this repository and
serving it takes a catalogue rebuild and a restart — no garm rebuild, no garm
release. That round trip is the thesis of the architecture, and this is where
it gets run for real.

## Status

Not yet seeded. Intent recorded; code arrives at Phase 6 of the split.
