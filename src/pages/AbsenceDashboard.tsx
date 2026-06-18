import { useEffect, useMemo, useRef, useState } from 'react';
import { addDays, format, startOfWeek } from 'date-fns';
import { useApiQuery } from '../hooks/useApiQuery';
import { useRealtime } from '../hooks/useRealtime';
import { useToast } from '../hooks/useToast';
import type { TeacherDashboardResponse } from '../types';
import WeekNavigator from '../components/teacher/WeekNavigator';
import DashboardView from '../components/teacher/DashboardView';
import Button from '../components/ui/Button';
import PageHeading from '../components/ui/PageHeading';
import LoadingSkeleton from '../components/ui/LoadingSkeleton';
import EmptyState from '../components/ui/EmptyState';

type Teacher = { id: string; username: string; role: string };

function yyyyMmDd(d: Date): string {
  const year = d.getFullYear();
  const month = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

export default function AbsenceDashboard() {
  const { addToast } = useToast();
  const [selectedTeacherId, setSelectedTeacherId] = useState<string | null>(null);
  const [weekStart, setWeekStart] = useState(() => startOfWeek(new Date(), { weekStartsOn: 1 }));

  const teachersQuery = useApiQuery<Teacher[]>('/api/v1/users?role=Teacher', []);
  const teachers = teachersQuery.data ?? [];
  const teachersLoading = teachersQuery.loading;

  const teacherDashboardPath = selectedTeacherId
    ? `/api/v1/teacher/dashboard?week_start=${yyyyMmDd(weekStart)}&teacher_id=${selectedTeacherId}`
    : null;

  const { data, loading, error, refetch } = useApiQuery<TeacherDashboardResponse>(teacherDashboardPath, [teacherDashboardPath]);

  // Real-time subscription: refetch on live attendance events
  const refetchRef = useRef(refetch);
  refetchRef.current = refetch;
  const rtChannels = selectedTeacherId
    ? ['teacher_dashboard', `teacher_dashboard:${selectedTeacherId}`]
    : [];
  useRealtime(
    rtChannels,
    () => { void refetchRef.current(); },
    { enabled: selectedTeacherId != null, debounceMs: 2000 },
  );

  useEffect(() => {
    if (error) {
      addToast('error', error.message ?? 'Failed to load dashboard');
    }
  }, [error, addToast]);

  useEffect(() => {
    if (teachersQuery.error) {
      addToast('error', teachersQuery.error.message ?? 'Failed to load teachers');
    }
  }, [teachersQuery.error, addToast]);

  const selectedTeacher = useMemo(
    () => teachers.find((t) => t.id === selectedTeacherId),
    [teachers, selectedTeacherId],
  );

  const subtitle = useMemo(() => {
    if (!data || !selectedTeacherId) return null;
    const weekEnd = addDays(weekStart, 6);
    return `${selectedTeacher?.username ?? selectedTeacherId} · ${format(weekStart, 'd MMM')} – ${format(weekEnd, 'd MMM yyyy')}`;
  }, [data, weekStart, selectedTeacher, selectedTeacherId]);

  return (
    <div>
      <div className="flex items-start justify-between mb-4">
        <div className="flex items-center gap-3">
          <PageHeading>Teacher Dashboard</PageHeading>
        </div>
        {selectedTeacherId ? <WeekNavigator weekStart={weekStart} onChange={setWeekStart} /> : null}
      </div>

      <div className="mb-4 flex flex-wrap items-center gap-3">
        <div className="flex items-center gap-2">
          <label htmlFor="teacher-select" className="text-sm font-medium text-gray-700 whitespace-nowrap">
            Teacher:
          </label>
          <select
            id="teacher-select"
            value={selectedTeacherId ?? ''}
            onChange={(e) => {
              setSelectedTeacherId(e.target.value || null);
            }}
            className="px-3 py-1.5 text-sm border border-gray-300 rounded-sm bg-white min-w-[200px]"
          >
            <option value="">All Teachers</option>
            {teachers.map((t) => (
              <option key={t.id} value={t.id}>
                {t.username}
              </option>
            ))}
          </select>
        </div>
      </div>

      {!selectedTeacherId ? (
        teachersLoading ? (
          <LoadingSkeleton type="table" lines={5} />
        ) : teachers.length === 0 ? (
          <EmptyState message="No teachers found." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-[13px]">
              <thead>
                <tr className="border-b-2 border-gray-300">
                  <th className="text-left py-2 px-2 font-semibold">ID</th>
                  <th className="text-left py-2 px-2 font-semibold">Username</th>
                  <th className="text-left py-2 px-2 font-semibold"></th>
                </tr>
              </thead>
              <tbody>
                {teachers.map((t) => (
                  <tr key={t.id} className="border-b border-gray-200 hover:bg-gray-50">
                    <td className="py-2 px-2 font-mono text-xs text-gray-600">{t.id}</td>
                    <td className="py-2 px-2 font-mono text-xs text-gray-600">{t.username}</td>
                    <td className="py-2 px-2">
                      <Button
                        variant="primary"
                        size="sm"
                        onClick={() => setSelectedTeacherId(t.id)}
                      >
                        View Dashboard
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )
      ) : (
        <div>
          {subtitle ? <p className="mb-4 text-sm text-gray-500">{subtitle}</p> : null}

          {error && !loading ? (
            <div className="rounded-sm border border-red-200 bg-red-50 p-4 text-sm text-red-700">
              Failed to load dashboard: {error.message}
              <button onClick={() => void refetch()} className="ml-3 underline hover:no-underline">Retry</button>
            </div>
          ) : data ? (
            <DashboardView data={data} weekStart={weekStart} onBackToToday={() => setWeekStart(startOfWeek(new Date(), { weekStartsOn: 1 }))} />
          ) : loading ? (
            <LoadingSkeleton type="table" lines={8} />
          ) : null}
        </div>
      )}
    </div>
  );
}
