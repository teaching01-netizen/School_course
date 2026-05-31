import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import CalendarCell from "./CalendarCell";
import type { SubjectSessions, SessionInSubject } from "@/types";

type CalendarGridProps = {
  subjectSessions: SubjectSessions[];
  onSelectionChange?: (absentIds: Set<string>) => void;
};

type DayColumn = {
  label: string;
  date: string;
  sessions: (SessionInSubject & { subjectCode: string })[];
};

const DAY_LABELS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"] as const;

function toLocalDate(iso: string): string {
  return iso.slice(0, 10);
}

function getDayOfWeek(dateStr: string): number {
  // 0=Sun, 1=Mon, … 6=Sat → convert to 0=Mon … 6=Sun
  const d = new Date(dateStr + "T12:00:00");
  return (d.getDay() + 6) % 7;
}

function formatTimeLabel(iso: string): string {
  const timePart = iso.split("T")[1];
  if (!timePart) return "";
  const parts = timePart.split(":");
  return `${parts[0]}:${parts[1]}`;
}

export default function CalendarGrid({
  subjectSessions,
  onSelectionChange,
}: CalendarGridProps) {
  // Flatten sessions
  const allSessions = useMemo(() => {
    const flat: (SessionInSubject & { subjectCode: string })[] = [];
    for (const subj of subjectSessions) {
      for (const s of subj.sessions) {
        flat.push({ ...s, subjectCode: subj.subject_code });
      }
    }
    return flat;
  }, [subjectSessions]);

  // Build columns per day
  const dayColumns = useMemo(() => {
    const map = new Map<string, (SessionInSubject & { subjectCode: string })[]>();
    for (const s of allSessions) {
      const date = toLocalDate(s.start_at);
      const existing = map.get(date);
      if (existing) existing.push(s);
      else map.set(date, [s]);
    }

    const columns: DayColumn[] = [];
    for (const [date, sessions] of map) {
      const dow = getDayOfWeek(date);
      sessions.sort((a, b) => a.start_at.localeCompare(b.start_at));
      columns.push({
        label: DAY_LABELS[dow],
        date,
        sessions,
      });
    }
    columns.sort((a, b) => a.date.localeCompare(b.date));
    return columns;
  }, [allSessions]);

  // Unique time rows (sorted)
  const timeRows = useMemo(() => {
    const times = new Set<string>();
    for (const s of allSessions) {
      times.add(formatTimeLabel(s.start_at));
    }
    return Array.from(times).sort();
  }, [allSessions]);

  // Absent session state — auto-select all on mount
  const allIds = useMemo(
    () => allSessions.map((s) => s.id),
    [allSessions],
  );

  const [absentIds, setAbsentIds] = useState<Set<string>>(() => new Set(allIds));

  // Auto-select all when sessions change (or on first render)
  useEffect(() => {
    const next = new Set(allIds);
    setAbsentIds(next);
    onSelectionChange?.(next);
    // Only run on mount and when session IDs change
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [allIds.join(",")]);

  const toggleAbsent = useCallback(
    (sessionId: string) => {
      setAbsentIds((prev) => {
        const next = new Set(prev);
        if (next.has(sessionId)) {
          next.delete(sessionId);
        } else {
          next.add(sessionId);
        }
        onSelectionChange?.(next);
        return next;
      });
    },
    [onSelectionChange],
  );

  // Build a lookup: date → startTime → session
  const cellLookup = useMemo(() => {
    const lookup = new Map<string, Map<string, SessionInSubject & { subjectCode: string }>>();
    for (const col of dayColumns) {
      const rowMap = new Map<string, SessionInSubject & { subjectCode: string }>();
      for (const s of col.sessions) {
        rowMap.set(formatTimeLabel(s.start_at), s);
      }
      lookup.set(col.date, rowMap);
    }
    return lookup;
  }, [dayColumns]);

  // Keyboard navigation
  const gridRef = useRef<HTMLDivElement>(null);

  const sortedCellIds = useMemo(() => {
    const ids: string[] = [];
    for (const col of dayColumns) {
      for (const time of timeRows) {
        const session = cellLookup.get(col.date)?.get(time);
        if (session) ids.push(session.id);
      }
    }
    return ids;
  }, [dayColumns, timeRows, cellLookup]);

  const handleGridKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      // Event delegation: identify focused cell from the active element
      const active = document.activeElement as HTMLElement;
      const testId = active?.getAttribute("data-testid") ?? "";
      if (!testId.startsWith("cell-")) return;
      const currentId = testId.replace("cell-", "");

      const idx = sortedCellIds.indexOf(currentId);
      if (idx === -1) return;

      let nextIdx = -1;
      if (e.key === "ArrowRight") nextIdx = idx + 1;
      else if (e.key === "ArrowLeft") nextIdx = idx - 1;
      else if (e.key === "ArrowDown") {
        const cols = dayColumns.length;
        nextIdx = idx + cols;
      } else if (e.key === "ArrowUp") {
        const cols = dayColumns.length;
        nextIdx = idx - cols;
      }

      if (nextIdx >= 0 && nextIdx < sortedCellIds.length) {
        e.preventDefault();
        const nextId = sortedCellIds[nextIdx];
        // Focus the actual cell element (button inside wrapper) via querySelector
        const cellEl = gridRef.current?.querySelector<HTMLElement>(
          `[data-testid="cell-${nextId}"]`,
        );
        cellEl?.focus();
      }
    },
    [sortedCellIds, dayColumns],
  );

  // Empty state
  if (allSessions.length === 0) {
    return (
      <div className="rounded-sm border border-gray-200 bg-white p-5 text-center text-gray-650 font-medium">
        No sessions found in this date range
      </div>
    );
  }

  return (
    <div
      ref={gridRef}
      role="grid"
      aria-label="Weekly session calendar"
      className="overflow-x-auto"
      onKeyDown={handleGridKeyDown}
    >
      {/* Day headers */}
      <div role="row" className="flex">
        <div role="columnheader" className="w-16 shrink-0" aria-hidden="true" />
        {dayColumns.map((col) => (
          <div
            key={col.date}
            role="columnheader"
            className="flex-1 px-2 py-2 text-center text-sm font-semibold text-gray-700"
          >
            {col.label}
            <span className="block text-xs text-gray-400">{col.date}</span>
          </div>
        ))}
      </div>

      {/* Time rows */}
      {timeRows.map((time) => (
        <div key={time} role="row" className="flex items-center border-t border-gray-100">
          {/* Time label */}
          <div
            role="rowheader"
            className="w-16 shrink-0 pr-2 text-right text-xs font-medium text-gray-500"
          >
            {time}
          </div>

          {/* Day cells */}
          {dayColumns.map((col) => {
            const session = cellLookup.get(col.date)?.get(time);
            if (!session) {
              return (
                <div
                  key={`${col.date}-${time}`}
                  role="gridcell"
                  className="flex-1 px-1 py-1"
                  aria-hidden="true"
                />
              );
            }

            return (
              <div
                key={session.id}
                className="flex-1 px-1 py-1"
              >
                <CalendarCell
                  sessionId={session.id}
                  startTime={formatTimeLabel(session.start_at)}
                  endTime={formatTimeLabel(session.end_at)}
                  status={absentIds.has(session.id) ? "absent" : "available"}
                  onToggleAbsent={toggleAbsent}
                  onToggleCover={() => {}}
                />
              </div>
            );
          })}
        </div>
      ))}
    </div>
  );
}
