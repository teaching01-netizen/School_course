# Recommendations and remaining decisions

The code changes below are implemented in the working tree and verified against the local PostgreSQL database. The remaining work is deployment/source verification, not an open implementation choice.

1. **Candidate conflict parity — implemented.** Candidate generation and resolver output now use expected sessions from the missed course or merge group over the inclusive local-date window. The form precheck includes attending sessions from the selected subject. Transactional final validation remains authoritative.
2. **Fail-closed schedule parsing — implemented.** A valid empty schedule requires the explicit “No schedules yet.” marker; unexpected blank or placeholder rows make the detail parse incomplete and prevent apply.
3. **Observation ordering — implemented.** Sync allocates a monotonic PostgreSQL sequence value before source fetch. Both appliers carry and compare that generation under the per-course advisory transaction lock.
4. **Missing confirmation — implemented.** A schedule needs two distinct complete missing observations, and at least 24 hours from first missing observation, before soft deletion. A source-present observation resets missing state and restores the existing session/mapping.
5. **Schedule `113373` repair — pending production evidence.** Confirm the row in a fresh parsed source response after deployment, then run the normal targeted refresh and verify session/mapping state. No production write was made during this task.

## Release checks that remain

- Confirm actual source page/auth/pagination behavior and the explicit empty marker against the live legacy service.
- Confirm all deployed sync workers share the migrated PostgreSQL sequence and no older writer can apply unguarded deletions.
- Run the targeted refresh for `113373` after rollout and verify `deleted_at`, mapping state, date/time, and duplicate count.
- Decide whether direct endpoint rollback and local-midnight boundary tests are required release gates; the helper-level and form behavior are covered locally.

## Formal result boundary

`AbsenceSitInLegacySync.lean` proves the named abstract properties without additional axioms. It does not prove the production pipeline. See [PROOF_REPORT.md](PROOF_REPORT.md) for commands, local test results, and limits.
