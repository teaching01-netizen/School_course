import { format } from 'date-fns';
import type { TeacherDashboardSession } from '../../types';

type DashboardSessionCellProps = {
  session: TeacherDashboardSession;
};

function tooltipContent(session: TeacherDashboardSession): string {
  const parts: string[] = [];
  const absStudents = session.absent_students ?? [];

  if (absStudents.length > 0) {
    const names = absStudents.slice(0, 5).map((s) => s.nickname ?? s.student_name ?? s.wcode);
    const label = names.join(', ');
    const remaining = absStudents.length - 5;
    parts.push(`Absent: ${label}${remaining > 0 ? ` …and ${remaining} more` : ''}`);
  }

  const sitInVisitors = session.sit_in_visitors ?? [];
  if (sitInVisitors.length > 0) {
    const names = sitInVisitors.slice(0, 3).map((v) => `${v.nickname ?? v.student_name ?? v.wcode} (from ${v.from_subject_name ?? v.from_course_code})`);
    const label = names.join(', ');
    const remaining = sitInVisitors.length - 3;
    parts.push(`Sit-in: ${label}${remaining > 0 ? ` …and ${remaining} more` : ''}`);
  }

  if (parts.length === 0) return 'All clear';
  return parts.join('\n');
}

export default function DashboardSessionCell({ session }: DashboardSessionCellProps) {
  const absStudents = session.absent_students ?? [];
  const sitInVisitors = session.sit_in_visitors ?? [];
  const hasAbsences = absStudents.length > 0;
  const hasSitIns = sitInVisitors.length > 0;

  let accentColor: string;
  if (hasAbsences && hasSitIns) {
    accentColor = 'border-l-red-500';
  } else if (hasAbsences) {
    accentColor = 'border-l-red-400';
  } else if (hasSitIns) {
    accentColor = 'border-l-amber-400';
  } else {
    accentColor = 'border-l-green-400';
  }

  const displayName = session.subject_name ?? session.course_name;

  return (
    <div
      className={`group relative h-full rounded-sm border border-gray-200 bg-white shadow-sm
        border-l-4 ${accentColor} px-2 py-1.5 text-[11px] leading-snug
        flex flex-col justify-center min-h-[44px]`}
      title={tooltipContent(session)}
    >
      <div className="flex items-start justify-between gap-1">
        <p className="font-semibold text-gray-900 truncate">{displayName}</p>
        <div className="flex items-center gap-1 shrink-0">
          {hasAbsences ? (
            <span className="inline-flex items-center rounded-sm bg-red-100 px-1 py-0.5 text-[10px] font-semibold text-red-700">
              {session.absent_count}
            </span>
          ) : null}
          {hasSitIns ? (
            <span className="inline-flex items-center rounded-sm bg-blue-100 px-1 py-0.5 text-[10px] font-semibold text-blue-700">
              {sitInVisitors.length}
            </span>
          ) : null}
        </div>
      </div>
      <p className="text-gray-400 truncate">
        {session.course_code}{session.room_name ? ` · ${session.room_name}` : null}
        {' · '}
        {format(new Date(session.start_at), 'HH:mm')}–{format(new Date(session.end_at), 'HH:mm')}
      </p>
    </div>
  );
}
