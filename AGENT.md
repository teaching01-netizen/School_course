# AGENT.md

## Mission

Treat this repository as a continually improving **Heuristic System**.
The agent “learns” by improving the system around the code: tests, checks, logs, scripts, docs, runbooks, fixtures/replays, and refactors.

## Default Development Loop (non-trivial work)

1. Observe (collect signals: failing tests, logs, screenshots, traces, user reports)
2. Reproduce (create a minimal repro: test, fixture, replay, or script)
3. Localize (identify owning module/layer and the violated invariant)
4. Patch (smallest safe change; avoid rewrites)
5. Verify (run relevant checks; expand coverage around the failure mode)
6. Remember (add at least one permanent artifact: regression test, doc, script, rule, fixture)
7. Compress (simplify if patches accumulate; refactor into an abstraction)

## Required “Memory Update”

After fixing a bug (or addressing a real failure mode), update at least one of:

- regression test (preferred)
- failure-case document
- diagnostic script
- agent rule (this file or `agent/*`)
- fixture/replay
- architecture note (ADR / design note)

If none are needed, state why in the PR notes.

## Compression Rule

If **3+ local patches** accumulate in the same area, stop adding conditionals:

- extract an abstraction (state machine / policy / shared helper)
- deduplicate tests/fixtures
- delete obsolete workarounds
- rewrite the doc/rule into one coherent source of truth

## Forbidden Behavior

Do not:

- patch without a reproducible case (unless explicitly time-critical and you record the gap)
- delete/disable checks to “make CI green”
- rewrite large modules unless the current design is proven unsafe
- trust UI-only state as proof of persisted data

## Repo Memory Locations

- `docs/failure-cases.md` (high-signal incident memory)
- `agent/` (rules, invariants, anti-forgetting checklist)
- `agent/production-first-systems-auditor-v2.md` (mission-critical production audit rubric)
- `scripts/diagnostics/` (support + debugging utilities)
- `docs/superpowers/` (design specs and plans)
