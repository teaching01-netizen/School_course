import { useEffect, useMemo, useRef, useState } from 'react';
import { addDays, format, startOfWeek } from 'date-fns';
import { useApiQuery } from '../hooks/useApiQuery';
import { useRealtime } from '../hooks/useRealtime';
import { useToast } from '../hooks/useToast';
import type { TeacherDashboardResponse } from '../types';
import WeekNavigator from '../components/teacher/WeekNavigator';
import DashboardView from '../components/teacher/DashboardView';
import PageHeading from '../components/ui/PageHeading';
import LoadingSkeleton from '../components/ui/LoadingSkeleton';

function yyyyMmDd(d: Date): string {
  const year = d.getFullYear();
  const month = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

export default function TeacherDashboard() {
  const { addToast } = useToast();
  const [weekStart, setWeekStart] = useState(() => startOfWeek(new Date(), { weekStartsOn: 1 }));

  const apiPath = `/api/v1/teacher/dashboard?week_start=${yyyyMmDd(weekStart)}`;
  const { data, loading, error, refetch } = useApiQuery<TeacherDashboardResponse>(apiPath, [apiPath]);

  // Real-time subscription: refetch on live attendance events
  const refetchRef = useRef(refetch);
  refetchRef.current = refetch;
  useRealtime(
    ['teacher_dashboard'],
    () => { void refetchRef.current(); },
    { debounceMs: 2000 },
  );

  useEffect(() => {
    if (error) {
      addToast('error', error.message ?? 'Failed to load dashboard');
    }
  }, [error, addToast]);

  const subtitle = useMemo(() => {
    if (!data) return null;
    const weekEnd = addDays(weekStart, 6);
    return `${data.teacher.username} · ${format(weekStart, 'd MMM')} – ${format(weekEnd, 'd MMM yyyy')}`;
  }, [data, weekStart]);

  if (loading && !data) return <LoadingSkeleton type="table" lines={10} />;

  return (
    <div>
      <div className="flex items-start justify-between mb-4">
        <div>
          <PageHeading>Dashboard</PageHeading>
          {subtitle ? <p className="text-sm text-gray-500">{subtitle}</p> : null}
        </div>
        <WeekNavigator weekStart={weekStart} onChange={setWeekStart} />
      </div>

      {error && !loading ? (
        <div className="rounded-sm border border-red-200 bg-red-50 p-4 text-sm text-red-700">
          Failed to load dashboard: {error.message}
          <button onClick={() => void refetch()} className="ml-3 underline hover:no-underline">Retry</button>
        </div>
      ) : data ? (
        <DashboardView data={data} weekStart={weekStart} />
      ) : loading ? (
        <LoadingSkeleton type="table" lines={8} />
      ) : null}
    </div>
  );
}
