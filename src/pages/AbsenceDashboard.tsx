import { useEffect, useState } from 'react';
import { subMonths, addMonths } from 'date-fns';
import { useApiQuery } from '../hooks/useApiQuery';
import { useToast } from '../hooks/useToast';
import type { TeacherDashboardResponse } from '../types';
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

function friendlyDashboardError(error: unknown): string {
  const msg = typeof error === 'object' && error !== null && 'message' in error
    ? String((error as { message: unknown }).message)
    : '';
  const statusValue = typeof error === 'object' && error !== null && 'status' in error
    ? (error as { status?: unknown }).status
    : undefined;
  const status = typeof statusValue === 'number' ? statusValue : undefined;

  if (status === 401 || status === 403)
    return 'Your session may have expired. Please refresh the page and log in again.';
  if (status === 404)
    return 'Your dashboard data could not be found. Please contact support.';
  if (status !== undefined && status >= 500)
    return 'A server error occurred. Please try again in a few minutes.';

  const m = msg.toLowerCase();
  if (m.includes('timeout') || m.includes('network') || m.includes('fetch'))
    return 'The server took too long to respond. Please check your connection and try again.';
  if (m.includes('unauthorized') || m.includes('forbidden'))
    return 'Your session may have expired. Please refresh the page and log in again.';
  if (m.includes('not found'))
    return 'Your dashboard data could not be found. Please contact support.';
  if (m.includes('internal'))
    return 'A server error occurred. Please try again in a few minutes.';

  return 'Something went wrong while loading your dashboard. Please try again.';
}

export default function AbsenceDashboard() {
  const { addToast } = useToast();
  const [selectedTeacherId, setSelectedTeacherId] = useState<string | null>(null);
  const [viewDate, setViewDate] = useState(() => new Date());
  const [initialLoadDone, setInitialLoadDone] = useState(false);

  const teachersQuery = useApiQuery<Teacher[]>('/api/v1/users?role=Teacher', []);
  const teachers = teachersQuery.data ?? [];
  const teachersLoading = teachersQuery.loading;

  const teacherDashboardPath = selectedTeacherId
    ? `/api/v1/teacher/dashboard?month_start=${yyyyMmDd(viewDate)}&teacher_id=${selectedTeacherId}`
    : null;

  const { data, loading, error, refetch } = useApiQuery<TeacherDashboardResponse>(teacherDashboardPath, [teacherDashboardPath]);

  useEffect(() => {
    if (!loading) {
      setInitialLoadDone(true);
    }
  }, [loading]);

  useEffect(() => {
    if (teachersQuery.error) {
      addToast('error', teachersQuery.error.message ?? 'Failed to load teachers');
    }
  }, [teachersQuery.error]);

  return (
    <div>
      <div className="mb-4 flex items-start justify-between">
        <PageHeading>Teacher Dashboard</PageHeading>
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
              <caption className="sr-only">Teacher dashboard</caption>
              <thead>
                <tr className="border-b-2 border-gray-300">
                  <th scope="col" className="text-left py-2 px-2 font-semibold">ID</th>
                  <th scope="col" className="text-left py-2 px-2 font-semibold">Username</th>
                  <th scope="col" className="text-left py-2 px-2 font-semibold"></th>
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
          {error && !loading ? (
            <div className="rounded-sm border border-red-200 bg-red-50 p-4 text-sm text-red-700">
              {friendlyDashboardError(error)}
              <button onClick={() => void refetch()} className="ml-3 underline hover:no-underline">Retry</button>
            </div>
          ) : data ? (
            <DashboardView
              data={data}
              viewDate={viewDate}
              loadingNewMonth={loading && initialLoadDone}
              onPrevMonth={() => setViewDate((d) => subMonths(d, 1))}
              onNextMonth={() => setViewDate((d) => addMonths(d, 1))}
              onToday={() => setViewDate(new Date())}
            />
          ) : loading ? (
            <LoadingSkeleton type="table" lines={8} />
          ) : null}
        </div>
      )}
    </div>
  );
}
