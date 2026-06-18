import { format } from 'date-fns';
import { Link } from 'react-router-dom';
import { ExternalLink } from 'lucide-react';
import type { TeacherDashboardSession } from '../../types';

type DayTimelineProps = {
  sessions: TeacherDashboardSession[];
};

export default function DayTimeline({ sessions }: DayTimelineProps) {
  if (sessions.length === 0) {
    return (
      <div className="rounded border border-gray-200 bg-white px-4 py-3 text-[13px] text-gray-400">
        No sessions today.
      </div>
    );
  }

  return (
    <div className="rounded border border-gray-200 bg-white">
      {sessions.map((s, i) => {
        const absCount = (s.absent_students ?? []).length;
        const sitInCount = (s.sit_in_visitors ?? []).length;
        const hasAbsences = absCount > 0;
        const hasSitInsOnly = !hasAbsences && sitInCount > 0;
        const allClear = !hasAbsences && !hasSitInsOnly;

        let dotColor: string;
        let statusLabel: string;
        if (allClear) {
          dotColor = 'bg-green-500';
          statusLabel = 'All clear';
        } else if (hasAbsences) {
          dotColor = 'bg-red-500';
          statusLabel = `${absCount} absent`;
        } else {
          dotColor = 'bg-amber-500';
          statusLabel = `${sitInCount} sit-in`;
        }

        const names = hasAbsences
          ? (s.absent_students ?? []).slice(0, 2).map((a) => a.nickname ?? a.student_name ?? a.wcode)
          : hasSitInsOnly
            ? (s.sit_in_visitors ?? []).slice(0, 2).map((v) => v.nickname ?? v.student_name ?? v.wcode)
            : [];

        const nameStr = names.length > 0
          ? names.join(', ') + ((hasAbsences ? (s.absent_students ?? []).length : (s.sit_in_visitors ?? []).length) > 2 ? '…' : '')
          : null;

        const displayName = s.subject_name ?? s.course_name;
        const firstAbsenceId = hasAbsences
          ? s.absent_students?.[0]?.absence_id
          : hasSitInsOnly
            ? s.sit_in_visitors?.[0]?.absence_id
            : null;

        return (
          <div
            key={s.id}
            className={`flex items-center gap-3 px-4 py-2.5 ${i < sessions.length - 1 ? 'border-b border-gray-50' : ''}`}
          >
            {/* Time */}
            <span className="shrink-0 w-[70px] text-[12px] text-gray-500 tabular-nums">
              {format(new Date(s.start_at), 'HH:mm')}–{format(new Date(s.end_at), 'HH:mm')}
            </span>

            {/* Subject */}
            <span className="text-[13px] font-medium text-gray-800 min-w-0 truncate w-[100px] shrink-0">
              {displayName}
            </span>

            {/* Status dot + label */}
            <span className="flex items-center gap-1 shrink-0 w-[80px]">
              <span className={`h-2 w-2 shrink-0 rounded-full ${dotColor}`} />
              <span className={`text-[12px] ${
                allClear ? 'text-green-600' : hasAbsences ? 'text-red-600' : 'text-amber-600'
              }`}>
                {statusLabel}
              </span>
            </span>

            {/* Student names */}
            <span className="text-[12px] text-gray-500 truncate min-w-0 flex-1">
              {nameStr ?? ''}
            </span>

            {/* View link */}
            {firstAbsenceId ? (
              <Link
                to={`/absences/${firstAbsenceId}`}
                className="shrink-0 text-[12px] font-medium text-[var(--color-wi-primary)] hover:underline"
              >
                View <ExternalLink className="inline h-3 w-3" />
              </Link>
            ) : (
              <span className="shrink-0 w-[42px]" />
            )}
          </div>
        );
      })}
    </div>
  );
}
