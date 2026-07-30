import { AlertCircle, Info, ArrowRight, X } from "lucide-react";
import Button from "../ui/Button";
import { issueConsequence, issueMessage } from "../../features/scheduleImpact/format";
import type { ImpactCandidate, ScheduleImpactIssue } from "../../features/scheduleImpact/types";

/* ------------------------------------------------------------------ */
/*  Helpers                                                           */
/* ------------------------------------------------------------------ */

type FieldKey = "time" | "room" | "teacher" | "course";

interface ChangedField {
  field: FieldKey;
  label: string;
  from: string;
  to: string;
}

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

function extractChanges(issue: ScheduleImpactIssue): ChangedField[] {
  const before = issue.change_context.before;
  const after = issue.change_context.after;
  if (!before || !after) return [];
  const changes: ChangedField[] = [];
  const timeStartBefore = (before.start_at as string) ?? (before.old_start_at as string) ?? issue.details.old_start_at;
  const timeEndBefore = (before.end_at as string) ?? (before.old_end_at as string) ?? null;
  const timeStartAfter = (after.start_at as string) ?? (after.new_start_at as string) ?? issue.details.new_start_at;
  const timeEndAfter = (after.end_at as string) ?? (after.new_end_at as string) ?? null;
  const oldTime = formatFieldTime(timeStartBefore, timeEndBefore);
  const newTime = formatFieldTime(timeStartAfter, timeEndAfter);
  if (oldTime !== newTime && (timeStartBefore || timeStartAfter)) {
    changes.push({ field: "time", label: "Time", from: oldTime, to: newTime });
  }
  const oldRoom = (before.room_name as string) ?? (before.room as string) ?? null;
  const newRoom = (after.room_name as string) ?? (after.room as string) ?? null;
  if (oldRoom !== newRoom && (oldRoom || newRoom)) {
    changes.push({ field: "room", label: "Room", from: oldRoom ?? "Not assigned", to: newRoom ?? "Not assigned" });
  }
  const oldTeacher = (before.teacher_name as string) ?? (before.teacher as string) ?? null;
  const newTeacher = (after.teacher_name as string) ?? (after.teacher as string) ?? null;
  if (oldTeacher !== newTeacher && (oldTeacher || newTeacher)) {
    changes.push({ field: "teacher", label: "Teacher", from: oldTeacher ?? "Not assigned", to: newTeacher ?? "Not assigned" });
  }
  const oldCourse = (before.course_code as string) ?? null;
  const newCourse = (after.course_code as string) ?? null;
  if (oldCourse !== newCourse && (oldCourse || newCourse)) {
    changes.push({ field: "course", label: "Course", from: oldCourse ?? "Unknown", to: newCourse ?? "Unknown" });
  }
  return changes;
}

/* ------------------------------------------------------------------ */
/*  Sub-sections                                                      */
/* ------------------------------------------------------------------ */

function SectionHeading({ id, children }: { id: string; children: React.ReactNode }) {
  return <h3 id={id} className="text-xs font-semibold uppercase tracking-wide text-gray-500">{children}</h3>;
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
    <div className="mt-2 flex items-start gap-2 rounded-sm border border-gray-200 bg-gray-50 px-3 py-2 text-xs text-gray-600" role="note">
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
    : formatFieldDate(issue.start_at);
  const time = snapshot
    ? formatFieldTime((snapshot.start_at as string) ?? null, (snapshot.end_at as string) ?? null)
    : formatFieldTime(issue.start_at, issue.end_at);
  const room = (snapshot?.room_name as string) ?? (snapshot?.room as string) ?? "Not assigned";
  const teacher = (snapshot?.teacher_name as string) ?? (snapshot?.teacher as string) ?? "Not assigned";

  return (
    <section aria-labelledby={sectionId} className="rounded-sm border border-gray-200 bg-gray-50 p-4">
      <SectionHeading id={sectionId}>Originally assigned</SectionHeading>
      {quality === "unavailable" ? (
        <LegacyStateBadge quality="unavailable" />
      ) : (
        <>
          <p className="mt-2 text-sm font-medium text-gray-900">{date}, {time}</p>
          <p className="mt-1 text-sm text-gray-700">{room}</p>
          <p className="text-sm text-gray-700">{teacher}</p>
          {assignedAt ? <p className="mt-2 text-xs text-gray-500">Captured when assigned on {formatDateShort(assignedAt)}</p> : null}
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
      <section aria-labelledby={sectionId} className="rounded-sm border border-gray-200 bg-white p-4">
        <SectionHeading id={sectionId}>Session now</SectionHeading>
        <div className="mt-2 flex items-start gap-2 text-sm text-gray-600">
          <X className="mt-0.5 h-4 w-4 shrink-0 text-red-500" aria-hidden="true" />
          <span>The original session has been deleted.</span>
        </div>
      </section>
    );
  }

  if (current.status === "deleted") {
    return (
      <section aria-labelledby={sectionId} className="rounded-sm border border-gray-200 bg-white p-4">
        <SectionHeading id={sectionId}>Session now</SectionHeading>
        <div className="mt-2 flex items-start gap-2 text-sm text-gray-600">
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
    <section aria-labelledby={sectionId} className="rounded-sm border border-gray-200 bg-white p-4">
      <SectionHeading id={sectionId}>Session now</SectionHeading>
      <p className="mt-2 text-sm font-medium text-gray-900">{date}, {time}</p>
      <p className="mt-1 text-sm text-gray-700">{room}</p>
      <p className="text-sm text-gray-700">{current.teacher_name}</p>
    </section>
  );
}

function ChangeSummary({ changes, issueId }: { changes: ChangedField[]; issueId: string }) {
  const sectionId = `resolution-changes-${issueId}`;
  if (changes.length === 0) return null;

  return (
    <section aria-labelledby={sectionId} className="rounded-sm border border-gray-200 bg-white p-4">
      <SectionHeading id={sectionId}>Changes</SectionHeading>
      <dl className="mt-2 space-y-1.5">
        {changes.map((change) => (
          <div key={change.field} className="flex items-baseline gap-3 text-sm">
            <dt className="w-20 shrink-0 font-medium text-gray-700">{change.label}</dt>
            <dd className="flex min-w-0 items-baseline gap-2 text-gray-600">
              <span className="shrink-0">{change.from}</span>
              <ArrowRight className="h-3 w-3 shrink-0 text-gray-400" aria-hidden="true" />
              <span className="shrink-0 font-medium text-gray-900">{change.to}</span>
            </dd>
          </div>
        ))}
      </dl>
    </section>
  );
}

function ImpactAndActions({
  issue,
  selectedCandidate,
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
  const sectionId = `resolution-actions-${issue.id}`;
  const hasConflict = issue.impact_context.reasons.some((r) =>
    r.code === "regular_session_overlap" || r.code === "sit_in_overlap"
  );
  const impactMessage = issueMessage(issue);
  const consequence = issueConsequence(issue);

  return (
    <section aria-labelledby={sectionId} className="rounded-sm border border-gray-200 bg-white p-4">
      <SectionHeading id={sectionId}>Impact and actions</SectionHeading>
      {resolutionError ? (
        <div className="mt-2 flex items-start gap-2 rounded-sm border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-800" role="alert">
          <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
          <span>{resolutionError}</span>
        </div>
      ) : null}
      <p className="mt-2 text-sm font-medium text-gray-800">{impactMessage}</p>
      <p className="mt-1 text-sm text-gray-600">{consequence}</p>
      {hasConflict ? (
        <div className="mt-3 rounded-sm border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900" role="note">
          The new time overlaps the student&apos;s regular class.
        </div>
      ) : null}

      <div className="mt-4 grid grid-cols-2 gap-2" role="group" aria-label="Resolution actions">
        <Button
          size="sm"
          disabled={!selectedCandidate || busy}
          onClick={() => onAction("reassign")}
          loading={busy}
        >
          Reassign sit-in
        </Button>
        <Button
          variant="secondary"
          size="sm"
          disabled={busy}
          onClick={() => onAction("keep")}
        >
          Keep current arrangement
        </Button>
        <Button
          variant="ghost"
          size="sm"
          disabled={busy}
          onClick={() => onAction("mark_for_review")}
        >
          Mark for manual review
        </Button>
        <Button
          variant="danger"
          size="sm"
          disabled={busy}
          onClick={() => onAction("cancel")}
        >
          Cancel arrangement
        </Button>
      </div>
    </section>
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
  const changes = extractChanges(issue);
  const headingId = `resolution-heading-${issue.id}`;

  return (
    <div className="space-y-3" role="region" aria-labelledby={headingId}>
      <h2 id={headingId} className="sr-only">
        Resolution comparison for {issue.student_name ?? issue.wcode}
      </h2>
      <OriginalAssignment issue={issue} />
      <CurrentSession issue={issue} />
      <ChangeSummary changes={changes} issueId={issue.id} />
      <ImpactAndActions
        issue={issue}
        selectedCandidate={selectedCandidate}
        onAction={onAction}
        busy={busy}
        resolutionError={resolutionError}
      />
    </div>
  );
}
