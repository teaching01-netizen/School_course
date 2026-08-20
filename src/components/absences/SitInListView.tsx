import { useMemo, useState } from "react";
import type { CalendarAbsence, CalendarAbsenceDay, CalendarSessionBrief } from "../../types";
import Button from "../ui/Button";
import EmptyState from "../ui/EmptyState";
import SitInTableRow, { type SitInListRow } from "./SitInTableRow";
import { getAbsenceSubjectLabel, getSessionLabel } from "./calendarDisplay";

type SortKey = "student" | "leaving" | "sit-in" | "date" | "method" | "status";
type SortDirection = "asc" | "desc";

export type SitInListMode = "sit-ins" | "absences";

type SitInListViewProps = {
  sessions: CalendarSessionBrief[];
  absenceDays: CalendarAbsenceDay[];
  absences: CalendarAbsence[];
  mode: SitInListMode;
  zone: string;
  hasFilters: boolean;
  hasAnySitIns: boolean;
  onClearFilters: () => void;
};

function sortValue(row: SitInListRow, key: SortKey): string {
  switch (key) {
    case "student":
      return row.absence
        ? row.absence.student_name?.trim() || row.absence.wcode
        : row.visitor!.nickname || row.visitor!.student_name || row.visitor!.wcode;
    case "leaving":
      return row.absence
        ? getAbsenceSubjectLabel(row.absence)
        : row.visitor!.from_course_name || row.visitor!.from_course_code;
    case "sit-in":
      return row.absence ? getAbsenceSubjectLabel(row.absence) : getSessionLabel(row.session!);
    case "date":
      return row.absence ? row.absence.date_from : row.session!.start_at;
    case "method":
      return row.absence?.sit_in_method || "physical";
    case "status":
      return row.absence?.status || "pending";
  }
}

export default function SitInListView({ sessions, absenceDays, absences, mode, zone, hasFilters, onClearFilters }: SitInListViewProps) {
  const [sortKey, setSortKey] = useState<SortKey>("date");
  const [sortDirection, setSortDirection] = useState<SortDirection>("asc");
  const [visibleCount, setVisibleCount] = useState(20);

  const absencesById = useMemo(() => {
    const map = new Map<string, CalendarAbsence>();
    for (const day of absenceDays) {
      for (const absence of day.absences) {
        map.set(absence.id, absence);
      }
    }
    return map;
  }, [absenceDays]);

  const rows = useMemo<SitInListRow[]>(() => {
    const data: SitInListRow[] =
      mode === "absences"
        ? absences.map((absence) => ({
            id: absence.id,
            index: 0,
            visitor: null,
            session: null,
            absence,
          }))
        : sessions.flatMap((session) =>
            (session.sit_in_students ?? []).map((visitor) => ({
              id: `${session.id}-${visitor.absence_id}-${visitor.wcode}`,
              index: 0,
              visitor,
              session,
              absence: absencesById.get(visitor.absence_id),
            })),
          );

    data.sort((a, b) => {
      const left = sortValue(a, sortKey);
      const right = sortValue(b, sortKey);
      const result = left.localeCompare(right);
      return sortDirection === "asc" ? result : -result;
    });

    return data.map((row, index) => ({ ...row, index: index + 1 }));
  }, [absences, absencesById, mode, sessions, sortDirection, sortKey]);

  function toggleSort(key: SortKey) {
    if (sortKey === key) {
      setSortDirection((current) => (current === "asc" ? "desc" : "asc"));
      return;
    }
    setSortKey(key);
    setSortDirection("asc");
  }

  const visibleRows = rows.slice(0, visibleCount);
  const headers: Array<[SortKey, string]> = [
    ["student", "Student"],
    ["leaving", "Leaving"],
    ["sit-in", "Sit-in"],
    ["date", "Date/Time"],
    ["method", "Method"],
    ["status", "Status"],
  ];

  if (rows.length === 0) {
    const isAbsenceList = mode === "absences";
    const message = hasFilters
      ? isAbsenceList
        ? "No absences match your filters."
        : "No sit-ins match your filters."
      : isAbsenceList
        ? "No absences recorded in this date range."
        : "No sit-ins recorded in this date range.";
    return (
      <div className="rounded-sm border border-wi-line bg-white">
        <EmptyState
          message={message}
          action={hasFilters ? <Button variant="secondary" size="sm" onClick={onClearFilters}>Clear filters</Button> : undefined}
        />
      </div>
    );
  }

  return (
    <div className="overflow-hidden rounded-sm border border-wi-line bg-white">
      <div className="overflow-x-auto">
        <table className="min-w-full text-left">
          <thead className="border-b border-wi-line bg-[var(--color-wi-row-alt)] text-xs font-semibold text-[var(--color-wi-text-light)]">
            <tr>
              <th className="px-3 py-2">#</th>
              {headers.map(([key, label]) => (
                <th key={key} className="px-3 py-2">
                  <button type="button" className="inline-flex items-center gap-1 hover:text-[var(--color-wi-text)]" onClick={() => toggleSort(key)}>
                    {label}
                    {sortKey === key ? <span aria-hidden="true">{sortDirection === "asc" ? "↑" : "↓"}</span> : null}
                  </button>
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {visibleRows.map((row) => (
              <SitInTableRow key={row.id} row={row} zone={zone} />
            ))}
          </tbody>
        </table>
      </div>
      {visibleCount < rows.length ? (
        <div className="border-t border-wi-line-soft p-3 text-center">
          <Button variant="secondary" size="sm" onClick={() => setVisibleCount((count) => count + 20)}>
            Load more
          </Button>
        </div>
      ) : null}
    </div>
  );
}