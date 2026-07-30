import { X } from "lucide-react";
import { issueMessage } from "../../features/scheduleImpact/format";
import type { ScheduleImpactIssue } from "../../features/scheduleImpact/types";

function formatTimeRange(start: string | null, end: string | null): string {
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

function getStatusLabel(issue: ScheduleImpactIssue): string {
  if (issue.status === "needs_review") return "Needs resolution";
  if (issue.status === "resolved") return "Resolved";
  if (issue.status === "dismissed") return "Dismissed";
  return "Needs resolution";
}

function getStatusClasses(issue: ScheduleImpactIssue): string {
  if (issue.status === "needs_review") return "text-amber-800";
  return "text-red-700";
}

export default function WorkQueueComparison({ issue }: { issue: ScheduleImpactIssue }) {
  const before = issue.change_context.before;
  const after = issue.change_context.after;
  const originalSnapshot = issue.assignment_context.original_session.snapshot;
  const originalTimeStart = (originalSnapshot?.start_at as string) ?? (before?.start_at as string) ?? issue.details.old_start_at ?? null;
  const originalTimeEnd = (originalSnapshot?.end_at as string) ?? (before?.end_at as string) ?? null;
  const currentSession = issue.assignment_context.current_session;
  const currentTimeStart = (after?.start_at as string) ?? currentSession?.start_at ?? issue.details.new_start_at ?? null;
  const currentTimeEnd = (after?.end_at as string) ?? currentSession?.end_at ?? null;
  const impactMessage = issueMessage(issue);
  const hasOverlap = issue.impact_context.reasons.some(
    (r) => r.code === "regular_session_overlap" || r.code === "sit_in_overlap"
  );

  const current = issue.assignment_context.current_session;
  const isDeleted = !current || current.status === "deleted";

  return (
    <div className="space-y-1.5" role="list" aria-label={`Schedule comparison for ${issue.student_name ?? issue.wcode}`}>
      <div className="flex items-baseline gap-3 text-sm" role="listitem">
        <span className="w-20 shrink-0 text-xs font-medium text-gray-500">Originally</span>
        <span className="text-gray-700">{formatTimeRange(originalTimeStart, originalTimeEnd)}</span>
      </div>
      <div className="flex items-baseline gap-3 text-sm" role="listitem">
        <span className="w-20 shrink-0 text-xs font-medium text-gray-500">Now</span>
        {isDeleted ? (
          <span className="flex items-center gap-1 text-gray-500">
            <X className="h-3 w-3" aria-hidden="true" />
            Session deleted
          </span>
        ) : (
          <span className="text-gray-700">{formatTimeRange(currentTimeStart, currentTimeEnd)}</span>
        )}
      </div>
      <div className="flex items-baseline gap-3 text-sm" role="listitem">
        <span className="w-20 shrink-0 text-xs font-medium text-gray-500">Impact</span>
        {hasOverlap ? (
          <span className="flex items-center gap-1 text-amber-800">
            <span aria-hidden="true">•</span>
            Overlaps regular class
          </span>
        ) : (
          <span className="text-gray-700">{impactMessage}</span>
        )}
      </div>
      <div className="flex items-baseline gap-3 text-sm" role="listitem">
        <span className="w-20 shrink-0 text-xs font-medium text-gray-500">Status</span>
        <span className={`font-medium ${getStatusClasses(issue)}`}>{getStatusLabel(issue)}</span>
      </div>
    </div>
  );
}
