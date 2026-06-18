import { format } from 'date-fns';
import type { TeacherDashboardSession } from '../../types';

type DashboardSessionCellProps = {
  session: TeacherDashboardSession;
};

export default function DashboardSessionCell({ session }: DashboardSessionCellProps) {
  const hasAbsences = session.absent_count > 0;
  const sitInVisitors = session.sit_in_visitors ?? [];
  const hasSitIns = sitInVisitors.length > 0;

  let accentColor: string;
  if (hasAbsences && hasSitIns) {
    accentColor = 'border-l-red-500';
  } else if (hasAbsences) {
    accentColor = 'border-l-red-400';
  } else if (hasSitIns) {
    accentColor = 'border-l-amber-400';
  } else {
    accentColor = 'border-l-[var(--color-wi-primary)]';
  }

  return (
    <div
      className={`h-full rounded-sm border border-gray-200 bg-white shadow-sm
        border-l-4 ${accentColor} px-2 py-1.5 text-[11px] leading-snug
        flex flex-col justify-center min-h-[44px]`}
    >
      <div className="flex items-start justify-between gap-1">
        <p className="font-semibold text-gray-900 truncate">{session.course_code}</p>
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
        {session.room_name ? `${session.room_name} · ` : null}
        {format(new Date(session.start_at), 'HH:mm')}–{format(new Date(session.end_at), 'HH:mm')}
      </p>
    </div>
  );
}
