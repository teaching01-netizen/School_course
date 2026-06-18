import { useMemo } from 'react';
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

  return (
    <div>
      {/* Header */}
      <div className="mb-3 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <button
            type="button"
            aria-label="Previous month"
            onClick={onPrevMonth}
            className="flex h-6 w-6 items-center justify-center rounded-sm border border-gray-200 text-gray-500 hover:bg-gray-100"
          >
            <ChevronLeft className="h-3.5 w-3.5" />
          </button>
          <h2 className="text-[14px] font-bold text-gray-900">{monthLabel}</h2>
          <button
            type="button"
            aria-label="Next month"
            onClick={onNextMonth}
            className="flex h-6 w-6 items-center justify-center rounded-sm border border-gray-200 text-gray-500 hover:bg-gray-100"
          >
            <ChevronRight className="h-3.5 w-3.5" />
          </button>
        </div>
        <button
          type="button"
          aria-label="Go to today"
          onClick={onToday}
          className="rounded-sm border border-gray-200 px-2 py-1 text-[11px] font-medium text-gray-600 hover:bg-gray-100"
        >
          Today
        </button>
      </div>

      {/* Day name row */}
      <div className="mb-1 grid grid-cols-7 gap-px">
        {DAY_NAMES.map((d) => (
          <div key={d} className="py-1 text-center text-[10px] font-semibold uppercase tracking-wider text-gray-500">
            {d}
          </div>
        ))}
      </div>

      {/* Week rows */}
      <div className="space-y-px">
        {weeks.map((week, wi) => (
          <div key={wi} className="grid grid-cols-7 gap-px">
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
