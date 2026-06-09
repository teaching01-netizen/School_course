import type { CalendarAbsence, CalendarSessionBrief, CalendarSitInStudent } from "../../types";
import { formatFullDayLabel, formatTime, getAbsenceSubjectLabel, getSitInLabel, getSessionLabel, statusBadgeClasses, titleCase } from "./calendarDisplay";

export type SitInListRow = {
  id: string;
  index: number;
  visitor: CalendarSitInStudent;
  session: CalendarSessionBrief;
  absence?: CalendarAbsence;
};

type SitInTableRowProps = {
  row: SitInListRow;
};

export default function SitInTableRow({ row }: SitInTableRowProps) {
  const studentName = row.visitor.nickname?.trim() || row.visitor.student_name?.trim() || row.visitor.wcode;
  const leaving = row.absence ? getAbsenceSubjectLabel(row.absence) : row.visitor.from_course_name || row.visitor.from_course_code;
  const status = row.absence?.status ?? "pending";

  return (
    <tr className="border-b border-gray-100 align-top hover:bg-gray-50">
      <td className="px-3 py-3 text-xs text-gray-500">{row.index}</td>
      <td className="px-3 py-3 text-sm">
        <p className="font-semibold text-gray-900">{studentName}</p>
        <p className="text-xs text-gray-500">{row.visitor.wcode}</p>
      </td>
      <td className="px-3 py-3 text-sm text-gray-700">{leaving}</td>
      <td className="px-3 py-3 text-sm text-gray-700">
        <p>{getSessionLabel(row.session)}</p>
        <p className="text-xs text-gray-500">{row.session.room_name ?? "Room TBC"}</p>
      </td>
      <td className="px-3 py-3 text-sm text-gray-700">
        <p>{formatFullDayLabel(row.session.start_at.slice(0, 10))}</p>
        <p className="text-xs text-gray-500">{formatTime(row.session.start_at)}</p>
      </td>
      <td className="px-3 py-3 text-sm text-gray-700">{row.absence ? getSitInLabel(row.absence) : "Physical"}</td>
      <td className="px-3 py-3">
        <span className={`rounded-full border px-2 py-0.5 text-[10px] font-medium ${statusBadgeClasses(status)}`}>
          {titleCase(status)}
        </span>
      </td>
    </tr>
  );
}
