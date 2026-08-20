import { AlertCircle, Info, X } from "lucide-react";
import { issueMessage } from "../../features/scheduleImpact/format";
import type { ImpactCandidate, ScheduleImpactIssue } from "../../features/scheduleImpact/types";

/* ------------------------------------------------------------------ */
/*  Helpers                                                           */
/* ------------------------------------------------------------------ */

function extractOriginalSnapshot(issue: ScheduleImpactIssue): Record<string, unknown> | null {
  const ctx = issue.assignment_context.original_session;
  if (ctx.quality === "unavailable") return null;
  return ctx.snapshot;
}

function extractCurrentSession(issue: ScheduleImpactIssue) {
  return issue.assignment_context.current_session;
}

function formatFieldTime(start: string | null, end: string | null): string {
  if (!start) return "Not set";
  const startDate = new Date(start);
  const timeStart = new Intl.DateTimeFormat("en-GB", {
    hour: "2-digit", minute: "2-digit", hour12: false, timeZone: "Asia/Bangkok",
  }).format(startDate);
  if (!end) return timeStart;
  const timeEnd = new Intl.DateTimeFormat("en-GB", {
    hour: "2-digit", minute: "2-digit", hour12: false, timeZone: "Asia/Bangkok",
  }).format(new Date(end));
  return `${timeStart}–${timeEnd}`;
}

function formatFieldDate(iso: string | null): string {
  if (!iso) return "";
  return new Intl.DateTimeFormat("en-GB", {
    weekday: "long", day: "numeric", month: "long", timeZone: "Asia/Bangkok",
  }).format(new Date(iso));
}

function formatDateShort(iso: string | null): string {
  if (!iso) return "";
  return new Intl.DateTimeFormat("en-GB", {
    day: "numeric", month: "short", timeZone: "Asia/Bangkok",
  }).format(new Date(iso));
}

/* ------------------------------------------------------------------ */
/*  Sub-sections                                                      */
/* ------------------------------------------------------------------ */

function SectionHeading({ id, children }: { id: string; children: React.ReactNode }) {
  return <h3 id={id} className="text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">{children}</h3>;
}

function LegacyStateBadge({ quality }: { quality: "exact" | "reconstructed" | "unavailable" }) {
  if (quality === "exact") return null;
  if (quality === "reconstructed") {
    return (
      <div className="mt-2 flex items-start gap-2 rounded-sm border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-900" role="note">
        <Info className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden="true" />
        <span>Original details reconstructed from schedule history</span>
      </div>
    );
  }
  return (
    <div className="mt-2 flex items-start gap-2 rounded-sm border border-[var(--color-wi-line)] bg-[var(--color-wi-row-alt)] px-3 py-2 text-xs text-[var(--color-wi-text-light)]" role="note">
      <Info className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden="true" />
      <span>Original assignment details unavailable. This arrangement was created before historical snapshots were recorded.</span>
    </div>
  );
}

function OriginalAssignment({ issue }: { issue: ScheduleImpactIssue }) {
  const original = issue.assignment_context.original_session;
  const snapshot = extractOriginalSnapshot(issue);
  const quality = original.quality;
  const assignedAt = issue.assignment_context.assigned_at;
  const sectionId = `resolution-original-${issue.id}`;

  const date = snapshot?.start_at
    ? formatFieldDate(snapshot.start_at as string)
    : formatFieldDate(null);
  const time = snapshot
    ? formatFieldTime((snapshot.start_at as string) ?? null, (snapshot.end_at as string) ?? null)
    : formatFieldTime(null, null);
  const room = (snapshot?.room_name as string) ?? (snapshot?.room as string) ?? "Not assigned";
  const teacher = (snapshot?.teacher_name as string) ?? (snapshot?.teacher as string) ?? "Not assigned";

  return (
    <section aria-labelledby={sectionId} className="rounded-sm border border-[var(--color-wi-line)] bg-[var(--color-wi-row-alt)] p-4">
      <SectionHeading id={sectionId}>Originally assigned</SectionHeading>
      {quality === "unavailable" ? (
        <LegacyStateBadge quality="unavailable" />
      ) : (
        <>
          <p className="mt-2 text-sm font-medium text-[var(--color-wi-text)]">{date}, {time}</p>
          <p className="mt-1 text-sm text-[var(--color-wi-text-light)]">{room}</p>
          <p className="text-sm text-[var(--color-wi-text-light)]">{teacher}</p>
          {assignedAt ? <p className="mt-2 text-xs text-[var(--color-wi-text-light)]">Captured when assigned on {formatDateShort(assignedAt)}</p> : null}
          {quality === "reconstructed" ? <LegacyStateBadge quality="reconstructed" /> : null}
        </>
      )}
    </section>
  );
}

function CurrentSession({ issue }: { issue: ScheduleImpactIssue }) {
  const current = extractCurrentSession(issue);
  const sectionId = `resolution-current-${issue.id}`;

  if (!current) {
    return (
      <section aria-labelledby={sectionId} className="rounded-sm border border-[var(--color-wi-line)] bg-white p-4">
        <SectionHeading id={sectionId}>Session now</SectionHeading>
        <div className="mt-2 flex items-start gap-2 text-sm text-[var(--color-wi-text-light)]">
          <X className="mt-0.5 h-4 w-4 shrink-0 text-red-500" aria-hidden="true" />
          <span>The original session has been deleted.</span>
        </div>
      </section>
    );
  }

  if (current.status === "deleted") {
    return (
      <section aria-labelledby={sectionId} className="rounded-sm border border-[var(--color-wi-line)] bg-white p-4">
        <SectionHeading id={sectionId}>Session now</SectionHeading>
        <div className="mt-2 flex items-start gap-2 text-sm text-[var(--color-wi-text-light)]">
          <X className="mt-0.5 h-4 w-4 shrink-0 text-red-500" aria-hidden="true" />
          <span>This session has been deleted from the schedule.</span>
        </div>
      </section>
    );
  }

  const date = formatFieldDate(current.start_at);
  const time = formatFieldTime(current.start_at, current.end_at);
  const room = current.room_name ?? "Not assigned";

  return (
    <section aria-labelledby={sectionId} className="rounded-sm border border-[var(--color-wi-line)] bg-white p-4">
      <SectionHeading id={sectionId}>Session now</SectionHeading>
      <p className="mt-2 text-sm font-medium text-[var(--color-wi-text)]">{date}, {time}</p>
      <p className="mt-1 text-sm text-[var(--color-wi-text-light)]">{room}</p>
      <p className="text-sm text-[var(--color-wi-text-light)]">{current.teacher_name}</p>
    </section>
  );
}

function ImpactExplanation({ issue }: { issue: ScheduleImpactIssue }) {
  const hasConflict = issue.impact_context.reasons.some(
    (r) => r.code === "regular_session_overlap" || r.code === "sit_in_overlap"
  );
  const isDeleted = !issue.assignment_context.current_session || issue.assignment_context.current_session.status === "deleted";

  if (isDeleted) {
    return <p className="text-sm text-[var(--color-wi-text-light)]">The assigned session has been deleted. The student needs a new arrangement.</p>;
  }
  if (hasConflict) {
    const overlapReason = issue.impact_context.reasons.find(
      (r) => r.code === "regular_session_overlap" || r.code === "sit_in_overlap"
    );
    return (
      <div className="space-y-2">
        <p className="text-sm font-medium text-[var(--color-wi-text)]">{overlapReason?.message ?? "This session now overlaps with the student's regular class."}</p>
        <p className="text-sm text-[var(--color-wi-text-light)]">The student cannot safely attend the current arrangement.</p>
      </div>
    );
  }
  if (issue.issue_type === "short_notice_change") {
    return <p className="text-sm text-[var(--color-wi-text-light)]">The student needs a clear update before the session begins.</p>;
  }
  if (issue.issue_type === "past_time_change") {
    return <p className="text-sm text-[var(--color-wi-text-light)]">The original arrangement can no longer be used.</p>;
  }
  return <p className="text-sm text-[var(--color-wi-text-light)]">{issueMessage(issue)}</p>;
}

function ResolutionActionSelector({
  issue,
  selectedCandidate: _selectedCandidate,
  onAction,
  busy,
  resolutionError,
}: {
  issue: ScheduleImpactIssue;
  selectedCandidate: ImpactCandidate | undefined;
  onAction: (action: "reassign" | "keep" | "cancel" | "mark_for_review") => void;
  busy: boolean;
  resolutionError: string | null;
}) {
  const policy = (issue.action_policy ?? []).filter(
    (a) => a.action === "reassign" || a.action === "keep" || a.action === "cancel" || a.action === "mark_for_review"
  );
  const actions = policy.length > 0 ? policy : [
    { action: "reassign" as const, allowed: true, reason_required: false, disabled_reason: null, notification_expected: true },
    { action: "keep" as const, allowed: true, reason_required: false, disabled_reason: null, notification_expected: true },
    { action: "cancel" as const, allowed: true, reason_required: false, disabled_reason: null, notification_expected: true },
    { action: "mark_for_review" as const, allowed: true, reason_required: true, disabled_reason: null, notification_expected: false },
  ];
  const actionLabels: Record<string, string> = {
    reassign: "Move to another session",
    keep: "Keep the current arrangement",
    cancel: "Cancel the sit-in",
    mark_for_review: "Ask another administrator to review",
  };
  return (
    <div className="space-y-3">
      {resolutionError ? (
        <div className="flex items-start gap-2 rounded-sm border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-800" role="alert">
          <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
          <span>{resolutionError}</span>
        </div>
      ) : null}
      <div className="space-y-2" role="radiogroup" aria-label="Resolution actions">
        {actions.filter((a) => a.allowed).map((ap) => (
          <label key={ap.action} className={`flex items-start gap-3 rounded-sm border p-3 text-sm ${ap.disabled_reason ? "cursor-not-allowed border-[var(--color-wi-line)] bg-[var(--color-wi-row-alt)] text-[var(--color-wi-text-light)]" : "cursor-pointer border-[var(--color-wi-line)] hover:border-[var(--color-wi-line)]"}`}>
            <input type="radio" name="resolution-action" className="mt-0.5" disabled={!!ap.disabled_reason || busy} onChange={() => onAction(ap.action as "reassign" | "keep" | "cancel" | "mark_for_review")} />
            <div>
              <span className="font-medium text-[var(--color-wi-text)]">{actionLabels[ap.action] ?? ap.action}</span>
              {ap.disabled_reason ? <p className="mt-0.5 text-xs text-[var(--color-wi-text-light)]">{ap.disabled_reason}</p> : null}
            </div>
          </label>
        ))}
        {actions.filter((a) => !a.allowed && a.disabled_reason).map((ap) => (
          <div key={ap.action} className="flex items-start gap-3 rounded-sm border border-[var(--color-wi-line)] bg-[var(--color-wi-row-alt)] p-3 text-sm text-[var(--color-wi-text-light)]">
            <input type="radio" name="resolution-action" className="mt-0.5" disabled />
            <div>
              <span className="font-medium">{actionLabels[ap.action] ?? ap.action}</span>
              <p className="mt-0.5 text-xs text-[var(--color-wi-text-light)]">{ap.disabled_reason}</p>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Main component                                                    */
/* ------------------------------------------------------------------ */

export interface ResolutionComparisonProps {
  issue: ScheduleImpactIssue;
  selectedCandidate: ImpactCandidate | undefined;
  onAction: (action: "reassign" | "keep" | "cancel" | "mark_for_review") => void;
  busy: boolean;
  resolutionError: string | null;
}

export default function ResolutionComparison({
  issue,
  selectedCandidate,
  onAction,
  busy,
  resolutionError,
}: ResolutionComparisonProps) {
  const headingId = `resolution-heading-${issue.id}`;

  return (
    <div className="space-y-4" role="region" aria-labelledby={headingId}>
      <h2 id={headingId} className="sr-only">
        Resolution for {issue.student_name ?? issue.wcode}
      </h2>

      {/* Section 1: What changed */}
      <section className="rounded-sm border border-[var(--color-wi-line)] bg-white p-4">
        <h3 className="text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">What changed</h3>
        <div className="mt-3 grid gap-3 sm:grid-cols-2">
          <OriginalAssignment issue={issue} />
          <CurrentSession issue={issue} />
        </div>

      </section>

      {/* Section 2: Why this needs attention */}
      <section className="rounded-sm border border-[var(--color-wi-line)] bg-white p-4">
        <h3 className="text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">Why this needs attention</h3>
        <ImpactExplanation issue={issue} />
      </section>

      {/* Section 3: What should happen */}
      <section className="rounded-sm border border-[var(--color-wi-line)] bg-white p-4">
        <h3 className="text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">What should happen?</h3>
        <ResolutionActionSelector
          issue={issue}
          selectedCandidate={selectedCandidate}
          onAction={onAction}
          busy={busy}
          resolutionError={resolutionError}
        />
      </section>
    </div>
  );
}
