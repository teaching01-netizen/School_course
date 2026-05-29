import { useState, useEffect, useRef } from "react";
import { Link } from "react-router-dom";
import type { UsePreflightReturn, PreflightStatus } from "@/hooks/usePreflight";
import type { Course, Room, User } from "@/types";
import { conflictKindLabel, formatTimeRange, getRequestedLabel } from "@/types";

const MAX_VISIBLE_CONFLICTS = 3;

function conflictSuggestion(kind: string): string {
  switch (kind) {
    case "room_overlap": return "Try a different room or time slot";
    case "teacher_overlap": return "Choose a different teacher or adjust the time";
    case "student_overlap": return "Reschedule or manage student attendance overrides";
    case "teacher_availability": return "Select a time within the teacher's working hours";
    case "room_availability": return "Select a time when the room is available";
    default: return "Adjust the schedule to resolve conflicts";
  }
}

type PreflightSnapshot = { status: PreflightStatus; loading: boolean };

export function getSaveButtonLabel(
  preflight: PreflightSnapshot,
  submitLabel: string,
  details?: { kind: string } | null,
): string {
  if (preflight.loading) return "Checking…";
  if (preflight.status === "blocked" && details) return `Blocked — ${conflictSuggestion(details.kind).toLowerCase()}`;
  if (preflight.status === "blocked") return "Blocked — fix conflicts";
  return submitLabel;
}

export function isSaveDisabled(preflight: PreflightSnapshot): boolean {
  if (preflight.loading) return true;
  if (preflight.status === "blocked" || preflight.status === "idle") return true;
  return false;
}

export type RequiredField = { label: string; value: string };

export type PreflightIndicatorProps = {
  preflight: UsePreflightReturn;
  coursesById: Map<string, Course>;
  teachersById: Map<string, User>;
  roomsById?: Map<string, Room>;
  requiredFields?: RequiredField[];
};

function getConflictItemLabel(
  conflict: { course_id: string; teacher_id: string; room_id: string | null },
  coursesById: Map<string, Course>,
  teachersById: Map<string, User>,
  roomsById?: Map<string, Room>,
): string {
  const course = coursesById.get(conflict.course_id);
  const teacher = teachersById.get(conflict.teacher_id);
  const courseStr = course ? `${course.code}` : conflict.course_id.slice(0, 8) + "\u2026";
  const teacherStr = teacher ? teacher.username : conflict.teacher_id.slice(0, 8) + "\u2026";
  let label = `${teacherStr} \u2013 ${courseStr}`;
  if (conflict.room_id && roomsById) {
    const room = roomsById.get(conflict.room_id);
    const roomStr = room ? room.name : conflict.room_id.slice(0, 8) + "\u2026";
    label += ` \u2022 ${roomStr}`;
  }
  return label;
}

type PreflightBadgeProps = {
  status: UsePreflightReturn["status"];
  details: UsePreflightReturn["details"];
  loading: boolean;
};

export function PreflightBadge({ status, details, loading }: PreflightBadgeProps) {
  if (loading) return (
    <span className="flex items-center gap-1 text-xs text-gray-500">
      <span data-testid="preflight-spinner-badge" className="inline-block w-2.5 h-2.5 border-2 border-gray-400 border-t-transparent rounded-full animate-spin" aria-hidden="true" />
      Checking…
    </span>
  );
  if (status === "available") return (
    <span className="inline-flex items-center gap-0.5 text-xs text-green-700">
      <span className="inline-flex items-center justify-center w-3 h-3 rounded-full bg-green-700 text-white text-[7px] font-bold leading-none" aria-hidden="true">✓</span>
      Available
    </span>
  );
  if (status === "provisional") return <span className="text-xs text-amber-700">Provisional</span>;
  if (status === "blocked") {
    return (
      <span className="flex items-center gap-1 text-xs text-red-700">
        <span>Blocked</span>
        {details && <span>— {conflictKindLabel(details.kind).label}</span>}
      </span>
    );
  }
  return null;
}

export function PreflightIndicator({ preflight, coursesById, teachersById, roomsById, requiredFields }: PreflightIndicatorProps) {
  const { status, details, occurrencesPlanned } = preflight;
  const isSeries = occurrencesPlanned != null;

  const conflictCount = details?.conflicts?.length ?? 0;
  const autoExpanded = details != null && conflictCount <= 2;
  const [conflictsManuallyToggled, setConflictsManuallyToggled] = useState(false);
  const conflictsExpanded = autoExpanded || conflictsManuallyToggled;
  const [studentsExpanded, setStudentsExpanded] = useState(true);

  const prevDetailsRef = useRef(details);
  useEffect(() => {
    if (prevDetailsRef.current !== details) {
      setConflictsManuallyToggled(false);
      setStudentsExpanded(true);
      prevDetailsRef.current = details;
    }
  }, [details]);

  return (
    <div className="rounded-sm border border-gray-200 bg-gray-50 px-3 py-2 text-sm">
      <div className="flex items-center justify-between">
        <div className="font-medium text-gray-800">Preflight</div>
        {preflight.loading ? (
          <div className="flex items-center gap-1.5 text-gray-500">
            <span data-testid="preflight-spinner" className="inline-block w-3 h-3 border-2 border-gray-400 border-t-transparent rounded-full animate-spin" aria-hidden="true" />
            <span>Checking schedule…</span>
          </div>
        ) : status === "available" ? (
          <div className="flex items-center gap-1 text-green-700">
            <span data-testid="preflight-check-icon" className="inline-flex items-center justify-center w-4 h-4 rounded-full bg-green-700 text-white text-[10px] font-bold leading-none" aria-hidden="true">✓</span>
            <span>Available</span>
          </div>
        ) : status === "provisional" ? (
          <div className="text-amber-700">Provisional</div>
        ) : status === "blocked" ? (
          <div className="flex items-center gap-1.5 text-red-700">
            <span>Blocked</span>
            {details && (
              <span className="text-xs text-red-600">— {conflictKindLabel(details.kind).label}</span>
            )}
          </div>
        ) : (() => {
          if (requiredFields) {
            const missingLabels = requiredFields.filter(f => !f.value).map(f => f.label);
            if (missingLabels.length > 0) {
              return <div className="text-gray-500">Fill in: {missingLabels.join(", ")}</div>;
            }
            return <div className="text-gray-500">Checking required fields…</div>;
          }
          return <div className="text-gray-500">Fill in course, teacher, and time to check availability</div>;
        })()}
      </div>

      {status === "provisional" && (
        <div data-testid="provisional-checklist" className="mt-1 flex items-center gap-3 text-xs text-amber-800">
          <span data-testid="checklist-student">Student ✅</span>
          <span data-testid="checklist-teacher">Teacher ✅</span>
          <span data-testid="checklist-room">Room ⏳</span>
        </div>
      )}

      {occurrencesPlanned != null && (
        <div className="mt-1 text-xs text-gray-700">Occurrences planned: {occurrencesPlanned}</div>
      )}

      {details && (
        <div className="mt-2 text-xs text-gray-700 space-y-2">
          {status === "available" && (
            <div className="bg-green-50 border border-green-200 rounded px-2 py-1">
              <span className="font-medium text-green-800">No conflicts found</span>
            </div>
          )}
          {status === "blocked" && (
            <div data-testid="conflict-group" className="bg-red-50 border border-red-200 rounded px-2 py-1">
              <div className="font-medium text-red-800 mb-1">
                {conflictKindLabel(details.kind).label}
                {conflictCount > 0 && (
                  <span className="ml-1.5 text-red-600 font-normal">{conflictCount} {conflictCount === 1 ? "conflict" : "conflicts"}</span>
                )}
              </div>
              <div className="text-red-700 text-[11px] mb-1">
                {conflictKindLabel(details.kind).detail}
              </div>
              <div className="text-blue-700 text-[11px] mt-1 mb-1">
                {conflictSuggestion(details.kind)}
              </div>
              <div className="mt-1">
                <div className="font-semibold text-gray-600 text-[11px]">
                  {isSeries ? "First blocked occurrence" : "Your requested time"}
                </div>
                <div className="text-gray-500">{formatTimeRange(details.requested.start_at, details.requested.end_at)}</div>
                <div className="text-gray-400 mt-0.5">{getRequestedLabel(details.requested, coursesById, teachersById)}</div>
                {details.requested.room_id && <div className="text-gray-400 mt-0.5">Room: {roomsById?.get(details.requested.room_id)?.name ?? details.requested.room_id}</div>}
              </div>
              {conflictCount > 0 && (
                <div className="mt-2">
                  <button
                    type="button"
                    onClick={() => setConflictsManuallyToggled(prev => !prev)}
                    className="flex items-center gap-1 text-xs font-semibold text-gray-600 hover:text-gray-800"
                  >
                    <span className="inline-block w-2">{conflictsExpanded ? "\u25BE" : "\u25B8"}</span>
                    {conflictCount === 1 ? "1 conflict" : `${conflictCount} conflicts`}
                  </button>
                  {conflictsExpanded && (
                    <ul className="list-disc pl-5 space-y-1 mt-1">
                      {(details.conflicts ?? []).slice(0, MAX_VISIBLE_CONFLICTS).map((c) => (
                        <li key={c.session_id} className="text-gray-600">
                          <Link
                            to={`/courses/${c.course_id}`}
                            className="font-medium text-red-700 hover:underline"
                          >
                            {getConflictItemLabel(c, coursesById, teachersById, roomsById)}
                          </Link>
                          <span className="text-gray-400 ml-1">({formatTimeRange(c.start_at, c.end_at)})</span>
                        </li>
                      ))}
                      {conflictCount > MAX_VISIBLE_CONFLICTS && (
                        <li className="text-gray-500 italic">+{conflictCount - MAX_VISIBLE_CONFLICTS} more conflicts</li>
                      )}
                    </ul>
                  )}
                </div>
              )}
              {details.conflicting_students && details.conflicting_students.length > 0 && (
                <div className="mt-2">
                  <button
                    type="button"
                    onClick={() => setStudentsExpanded(prev => !prev)}
                    className="flex items-center gap-1 text-xs font-semibold text-gray-600 hover:text-gray-800"
                  >
                    <span className="inline-block w-2">{studentsExpanded ? "\u25BE" : "\u25B8"}</span>
                    Affected students ({details.conflicting_students.length})
                  </button>
                  {studentsExpanded && (
                    <ul className="list-disc pl-5 space-y-0.5 mt-1">
                      {details.conflicting_students.map((cs) => (
                        <li key={cs.student_id} className="text-gray-600">
                          <span className="text-red-700">{cs.full_name}</span>
                          <span className={`ml-1 text-[10px] font-medium ${cs.status === "draft" ? "text-amber-600" : "text-green-600"}`}>
                            ({cs.status})
                          </span>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
