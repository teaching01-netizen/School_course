import type { TeacherDashboardSession } from '../../types';
import { formatUTCToZone } from '../../utils/timezone';

type CalendarDayCellProps = {
  dateKey: string;
  label: string;
  dayNumber: string;
  sessions: TeacherDashboardSession[];
  isToday: boolean;
  isCurrentMonth: boolean;
  isSelected: boolean;
  todayPulse?: number;
  zone: string;
  onClick: () => void;
};

const stateDot = (hasAbsences: boolean, hasVisitors: boolean): string => {
  if (hasAbsences) return 'bg-[var(--color-wi-red)]';
  if (hasVisitors) return 'bg-[var(--color-wi-amber)]';
  return 'bg-[var(--color-wi-green)]';
};

const stateChip = (hasAbsences: boolean, hasVisitors: boolean): string => {
  if (hasAbsences) return 'bg-[var(--color-wi-danger-bg)] text-[var(--color-wi-red-dark)]';
  if (hasVisitors) return 'bg-[var(--color-wi-amber-bg)] text-[var(--color-wi-amber)]';
  return 'bg-[var(--color-wi-green-bg)] text-[var(--color-wi-green-dark)]';
};

export default function CalendarDayCell({
  label,
  dayNumber,
  sessions,
  isToday,
  isCurrentMonth,
  isSelected,
  todayPulse,
  zone,
  onClick,
}: CalendarDayCellProps) {
  const totalAbsences = sessions.reduce((sum, s) => sum + (s.absent_students?.length ?? 0), 0);
  const totalSitIns = sessions.reduce((sum, s) => sum + (s.sit_in_visitors?.length ?? 0), 0);

  const displaySessions = sessions.slice(0, 3);
  const overflow = sessions.length - 3;
  const statusLabel = sessions.length === 0
    ? 'No sessions'
    : `${sessions.length} ${sessions.length === 1 ? 'session' : 'sessions'}, ${totalAbsences} ${totalAbsences === 1 ? 'absence' : 'absences'}, ${totalSitIns} ${totalSitIns === 1 ? 'sit-in' : 'sit-ins'}`;

  return (
    <button
      type="button"
      onClick={onClick}
      aria-label={`${label}. ${statusLabel}`}
      className={`flex min-h-[96px] flex-col border-b border-r border-r-[var(--color-wi-line)] p-1 text-left ${
        isSelected ? 'ring-1 ring-inset ring-[var(--color-wi-primary)]' : ''
      } ${isCurrentMonth ? 'bg-white' : 'bg-[var(--color-wi-row-alt)]'} ${
        todayPulse ? 'animate-today-pulse' : ''
      }`}
    >
      <span
        className={`mb-1 flex h-5 w-full text-[10px] leading-none ${
          isToday
            ? 'items-center justify-center'
            : 'items-center justify-end font-medium text-[var(--color-wi-faint)]'
        }`}
      >
        {isToday ? (
          <span className="flex h-5 w-5 items-center justify-center rounded-full bg-[var(--color-wi-nav)] font-bold text-white">
            {dayNumber}
          </span>
        ) : (
          dayNumber
        )}
      </span>
      <span className="mt-auto flex min-h-3 w-full items-center justify-center gap-px sm:hidden" aria-hidden="true">
        {displaySessions.map((s) => {
          const a = (s.absent_students?.length ?? 0) > 0;
          const v = !a && (s.sit_in_visitors?.length ?? 0) > 0;
          return <span key={s.id} className={`h-1.5 w-1.5 rounded-full ${stateDot(a, v)}`} />;
        })}
        {overflow > 0 ? <span className="text-[8px] leading-none text-[var(--color-wi-text-light)]">+{overflow}</span> : null}
      </span>
      <div className="hidden sm:contents">
        {displaySessions.map((s) => {
          const a = (s.absent_students?.length ?? 0) > 0;
          const v = !a && (s.sit_in_visitors?.length ?? 0) > 0;
          const start = formatUTCToZone(s.start_at, zone, 'HH:mm');
          return (
            <span
              key={s.id}
              className={`mb-1 flex h-[20px] w-full items-center gap-1 overflow-hidden rounded-sm px-1.5 text-[11px] leading-none ${stateChip(a, v)}`}
            >
              <span className={`h-2 w-2 shrink-0 rounded-full ${stateDot(a, v)}`} />
              <span className="shrink-0 tabular-nums text-[10px] opacity-70">{start ?? '--:--'}</span>
              <span className="truncate">{s.subject_name?.trim() || s.course_name}</span>
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
            <span className="text-[10px] text-[var(--color-wi-text-light)] leading-none">
              +{overflow} more{hiddenAbsences > 0 ? ` (${hiddenAbsences} absent)` : ''}
            </span>
          );
        })() : null}
        {(totalAbsences > 0 || totalSitIns > 0) && sessions.length > 0 ? (
          <div className="mt-auto flex items-center gap-2 pt-1">
            {totalAbsences > 0 ? (
              <span className="inline-flex items-center gap-1 text-[10px] leading-none text-[var(--color-wi-red)]">
                <span className="h-1.5 w-1.5 rounded-full bg-[var(--color-wi-red)]" />
                {totalAbsences} {totalAbsences === 1 ? 'absence' : 'absences'}
              </span>
            ) : null}
            {totalSitIns > 0 ? (
              <span className="inline-flex items-center gap-1 text-[10px] leading-none text-[var(--color-wi-amber)]">
                <span className="h-1.5 w-1.5 rounded-full bg-[var(--color-wi-amber)]" />
                {totalSitIns} {totalSitIns === 1 ? 'sit-in' : 'sit-ins'}
              </span>
            ) : null}
          </div>
        ) : null}
      </div>
    </button>
  );
}