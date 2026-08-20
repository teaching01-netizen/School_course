import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { AlertTriangle, ArrowLeft, CheckCircle2, ClipboardCheck, RefreshCw } from "lucide-react";
import { Link } from "react-router-dom";
import { apiJson } from "../api/client";
import { cachePolicies, queryKeys } from "../query/cache";
import Button from "../components/ui/Button";
import PageHeading from "../components/ui/PageHeading";
import { causeCopy, conflictTypeCopy, errorMessage, formatCount, formatTime, type LegacyAudit } from "./LegacyAudit.model";

export default function LegacyAudit() {
  const auditQuery = useQuery({
    queryKey: queryKeys.legacySync.audit,
    queryFn: () => apiJson<LegacyAudit>("/api/v1/admin/legacy-sync/audit"),
    ...cachePolicies.operational,
  });

  const audit = auditQuery.data;
  const totals = useMemo(
    () =>
      audit
        ? [
            { label: "Linked courses", value: audit.totals.linked_courses, detail: `${audit.totals.synced_courses} synced · ${audit.totals.archived_courses} archived`, tone: "text-[var(--color-wi-primary)]" },
            { label: "Legacy sessions", value: audit.totals.active_sessions, detail: `${audit.totals.soft_deleted_sessions} removed from source · ${audit.totals.legacy_sessions} ever`, tone: "text-[var(--color-wi-primary)]" },
            { label: "External series", value: audit.totals.external_series, detail: "Source-kind session series", tone: "text-[var(--color-wi-primary)]" },
            { label: "Students imported", value: audit.totals.students_imported, detail: "From legacy rosters", tone: "text-[var(--color-wi-primary)]" },
            { label: "Rooms mapped", value: audit.totals.mapped_rooms, detail: "Legacy room references", tone: "text-[var(--color-wi-primary)]" },
            { label: "Teachers mapped", value: audit.totals.mapped_teachers, detail: "Legacy teacher references", tone: "text-[var(--color-wi-primary)]" },
            { label: "Subjects mapped", value: audit.totals.mapped_subjects, detail: "Legacy subject references", tone: "text-[var(--color-wi-primary)]" },
          ]
        : [],
    [audit],
  );

  const runs = audit?.runs;
  const skips = audit?.skips;
  const skippedSessions = audit?.skipped_sessions ?? [];
  const skippedCourses = audit?.skipped_courses ?? [];
  const deadLetters = audit?.dead_letters ?? [];

  return (
    <div className="max-w-[1100px] space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <PageHeading>Legacy data audit</PageHeading>
          <p className="mt-2 max-w-2xl text-sm leading-6 text-[var(--color-wi-text-light)]">
            How much data came over from the old site, and which sessions and courses the sync skipped — recorded open conflicts, dead letters, and partial snapshots.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Link to="/admin/legacy-sync" className="inline-flex min-h-[34px] items-center rounded-sm border border-wi-line bg-white px-3 py-1.5 text-sm font-medium hover:bg-[var(--color-wi-row-alt)]">
            <ArrowLeft className="mr-2 h-4 w-4" aria-hidden="true" />
            Sync controls
          </Link>
          <Button variant="secondary" size="md" onClick={() => void auditQuery.refetch()} loading={auditQuery.isFetching}>
            <RefreshCw className="mr-2 h-4 w-4" aria-hidden="true" />
            Refresh
          </Button>
        </div>
      </div>

      {auditQuery.isError ? (
        <section className="border border-[var(--color-wi-red)]/30 bg-red-50 p-4" role="alert">
          <div className="flex items-start gap-3">
            <AlertTriangle className="mt-0.5 h-5 w-5 text-[var(--color-wi-red)]" aria-hidden="true" />
            <div>
              <p className="font-semibold text-[var(--color-wi-text)]">Audit data unavailable</p>
              <p className="mt-1 text-sm text-[var(--color-wi-text-light)]">{errorMessage(auditQuery.error)}</p>
              <Button className="mt-3" variant="secondary" size="sm" onClick={() => void auditQuery.refetch()}>Try again</Button>
            </div>
          </div>
        </section>
      ) : audit ? (
        <>
          <section className="border border-[var(--color-wi-border)] bg-white p-5" aria-labelledby="imported-heading">
            <div className="flex items-center justify-between">
              <h2 id="imported-heading" className="text-lg font-semibold text-[var(--color-wi-text)]">Imported from the old site</h2>
              <span className="text-xs text-[var(--color-wi-text-light)]">Snapshot {formatTime(audit.generated_at)}</span>
            </div>
            <div className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
              {totals.map((metric) => (
                <div key={metric.label} className="border border-wi-line bg-[var(--color-wi-row-alt)] px-4 py-3">
                  <p className="text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">{metric.label}</p>
                  <p className={`mt-1 text-2xl font-bold tabular-nums ${metric.tone}`}>{formatCount(metric.value)}</p>
                  <p className="mt-1 text-xs text-[var(--color-wi-text-light)]">{metric.detail}</p>
                </div>
              ))}
            </div>
            {runs ? (
              <dl className="mt-4 grid gap-x-6 gap-y-2 border-t border-wi-line pt-4 text-sm sm:grid-cols-2 lg:grid-cols-5">
                <div><dt className="text-[var(--color-wi-text-light)]">Completed runs</dt><dd className="font-medium tabular-nums text-[var(--color-wi-text)]">{runs.completed_runs}</dd></div>
                <div><dt className="text-[var(--color-wi-text-light)]">Entities parsed</dt><dd className="font-medium tabular-nums text-[var(--color-wi-text)]">{formatCount(runs.entities_parsed)}</dd></div>
                <div><dt className="text-[var(--color-wi-text-light)]">Entities applied</dt><dd className="font-medium tabular-nums text-[var(--color-wi-text)]">{formatCount(runs.entities_applied)}</dd></div>
                <div><dt className="text-[var(--color-wi-text-light)]">Parse failures</dt><dd className={`font-medium tabular-nums ${runs.parse_failures ? "text-[var(--color-wi-red)]" : "text-[var(--color-wi-text)]"}`}>{formatCount(runs.parse_failures)}</dd></div>
                <div><dt className="text-[var(--color-wi-text-light)]">Reconcile mismatches</dt><dd className={`font-medium tabular-nums ${runs.reconciliation_mismatches ? "text-[var(--color-wi-red)]" : "text-[var(--color-wi-text)]"}`}>{formatCount(runs.reconciliation_mismatches)}</dd></div>
              </dl>
            ) : null}
            {runs && runs.last_successful_at ? <p className="mt-2 text-xs text-[var(--color-wi-text-light)]">Last successful run {formatTime(runs.last_successful_at)}</p> : null}
          </section>

          {skips ? (
            <section className="border border-[var(--color-wi-border)] bg-white p-5" aria-labelledby="skips-heading">
              <h2 id="skips-heading" className="text-lg font-semibold text-[var(--color-wi-text)]">Skipped data</h2>
              <p className="mt-1 text-sm text-[var(--color-wi-text-light)]">
                Schedule rows the sync could not apply are recorded in the conflict ledger; a course whose refresh job exhausted its retries lands in dead letters; a course whose last apply left rows out carries a partial snapshot (still retryable).
              </p>
              <div className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
                <div className="border border-wi-line bg-[var(--color-wi-row-alt)] px-4 py-3">
                  <p className="text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">Skipped sessions</p>
                  <p className={`mt-1 text-2xl font-bold tabular-nums ${skips.sessions_skipped_open ? "text-[var(--color-wi-red)]" : "text-[var(--color-wi-green)]"}`}>{skips.sessions_skipped_total}</p>
                  <p className="mt-1 text-xs text-[var(--color-wi-text-light)]">{skips.sessions_skipped_open} open · recorded in conflicts</p>
                </div>
                <div className="border border-wi-line bg-[var(--color-wi-row-alt)] px-4 py-3">
                  <p className="text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">Skipped courses</p>
                  <p className={`mt-1 text-2xl font-bold tabular-nums ${skips.courses_skipped_open ? "text-[var(--color-wi-red)]" : "text-[var(--color-wi-green)]"}`}>{skips.courses_skipped_total}</p>
                  <p className="mt-1 text-xs text-[var(--color-wi-text-light)]">{skips.courses_skipped_open} open · conflicts + dead letters</p>
                </div>
                <div className="border border-wi-line bg-[var(--color-wi-row-alt)] px-4 py-3">
                  <p className="text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">Partial snapshots</p>
                  <p className={`mt-1 text-2xl font-bold tabular-nums ${skips.partial_snapshots ? "text-[var(--color-wi-amber)]" : "text-[var(--color-wi-green)]"}`}>{skips.partial_snapshots}</p>
                  <p className="mt-1 text-xs text-[var(--color-wi-text-light)]">Courses with rows not yet applied</p>
                </div>
                <div className="border border-wi-line bg-[var(--color-wi-row-alt)] px-4 py-3">
                  <p className="text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">Dead letters</p>
                  <p className={`mt-1 text-2xl font-bold tabular-nums ${deadLetters.length ? "text-[var(--color-wi-red)]" : "text-[var(--color-wi-green)]"}`}>{deadLetters.length}</p>
                  <p className="mt-1 text-xs text-[var(--color-wi-text-light)]">Latest shown below</p>
                </div>
              </div>
              {skips.by_cause.length ? (
                <div className="mt-5 overflow-x-auto">
                  <table className="min-w-full text-left text-sm">
                    <thead className="bg-[var(--color-wi-row-alt)] text-xs uppercase tracking-wide text-[var(--color-wi-text-light)]">
                      <tr>
                        <th className="px-4 py-2 font-semibold">Cause</th>
                        <th className="px-4 py-2 font-semibold">Entity</th>
                        <th className="px-4 py-2 font-semibold">Reason</th>
                        <th className="px-4 py-2 text-right font-semibold">Count</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-wi-line">
                      {skips.by_cause.map((bucket) => (
                        <tr key={`${bucket.cause}-${bucket.entity_type}-${bucket.key}`}>
                          <td className="px-4 py-2 font-medium text-[var(--color-wi-text)]">{causeCopy[bucket.cause]}</td>
                          <td className="px-4 py-2 text-[var(--color-wi-text-light)]">{bucket.entity_type || "—"}</td>
                          <td className="px-4 py-2 text-[var(--color-wi-text)]">{conflictTypeCopy(bucket.key) || "—"}</td>
                          <td className="px-4 py-2 text-right font-medium tabular-nums text-[var(--color-wi-text)]">{bucket.count}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : (
                <div className="mt-5 flex items-center gap-2 border border-wi-line bg-[var(--color-wi-row-alt)] px-4 py-3 text-sm text-[var(--color-wi-text-light)]">
                  <CheckCircle2 className="h-4 w-4 text-[var(--color-wi-green)]" aria-hidden="true" />
                  Nothing skipped — every course and session synced cleanly.
                </div>
              )}
            </section>
          ) : null}

          <div className="grid gap-6 lg:grid-cols-2">
            <section className="border border-[var(--color-wi-border)] bg-white" aria-labelledby="skipped-sessions-heading">
              <div className="flex items-center justify-between border-b border-wi-line px-5 py-4">
                <h2 id="skipped-sessions-heading" className="flex items-center gap-2 text-lg font-semibold text-[var(--color-wi-text)]">
                  <ClipboardCheck className="h-5 w-5 text-[var(--color-wi-text-light)]" aria-hidden="true" />
                  Skipped sessions
                </h2>
                <span className="text-xs text-[var(--color-wi-text-light)]">Latest {skippedSessions.length}</span>
              </div>
              <div className="overflow-x-auto">
                <table className="min-w-full text-left text-sm">
                  <thead className="bg-[var(--color-wi-row-alt)] text-xs uppercase tracking-wide text-[var(--color-wi-text-light)]">
                    <tr><th className="px-5 py-3 font-semibold">Course</th><th className="px-5 py-3 font-semibold">Schedule</th><th className="px-5 py-3 font-semibold">Reason</th><th className="px-5 py-3 font-semibold">Status</th></tr>
                  </thead>
                  <tbody className="divide-y divide-wi-line">
                    {skippedSessions.map((session) => (
                      <tr key={`${session.legacy_course_id}-${session.legacy_schedule_id}-${session.created_at}`}>
                        <td className="px-5 py-3">
                          <div className="font-medium text-[var(--color-wi-text)]">{session.course_name ?? session.course_code ?? "Unlinked course"}</div>
                          <div className="text-xs text-[var(--color-wi-text-light)]">legacy {session.legacy_course_id}{session.course_code ? ` · ${session.course_code}` : ""}</div>
                        </td>
                        <td className="px-5 py-3">
                          <div className="font-mono text-xs text-[var(--color-wi-text)]">{session.legacy_schedule_id}</div>
                          <div className="text-xs text-[var(--color-wi-text-light)]">{[session.date, session.begin, session.end].filter(Boolean).join(" ") || "No date recorded"}</div>
                        </td>
                        <td className="max-w-[220px] px-5 py-3">
                          <div className="text-[var(--color-wi-text)]">{conflictTypeCopy(session.conflict_type)}</div>
                          {session.message ? <div className="truncate text-xs text-[var(--color-wi-text-light)]" title={session.message}>{session.message}</div> : null}
                        </td>
                        <td className="px-5 py-3">
                          <span className={session.status === "open" ? "font-medium text-[var(--color-wi-red)]" : session.status === "resolved" ? "font-medium text-[var(--color-wi-green)]" : "text-[var(--color-wi-text-light)]"}>{session.status}</span>
                          <div className="text-xs text-[var(--color-wi-text-light)]">{formatTime(session.created_at)}</div>
                        </td>
                      </tr>
                    ))}
                    {skippedSessions.length === 0 ? <tr><td colSpan={4} className="px-5 py-8 text-center text-[var(--color-wi-text-light)]">No skipped sessions.</td></tr> : null}
                  </tbody>
                </table>
              </div>
            </section>

            <section className="border border-[var(--color-wi-border)] bg-white" aria-labelledby="skipped-courses-heading">
              <div className="flex items-center justify-between border-b border-wi-line px-5 py-4">
                <h2 id="skipped-courses-heading" className="flex items-center gap-2 text-lg font-semibold text-[var(--color-wi-text)]">
                  <ClipboardCheck className="h-5 w-5 text-[var(--color-wi-text-light)]" aria-hidden="true" />
                  Skipped courses
                </h2>
                <span className="text-xs text-[var(--color-wi-text-light)]">Latest {skippedCourses.length}</span>
              </div>
              <div className="overflow-x-auto">
                <table className="min-w-full text-left text-sm">
                  <thead className="bg-[var(--color-wi-row-alt)] text-xs uppercase tracking-wide text-[var(--color-wi-text-light)]">
                    <tr><th className="px-5 py-3 font-semibold">Course</th><th className="px-5 py-3 font-semibold">Reason</th><th className="px-5 py-3 font-semibold">Status</th></tr>
                  </thead>
                  <tbody className="divide-y divide-wi-line">
                    {skippedCourses.map((course) => (
                      <tr key={`${course.reason_kind}-${course.external_id}-${course.created_at}`}>
                        <td className="px-5 py-3">
                          <div className="font-medium text-[var(--color-wi-text)]">{course.course_name ?? course.course_code ?? "No local course"}</div>
                          <div className="text-xs text-[var(--color-wi-text-light)]">legacy {course.external_id}{course.course_code ? ` · ${course.course_code}` : ""}</div>
                        </td>
                        <td className="max-w-[220px] px-5 py-3">
                          <div className="text-[var(--color-wi-text)]">{conflictTypeCopy(course.conflict_type)}</div>
                          {course.message ? <div className="truncate text-xs text-[var(--color-wi-text-light)]" title={course.message}>{course.message}</div> : null}
                          {course.error_category ? <div className="text-xs text-[var(--color-wi-text-light)]">{course.error_category}</div> : null}
                        </td>
                        <td className="px-5 py-3">
                          <span className={course.status === "open" || course.status === "dead" ? "font-medium text-[var(--color-wi-red)]" : course.status === "resolved" ? "font-medium text-[var(--color-wi-green)]" : "text-[var(--color-wi-text-light)]"}>{course.status}</span>
                          <div className="text-xs text-[var(--color-wi-text-light)]">{formatTime(course.created_at)}</div>
                        </td>
                      </tr>
                    ))}
                    {skippedCourses.length === 0 ? <tr><td colSpan={3} className="px-5 py-8 text-center text-[var(--color-wi-text-light)]">No skipped courses.</td></tr> : null}
                  </tbody>
                </table>
              </div>
            </section>
          </div>

          <section className="border border-[var(--color-wi-border)] bg-white" aria-labelledby="dead-letters-heading">
            <div className="flex items-center justify-between border-b border-wi-line px-5 py-4">
              <h2 id="dead-letters-heading" className="text-lg font-semibold text-[var(--color-wi-text)]">Dead letters</h2>
              <span className="text-xs text-[var(--color-wi-text-light)]">Latest {deadLetters.length}</span>
            </div>
            <div className="overflow-x-auto">
              <table className="min-w-full text-left text-sm">
                <thead className="bg-[var(--color-wi-row-alt)] text-xs uppercase tracking-wide text-[var(--color-wi-text-light)]">
                  <tr><th className="px-5 py-3 font-semibold">Job</th><th className="px-5 py-3 font-semibold">Entity</th><th className="px-5 py-3 font-semibold">Error</th><th className="px-5 py-3 font-semibold">Attempts</th><th className="px-5 py-3 font-semibold">When</th></tr>
                </thead>
                <tbody className="divide-y divide-wi-line">
                  {deadLetters.map((letter) => (
                    <tr key={letter.id}>
                      <td className="px-5 py-3">
                        <div className="font-medium text-[var(--color-wi-text)]">{letter.job_type}</div>
                        <div className="text-xs text-[var(--color-wi-text-light)]">{letter.external_id ?? "—"}</div>
                      </td>
                      <td className="px-5 py-3 text-[var(--color-wi-text)]">{letter.entity_type ?? "—"}</td>
                      <td className="max-w-[280px] px-5 py-3">
                        <div className="truncate text-xs text-[var(--color-wi-red)]" title={letter.last_error}>{letter.last_error}</div>
                        {letter.error_category ? <div className="text-xs text-[var(--color-wi-text-light)]">{letter.error_category}</div> : null}
                      </td>
                      <td className="px-5 py-3 tabular-nums text-[var(--color-wi-text-light)]">{letter.attempts}</td>
                      <td className="px-5 py-3 text-xs text-[var(--color-wi-text-light)]">{formatTime(letter.created_at)}</td>
                    </tr>
                  ))}
                  {deadLetters.length === 0 ? <tr><td colSpan={5} className="px-5 py-8 text-center text-[var(--color-wi-text-light)]">No dead letters.</td></tr> : null}
                </tbody>
              </table>
            </div>
          </section>
        </>
      ) : (
        <div className="border border-wi-line bg-white p-8 text-sm text-[var(--color-wi-text-light)]">Loading audit data…</div>
      )}
    </div>
  );
}
