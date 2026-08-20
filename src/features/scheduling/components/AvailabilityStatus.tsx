import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import type { UsePreflightReturn } from "@/features/scheduling/hooks/usePreflight";
import type { Course } from "@/features/courses/types";
import type { User } from "@/types/shared";
import type { Room } from "@/features/scheduling/types";
import { formatTimeRange } from "@/features/scheduling/domain/time";
import { conflictHeadline, conflictSharedResourceName } from "@/features/scheduling/domain/conflicts";
import { conflictSuggestion } from "@/components/PreflightIndicator";

const MAX_VISIBLE_CONFLICTS = 3;

/**
 * Compact availability summary for the session editor. Unlike the full
 * PreflightIndicator report used in the create modals, this strip stays quiet:
 * one calm line for the common cases, expandable conflict details only when a
 * schedule conflict actually exists.
 */
export function SessionAvailabilityStatus({
  preflight,
  coursesById,
  teachersById,
  roomsById,
  missingFields,
  roomMissing,
  actionVerb = "save",
}: {
  preflight: UsePreflightReturn;
  coursesById: Map<string, Course>;
  teachersById: Map<string, User>;
  roomsById?: Map<string, Room>;
  /** Labels of required fields that are still empty; shown when idle. */
  missingFields?: string[];
  /** No classroom assigned — explains the provisional state when set. */
  roomMissing?: boolean;
  /** Verb used by the provisional hint ("you can still <verb> it."). */
  actionVerb?: string;
}) {
  const { status, details } = preflight;
  const conflictCount = details?.conflicts?.length ?? 0;
  const [conflictsExpanded, setConflictsExpanded] = useState(() => conflictCount > 0 && conflictCount <= 2);
  const [studentsExpanded, setStudentsExpanded] = useState(() => details?.kind === "student_overlap");

  // Each fresh preflight result starts with the small clash sets shown, so the
  // reason is visible the moment a conflict appears.
  const prevDetailsRef = useRef(details);
  useEffect(() => {
    if (prevDetailsRef.current !== details) {
      prevDetailsRef.current = details;
      const count = details?.conflicts?.length ?? 0;
      setConflictsExpanded(count > 0 && count <= 2);
      setStudentsExpanded(details?.kind === "student_overlap");
    }
  }, [details, conflictCount]);

  if (preflight.loading) {
    return (
      <div className="flex items-center gap-2 rounded-md border border-[var(--color-wi-line)] bg-[var(--color-wi-bg)] px-3 py-2 text-[13px] text-[var(--color-wi-text-light)]">
        <span data-testid="session-availability-spinner" className="inline-block h-3 w-3 animate-spin rounded-full border-2 border-[var(--color-wi-line)] border-t-transparent" aria-hidden="true" />
        <span>Checking availability…</span>
      </div>
    );
  }

  if (status === "available") {
    return (
      <div className="flex items-center gap-2 rounded-md border border-[var(--color-wi-line)] bg-[var(--color-wi-bg)] px-3 py-2 text-[13px] text-[var(--color-wi-text)]">
        <span aria-hidden="true" className="flex h-4 w-4 shrink-0 items-center justify-center rounded-full bg-[var(--color-wi-green)] text-[10px] font-bold text-white">✓</span>
        <span>
          <span className="font-medium text-[var(--color-wi-green)]">Available</span>
          <span className="text-[var(--color-wi-text-light)]"> — no conflicts with this arrangement</span>
        </span>
      </div>
    );
  }

  if (status === "provisional") {
    return (
      <div className="flex items-center gap-2 rounded-md border border-[var(--color-wi-line)] bg-[var(--color-wi-bg)] px-3 py-2 text-[13px] text-[var(--color-wi-text)]">
        <span aria-hidden="true" className="h-2 w-2 shrink-0 rounded-full bg-[var(--color-wi-amber)]" />
        <span>
          <span className="font-medium text-[var(--color-wi-amber)]">Provisional</span>
          <span className="text-[var(--color-wi-text-light)]">
            {" "}— {roomMissing ? `no classroom assigned; you can still ${actionVerb} it.` : "some details are unconfirmed."}
          </span>
        </span>
      </div>
    );
  }

  if (status === "blocked" && details) {
    const totalConflicts = details.total_conflicts ?? conflictCount;
    const headline = conflictHeadline(details, { coursesById, teachersById, roomsById });
    const studentCount = details.conflicting_students?.length ?? 0;
    const clashCountLabel =
      conflictCount === 1
        ? "1 clash"
        : totalConflicts > conflictCount
          ? `Showing ${conflictCount} of ${totalConflicts} clashes`
          : `${conflictCount} clashes`;

    // Carry the conflict into the slot finder so the next page can restate
    // the reason instead of greeting a blank search form.
    const requestedRoomName = details.requested.room_id ? roomsById?.get(details.requested.room_id)?.name ?? null : null;
    const requestedTeacherName = teachersById.get(details.requested.teacher_id)?.username ?? null;
    const alternativeSearch = new URLSearchParams();
    alternativeSearch.set("kind", details.kind);
    alternativeSearch.set("course_id", details.requested.course_id);
    alternativeSearch.set("teacher_id", details.requested.teacher_id);
    alternativeSearch.set("start_at", details.requested.start_at);
    alternativeSearch.set("end_at", details.requested.end_at);
    if (details.requested.room_id) alternativeSearch.set("room_id", details.requested.room_id);
    if (requestedRoomName) alternativeSearch.set("room", requestedRoomName);
    if (requestedTeacherName) alternativeSearch.set("teacher", requestedTeacherName);
    if (details.kind === "student_overlap") {
      alternativeSearch.set("student_count", String(studentCount));
      const firstStudent = details.conflicting_students?.[0];
      if (firstStudent) {
        alternativeSearch.set("student_id", firstStudent.student_id);
        alternativeSearch.set("student", firstStudent.full_name);
      }
    }
    return (
      <div className="rounded-md border border-red-200 bg-[var(--color-wi-danger-bg)] px-3 py-2 text-[13px]" role="alert">
        <div className="flex items-start gap-2">
          <span aria-hidden="true" className="mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center rounded-full bg-[var(--color-wi-red)] text-[10px] font-bold text-white">✕</span>
          <div className="min-w-0 flex-1">
            {/* The reason, naming the actual room / teacher / students involved. */}
            <p className="font-medium text-[var(--color-wi-red)]">
              {headline}
              {conflictCount > 1 && (
                <span className="ml-1.5 font-normal text-[var(--color-wi-text-light)]">
                  {clashCountLabel}
                </span>
              )}
            </p>

            {/* What the user asked for. */}
            <p className="mt-0.5 text-xs text-[var(--color-wi-text-light)]">
              Requested · <span className="font-medium text-[var(--color-wi-text)]">{formatTimeRange(details.requested.start_at, details.requested.end_at)}</span>
            </p>

            {conflictCount > 0 && (
              <div className="mt-1.5">
                {conflictsExpanded ? (
                  <>
                    <ul className="space-y-1">
                      {(details.conflicts ?? []).slice(0, MAX_VISIBLE_CONFLICTS).map((c) => {
                        const shared = conflictSharedResourceName(details.kind, c, { roomsById, teachersById });
                        const courseCode = coursesById.get(c.course_id)?.code ?? `${c.course_id.slice(0, 8)}…`;
                        const teacherName = teachersById.get(c.teacher_id)?.username ?? `${c.teacher_id.slice(0, 8)}…`;
                        const roomName = c.room_id ? roomsById?.get(c.room_id)?.name ?? `${c.room_id.slice(0, 8)}…` : null;
                        const highlight = shared ? (
                          <span className="font-semibold text-[var(--color-wi-red)]">{shared}</span>
                        ) : null;
                        return (
                          <li key={c.session_id} className="rounded-[4px] border border-red-100 bg-white/70 px-2 py-1">
                            <div className="flex items-baseline justify-between gap-2">
                              <Link
                                to={`/courses/${c.course_id}`}
                                className="truncate text-xs font-semibold text-[var(--color-wi-red)] hover:underline"
                              >
                                {courseCode}
                              </Link>
                              <span className="shrink-0 font-mono text-[11px] text-[var(--color-wi-text-light)]">
                                {formatTimeRange(c.start_at, c.end_at)}
                              </span>
                            </div>
                            <p className="mt-0.5 truncate text-xs text-[var(--color-wi-text-light)]">
                              {shared === teacherName ? highlight : teacherName}
                              {roomName ? (
                                <>
                                  <span> · </span>
                                  {shared === roomName ? highlight : roomName}
                                </>
                              ) : null}
                            </p>
                          </li>
                        );
                      })}
                    </ul>
                    {totalConflicts > conflictCount && (
                      <p className="mt-1 text-[11px] italic text-[var(--color-wi-text-light)]">
                        +{totalConflicts - MAX_VISIBLE_CONFLICTS > 0 ? totalConflicts - MAX_VISIBLE_CONFLICTS : totalConflicts - conflictCount} more
                      </p>
                    )}
                    {totalConflicts > 2 && (
                      <button
                        type="button"
                        onClick={() => setConflictsExpanded(false)}
                        className="mt-1 flex items-center gap-1 text-xs font-semibold text-[var(--color-wi-text-light)] hover:text-[var(--color-wi-text)]"
                      >
                        <span className="inline-block w-2" aria-hidden="true">▾</span>
                        {clashCountLabel}
                      </button>
                    )}
                  </>
                ) : (
                  <button
                    type="button"
                    onClick={() => setConflictsExpanded(true)}
                    className="flex items-center gap-1 text-xs font-semibold text-[var(--color-wi-text-light)] hover:text-[var(--color-wi-text)]"
                  >
                    <span className="inline-block w-2" aria-hidden="true">▸</span>
                    Show {conflictCount === 1 ? "1 clash" : `${conflictCount} clashes`}
                  </button>
                )}
              </div>
            )}

            {studentCount > 0 && (
              <div className="mt-1.5">
                <button
                  type="button"
                  onClick={() => setStudentsExpanded((prev) => !prev)}
                  className="flex items-center gap-1 text-xs font-semibold text-[var(--color-wi-text-light)] hover:text-[var(--color-wi-text)]"
                >
                  <span className="inline-block w-2" aria-hidden="true">{studentsExpanded ? "▾" : "▸"}</span>
                  Affected students ({studentCount})
                </button>
                {studentsExpanded && (
                  <ul className="mt-1 list-disc space-y-0.5 pl-5 text-xs text-[var(--color-wi-text-light)]">
                    {details.conflicting_students?.map((cs) => (
                      <li key={cs.student_id}>
                        <span className="text-[var(--color-wi-red)]">{cs.full_name}</span>
                        <span className="ml-1 text-[10px] font-medium text-[var(--color-wi-text-light)]">({cs.status})</span>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            )}

            <p className="mt-1.5 text-xs text-[var(--color-wi-text-light)]">{conflictSuggestion(details.kind)}</p>
            <Link
              to={{ pathname: "/slot-finder", search: alternativeSearch.toString() }}
              className="mt-1.5 inline-flex items-center gap-1 text-xs font-medium text-[var(--color-wi-primary)] hover:underline"
            >
              Find alternative slots →
            </Link>
          </div>
        </div>
      </div>
    );
  }

  if (status === "error") {
    return (
      <div className="flex items-center justify-between gap-2 rounded-md border border-amber-200 bg-[var(--color-wi-amber-bg)] px-3 py-2 text-[13px] text-amber-800" role="alert">
        <span>Couldn't check availability right now.</span>
        <button
          type="button"
          onClick={() => { if (preflight.lastParams) preflight.check(preflight.lastParams); }}
          className="shrink-0 rounded-sm bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-800 transition-colors duration-150 hover:bg-amber-200 motion-reduce:transition-none"
        >
          Try again
        </button>
      </div>
    );
  }

  // idle — not enough of the form filled in to check yet.
  const missing = (missingFields ?? []).filter(Boolean);
  return (
    <div className="flex items-center gap-2 rounded-md border border-[var(--color-wi-line)] bg-[var(--color-wi-bg)] px-3 py-2 text-[13px] text-[var(--color-wi-text-light)]">
      <span aria-hidden="true" className="h-2 w-2 shrink-0 rounded-full border border-[var(--color-wi-line)]" />
      <span>
        {missing.length > 0
          ? `Set ${missing.join(", ")} to check availability`
          : "Fill in the schedule to check availability"}
      </span>
    </div>
  );
}

/** Save-button label that speaks the availability state. */
export function getSessionSaveLabel(preflight: UsePreflightReturn, submitLabel = "Save", provisionalLabel = "Save as provisional"): string {
  if (preflight.loading) return "Checking…";
  if (preflight.status === "blocked") return "Fix conflicts to save";
  if (preflight.status === "error") return "Couldn't check — try again";
  if (preflight.status === "provisional") return provisionalLabel;
  return submitLabel;
}