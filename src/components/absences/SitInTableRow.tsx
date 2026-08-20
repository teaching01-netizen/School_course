import type { CalendarAbsence, CalendarSessionBrief, CalendarSitInStudent } from "../../types";
import { formatUTCToZone, formatZoneDateKey, utcISOToZoneDate } from "../../utils/timezone";
import { formatFullDayLabel, formatTime, getAbsenceSubjectLabel, getSitInLabel, getSessionLabel, statusBadgeClasses, titleCase } from "./calendarDisplay";

export type SitInListRow = {
  id: string;
  index: number;
  visitor: CalendarSitInStudent | null;
  session: CalendarSessionBrief | null;
  absence?: CalendarAbsence | null;
};

type SitInTableRowProps = {
  row: SitInListRow;
  zone: string;
};

function formatAbsenceRange(absence: CalendarAbsence, zone: string): string | null {
  const from = formatZoneDateKey(absence.date_from, zone, "d MMM yyyy");
  if (!from) return null;
  if (absence.date_to && absence.date_to !== absence.date_from) {
    const to = formatZoneDateKey(absence.date_to, zone, "d MMM yyyy");
    if (to) {
      const fromShort = formatZoneDateKey(absence.date_from, zone, "d MMM");
      return `${fromShort ?? from} – ${to}`;
    }
  }
  return from;
}

function formatSessionDay(session: CalendarSessionBrief, zone: string): string {
  const dayKey = utcISOToZoneDate(session.start_at, zone) ?? session.start_at.slice(0, 10);
  return formatZoneDateKey(dayKey, zone, "EEEE, d MMMM yyyy") ?? formatFullDayLabel(dayKey);
}

export default function SitInTableRow({ row, zone }: SitInTableRowProps) {
  const isAbsenceRow = Boolean(row.absence && !row.session);
  const studentName = isAbsenceRow
    ? row.absence!.student_name?.trim() || row.absence!.wcode
    : row.visitor!.nickname?.trim() || row.visitor!.student_name?.trim() || row.visitor!.wcode;
  const wcodeLine = isAbsenceRow ? row.absence!.wcode : row.visitor!.wcode;
  const leaving = row.absence
    ? getAbsenceSubjectLabel(row.absence)
    : row.visitor!.from_course_name || row.visitor!.from_course_code;
  const status = row.absence?.status ?? "pending";

  return (
    <tr className="border-b border-b-[var(--color-wi-line)] align-top hover:bg-[var(--color-wi-row-alt)]">
      <td className="px-3 py-3 text-xs text-[var(--color-wi-text-light)]">{row.index}</td>
      <td className="px-3 py-3 text-sm">
        <p className="font-semibold text-[var(--color-wi-text)]">{studentName}</p>
        <p className="text-xs text-[var(--color-wi-text-light)]">{wcodeLine}</p>
      </td>
      <td className="px-3 py-3 text-sm text-[var(--color-wi-text-light)]">{leaving}</td>
      <td className="px-3 py-3 text-sm text-[var(--color-wi-text-light)]">
        {isAbsenceRow ? (
          <p>{getSitInLabel(row.absence!)}</p>
        ) : (
          <>
            <p>{getSessionLabel(row.session!)}</p>
            {row.session!.room_name ? (
              <p className="text-xs text-[var(--color-wi-text-light)]">{row.session!.room_name}</p>
            ) : null}
          </>
        )}
      </td>
      <td className="px-3 py-3 text-sm text-[var(--color-wi-text-light)]">
        {isAbsenceRow ? (
          <>
            <p>{formatAbsenceRange(row.absence!, zone) ?? formatFullDayLabel(row.absence!.date_from)}</p>
            <p className="text-xs text-[var(--color-wi-text-light)]">All day</p>
          </>
        ) : (
          <>
            <p>{formatSessionDay(row.session!, zone)}</p>
            <p className="text-xs text-[var(--color-wi-text-light)]">
              {formatUTCToZone(row.session!.start_at, zone, "HH:mm") ?? formatTime(row.session!.start_at)}
            </p>
          </>
        )}
      </td>
      <td className="px-3 py-3 text-sm text-[var(--color-wi-text-light)]">
        {isAbsenceRow ? titleCase(row.absence!.sit_in_method ?? "physical") : row.absence ? getSitInLabel(row.absence) : "Physical"}
      </td>
      <td className="px-3 py-3">
        <span className={`rounded-full border px-2 py-0.5 text-[10px] font-medium ${statusBadgeClasses(status)}`}>
          {titleCase(status)}
        </span>
      </td>
    </tr>
  );
}