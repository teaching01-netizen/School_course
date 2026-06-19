import { useMemo, useState } from 'react';
import type { TeacherDashboardResponse } from '../../types';
import CalendarMonth from './CalendarMonth';
import DayPanel from './DayPanel';
import AbsenceRequestTable from './AbsenceRequestTable';

type DashboardViewProps = {
  data: TeacherDashboardResponse;
  viewDate: Date;
  onPrevMonth: () => void;
  onNextMonth: () => void;
  onToday: () => void;
};

function yyyyMmDd(d: Date): string {
  const year = d.getFullYear();
  const month = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

export default function DashboardView({ data, viewDate, onPrevMonth, onNextMonth, onToday }: DashboardViewProps) {
  const [selectedDay, setSelectedDay] = useState<Date | null>(null);
  const [mode, setMode] = useState<'calendar' | 'table'>('calendar');

  const dayModalSessions = useMemo(() => {
    if (!selectedDay) return [];
    const key = yyyyMmDd(selectedDay);
    return data.sessions.filter((s) => yyyyMmDd(new Date(s.start_at)) === key);
  }, [data.sessions, selectedDay]);

  return (
    <div className="space-y-6">
      {/* Mode tabs */}
      <div className="flex gap-4 text-sm border-b border-gray-100" aria-label="Dashboard view mode">
        <button
          type="button"
          onClick={() => setMode('calendar')}
          className={`border-b-2 px-1 pb-2 font-medium transition-colors ${
            mode === 'calendar'
              ? 'border-[var(--color-wi-primary)] text-[var(--color-wi-primary)]'
              : 'border-transparent text-gray-500 hover:text-gray-900'
          }`}
        >
          Calendar
        </button>
        <button
          type="button"
          onClick={() => setMode('table')}
          className={`border-b-2 px-1 pb-2 font-medium transition-colors ${
            mode === 'table'
              ? 'border-[var(--color-wi-primary)] text-[var(--color-wi-primary)]'
              : 'border-transparent text-gray-500 hover:text-gray-900'
          }`}
        >
          Table
        </button>
      </div>

      {/* Calendar mode */}
      {mode === 'calendar' ? (
        <>
          <CalendarMonth
            viewDate={viewDate}
            sessions={data.sessions}
            selectedDay={selectedDay}
            onSelectDay={setSelectedDay}
            onPrevMonth={onPrevMonth}
            onNextMonth={onNextMonth}
            onToday={onToday}
          />

        </>
      ) : (
        <AbsenceRequestTable sessions={data.sessions} />
      )}

      {/* Bottom spacer */}
      <div className="h-4" />

      {/* Day detail bottom sheet (calendar mode only) */}
      {mode === 'calendar' && selectedDay ? (
        <DayPanel
          date={selectedDay}
          sessions={dayModalSessions}
          onClose={() => setSelectedDay(null)}
        />
      ) : null}
    </div>
  );
}
