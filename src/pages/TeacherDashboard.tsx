import { useCallback, useEffect, useMemo, useState } from 'react';
import { addDays, format, startOfWeek } from 'date-fns';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import { apiJson } from '../api/client';
import { useToast } from '../hooks/useToast';
import type { TeacherDashboardResponse } from '../types';
import TeacherDashboardAlerts from '../components/teacher/TeacherDashboardAlerts';
import TeacherDashboardTable from '../components/teacher/TeacherDashboardTable';
import Button from '../components/ui/Button';
import PageHeading from '../components/ui/PageHeading';
import LoadingSkeleton from '../components/ui/LoadingSkeleton';

type DashboardTab = 'alerts' | 'schedule';

export default function TeacherDashboard() {
  const { addToast } = useToast();
  const [data, setData] = useState<TeacherDashboardResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [tab, setTab] = useState<DashboardTab>('schedule');
  const [weekStart, setWeekStart] = useState(() => startOfWeek(new Date(), { weekStartsOn: 1 }));

  const weekEnd = useMemo(() => addDays(weekStart, 6), [weekStart]);

  const loadData = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const weekStartStr = format(weekStart, 'yyyy-MM-dd');
      const result = await apiJson<TeacherDashboardResponse>(
        `/api/v1/teacher/dashboard?week_start=${weekStartStr}`,
        { method: 'GET' },
      );
      setData(result);
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Failed to load dashboard';
      setError(msg);
      addToast('error', msg);
    } finally {
      setLoading(false);
    }
  }, [weekStart, addToast]);

  useEffect(() => {
    void loadData();
  }, [loadData]);

  if (loading && !data) return <LoadingSkeleton type="table" lines={10} />;

  return (
    <div>
      <div className="flex items-start justify-between mb-4">
        <div>
          <PageHeading>Dashboard</PageHeading>
          {data ? (
            <p className="text-sm text-gray-500">
              {data.teacher.username} &middot; {format(weekStart, 'd MMM')} &ndash;{' '}
              {format(weekEnd, 'd MMM yyyy')}
            </p>
          ) : null}
        </div>
        <div className="flex items-center gap-1.5">
          <Button variant="ghost" size="sm" onClick={() => setWeekStart((prev) => addDays(prev, -7))}>
            <ChevronLeft className="h-4 w-4" />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setWeekStart(startOfWeek(new Date(), { weekStartsOn: 1 }))}
          >
            Today
          </Button>
          <Button variant="ghost" size="sm" onClick={() => setWeekStart((prev) => addDays(prev, 7))}>
            <ChevronRight className="h-4 w-4" />
          </Button>
        </div>
      </div>

      {error ? (
        <div className="rounded-sm border border-red-200 bg-red-50 p-4 text-sm text-red-700">
          Failed to load dashboard: {error}
          <button onClick={() => void loadData()} className="ml-3 underline hover:no-underline">Retry</button>
        </div>
      ) : data ? (
        <>
          <div className="mb-4 rounded-sm border border-gray-200 bg-white px-4 py-2.5 text-sm text-gray-600">
            <strong className="text-gray-900">{data.summary.total_sessions}</strong> sessions
            &middot; <strong className="text-gray-900">{data.summary.total_absences}</strong> absences
            &middot; <strong className="text-gray-900">{data.summary.total_sit_ins}</strong> sit-in visitors
          </div>

          <div className="mb-4 flex rounded-sm border border-gray-300 bg-white text-sm">
            <button
              onClick={() => setTab('alerts')}
              className={`flex items-center gap-1 px-4 py-1.5 ${
                tab === 'alerts'
                  ? 'bg-gray-100 text-gray-900 font-medium'
                  : 'text-gray-500 hover:text-gray-900'
              }`}
            >
              Today's Alerts
            </button>
            <button
              onClick={() => setTab('schedule')}
              className={`flex items-center gap-1 px-4 py-1.5 ${
                tab === 'schedule'
                  ? 'bg-gray-100 text-gray-900 font-medium'
                  : 'text-gray-500 hover:text-gray-900'
              }`}
            >
              Schedule
            </button>
          </div>

          {tab === 'alerts' ? (
            <TeacherDashboardAlerts sessions={data.sessions} />
          ) : (
            <div className="rounded-sm border border-gray-200 bg-white">
              <TeacherDashboardTable sessions={data.sessions} weekStart={weekStart} />
            </div>
          )}

          {data.sessions.length === 0 ? (
            <p className="py-8 text-center text-sm text-gray-400">No sessions this week.</p>
          ) : null}
        </>
      ) : loading ? (
        <LoadingSkeleton type="table" lines={8} />
      ) : null}
    </div>
  );
}
