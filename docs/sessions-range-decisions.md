# Decision Register - Sessions-in-Range Behavioral Conflicts (Step 2)

Status: PROPOSED (requires product/staff sign-off where marked). Each item has
the implementation direction from the plan plus the concrete contract impact.

## D1. Staff lifetime range vs 366-day cap - DECISION: keep cap, add explicit lifetime=true (IMPLEMENTED Step 17)

Direction: the plan asks to "support the authorized lifetime lookup through
bounded, student-relevant access". Verified current behavior: BOTH legacy and V2
reject 1970-01-01->2100-01-01 with 400 date_range_exceeded BEFORE any session
query (maxStaffSessionsRangeDays=366, routes.go:204; shadow error_parity locks
it in). Removing the cap is an explicit behavior change with unbounded-work
consequences (set-3 is now bounded by window+cutoff since Step 14, 2026-09-06:
loadBundleSessionsBounded + unrelated-history gate — the remaining unbounded
work for a lifetime lookup is the SET-1/2 display path, i.e. Step 17 scope).
DECISION: keep the 366-day staff cap; treat lifetime-lookup support as
a separate, explicitly scoped feature (Step 17) gated on Step 14. Do NOT claim
the "pathological staff query" SLO passes - it currently passes vacuously via
rejection, documented in docs/sessions-range-baseline.md.

Regression spec: keep lifetime_range + admin_over_cap in shadow error_parity.

Step-17 implementation (2026-09-06, branch deepswe-absence-consistency):
an explicit OPT-IN staff flag `lifetime=true` bypasses the cap; the unflagged
1970-2100 request still rejects on both paths (cap + parity preserved).
Semantics: admin + lifetime=true + explicit date_from/date_to only; the
cap-bypass lands on the lookup type (StaffSessionLookup.Lifetime /
StaffAllSubjectsLookup.Lifetime; StudentSessionLookup cannot represent it),
and the set-1/2 display predicate additionally admits
administratively-excluded sessions (manual session_attendance override),
which remain scope-relevant history. All other shapes fail closed:
bad_lifetime (false/empty/bogus value, missing range, non-admin use on the
staff endpoint), lifetime_not_allowed (any presence on the student endpoint,
even empty/false). Legacy path mirrors V2 (sessionsInRangeLifetimeSelectSQL
+ identical cap-bypass order). Gate: TestSessionsRangeV2_LifetimeStaffLookup
(500 relevant lifetime sessions, legacy-vs-V2 parity, identical trips before
/after +500 unrelated sessions: plain=2 batch=3) + TestStudentSessionsRejectsLifetime.

## D2. Legacy timezone defects - DECISION: fix both paths, break parity where the old answer was wrong

D2a. resolveDateRangeForSessionStartsInZone builds midnight in time.UTC instead
of the institute location (routes.go:1691). Both legacy and V2 call it, so both
are wrong by the zone offset for sit-in resolve windows. FIX both (Step 4);
existing test TestResolveDateRangeForSessionStartsUsesInstituteTimezone
compares only the calendar date in UTC rendering, which masks the bug -
extend it to assert the instant (Bangkok midnight = 17:00Z previous day).

D2b. student_is_expected_at_course_time hardcodes AT TIME ZONE 'Asia/Bangkok'
(00120/00121). FIX via new migration with tz-aware function (Step 5); keep a
compat wrapper while mixed-version replicas exist.

D2c. sessionDateKey UTC-slice fallback (routes.go:319-322) and V2 silent skip
(domain.go:34-67) disagree on corrupt timestamps. DECISION: controlled error
(see D6), applied to both paths.

## D3. Multi-day merged display ranges - DECISION: specify source-day grouping, then conform legacy

Verified: legacy mergedSessionRangesSQL partitions by SIBLING day and collapses
per-source via unordered map overwrite (routes.go:49-98) - nondeterministic
when one merge group spans several institute days in the window. V2 groups by
SOURCE session day deterministically (sessions_range_domain.go:120-125, marked
"documented hardening"). DECISION: source-day grouping is the specified
behavior (each session's range = min/max over same-group same-day siblings;
day = source session's institute day). Concrete example for the regression test
(Step 3.5): merge group G with session A Mon 09:00 and session B Tue 09:00 in
one window -> A shows Mon min/max, B shows Tue min/max (V2). Legacy currently
may show either. After decision, port V2 semantics back into the legacy SQL
(partition key change) so the shadow suite asserts the CORRECT answer, not
parity-with-bug.

## D4. Student subject_ids - DECISION: reject presence (contract change, documented)

Verified: student endpoint rejects wcode/bypass_timing/include_all_subjects
PRESENCE (self_service_routes.go:130-142) but parses-and-ignores subject_ids
(request.go:210-214 -> ignored in enrolled modes). The FE student client SENDS
subjectIds today (absenceFormApi.ts studentSessionsPath + test asserting
subject_ids on the student path). DECISION: reject subject_ids presence on the
student endpoint like the other staff-only keys (new code
subject_ids_not_allowed), because silent-ignore is a drift hazard. MIGRATION
REQUIRED: update FE studentSessionsPath/StudentSessionsOptions to stop sending
subject_ids, update absenceFormApi.test.ts studentSessionsPath expectations,
and announce the validation change (clients sending it get 400). Empty
occurrence (?subject_ids=) also rejected (presence check, consistent with the
other three keys).

## D5. sat_verbal_after_priority - DECISION: display input, never eligibility

Verified: accepted on ALL lookup types incl. StudentSessionLookup
(request.go:55-61,199-207); only feeds resolveMappedSatVerbalSitIn AfterPriority
filter (sessions_range_mapped.go:109, resolver.go:834) which selects WHICH
priorities are shown after a computed level. It does not alter enrollment,
visibility, timing, or limit outcomes. DECISION: keep accepting it on student
requests as a display cursor, with a regression test asserting identical
eligibility (sessions/already_absent/limits) for after_priority=0 vs N on the
same world. It must never gate eligibility - pin that property.

## D6. Corrupt timestamp behavior - DECISION: controlled 500 with stable code

sessions.start_at/end_at are NOT NULL timestamptz, so corrupt rows can only
arise from decode failure, not NULL. Legacy scan path fails the request via
ClassifyDBErr (500 internal/db_error); V2 skips the row and returns 200 with
fewer sessions (domain.go) while the service 500-guards only the pre-filter
count mismatch (service.go:74). DECISION: both paths return controlled 500
with a stable code on undecodable session timestamps (no silent row drop, no
UTC-slice fallback - remove routes.go:319-322 slicing). Add a fault-injection
regression test at the normalize/domain level (DB CHECK prevents inserting
truly corrupt timestamptz; test via unit-level malformed fact rows).

## D7. Sit-in exclusivity - DECISION: same-student/session, NOT global capacity

Verified constraints: UNIQUE(absence_id, session_id) on absence_sit_ins and
absence_missed_sessions - i.e. no duplicate assignment within one absence.
There is NO global UNIQUE(session_id): the attachments do not establish that
one physical session accepts only one student, so NO such constraint will be
added. The enforced rule (submission_helpers.go ensureSitInSessionsAvailable +
write paths under student FOR UPDATE lock): the SAME student may not hold the
same sit-in session on two active absences (cross-absence same-student
conflict -> 409 sit_in_session_already_used). Overlap-vs-missed (physical
non-overlap, ValidSitInSessionOverlap) is a separate per-submission validation.
Capacity across DIFFERENT students is unregulated by design. Regression tests
(Step 13): same-student/same-session race -> exactly one winner + deterministic
409; different-student/same-session -> both succeed (pins D7).
