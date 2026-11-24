# Make-up booking formal verification

## Follow-up fix

Suggested make-up submissions now hold a shared lock on the absence settings row until commit. Policy writers lock that same row before merging updates. If a disable commits first, the submission reads the disabled setting and rolls back. If the booking takes the lock first, the disable waits for the booking to commit. The PostgreSQL regression `TestSuggestedSitInSerializesWithPolicyDisable` exercises both orderings for the staff and public endpoints. The report below preserves the model and implementation evidence from before this change. Run the historical counterexample probe against the source pinned by its manifest.

## Verification target

The five recent student and staff make-up booking fixes on the local application tree, plus batch publication and retained-key replay. The critical properties are deterministic rank selection, matching audience visibility, correct merge-rule precedence, institute-day boundaries, preserving per-target selections, and atomic publication after successful validation.

## Verdict

| Dimension | Status | Evidence boundary |
| --- | --- | --- |
| Formal | **PROVED (scoped)** | Lean checked 25 audited theorem statements about the policy, grouping and transaction models. The statement about eligibility at commit has a proved counterexample in the transaction model. |
| Correspondence | **REVIEWED_WITH_TESTS** | Go, SQL and TypeScript source review, 11 PostgreSQL test groups, and 509 frontend absence tests. The application sources have no mechanized refinement proof. |
| Production evidence | **PARTIAL** | Real PostgreSQL and local HTTP handlers with independent DB pools. Read-only Railway metadata showed an older build. Multi-process deployment, failover, live configuration and load behavior remain unverified. |
| Release decision | **CONDITIONAL** | Evidence supports the stated validation-time behavior under the documented assumptions. Commit-time eligibility is contradicted by a real execution. Production correctness is not established. |

## Reproduced counterexample

1. Start an ordinary admin make-up booking for an eligible session.
2. Pause the booking connection immediately before it sends COMMIT. This places the barrier after validation, after the record writes and after the idempotency response was prepared.
3. Through a second real HTTP handler and DB pool, turn off automatic make-ups. The change commits and returns HTTP 200.
4. Release the original booking. It returns HTTP 201 and its chosen make-up assignment is found in PostgreSQL.

The probe uses the actual booking and policy-update handlers, with a pgx query tracer only to control scheduling. It does not rely on sleeps. Repeat it on a fresh local database using the checked runner. See [probe source](evidence/policy-publication-probe.go.txt) and [recorded result](evidence/policy-publication.log). The execution is consistent with a policy effective at validation. If the business rule requires a policy effective at commit, booking and policy writers need a common consistency mechanism.

## Proof model and audit

- [Policy](proof/Policy.lean) models lexicographic rank selection, audience filtering, merge overrides and half-open institute-day intervals. The calendar theorem assumes an ordered midnight map and a containing-day relation. It allows unequal lengths between days.
- [Batch](proof/Batch.lean) models bucketing after each selected row has a resolved target and a canonical list of selected session IDs. It demonstrates that independent targets cannot join one bucket and that a shared merged-day choice can use the first alias.
- [Traces](proof/Traces.lean) models arbitrary natural-number attempt IDs and operation keys over arbitrary finite interleavings. Catalog updates, staging, rejection, abort, commit, lost acknowledgement and replay are separate transitions. Publication is one assumed atomic journal update. The invariant is initialized from an arbitrary catalog and preserved by each modeled step. The successful witness stages and commits two items and replays the retained response.

All proof modules passed Lake and direct Lean checking at trust level zero. The transitive axiom reports contain only `propext`, `Classical.choice` and `Quot.sound`, with no unchecked placeholders or native evaluation. The pinned toolchain does not package `leanchecker`. Direct checking uses the Lean kernel and is not an independent checker. [Build and axiom log](evidence/check.log), [direct check log](evidence/trust-zero.log), [independent checker status](evidence/leanchecker.log).

## Reproducibility

Base revision `69cccef7ff9bb980f9a01a2db63cabadc40ac6f0` plus local modifications. The [implementation SHA-256 manifest](evidence/implementation.sha256) pins the reviewed files, including the local correspondence test. The [proof SHA-256 manifest](evidence/proof.sha256) pins the formal source, runner and probe. Railway reported successful deployed source `e83d8a1a965d3bcd061571dfa2df5d0671e8ca2a`. [Deployment evidence](evidence/railway-deployment.txt).

Pinned Lean 4.19.0 at compiler commit `6caaee842e94`, Lake 5.0.0, no external Lean packages. Tests used Go 1.26.3, Node 26.0.0 and local PostgreSQL 14.23 with schema migration 133. [Version output](evidence/versions.log).

| Check | Recorded result |
| --- | --- |
| `python3 check.py` | Lake build and all 25 audited declarations pass. All six targeted model mutations cause the corresponding theorem to fail. |
| `lake env lean --trust=0 FILE` for each of the three proof modules | Exit 0. |
| `python3 check.py --go-probes` with both database URLs set to the dedicated local scratch database | Exit 0. It reproduced the publication race, then intentionally restored the old UTC/two-day range through a Go overlay. The public form regression rejected that mutant with HTTP 400 `invalid_sessions`. |
| Selected `go test -v ./internal/httpapi/absenceshttp` with a dedicated local DB | Exit 0. Eleven top-level groups passed. The focused resolver comparison used 128 seeded catalogs and 28,450 tested selections, with 2,526 accepted and 25,924 rejected. [PostgreSQL test log](evidence/postgresql.log). |
| `npm run test:absence` | Exit 0. All 509 tests across 31 files passed. [Frontend log](evidence/frontend.log). |

To repeat the checks from the repository root, create a dedicated local PostgreSQL database and run these commands. Use a new database for repeat runs with tests that share catalog settings.

```sh
export TEST_DATABASE_URL='postgres://localhost/warwick_booking_verify?sslmode=disable'
export SAT_POLICY_TEST_DATABASE_URL="$TEST_DATABASE_URL"
createdb warwick_booking_verify
(
  cd backend
  go test -v ./internal/httpapi/absenceshttp -run '^(TestFullChain_StudentCanSubmitSuggestedRankChainTarget|TestStaffCreate_AcceptsSuggestedSessionFromStaffLookup|TestPolicyAuditMergedCourseUsesRootPrioritiesUnlessOverridden|TestSatVerbalSubmitUsesInstituteDateForEarlyMissedClass|TestSatVerbalFullChain|TestFullChain_FinalSessionsKeepStudentValidation|TestStaffCreate_ExplicitSpecialSitIn|TestFormalCorrespondence|TestIdempotency_SameKeyAcrossReplicasOneRow|TestMergeMembership_ChangeSerializesWithSubmissionLock)' -count=1
)
npm run test:absence
(
  cd docs/verification/absence-booking-consistency
  python3 check.py --go-probes
)
```

## Assurance limits and next steps

The true premises and witnesses were checked, including a nonempty batch with a successful commit and replay. The model does not promise that every lookup succeeds at submission. It also does not prove exact Go/SQL semantics, runtime timezone data, production durability after failover, liveness, scheduler fairness, key-expiry behavior or capacity. The policy change counterexample and two per-item student-lock early-return omissions are recorded without changing application code during this verification.

- [Specification](SPEC.md)
- [Assumptions and their owners](ASSUMPTIONS.md)
- [Traceability and adversarial executions](TRACEABILITY.md)
- [Production test plan](PRODUCTION_TEST_PLAN.md)
- [Follow-up decisions](RECOMMENDATIONS.md)
