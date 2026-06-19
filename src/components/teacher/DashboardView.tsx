import { useMemo, useState } from 'react';
import { format } from 'date-fns';
import { Link } from 'react-router-dom';
import type { TeacherDashboardResponse } from '../../types';
import CalendarMonth from './CalendarMonth';
import WeekSummary from './WeekSummary';
import DayPanel from './DayPanel';
import PendingAbsenceTable from './PendingAbsenceTable';
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

  const today = useMemo(() => new Date(), []);
  const todayDateStr = useMemo(() => yyyyMmDd(today), [today]);

  const todaySessions = useMemo(() => {
    return data.sessions.filter((s) => yyyyMmDd(new Date(s.start_at)) === todayDateStr);
  }, [data.sessions, todayDateStr]);

  const dayModalSessions = useMemo(() => {
    if (!selectedDay) return [];
    const key = yyyyMmDd(selectedDay);
    return data.sessions.filter((s) => yyyyMmDd(new Date(s.start_at)) === key);
  }, [data.sessions, selectedDay]);

  const needsAttention = useMemo(() => {
    return data.sessions
      .filter((s) => (s.absent_students?.length ?? 0) > 0)
      .flatMap((s) =>
        (s.absent_students ?? []).map((a) => ({
          ...a,
          sessionSubject: s.subject_name ?? s.course_name,
          sessionStart: s.start_at,
          courseId: s.course_id,
        })),
      )
      .slice(0, 5);
  }, [data.sessions]);

  const sortedToday = [...todaySessions].sort(
    (a, b) => new Date(a.start_at).getTime() - new Date(b.start_at).getTime(),
  );

  const pendingCount = data.pending_absence_requests?.length ?? 0;

  return (
    <div className="space-y-5">
      {/* Pending summary banner */}
      {pendingCount > 0 ? (
        <a
          href="#pending-requests"
          className="flex items-center justify-between rounded-sm border border-amber-200 bg-amber-50 px-3 py-2 text-sm no-underline hover:border-amber-300"
        >
          <span className="font-medium text-amber-800">{pendingCount} pending {pendingCount === 1 ? 'request' : 'requests'}</span>
          <span className="text-amber-600 text-xs">Review →</span>
        </a>
      ) : null}

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

          <div className="h-px bg-gray-100" />

          <div>
            <h4 className="mb-2 text-[11px] font-semibold uppercase tracking-wider text-gray-500">This Week</h4>
            <WeekSummary
              totalSessions={data.summary.total_sessions}
              totalAbsences={data.summary.total_absences}
              totalSitIns={data.summary.total_sit_ins}
            />
          </div>

          <div className="h-px bg-gray-100" />

          <div>
            <h4 className="mb-2 text-[11px] font-semibold uppercase tracking-wider text-gray-500">
              Today &middot; {format(today, 'EEE, d MMM')}
            </h4>
            {sortedToday.length === 0 ? (
              <p className="text-[12px] text-gray-400">No sessions today.</p>
            ) : (
              <div className="space-y-px">
                {sortedToday.map((s) => {
                  const a = (s.absent_students?.length ?? 0) > 0;
                  const v = !a && (s.sit_in_visitors?.length ?? 0) > 0;
                  return (
                    <button
                      key={s.id}
                      type="button"
                      onClick={() => setSelectedDay(new Date(s.start_at))}
                      className="flex w-full items-center gap-3 rounded-sm border border-gray-200 bg-white px-3 py-2 text-left hover:border-gray-300"
                    >
                      <span className="shrink-0 w-[52px] text-[12px] font-medium tabular-nums text-gray-900">
                        {format(new Date(s.start_at), 'HH:mm')}
                      </span>
                      <span className="min-w-0 flex-1 truncate text-[13px] text-gray-800">
                        {s.subject_name ?? s.course_name}
                      </span>
                      {a ? (
                        <span className="flex items-center gap-1 shrink-0 text-[12px] text-red-600">
                          <span className="h-1.5 w-1.5 rounded-full bg-red-500" />
                          {s.absent_students!.length}
                        </span>
                      ) : v ? (
                        <span className="flex items-center gap-1 shrink-0 text-[12px] text-amber-600">
                          <span className="h-1.5 w-1.5 rounded-full bg-amber-500" />
                          {s.sit_in_visitors!.length}
                        </span>
                      ) : (
                        <span className="flex items-center gap-1 shrink-0 text-[12px] text-green-600">
                          <span className="h-1.5 w-1.5 rounded-full bg-green-500" />
                          OK
                        </span>
                      )}
                    </button>
                  );
                })}
              </div>
            )}
          </div>

          <div className="h-px bg-gray-100" />

          <div>
            <h4 className="mb-2 text-[11px] font-semibold uppercase tracking-wider text-gray-500">Need Attention</h4>
            {needsAttention.length === 0 ? (
              <p className="text-[12px] text-gray-400">All clear.</p>
            ) : (
              <div className="space-y-px">
                {needsAttention.map((a) => (
                  <div key={a.absence_id} className="flex items-center gap-2 rounded-sm border border-gray-200 bg-white px-3 py-2">
                    <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-red-500" />
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-[13px] font-medium text-gray-800">
                        {a.nickname ?? a.student_name ?? a.wcode}
                      </p>
                      <p className="truncate text-[11px] text-gray-500">{a.sessionSubject}</p>
                    </div>
                    <Link
                      to={`/absences/${a.absence_id}`}
                      className="shrink-0 text-[12px] font-medium text-[var(--color-wi-primary)] hover:underline"
                    >
                      View
                    </Link>
                  </div>
                ))}
              </div>
            )}
          </div>
        </>
      ) : (
        <AbsenceRequestTable sessions={data.sessions} pendingRequests={data.pending_absence_requests} />
      )}

      {/* Pending Absence Requests */}
      <div className="h-px bg-gray-100" />
      <div id="pending-requests">
        <h4 className="mb-2 text-[11px] font-semibold uppercase tracking-wider text-gray-500">
          Pending Requests
          {pendingCount > 0 ? (
            <span className="ml-1.5 inline-flex min-w-5 items-center justify-center rounded-full bg-amber-500 px-1.5 py-0.5 text-[10px] font-semibold text-white">
              {pendingCount}
            </span>
          ) : null}
        </h4>
        <PendingAbsenceTable requests={data.pending_absence_requests} />
      </div>

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
