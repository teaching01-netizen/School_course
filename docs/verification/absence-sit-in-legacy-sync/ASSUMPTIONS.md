# Assumptions and owners

| Assumption / decision | Owner / code anchor | Evidence or remaining validation | Criticality |
|---|---|---|---|
| Interval endpoints are compared as instants; displayed dates use the configured institute timezone. | Absence service, `student_is_expected_at_session_tz`, `sessions_range_*` | Form and PostgreSQL tests cover overlap and date scope. Local-midnight/date-boundary coverage remains a release-test option. | REQ-SI-1 |
| The reported Oct 13 sessions are expected for the student and fall in the missed course/merge-group scope. | Existing read-only audit and `ValidSitInSessionOverlap` | A local integration fixture confirms sibling merge-group sessions are included. Historical source HTML is unavailable. | Reproducing the example |
| Static sit-in eligibility checks pass in the example; overlap is the modeled rejection reason. | `sat_verbal_policy.go` and resolver inputs | The Lean counterexample isolates overlap; unrelated policy failures are outside that model. | Proof boundary |
| A complete empty schedule requires the exact explicit `No schedules yet.` marker; only a successful client fetch followed by parser success reaches the applier. | `course_detail.go`, `syncer.go` | Parser and applier tests pass. Whether the live source can return a syntactically valid but incomplete page remains unverified. | REQ-LS-1 |
| Every worker allocates generations from the same PostgreSQL sequence before fetching. | Migration `00130`; `cmd/legacy-sync/syncer.go` | Local integration tests pass. Production deployment topology and all writer versions remain unknown. | REQ-LS-3 |
| Two consecutive complete omissions plus 24 hours from first omission is the chosen removal policy. | `apply/schedule.go`, migration `00130` | Implemented and tested locally; this is the policy chosen for this fix. | Deletion timing |
| Course and schedule apply writes are serialized by the existing per-course advisory transaction lock. | `CourseApplier.Apply`, `ScheduleApplier.Apply` | Full local applier package passes, including same-course concurrency coverage. Multi-instance production topology is not verified. | Atomic apply ordering |
| No other deployed writer bypasses the legacy appliers for legacy session deactivation. | Migrations, appliers, deployment | Repository call paths were updated; deployed binaries and any external writer still require audit. | Production safety |
| Schedule `113373` remains available in the source. | User's source check | Reconfirm from a fresh parsed response before targeted production refresh; no current response payload is retained here. | Repair evidence |

Unknown production worker count, source response body, replica topology, deployed schema/code version, and production write paths remain unknown; this report does not infer them.
