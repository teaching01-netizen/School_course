import { useMemo } from "react";
import SessionChip from "./SessionChip";
import type { SubjectSessions } from "@/types";

type SessionGridProps = {
  subjects: SubjectSessions[];
  selectedSessionIds: Set<string>;
  onToggleSession: (sessionId: string) => void;
  onToggleAll: () => void;
  onToggleSubject: (subjectId: string) => void;
  allSelected: boolean;
};

function extractTime(iso: string): string {
  const timePart = iso.split("T")[1];
  if (!timePart) return "";
  const [h, m] = timePart.split(":");
  return `${h}:${m}`;
}

function formatDateRange(subjects: SubjectSessions[]): string {
  const allDates = subjects.flatMap((s) => s.sessions.map((sess) => sess.date));
  if (allDates.length === 0) return "";

  const sorted = [...allDates].sort();
  const first = sorted[0];
  const last = sorted[sorted.length - 1];

  if (first === last) {
    return formatDateShort(first);
  }
  return `${formatDateShort(first)}–${formatDateShort(last)}`;
}

function formatDateShort(dateStr: string): string {
  const d = new Date(dateStr + "T00:00:00");
  const month = d.toLocaleDateString("en-GB", { month: "short" });
  const day = d.getDate();
  return `${day} ${month}`;
}

function countSessions(subjects: SubjectSessions[]): number {
  return subjects.reduce((sum, s) => sum + s.sessions.length, 0);
}

export default function SessionGrid({
  subjects,
  selectedSessionIds,
  onToggleSession,
  onToggleAll,
  onToggleSubject,
  allSelected,
}: SessionGridProps) {
  const totalCount = useMemo(() => countSessions(subjects), [subjects]);
  const selectedCount = useMemo(() => {
    let count = 0;
    for (const id of selectedSessionIds) {
      if (id) count++;
    }
    return count;
  }, [selectedSessionIds]);

  if (subjects.length === 0) {
    return (
      <div className="rounded-sm border border-gray-200 bg-white p-5 text-center text-gray-650 font-medium">
        No classes found in this date range
      </div>
    );
  }

  const dateRange = formatDateRange(subjects);

  return (
    <div className="space-y-4">
      {/* Header */}
      <h2 className="text-lg font-semibold text-gray-900">
        Classes in range{dateRange ? ` (${dateRange})` : ""}
      </h2>

      {/* Live region for screen readers */}
      <div role="status" className="sr-only" aria-live="polite">
        {selectedCount} of {totalCount} sessions selected
      </div>

      {/* Master toggle */}
      <label className="flex items-center gap-2 cursor-pointer select-none text-sm font-medium text-gray-700">
        <input
          type="checkbox"
          role="checkbox"
          aria-checked={allSelected}
          aria-label="Select all sessions"
          checked={allSelected}
          onChange={onToggleAll}
          className="h-4 w-4 rounded border-gray-300 text-green-600 focus:ring-green-500"
        />
        All selected
      </label>

      {/* Subject groups */}
      {subjects.map((subject) => {
        const subjectSessionCount = subject.sessions.length;
        const subjectSelectedCount = subject.sessions.filter((s) =>
          selectedSessionIds.has(s.id)
        ).length;
        const subjectAllSelected =
          subjectSessionCount > 0 && subjectSelectedCount === subjectSessionCount;

        return (
          <div
            key={subject.subject_id}
            role="group"
            aria-label={`${subject.subject_code} sessions`}
            className="rounded-sm border border-gray-200 bg-white overflow-hidden"
          >
            {/* Subject header */}
            <div className="flex items-center gap-2 border-b border-gray-100 bg-gray-50 px-4 py-2.5">
              <input
                type="checkbox"
                role="checkbox"
                aria-checked={subjectAllSelected}
                aria-label={`Toggle all ${subject.subject_code} sessions`}
                checked={subjectAllSelected}
                onChange={() => onToggleSubject(subject.subject_id)}
                className="h-4 w-4 rounded border-gray-300 text-green-600 focus:ring-green-500"
              />
              <span className="text-sm font-semibold text-gray-800">
                {subject.subject_code}
              </span>
              <span className="text-sm text-gray-655 font-medium">
                ({subject.subject_name})
              </span>
            </div>

            {/* Session chips */}
            <div className="max-h-64 overflow-y-auto max-sm:flex max-sm:flex-col max-sm:gap-1.5 max-sm:p-2 flex flex-wrap gap-2 p-3 sm:max-h-none sm:overflow-visible">
              {subject.sessions.map((session) => (
                <SessionChip
                  key={session.id}
                  id={session.id}
                  date={session.date}
                  startTime={extractTime(session.start_at)}
                  endTime={extractTime(session.end_at)}
                  selected={selectedSessionIds.has(session.id)}
                  alreadyAbsent={session.already_absent}
                  onToggle={onToggleSession}
                  subjectCode={subject.subject_code}
                />
              ))}
            </div>
          </div>
        );
      })}
    </div>
  );
}
