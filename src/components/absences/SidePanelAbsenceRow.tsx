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
    <article className={`rounded-sm border var(--color-wi-line) border-l-2 bg-white p-3 text-sm shadow-sm ${absenceInlineClasses(absence)}`}>
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <p className="truncate font-semibold text-[var(--color-wi-text)]">{getAbsenceStudentLabel(absence)}</p>
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
          className="inline-flex min-h-[28px] items-center rounded-sm border var(--color-wi-line) bg-white px-2 py-1 text-xs font-medium text-[var(--color-wi-text-light)] hover:bg-[var(--color-wi-row-alt)]"
        >
          View details
        </Link>
      </div>
    </article>
  );
}
