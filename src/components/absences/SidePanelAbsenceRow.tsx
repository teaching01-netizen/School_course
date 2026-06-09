import { Link } from "react-router-dom";
import type { CalendarAbsence } from "../../types";
import {
  absenceInlineClasses,
  getAbsenceStudentLabel,
  getAbsenceSubjectLabel,
  getSitInLabel,
  statusBadgeClasses,
  titleCase,
} from "./calendarDisplay";

type SidePanelAbsenceRowProps = {
  absence: CalendarAbsence;
};

export default function SidePanelAbsenceRow({ absence }: SidePanelAbsenceRowProps) {
  return (
    <article className={`rounded-sm border border-gray-100 border-l-2 bg-white p-3 text-sm shadow-sm ${absenceInlineClasses(absence)}`}>
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <p className="truncate font-semibold text-gray-900">{getAbsenceStudentLabel(absence)}</p>
          <p className="mt-0.5 truncate text-xs text-amber-700">
            <span className="font-semibold">Leave:</span> {getAbsenceSubjectLabel(absence)}
          </p>
          <p className="truncate text-xs text-sky-700">
            <span className="font-semibold">Sit-in:</span> {getSitInLabel(absence)}
          </p>
        </div>
        <span className={`inline-flex shrink-0 rounded-full border px-2 py-0.5 text-[10px] font-medium ${statusBadgeClasses(absence.status)}`}>
          {titleCase(absence.status)}
        </span>
      </div>
      <div className="mt-3 flex justify-end">
        <Link
          to={`/absences/${absence.id}`}
          aria-label={`View details for ${getAbsenceStudentLabel(absence)}`}
          className="inline-flex min-h-[28px] items-center rounded-sm border border-gray-300 bg-white px-2 py-1 text-xs font-medium text-gray-700 hover:bg-gray-50"
        >
          View details
        </Link>
      </div>
    </article>
  );
}
