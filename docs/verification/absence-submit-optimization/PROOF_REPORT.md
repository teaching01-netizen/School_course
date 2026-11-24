# Absence submit optimization: verification report

## Verification target

Ordinary policy-enforced physical sit-in submission after the selected-target and selected-session optimizations. Baseline implementation: **`8706618843f67d24805b3281a6100728ef92276c`**. This engagement adds formal artifacts and correspondence tests; production application code is unchanged. The shared test fixture now creates a unique admin username so two handler instances can use the same fixture.

## Critical guarantees

Preserve selected-course authorization, reject strict expected-class overlap, keep staged writes invisible on abort, avoid conflicting active submissions and duplicate retained-key effects in the modeled lock protocol, and return fixed public error responses. See SPEC.md for exact quantifiers, exceptions and time/retention boundaries.

## Scope / production assumptions

The authorization model receives a fixed coherent catalog/attendance snapshot. The transaction model represents one student's state, arbitrary actor/key IDs and arbitrary finite interleavings, with explicit acquire, validate, commit, abort, cancel, replay, ACK and wait/lost-ACK actions. A PostgreSQL commit is abstracted as atomic only under A1/A10. An authenticated operation key means the complete actor/scope/key tuple after fingerprint checks, while its idempotency record is retained.

The Go, SQL, PostgreSQL engine, configuration mutation paths, network, storage and deployment are **not mechanically verified by these Lean proofs**. Production topology and deployed SHA were not read. Special-approved staff policy overrides are intentionally excluded from the ordinary authorization theorem. Whole-production correctness is **NOT ESTABLISHED**.

## Verdict

| Dimension | Status | Exact meaning |
| --- | --- | --- |
| Formal status | **PROVED** | All 23 theorem statements in this pinned model passed Lean checking and the transitive axiom audit. |
| Correspondence status | **REVIEWED_WITH_TESTS** | Source review, seeded differential execution, real PostgreSQL regressions and local HTTP/transaction fault tests support the mapping. No Go/SQL semantics-preserving translation is proved. |
| Production evidence | **PARTIAL** | Local PostgreSQL and two in-process HTTP handlers with independent pools were exercised. Actual multi-process deployment, failover, production initial state, changing configuration and sustained-load behavior remain unverified. |
| Delivery verdict | **CONDITIONAL** | The static coherent snapshot/cooperating-writer envelope is supported. Extending this to production or commit-time eligibility requires the critical obligations in ASSUMPTIONS.md and RECOMMENDATIONS.md. |

## Pinned tooling and source

- Lean **4.19.0**, `arm64-apple-darwin23.6.0`, commit **`6caaee842e94`**; Lake **5.0.0-6caaee8**.
- `lean-toolchain`, `lakefile.lean`, `lake-manifest.json` and imports are retained. Core Lean/Std only, **no external Lake packages**.
- Go **1.26.3 darwin/arm64** (the repository's go.mod requires at least 1.25.7).
- PostgreSQL **14.23**, READ COMMITTED, `synchronous_commit=on`, `fsync=on`; scratch schema version **133**. No production connection was used.
- `evidence/implementation-and-environment.json` pins the implementation SHA, migrations Git tree and SHA-256 hashes of inspected implementation/test files.
- `evidence/proof-audit.json` pins proof sources, the build configuration, checker script, exact versions, theorem count and the used/allowed axiom sets. The proof source hashes describe this working tree, not an invented committed proof revision.

## Formal results

All statements and axiom lists are retained verbatim in `evidence/theorems-and-axioms.log`.

| Obligation | Checked theorems | Limit |
| --- | --- | --- |
| AUTH | `focus_preserves_authorization`, `focused_rows_are_in_scope` | Arbitrary flattened candidate lists and selected IDs, including nonempty selection and target-presence requirements. Does not prove earlier Go rule execution/greedy dedup or SQL snapshot coherence. |
| CONFLICT | `selected_query_includes_every_conflict`, `selected_conflict_is_rejected`, `overlap_symmetric`, `adjacent_not_overlapping` | Completeness of expected attendance is an input assumption. The SQL distinct-ID condition is modeled. No theorem equates the selected-only query with every incidental blocker produced by unrelated discovery candidates. |
| Protocol initiation/preservation | `init_invariant`, `step_preserves`, `all_reachable_safe` | General induction over all actions and finite reachable traces from any permitted initial state; does not assert fairness or eventual completion. |
| Business safety | `retained_operations_not_duplicated`, `active_requests_do_not_conflict`, `committed_request_was_authorized` | Cooperating per-student lock protocol, retained keys, and stable certified facts; initial active rows must already be compatible. |
| Publication/ACK semantics | `validate_publishes_no_effect`, `abort_publishes_no_staged_effect`, `replay_does_not_create_effect`, `acknowledgement_is_not_commit` | Atomic/durable DB abstraction. Actual process kill and failover were not exercised. |
| Public error boundary | `response_independent_of_private_details`, `deadline_is_server_timeout` | Fixed response function for classified errors; the Go wrapped-error classifier is reviewed/tested rather than formally translated. |
| Non-vacuity / hostile examples | `a_commit_is_reachable`, `authorization_is_nonvacuous`, `naive_read_check_write_counterexample`, `fresh_key_is_not_same_operation`, `expiry_ends_deduplication_guarantee` | Concrete kernel-checked witnesses complement the general theorems. They demonstrate real allowed commits, permitted/rejected selections, a stale two-reader race, and the limits of key identity/retention. |

### Axiom audit

The transitive dependencies use **only `propext` and `Quot.sound`**, Lean's standard propositional extensionality and quotient-soundness axioms. Both are explicitly permitted. `Classical.choice` is permitted by the script's standard-foundations allowlist but is not used by these theorems. There is **no `sorryAx`, custom axiom, `sorry`, `admit`, `unsafe` proof or `native_decide` closure** in the checked sources/dependency audit. `by decide` examples are ordinary kernel-checkable proof terms, not native-evaluation assurances.

`python3 check.py` rebuilds every proof module, prints/checks each theorem statement, requires a matching transitive axiom listing for every theorem, rejects proof placeholders/native shortcuts, and fails on any axiom outside the allowlist. `lake build` explicitly includes the proof roots and audit module; the report does not rely on uncompiled files.

Independent `leanchecker` was attempted but this pinned distribution lacks that optional binary. Its exact error is retained in `evidence/leanchecker-unavailable.log`. Independent rechecking is **NOT_RUN**; ordinary Lean elaboration/kernel checking and the axiom audit succeeded. No optional-checker success is claimed.

## Executed commands and results

From this directory:

```text
python3 check.py
PROVED: 23 model theorems; transitive axiom audit passed: ['Quot.sound', 'propext']
Implementation correspondence and production correctness require separate evidence.
exit 0
```

The script's exact command arguments, exit codes and output filenames are in `evidence/lean-commands.json`. It runs `lake build`, `lake env lean proof/Audit.lean`, and version checks. Both proof build and theorem/axiom audit exited **0**.

Create a disposable local database with `createdb warwick_absence_formal_20261116`, then run the following from backend/. These tests migrate the schema and replace SAT policy fixtures; never point them at an existing application/production database.

```sh
TEST_DATABASE_URL='postgres://rd-cream@localhost:5432/warwick_absence_formal_20261116?sslmode=disable' \
SAT_POLICY_TEST_DATABASE_URL='postgres://rd-cream@localhost:5432/warwick_absence_formal_20261116?sslmode=disable' \
go test -race ./internal/httpapi/absenceshttp \
  -run 'TestFormalCorrespondence|TestSitInSubmitValidation|TestSatVerbal|TestFullChain|TestPolicyAudit|TestResolveMappedSatVerbal|TestWriteSitInValidationError|TestSubmission_|TestFailure_|TestIdempotency_SameKeyAcrossReplicasOneRow|TestPublicBatchReplaysSameKeyWithoutDuplicatingAbsence|TestBatch_OppositeOrderNoDeadlock|TestMergeMembership_ChangeSerializesWithSubmissionLock' \
  -count=1 -v
```

**Exit 0:** 39 top-level tests plus their subtests passed; no race-detector finding. See `evidence/postgres-race-tests.log` and `evidence/test-summary.json`.

- Seed `PCG(42, 8706618)`: **128 coherent catalogs, 28,450 single/paired selections**, with 2,526 accepted and 25,924 rejected; actual focused and full resolver decisions agreed. Both positive and negative cases are required by the test. This is **bounded empirical correspondence**, not an unbounded Go equivalence proof.
- Injecting `io.ErrClosedPipe` when writing the first committed physical-submit response, followed by **16 simultaneous replays through two handlers and independent pools**, left **one absence, one sit-in, one missed-session row** and byte-identical replay responses. These handlers run in one process, not a claimed production replica topology.
- Existing barriers exercised same-student/same-session contention, submit versus reassignment, cancellation, different students sharing a session, opposite-order batches, membership changes, and same-key handlers.
- Failure injection covered partial SQL work, rollback before commit and idempotency acquisition rollback. Wrapped validation timeouts/internal errors and policy/final-class/merged-member constraints passed.

Other checks:

```text
go test ./...            exit 0 (some unchanged packages cached; DB integration tests
                                without their environment variables are skipped)
go vet ./internal/httpapi/absenceshttp    exit 0
git diff --check         exit 0
```

The separately configured PostgreSQL command above is the integration evidence; the all-backend invocation is not represented as an all-database-suite run. The local scratch database was used only for verification and is disposable.

## Hostile executions and correspondence gaps

1. **Two stale readers:** the checked counterexample has two requests pass against empty active state, then form conflicting active state without a common lock/fresh check. The safe model represents read and commit separately; all write-changing steps must own the same student lock. Go barrier tests support that protocol for the inspected callers, not every possible SQL writer.
2. **Commit / lost ACK / retry:** the model permits a nonempty committed history with no acknowledgement. The injected write-failure test exercises the real HTTP/DB order. Safety does not imply the client receives a response or a revoked student session can retry without renewed authentication.
3. **Different keys or expired retention:** these are outside retained-key at-most-once. `expiry_ends_deduplication_guarantee` explicitly checks two effects after forgetting a completed key; no perpetual exactly-once claim is hidden behind the retry theorem.
4. **Changed schedules/config:** a validation certificate is stable in the model. Policy/settings changes, roster mutation, clock/date conversion and selected-session edits are not proven to preserve that certificate in the application. In particular, staff validation precedes a later session fence and versions are optional. This is an open refinement obligation, not a confirmed bug reproduction or a proof of commit-time eligibility.
5. **Cancellation:** removing active requests preserves the modeled invariant; real status cancellation uses its own absence/version/snapshot locks. Existing race tests support the inspected behavior; a complete linearizability proof of cancellation/reassignment APIs is not supplied.
6. **Crash/deadlock/connection loss:** abort transitions preserve committed state and release transient state. Rollback tests exercise local failure cases. Process death, storage loss, failover, queues, partitions and mixed deployments remain untested.
7. **External effects and overload:** after-commit SMS/email, sustained throughput, tail latency and retry storms are outside the model. No notification-delivery or performance guarantee is proved.

The detailed code/DB mapping is TRACEABILITY.md. Every unmapped transition/fact remains a refinement obligation.

## Anti-vacuity review

- Init includes arbitrary safe retained histories and active subsets. Production health is an explicit unresolved assumption; malformed initial data was not assumed away as “verified.”
- A real nonempty commit is reachable; positive and negative authorization witnesses are checked. The invariant is established from Init and preserved per transition, not asserted as a theorem premise without a preservation proof.
- Actor IDs, request keys, candidate lists and finite retry/interleaving lengths are arbitrary in the relevant theorems. The Go randomized and barrier runs remain explicitly bounded tests.
- Validation/read and commit are separate. Atomic commit and lock exclusion have named DB owners and unresolved deployment/refinement conditions.
- Commit and acknowledgement are separate; notification delivery and liveness are not inferred from safety.
- Standard foundational axioms are listed; no custom or placeholder axiom closes the proof. All proof modules are built and hashed.
- There is no verified Go/SQL translation, whole-API linearizability proof, production topology validation, failover test or load/soak claim. These limits prevent a misleading end-to-end “verified production” label.

See RECOMMENDATIONS.md for the smallest concrete follow-up changes and PRODUCTION_TEST_PLAN.md for falsifiable production checks. No deployment or additional runtime optimization was performed.
