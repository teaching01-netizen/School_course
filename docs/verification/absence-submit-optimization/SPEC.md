# Absence submit optimization: verification scope

Implementation baseline: `8706618843f67d24805b3281a6100728ef92276c` (2026-11-16).

The target is ordinary, policy-enforced physical sit-in selection at absence submission: fresh policy authorization, expected-class conflict rejection, the selected-target optimization, and the transaction protocol that persists an absence with its assignments and idempotency response. PostgreSQL is authoritative. Concurrent actors include independent HTTP requests, retries, staff reassignment/cancellation, and schedule/configuration writers. The model proves properties of a fixed policy/attendance snapshot and, separately, a per-student submission protocol with explicit lock/check/commit/abort/replay actions. Application correspondence and deployed behavior require separate evidence. Explicit special-approved staff overrides intentionally bypass policy validation and are excluded from AUTH/CONFLICT; the proof does not silently remove that product exception.

## Obligations

| ID | Guarantee | Kind |
| --- | --- | --- |
| AUTH | Restricting already-derived candidates to the declared course and its merged peers preserves authorization for every selected ID, with coherent course/group metadata. No unselected course becomes authorized. | Safety/refinement |
| CONFLICT | For any submitted candidate and distinct expected session with strict interval overlap, the selected-only blocker query followed by filtering rejects that candidate. Equality at an endpoint is allowed. | Safety |
| RETRY | During idempotency-record retention, arbitrary retries of the same actor/scope/key cannot produce a second committed operation in the protected protocol. Changed payloads must be rejected by the implementation. | Safety |
| ATOMIC | Aborts, deadlines, deadlocks, and process loss before commit do not publish staged domain effects; commit can succeed even when its acknowledgement is lost. | Atomicity/durability abstraction |
| SERIAL | Cooperating writers for one student cannot commit conflicting active requests, including different session IDs with overlapping times. Merged components within one request remain allowed by the internal compatibility predicate. | Safety |
| ERROR | Deadline, cancellation, and unexpected internal failures use fixed public messages and server status codes; internal details are not interpolated. | Safety/privacy |

## Exact boundaries

- Intervals are half-open instants: `startA < endB ∧ startB < endA`. Lean uses integer instants, not local-date arithmetic.
- AUTH receives the flattened top-level, priority, per-missed and nested candidate rows. The equality between actual resolver execution and this representation remains a refinement obligation; the model does not implement SAT Verbal rules or SQL.
- CONFLICT assumes the expected-session input is complete according to the current institute-timezone attendance predicate and effective enrollment. The SQL self-ID exclusion is explicit.
- RETRY identifies an operation by the existing actor/scope/key, with the fingerprint checked separately. A fresh key is a distinct operation. Current UI expiry is 24 hours and cleanup can delete expired records; no all-time exactly-once claim is made.
- SERIAL is per student. Different students can attend the same class. Schedule/configuration changes and uncooperative direct SQL writes are not silently treated as safe transitions: the corresponding production obligations are listed in ASSUMPTIONS.md.
- Init permits any safe existing history/active subset, not just an empty database. Production initial-state health is UNKNOWN.
- No theorem claims eventual request completion, notification delivery, linearizability of all APIs, PostgreSQL correctness, failover durability, a latency bound, or capacity under production traffic.

Formal, correspondence, and production statuses are reported separately in PROOF_REPORT.md.
