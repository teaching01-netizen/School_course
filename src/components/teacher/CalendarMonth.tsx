import { useMemo, useState, useCallback, useEffect } from 'react';
import {
  startOfMonth,
  endOfMonth,
  subDays,
  addDays,
  eachDayOfInterval,
  isSameDay,
  isToday as fnsIsToday,
  isSameMonth,
  format,
} from 'date-fns';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import type { TeacherDashboardSession } from '../../types';
import CalendarDayCell from './CalendarDayCell';

type CalendarMonthProps = {
  viewDate: Date;
  sessions: TeacherDashboardSession[];
  selectedDay: Date | null;
  onSelectDay: (day: Date) => void;
  onPrevMonth: () => void;
  onNextMonth: () => void;
  onToday: () => void;
};

const DAY_NAMES = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];

function yyyyMmDd(d: Date): string {
  const year = d.getFullYear();
  const month = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

export default function CalendarMonth({
  viewDate,
  sessions,
  selectedDay,
  onSelectDay,
  onPrevMonth,
  onNextMonth,
  onToday,
}: CalendarMonthProps) {
  const sessionsByDay = useMemo(() => {
    const map = new Map<string, TeacherDashboardSession[]>();
    for (const s of sessions) {
      const key = yyyyMmDd(new Date(s.start_at));
      const existing = map.get(key);
      if (existing) {
        existing.push(s);
      } else {
        map.set(key, [s]);
      }
    }
    return map;
  }, [sessions]);

  const gridDays = useMemo(() => {
    const monthStart = startOfMonth(viewDate);
    const monthEnd = endOfMonth(viewDate);
    const startDow = monthStart.getDay();
    const startPad = startDow === 0 ? 6 : startDow - 1;
    const endDow = monthEnd.getDay();
    const endPad = endDow === 0 ? 0 : 7 - endDow;
    const gridStart = subDays(monthStart, startPad);
    const gridEnd = addDays(monthEnd, endPad);
    return eachDayOfInterval({ start: gridStart, end: gridEnd });
  }, [viewDate]);

  const weeks = useMemo(() => {
    const result: Date[][] = [];
    for (let i = 0; i < gridDays.length; i += 7) {
      result.push(gridDays.slice(i, i + 7));
    }
    return result;
  }, [gridDays]);

  const monthLabel = format(viewDate, 'MMMM yyyy');

  const [todayPulse, setTodayPulse] = useState(0);
  const handleToday = useCallback(() => {
    onToday();
    setTodayPulse((c) => c + 1);
    setTimeout(() => setTodayPulse(0), 1200);
  }, [onToday]);

  // Keyboard navigation: left/right arrows for prev/next month, T for today
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === 'ArrowLeft') { onPrevMonth(); e.preventDefault(); }
      if (e.key === 'ArrowRight') { onNextMonth(); e.preventDefault(); }
      if (e.key === 't' || e.key === 'T') { handleToday(); e.preventDefault(); }
    }
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [onPrevMonth, onNextMonth, handleToday]);

  return (
    <div>
      {/* Header */}
      <div className="mb-4 flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <button
            type="button"
            aria-label="Previous month"
            onClick={onPrevMonth}
            className="flex h-11 w-11 items-center justify-center rounded-sm border border-gray-200 text-gray-500 hover:bg-gray-100 sm:h-6 sm:w-6"
          >
            <ChevronLeft className="h-3.5 w-3.5" />
          </button>
          <h2 className="whitespace-nowrap text-[14px] font-bold text-gray-900 sm:text-[16px]">{monthLabel}</h2>
          <button
            type="button"
            aria-label="Next month"
            onClick={onNextMonth}
            className="flex h-11 w-11 items-center justify-center rounded-sm border border-gray-200 text-gray-500 hover:bg-gray-100 sm:h-6 sm:w-6"
          >
            <ChevronRight className="h-3.5 w-3.5" />
          </button>
        </div>
        <button
          type="button"
          aria-label="Go to today"
          onClick={handleToday}
          className="flex min-h-11 items-center gap-1.5 rounded-sm border border-gray-200 px-3 py-1.5 text-[12px] font-medium text-gray-700 hover:bg-gray-100 sm:min-h-0"
        >
          Today
          <kbd className="hidden sm:inline-flex items-center rounded-sm border border-gray-300 bg-white px-1 py-0.5 text-[10px] font-mono text-gray-400">T</kbd>
        </button>
      </div>

      {/* Day name row */}
      <div className="mb-1.5 grid grid-cols-7 gap-0.5">
        {DAY_NAMES.map((d) => (
          <div key={d} className="py-1.5 text-center text-[10px] font-semibold uppercase tracking-normal text-gray-500 sm:text-[11px] sm:tracking-wider">
            <span className="sm:hidden">{d.slice(0, 1)}</span><span className="hidden sm:inline">{d}</span>
          </div>
        ))}
      </div>

      {/* Week rows */}
      <div className="space-y-0.5">
        {weeks.map((week, wi) => (
          <div key={wi} className="grid grid-cols-7 gap-0.5">
            {week.map((day) => {
              const key = yyyyMmDd(day);
              const daySessions = sessionsByDay.get(key) ?? [];
              return (
                <CalendarDayCell
                  key={key}
                  date={day}
                  sessions={daySessions}
                  isToday={fnsIsToday(day)}
                  isCurrentMonth={isSameMonth(day, viewDate)}
                  isSelected={selectedDay ? isSameDay(day, selectedDay) : false}
                  todayPulse={todayPulse}
                  onClick={() => onSelectDay(day)}
                />
              );
            })}
          </div>
        ))}
      </div>
    </div>
  );
}
