import type { CalendarAbsence } from "../../types";
import EmptyState from "../ui/EmptyState";
import {
  formatFullDayLabel,
  getAbsenceStudentLabel,
  getAbsenceSubjectLabel,
  getSitInLabel,
  statusBadgeClasses,
  titleCase,
} from "./calendarDisplay";

type SidePanelStudentDetailProps = {
  absence: CalendarAbsence;
  absences: CalendarAbsence[];
  dayLabel: string;
  onBack: () => void;
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

export default function SidePanelStudentDetail({ absence, absences, dayLabel, onBack }: SidePanelStudentDetailProps) {
  const studentAbsences = absences.filter((item) => item.wcode === absence.wcode);
  const sitInSessions = studentAbsences.flatMap((item) => item.sit_in_sessions ?? []);

  return (
    <div className="space-y-4">
      <button type="button" className="text-xs font-medium text-gray-500 hover:text-gray-900" onClick={onBack}>
        {dayLabel} &gt; {absence.student_name?.trim() || absence.wcode}
      </button>

      <section className="flex items-center gap-3 rounded-sm border border-gray-100 bg-gray-50 p-3">
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-[var(--color-wi-primary)] text-xs font-bold text-white">
          {initials(absence)}
        </div>
        <div className="min-w-0">
          <h3 className="truncate text-sm font-semibold text-gray-900">{getAbsenceStudentLabel(absence)}</h3>
          <p className="text-xs text-gray-500">Focused absence and sit-in history for this calendar range.</p>
        </div>
      </section>

      <section>
        <h4 className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Absence History ({studentAbsences.length})</h4>
        {studentAbsences.length === 0 ? (
          <EmptyState message="No absences recorded for this student in this range." />
        ) : (
          <div className="space-y-2">
            {studentAbsences.map((item) => (
              <article key={item.id} className="rounded-sm border border-gray-100 bg-white p-3 text-sm shadow-sm">
                <div className="flex items-start justify-between gap-2">
                  <div>
                    <p className="font-medium text-gray-900">{getAbsenceSubjectLabel(item)}</p>
                    <p className="text-xs text-gray-500">
                      {formatFullDayLabel(item.date_from)}
                      {item.date_to !== item.date_from ? ` - ${formatFullDayLabel(item.date_to)}` : ""}
                    </p>
                  </div>
                  <span className={`rounded-full border px-2 py-0.5 text-[10px] font-medium ${statusBadgeClasses(item.status)}`}>
                    {titleCase(item.status)}
                  </span>
                </div>
                <p className="mt-2 text-xs text-sky-700">Sit-in: {getSitInLabel(item)}</p>
              </article>
            ))}
          </div>
        )}
      </section>

      <section>
        <h4 className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Upcoming Sit-ins ({sitInSessions.length})</h4>
        {sitInSessions.length === 0 ? (
          <EmptyState message="No assigned sit-in sessions for this student in this range." />
        ) : (
          <div className="space-y-2">
            {sitInSessions.map((session) => (
              <article key={`${session.id}-${session.start_at}`} className="rounded-sm border border-gray-100 bg-white p-3 text-sm shadow-sm">
                <p className="font-medium text-gray-900">{session.subject_name || session.course_name || session.course_code}</p>
                <p className="text-xs text-gray-500">
                  {formatFullDayLabel(session.start_at.slice(0, 10))}
                  {session.room_name ? ` · ${session.room_name}` : ""}
                </p>
              </article>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
