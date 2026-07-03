import { useEffect, useMemo, useState } from 'react';
import type { TeacherDashboardResponse } from '../../types';
import CalendarMonth from './CalendarMonth';
import DayPanel from './DayPanel';
import AbsenceRequestTable from './AbsenceRequestTable';
import WeekSummary from './WeekSummary';
import { utcISOToZoneDate } from '../../utils/timezone';

type DashboardViewProps = {
  data: TeacherDashboardResponse;
  viewMonthKey: string;
  todayKey: string | null;
  zone: string;
  loadingNewMonth?: boolean;
  onPrevMonth: () => void;
  onNextMonth: () => void;
  onToday: () => void;
};

export default function DashboardView({
  data,
  viewMonthKey,
  todayKey,
  zone,
  loadingNewMonth,
  onPrevMonth,
  onNextMonth,
  onToday,
}: DashboardViewProps) {
  const [selectedDayKey, setSelectedDayKey] = useState<string | null>(null);
  const [mode, setMode] = useState<'calendar' | 'table'>('calendar');

  useEffect(() => {
    setSelectedDayKey(null);
  }, [viewMonthKey]);

  const dayModalSessions = useMemo(() => {
    if (!selectedDayKey) return [];
    return data.sessions.filter((s) => utcISOToZoneDate(s.start_at, zone) === selectedDayKey);
  }, [data.sessions, selectedDayKey, zone]);

  return (
    <div className="space-y-6">
      <WeekSummary
        totalSessions={data.summary.total_sessions}
        totalAbsences={data.summary.total_absences}
        totalSitIns={data.summary.total_sit_ins}
      />

      <div className="flex gap-4 border-b border-gray-100 text-sm" aria-label="Dashboard view mode">
        <button
          type="button"
          onClick={() => setMode('calendar')}
          className={`min-h-11 border-b-2 px-2 font-medium transition-colors sm:min-h-0 sm:px-1 sm:pb-2 ${
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
          className={`min-h-11 border-b-2 px-2 font-medium transition-colors sm:min-h-0 sm:px-1 sm:pb-2 ${
            mode === 'table'
              ? 'border-[var(--color-wi-primary)] text-[var(--color-wi-primary)]'
              : 'border-transparent text-gray-500 hover:text-gray-900'
          }`}
        >
          Table
        </button>
      </div>

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
              viewMonthKey={viewMonthKey}
              sessions={data.sessions}
              todayKey={todayKey}
              zone={zone}
              selectedDayKey={selectedDayKey}
              onSelectDay={setSelectedDayKey}
              onPrevMonth={onPrevMonth}
              onNextMonth={onNextMonth}
              onToday={onToday}
            />
          </div>
        ) : (
          <AbsenceRequestTable sessions={data.sessions} zone={zone} />
        )}
      </div>

      <div className="h-4" />

      {mode === 'calendar' && selectedDayKey ? (
        <DayPanel
          dateKey={selectedDayKey}
          zone={zone}
          sessions={dayModalSessions}
          onClose={() => setSelectedDayKey(null)}
        />
      ) : null}
    </div>
  );
}
