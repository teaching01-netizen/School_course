import { useMemo, useState, useCallback, useEffect } from 'react';
import { DateTime } from 'luxon';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import type { TeacherDashboardSession } from '../../types';
import CalendarDayCell from './CalendarDayCell';
import { utcISOToZoneDate } from '../../utils/timezone';

type CalendarMonthProps = {
  viewMonthKey: string;
  sessions: TeacherDashboardSession[];
  todayKey: string | null;
  zone: string;
  selectedDayKey: string | null;
  onSelectDay: (dayKey: string) => void;
  onPrevMonth: () => void;
  onNextMonth: () => void;
  onToday: () => void;
};

const DAY_NAMES = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];

export default function CalendarMonth({
  viewMonthKey,
  sessions,
  todayKey,
  zone,
  selectedDayKey,
  onSelectDay,
  onPrevMonth,
  onNextMonth,
  onToday,
}: CalendarMonthProps) {
  const sessionsByDay = useMemo(() => {
    const map = new Map<string, TeacherDashboardSession[]>();
    for (const s of sessions) {
      const key = utcISOToZoneDate(s.start_at, zone);
      if (!key) continue;
      const existing = map.get(key);
      if (existing) {
        existing.push(s);
      } else {
        map.set(key, [s]);
      }
    }
    return map;
  }, [sessions, zone]);

  const viewMonth = useMemo(
    () => DateTime.fromISO(viewMonthKey, { zone }).startOf('month').startOf('day'),
    [viewMonthKey, zone],
  );

  const gridDays = useMemo(() => {
    const startPad = viewMonth.weekday - 1;
    const monthEnd = viewMonth.endOf('month').startOf('day');
    const endPad = monthEnd.weekday === 7 ? 0 : 7 - monthEnd.weekday;
    const gridStart = viewMonth.minus({ days: startPad });
    const gridEnd = monthEnd.plus({ days: endPad });
    const days: DateTime[] = [];
    for (let cursor = gridStart; cursor.toMillis() <= gridEnd.toMillis(); cursor = cursor.plus({ days: 1 })) {
      days.push(cursor);
    }
    return days;
  }, [viewMonth]);

  const weeks = useMemo(() => {
    const result: DateTime[][] = [];
    for (let i = 0; i < gridDays.length; i += 7) {
      result.push(gridDays.slice(i, i + 7));
    }
    return result;
  }, [gridDays]);

  const monthLabel = viewMonth.setLocale('en-GB').toFormat('MMMM yyyy');

  const [todayPulse, setTodayPulse] = useState(0);
  const handleToday = useCallback(() => {
    onToday();
    setTodayPulse((c) => c + 1);
    setTimeout(() => setTodayPulse(0), 1200);
  }, [onToday]);

  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      const target = e.target;
      // Let editors, form controls, and open dialogs handle their own keys.
      if (target instanceof HTMLElement && target.closest('input, textarea, select, [contenteditable="true"], [role="dialog"]')) {
        return;
      }
      if (e.key === 'ArrowLeft') { onPrevMonth(); e.preventDefault(); }
      if (e.key === 'ArrowRight') { onNextMonth(); e.preventDefault(); }
      if (e.key === 't' || e.key === 'T') { handleToday(); e.preventDefault(); }
    }
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [handleToday, onNextMonth, onPrevMonth]);

  return (
    <div>
      <div className="mb-4 flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <button
            type="button"
            aria-label="Previous month"
            onClick={onPrevMonth}
            className="flex h-11 w-11 items-center justify-center rounded-sm border border-[var(--color-wi-line)] text-[var(--color-wi-text-light)] hover:bg-[var(--color-wi-row-alt)] sm:h-6 sm:w-6"
          >
            <ChevronLeft className="h-3.5 w-3.5" />
          </button>
          <h2 className="whitespace-nowrap text-[14px] font-bold text-[var(--color-wi-text)] sm:text-[16px]">{monthLabel}</h2>
          <button
            type="button"
            aria-label="Next month"
            onClick={onNextMonth}
            className="flex h-11 w-11 items-center justify-center rounded-sm border border-[var(--color-wi-line)] text-[var(--color-wi-text-light)] hover:bg-[var(--color-wi-row-alt)] sm:h-6 sm:w-6"
          >
            <ChevronRight className="h-3.5 w-3.5" />
          </button>
        </div>
        <button
          type="button"
          aria-label="Go to today"
          onClick={handleToday}
          className="flex min-h-11 items-center gap-1.5 rounded-sm border border-[var(--color-wi-line)] px-3 py-1.5 text-[12px] font-medium text-[var(--color-wi-text-light)] hover:bg-[var(--color-wi-row-alt)] sm:min-h-0"
        >
          Today
          <kbd className="hidden sm:inline-flex items-center rounded-sm border border-[var(--color-wi-line)] bg-white px-1 py-0.5 text-[10px] font-mono text-[var(--color-wi-text-light)]">T</kbd>
        </button>
      </div>

      <div className="mb-3 flex items-center justify-center gap-3 text-[11px] text-[var(--color-wi-text-light)]">
        <span className="flex items-center gap-1">
          <span className="h-2 w-2 rounded-full bg-[var(--color-wi-red)]" />
          Has absences
        </span>
        <span className="flex items-center gap-1">
          <span className="h-2 w-2 rounded-full bg-[var(--color-wi-amber)]" />
          Has sit-ins
        </span>
        <span className="flex items-center gap-1">
          <span className="h-2 w-2 rounded-full bg-[var(--color-wi-green)]" />
          All OK
        </span>
      </div>

      <div className="overflow-hidden rounded-sm border border-[var(--color-wi-line)] bg-white">
        <div className="grid grid-cols-7 border-b border-b-[var(--color-wi-line)] bg-[var(--color-wi-row-alt)] text-center text-[10px] font-semibold uppercase tracking-normal text-[var(--color-wi-text-light)] sm:tracking-wider">
          {DAY_NAMES.map((d) => (
            <div key={d} className="py-1.5">
              <span className="sm:hidden">{d.slice(0, 1)}</span><span className="hidden sm:inline">{d}</span>
            </div>
          ))}
        </div>
        {weeks.map((week, wi) => (
          <div key={wi} className="grid grid-cols-7">
            {week.map((day) => {
              const key = day.toFormat('yyyy-MM-dd');
              const daySessions = sessionsByDay.get(key) ?? [];
              const label = day.setLocale('en-GB').toFormat('EEEE, d MMMM yyyy');
              return (
                <CalendarDayCell
                  key={key}
                  dateKey={key}
                  label={label}
                  dayNumber={String(day.day)}
                  sessions={daySessions}
                  isToday={todayKey === key}
                  isCurrentMonth={day.month === viewMonth.month && day.year === viewMonth.year}
                  isSelected={selectedDayKey === key}
                  todayPulse={todayPulse}
                  zone={zone}
                  onClick={() => onSelectDay(key)}
                />
              );
            })}
          </div>
        ))}
      </div>
    </div>
  );
}