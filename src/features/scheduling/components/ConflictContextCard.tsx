import type { ConflictDetails } from "@/features/scheduling/types";
import type { Course } from "@/features/courses/types";
import type { User } from "@/types/shared";
import { conflictHeadline } from "@/features/scheduling/domain/conflicts";
import { conflictSuggestion } from "@/components/PreflightIndicator";
import { formatTimeRange } from "@/features/scheduling/domain/time";

/**
 * Reason-first summary of the conflict a user is trying to solve. The blocked
 * availability strip carries its ConflictDetails into the slot finder as query
 * params; the card restates the reason there, so the "find alternatives" page
 * never loses the context of what went wrong.
 */
export type ConflictContext = {
  details: ConflictDetails;
  teacherName?: string;
  roomName?: string;
  studentName?: string;
  studentId?: string;
};

/** Rebuild a ConflictContext from the query params the strip links with. */
export function parseConflictContext(search: URLSearchParams): ConflictContext | null {
  const kind = search.get("kind");
  const course_id = search.get("course_id");
  const teacher_id = search.get("teacher_id");
  const start_at = search.get("start_at");
  const end_at = search.get("end_at");
  if (!kind || !course_id || !teacher_id || !start_at || !end_at) return null;
  const rawCount = search.get("student_count");
  return {
    details: {
      kind,
      requested: { course_id, teacher_id, room_id: search.get("room_id"), start_at, end_at },
      conflicts: [],
      student_count: rawCount && !Number.isNaN(Number(rawCount)) ? Number(rawCount) : undefined,
    },
    teacherName: search.get("teacher") ?? undefined,
    roomName: search.get("room") ?? undefined,
    studentName: search.get("student") ?? undefined,
    studentId: search.get("student_id") ?? undefined,
  };
}

export function ConflictContextCard({
  context,
  coursesById,
  onDismiss,
}: {
  context: ConflictContext;
  coursesById: Map<string, Course>;
  /** Renders an ✕ that hands the page back to a blank search. */
  onDismiss?: () => void;
}) {
  const { details, teacherName, roomName } = context;
  const teachersById: Map<string, User> | undefined =
    teacherName && details.requested.teacher_id
      ? new Map([[details.requested.teacher_id, { id: details.requested.teacher_id, username: teacherName, full_name: teacherName, role: "Teacher" }]])
      : undefined;
  const roomsById = roomName && details.requested.room_id
    ? new Map([[details.requested.room_id, { id: details.requested.room_id, name: roomName, capacity: null }]])
    : undefined;
  const headline = conflictHeadline(details, { coursesById, teachersById, roomsById });

  return (
    <div
      className="rounded-md border border-red-200 bg-[var(--color-wi-danger-bg)] px-3 py-2.5 text-[13px]"
      role="alert"
      aria-label="Conflict you are finding alternatives for"
    >
      <div className="flex items-start gap-2">
        <span aria-hidden="true" className="mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center rounded-full bg-[var(--color-wi-red)] text-[10px] font-bold text-white">✕</span>
        <div className="min-w-0 flex-1">
          <p className="text-[11px] font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">
            Finding alternatives for
          </p>
          <p className="mt-0.5 font-medium text-[var(--color-wi-red)]">{headline}</p>
          <p className="mt-0.5 text-xs text-[var(--color-wi-text-light)]">
            Requested · <span className="font-medium text-[var(--color-wi-text)]">{formatTimeRange(details.requested.start_at, details.requested.end_at)}</span>
          </p>
          <p className="mt-1 text-xs text-[var(--color-wi-text-light)]">{conflictSuggestion(details.kind)}</p>
        </div>
        {onDismiss && (
          <button
            type="button"
            aria-label="Dismiss conflict context"
            title="Start a fresh search"
            onClick={onDismiss}
            className="shrink-0 rounded p-0.5 text-[var(--color-wi-text-light)] transition-colors duration-150 hover:text-[var(--color-wi-text)] motion-reduce:transition-none"
          >
            ✕
          </button>
        )}
      </div>
    </div>
  );
}