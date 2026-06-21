import { useMemo, useState } from 'react';
import type { TeacherDashboardResponse } from '../../types';
import CalendarMonth from './CalendarMonth';
import DayPanel from './DayPanel';
import AbsenceRequestTable from './AbsenceRequestTable';
import WeekSummary from './WeekSummary';

type DashboardViewProps = {
  data: TeacherDashboardResponse;
  viewDate: Date;
  loadingNewMonth?: boolean;
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

export default function DashboardView({ data, viewDate, loadingNewMonth, onPrevMonth, onNextMonth, onToday }: DashboardViewProps) {
  const [selectedDay, setSelectedDay] = useState<Date | null>(null);
  const [mode, setMode] = useState<'calendar' | 'table'>('calendar');

  const dayModalSessions = useMemo(() => {
    if (!selectedDay) return [];
    const key = yyyyMmDd(selectedDay);
    return data.sessions.filter((s) => yyyyMmDd(new Date(s.start_at)) === key);
  }, [data.sessions, selectedDay]);

  return (
    <div className="space-y-6">
      {/* Week summary */}
      <WeekSummary
        totalSessions={data.summary.total_sessions}
        totalAbsences={data.summary.total_absences}
        totalSitIns={data.summary.total_sit_ins}
      />

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

      {/* Calendar or table with fade-in on mode switch */}
      <div key={mode} className="animate-fade-in">
        {mode === 'calendar' ? (
          <div className="relative">
            {loadingNewMonth ? (
              <div className="absolute inset-0 z-10 flex items-center justify-center rounded-sm bg-white/70">
                <div className="flex items-center gap-2 text-sm text-gray-500">
                  <svg className="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none">
                    <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                    <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                  </svg>
                  Loading…
                </div>
              </div>
            ) : null}
            <CalendarMonth
              viewDate={viewDate}
              sessions={data.sessions}
              selectedDay={selectedDay}
              onSelectDay={setSelectedDay}
              onPrevMonth={onPrevMonth}
              onNextMonth={onNextMonth}
              onToday={onToday}
            />
          </div>
        ) : (
          <AbsenceRequestTable sessions={data.sessions} />
        )}
      </div>

      {/* Bottom spacer */}
      <div className="h-4" />

      {/* Day detail modal (calendar mode only) */}
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
