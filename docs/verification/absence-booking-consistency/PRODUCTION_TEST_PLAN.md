# Production-style verification plan

Use a staging environment with the deployed build and schema, disposable student records, notifications disabled, and production-equivalent pool settings. Record revision, migration version, schema constraints, isolation and configured timezone before testing.

| Schedule | Procedure | Oracle |
| --- | --- | --- |
| Five functional cases | Run P1–P5 through both relevant forms. Cover same-rank classes, hidden target, inherited priorities, the local midnight boundary and two make-up targets. | Choices and saved assignments match the documented audience and target policy. Check the source and make-up session IDs in the durable records. |
| Same-key concurrency and lost response | Release requests against multiple API instances from one barrier. Drop the first response after commit. Replay identical and changed fingerprints. | Identical replays return the retained bytes. Changed fingerprints are rejected. There is one batch of durable records for the retained key. Also measure lock waits and elapsed time. |
| Pre-commit abort | Pause after the first item, cancel the request or terminate the database connection. Include a failing second item variant. | No partial absence, missed-session or make-up assignment rows persist. A retry with the same key sees either a committed whole batch or a fresh operation. |
| Policy update during submit | Reproduce the instrumented pre-COMMIT schedule, then test schedule or visibility mutations independently. | Current policy-at-validation behavior allows the observed successful commit. If commit-time policy is required, an intervening committed change must force revalidation or fail the submission. |
| Failover and version skew | Interrupt the primary before and after commit in staging, then retry through another instance. Exercise old form assets against the new API before rollout. | Document recovery and response semantics, verify journal consistency and membership, and compare frontend payload compatibility. |
| Sustained workload | Use an agreed traffic profile and gradually increase concurrent students and retry rate. Run a soak with edits, batches and regular reads. | Record p50/p95/p99, timeouts, HTTP conflicts, lock waits, pool saturation, invariant failures and recovery. Throughput and capacity have no Lean guarantee. |

Local coverage includes the two-handler replay, real transactional rollback and the deterministic policy update race. Multi-process deployment, failover and capacity tests remain pending.
