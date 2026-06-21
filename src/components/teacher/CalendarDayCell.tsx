import type { TeacherDashboardSession } from '../../types';

type CalendarDayCellProps = {
  date: Date;
  sessions: TeacherDashboardSession[];
  isToday: boolean;
  isCurrentMonth: boolean;
  isSelected: boolean;
  todayPulse?: number;
  onClick: () => void;
};

function shortTime(iso: string): string {
  const d = new Date(iso);
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
}

export default function CalendarDayCell({ date, sessions, isToday, isCurrentMonth, isSelected, todayPulse, onClick }: CalendarDayCellProps) {
  const hasAbsences = sessions.some((s) => (s.absent_students?.length ?? 0) > 0);
  const hasVisitors = !hasAbsences && sessions.some((s) => (s.sit_in_visitors?.length ?? 0) > 0);

  const totalAbsences = sessions.reduce((sum, s) => sum + (s.absent_students?.length ?? 0), 0);
  const totalSitIns = sessions.reduce((sum, s) => sum + (s.sit_in_visitors?.length ?? 0), 0);

  const displaySessions = sessions.slice(0, 3);
  const overflow = sessions.length - 3;

  let cellAccent: string;
  if (hasAbsences) {
    cellAccent = 'border-l-red-500';
  } else if (hasVisitors) {
    cellAccent = 'border-l-amber-500';
  } else if (sessions.length > 0) {
    cellAccent = 'border-l-green-500';
  } else {
    cellAccent = 'border-l-transparent';
  }

  return (
    <button
      type="button"
      onClick={onClick}
      className={`flex flex-col items-start gap-1 rounded-sm border px-2 py-2 text-left transition-all duration-75 ${
        isSelected
          ? 'border-[var(--color-wi-primary)] ring-1 ring-[var(--color-wi-primary)]'
          : isToday
            ? 'border-gray-200 ring-1 ring-gray-900'
            : 'border-gray-200'
      } ${
        isCurrentMonth ? 'bg-white' : 'bg-gray-50'
      } ${cellAccent} ${isCurrentMonth ? 'hover:border-gray-400' : 'hover:border-gray-300'} active:scale-[0.97] active:bg-gray-100 ${
        todayPulse ? 'animate-today-pulse' : ''
      }`}
      style={{ minHeight: 76 }}
    >
      <span className={`text-[13px] tabular-nums leading-none ${
        isToday ? 'flex h-5 w-5 items-center justify-center rounded-sm bg-gray-900 font-bold text-white' : isCurrentMonth ? 'font-medium text-gray-900' : 'text-gray-300'
      }`}>
        {date.getDate()}
      </span>
      {displaySessions.map((s) => {
        const a = (s.absent_students?.length ?? 0) > 0;
        const v = !a && (s.sit_in_visitors?.length ?? 0) > 0;
        return (
          <span
            key={s.id}
            className={`inline-flex h-[20px] w-full items-center gap-1 overflow-hidden rounded-sm px-1.5 text-[11px] leading-none ${
              a ? 'bg-red-50 text-red-700' : v ? 'bg-amber-50 text-amber-700' : 'bg-green-50 text-green-700'
            }`}
          >
            <span className={`h-2 w-2 shrink-0 rounded-full ${
              a ? 'bg-red-500' : v ? 'bg-amber-500' : 'bg-green-500'
            }`} />
            <span className="shrink-0 tabular-nums text-[10px] opacity-70">{shortTime(s.start_at)}</span>
            <span className="truncate">{s.course_code}</span>
            {a ? <span className="tabular-nums">{s.absent_students!.length}</span> : null}
            {v ? <span className="tabular-nums">{s.sit_in_visitors!.length}</span> : null}
          </span>
        );
      })}
      {overflow > 0 ? (() => {
        const hiddenAbsences = sessions.slice(3).filter(
          (s) => (s.absent_students?.length ?? 0) > 0,
        ).length;
        return (
          <span className="text-[10px] text-gray-400 leading-none">
            +{overflow} more{hiddenAbsences > 0 ? ` (${hiddenAbsences} absent)` : ''}
          </span>
        );
      })() : null}
      {(totalAbsences > 0 || totalSitIns > 0) && sessions.length > 0 ? (
        <div className="mt-auto flex items-center gap-2 pt-1">
          {totalAbsences > 0 ? (
            <span className="inline-flex items-center gap-1 text-[10px] leading-none text-red-600">
              <span className="h-1.5 w-1.5 rounded-full bg-red-500" />
              {totalAbsences} {totalAbsences === 1 ? 'absence' : 'absences'}
            </span>
          ) : null}
          {totalSitIns > 0 ? (
            <span className="inline-flex items-center gap-1 text-[10px] leading-none text-amber-600">
              <span className="h-1.5 w-1.5 rounded-full bg-amber-500" />
              {totalSitIns} {totalSitIns === 1 ? 'sit-in' : 'sit-ins'}
            </span>
          ) : null}
        </div>
      ) : null}
    </button>
  );
}
