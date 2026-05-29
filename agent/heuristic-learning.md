# Heuristic Learning (Software Development)

The agent does not “learn” mainly by changing model weights.
It “learns” by improving the **software system around it**: tests, rules, logs, playbooks, checks, scripts, docs, failure memories, and refactors.

## Principle

Every bug/incident/edge-case should produce at least one permanent asset:

- a regression test
- a diagnostic script
- a failure-case note
- a fixture/replay
- an invariant/rule
- an ADR / architecture note

## Two modes

### Absorb (urgent fixes)

- add test/repro
- patch
- add logs/diagnostics
- record failure memory

### Compress (after several fixes)

- refactor into an abstraction
- dedupe tests/fixtures/rules
- delete obsolete workarounds
- update the source-of-truth docs

## Loop

Observe → Reproduce → Localize → Patch → Verify → Remember → Compress

