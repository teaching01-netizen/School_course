import { Link } from "react-router-dom";
import type { CalendarAbsence } from "../../types";
import {
  absenceInlineClasses,
  formatFullDayLabel,
  getAbsenceStudentLabel,
  getAbsenceSubjectLabel,
  getSitInLabel,
  statusBadgeClasses,
  titleCase,
} from "./calendarDisplay";

type SidePanelSitInCardProps = {
  absence: CalendarAbsence;
  onViewStudent: (absence: CalendarAbsence) => void;
};

function initials(absence: CalendarAbsence): string {
  const label = absence.student_name?.trim() || absence.wcode;
  return label
    .split(/\s+/)
    .map((part) => part[0])
    .join("")
    .slice(0, 2)
    .toUpperCase();
}

export default function SidePanelSitInCard({ absence, onViewStudent }: SidePanelSitInCardProps) {
  const session = absence.sit_in_sessions?.[0];
  const sessionDetail = session
    ? `${formatFullDayLabel(session.start_at.slice(0, 10))} ${new Date(session.start_at).toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit" })}${session.room_name ? ` · ${session.room_name}` : ""}`
    : getSitInLabel(absence);

  return (
    <article className={`rounded-sm border border-gray-100 border-l-2 bg-white p-3 text-sm shadow-sm ${absenceInlineClasses(absence)}`}>
      <div className="flex items-start gap-3">
        <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-[var(--color-wi-primary)] text-xs font-bold text-white">
          {initials(absence)}
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-start justify-between gap-2">
            <div className="min-w-0">
              <p className="truncate font-semibold text-gray-900">{getAbsenceStudentLabel(absence)}</p>
              <p className="text-xs text-gray-500"><span className="font-semibold">Leave:</span> {getAbsenceSubjectLabel(absence)}</p>
            </div>
            <span className={`inline-flex shrink-0 rounded-full border px-2 py-0.5 text-[10px] font-medium ${statusBadgeClasses(absence.status)}`}>
              {titleCase(absence.status)}
            </span>
          </div>
          <p className="mt-2 text-xs text-gray-700"><span className="font-semibold">Sit-in:</span> → {sessionDetail}</p>
          <div className="mt-2 flex flex-wrap gap-1">
            <span className="rounded-full border border-gray-200 bg-white px-2 py-0.5 text-[10px] text-gray-600">{getSitInLabel(absence)}</span>
            <span className="rounded-full border border-gray-200 bg-white px-2 py-0.5 text-[10px] text-gray-600">{absence.sit_in_method ?? "pending"}</span>
          </div>
          <div className="mt-3 flex items-center justify-between gap-2">
            <button
              type="button"
              className="text-xs font-medium text-[var(--color-wi-primary)] hover:underline"
              onClick={() => onViewStudent(absence)}
            >
              View Student ▸
            </button>
            <Link
              to={`/absences/${absence.id}`}
              aria-label={`View details for ${getAbsenceStudentLabel(absence)}`}
              className="text-xs font-medium text-gray-600 hover:text-gray-900"
            >
              View details
            </Link>
          </div>
        </div>
      </div>
    </article>
  );
}
