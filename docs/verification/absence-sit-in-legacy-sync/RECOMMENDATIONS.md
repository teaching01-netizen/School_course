# Recommendations and remaining decisions

The original reported-path changes and the eight fixes AP-1 through AP-8 are implemented in the current worktree. Focused local tests cover the repaired policy paths; see [POLICY_AUDIT.md](POLICY_AUDIT.md) and [PROOF_REPORT.md](PROOF_REPORT.md). Lean proves the abstract guards, not production code.

1. **Policy candidate and submission safety — implemented and locally verified.** The shared output filter covers all policy result shapes with authoritative attendance blockers; the form checks selected-subject attendance and selected make-up intervals; submission recomputes policy target eligibility and checks persisted interval conflicts. Generic final-target exclusions, scoped auto-resolution switches, and cross-section source exclusion are enforced at their resolution boundaries.
2. **Fail-closed schedule parsing — implemented.** A valid empty schedule requires the explicit “No schedules yet.” marker; unexpected blank or placeholder rows make the detail parse incomplete and prevent apply.
3. **Observation ordering — implemented.** Sync allocates a monotonic PostgreSQL sequence value before source fetch. Both appliers carry and compare that generation under the per-course advisory transaction lock.
4. **Missing confirmation — implemented.** A schedule needs two distinct complete missing observations, and at least 24 hours from first missing observation, before soft deletion. A source-present observation resets missing state and restores the existing session/mapping.
5. **Schedule `113373` repair — pending production evidence.** Confirm the row in a fresh parsed source response after deployment, then run the normal targeted refresh and verify session/mapping state. No production write was made during this task.

## Release checks that remain

- Exercise direct forged-submit rollback and concurrent different-session / multi-item batch overlaps.
- Decide whether the final missed class, the final target class, or both should be excluded.
- Confirm actual source page/auth/pagination behavior and the explicit empty marker against the live legacy service.
- Confirm all deployed sync workers share the migrated PostgreSQL sequence and no older writer can apply unguarded deletions.
- Run the targeted refresh for `113373` after rollout and verify `deleted_at`, mapping state, date/time, and duplicate count.
- Verify local-midnight boundaries in the authoritative picker query; form and half-open interval adjacency are covered locally.

## Formal result boundary

`AbsenceSitInLegacySync.lean` and `proof/PolicyAudit.lean` prove the named abstract properties. The generalized list-filter theorem depends on Lean's standard `propext` and `Quot.sound`; the new concrete guard theorems have no axioms. These proofs do not prove the application pipeline. See [PROOF_REPORT.md](PROOF_REPORT.md) for commands, local test results, and limits.
