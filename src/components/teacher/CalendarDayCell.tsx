import type { TeacherDashboardSession } from '../../types';

type CalendarDayCellProps = {
  date: Date;
  sessions: TeacherDashboardSession[];
  isToday: boolean;
  isCurrentMonth: boolean;
  isSelected: boolean;
  onClick: () => void;
};

export default function CalendarDayCell({ date, sessions, isToday, isCurrentMonth, isSelected, onClick }: CalendarDayCellProps) {
  const hasAbsences = sessions.some((s) => (s.absent_students?.length ?? 0) > 0);
  const hasVisitors = !hasAbsences && sessions.some((s) => (s.sit_in_visitors?.length ?? 0) > 0);

  const displaySessions = sessions.slice(0, 2);
  const overflow = sessions.length - 2;

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
      className={`flex flex-col items-start gap-0.5 rounded-sm border px-1.5 py-1.5 text-left transition-colors ${
        isSelected
          ? 'border-[var(--color-wi-primary)] ring-1 ring-[var(--color-wi-primary)]'
          : isToday
            ? 'border-gray-900'
            : 'border-gray-200'
      } ${
        isCurrentMonth ? 'bg-white' : 'bg-gray-50'
      } ${cellAccent} ${isCurrentMonth ? 'hover:border-gray-400' : 'hover:border-gray-300'}`}
      style={{ minHeight: 52 }}
    >
      <span className={`text-[11px] tabular-nums leading-none ${
        isToday ? 'flex h-4 w-4 items-center justify-center rounded-sm bg-gray-900 font-bold text-white' : isCurrentMonth ? 'font-medium text-gray-900' : 'text-gray-300'
      }`}>
        {date.getDate()}
      </span>
      {displaySessions.map((s) => {
        const a = (s.absent_students?.length ?? 0) > 0;
        const v = !a && (s.sit_in_visitors?.length ?? 0) > 0;
        return (
          <span
            key={s.id}
            className={`inline-flex h-[18px] w-full items-center gap-0.5 overflow-hidden rounded-sm px-1 text-[10px] leading-none ${
              a ? 'bg-red-50 text-red-700' : v ? 'bg-amber-50 text-amber-700' : 'bg-green-50 text-green-700'
            }`}
          >
            <span className={`h-1.5 w-1.5 shrink-0 rounded-full ${
              a ? 'bg-red-500' : v ? 'bg-amber-500' : 'bg-green-500'
            }`} />
            <span className="truncate">{s.subject_name ?? s.course_name}</span>
            {a ? <span className="tabular-nums">{s.absent_students!.length}</span> : null}
            {v ? <span className="tabular-nums">{s.sit_in_visitors!.length}</span> : null}
          </span>
        );
      })}
      {overflow > 0 ? (
        <span className="text-[10px] text-gray-400 leading-none">+{overflow} more</span>
      ) : null}
    </button>
  );
}
