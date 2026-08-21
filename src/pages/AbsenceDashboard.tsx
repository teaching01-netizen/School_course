import { useEffect, useMemo, useState } from 'react';
import { useApiQuery } from '../hooks/useApiQuery';
import { useToast } from '../hooks/useToast';
import type { TeacherDashboardResponse } from '../types';
import DashboardView from '../components/teacher/DashboardView';
import Button from '../components/ui/Button';
import PageHeading from '../components/ui/PageHeading';
import LoadingSkeleton from '../components/ui/LoadingSkeleton';
import EmptyState from '../components/ui/EmptyState';
import useInstituteMeta from '../hooks/useInstituteMeta';
import { shiftZoneMonthKey, startOfZoneMonthKey, utcISOToZoneDate } from '../utils/timezone';

type Teacher = { id: string; username: string; full_name?: string | null; role: string };

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
  const { serverNow, instituteTZ, loaded: instituteMetaLoaded } = useInstituteMeta();
  const zone = instituteTZ ?? 'Asia/Bangkok';
  const fallbackNowIso = useMemo(() => new Date().toISOString(), []);
  const [viewMonthKey, setViewMonthKey] = useState<string | null>(null);

  const todayKey = useMemo(
    () => utcISOToZoneDate(serverNow ?? fallbackNowIso, zone),
    [fallbackNowIso, serverNow, zone],
  );
  const todayMonthKey = useMemo(
    () => (todayKey ? startOfZoneMonthKey(todayKey, zone) : null),
    [todayKey, zone],
  );
  const activeViewMonthKey = viewMonthKey ?? todayMonthKey ?? startOfZoneMonthKey(todayKey ?? fallbackNowIso.slice(0, 10), zone) ?? '';

  useEffect(() => {
    if (!instituteMetaLoaded || viewMonthKey !== null) return;
    const initialDayKey = todayKey ?? utcISOToZoneDate(fallbackNowIso, zone);
    const initialMonthKey = initialDayKey ? startOfZoneMonthKey(initialDayKey, zone) : null;
    if (initialMonthKey) setViewMonthKey(initialMonthKey);
  }, [fallbackNowIso, instituteMetaLoaded, todayKey, viewMonthKey, zone]);

  const teachersQuery = useApiQuery<Teacher[]>('/api/v1/users?role=Teacher', []);
  const teachers = teachersQuery.data ?? [];
  const teachersLoading = teachersQuery.loading;

  const teacherDashboardPath = selectedTeacherId
    ? `/api/v1/teacher/dashboard?month_start=${activeViewMonthKey}&teacher_id=${selectedTeacherId}`
    : null;

  const { data, loading, error, refetch } = useApiQuery<TeacherDashboardResponse>(teacherDashboardPath, [teacherDashboardPath]);

  useEffect(() => {
    if (teachersQuery.error) {
      addToast('error', teachersQuery.error.message ?? 'Failed to load teachers');
    }
  }, [teachersQuery.error]);

  const handlePrevMonth = () => {
    setViewMonthKey((current) => (current ? shiftZoneMonthKey(current, zone, -1) ?? current : current));
  };

  const handleNextMonth = () => {
    setViewMonthKey((current) => (current ? shiftZoneMonthKey(current, zone, 1) ?? current : current));
  };

  const handleToday = () => {
    setViewMonthKey((current) => todayMonthKey ?? current);
  };

  return (
    <div>
      <div className="mb-4 flex items-start justify-between">
        <PageHeading>Teacher Dashboard</PageHeading>
      </div>

      <div className="mb-4 flex flex-wrap items-center gap-3">
        <div className="flex items-center gap-2">
          <label htmlFor="teacher-select" className="text-sm font-medium text-[var(--color-wi-text-light)] whitespace-nowrap">
            Teacher:
          </label>
          <select
            id="teacher-select"
            value={selectedTeacherId ?? ''}
            onChange={(e) => {
              setSelectedTeacherId(e.target.value || null);
            }}
            className="px-3 py-1.5 text-sm border border-wi-line rounded-sm bg-white min-w-[200px]"
          >
            <option value="">All Teachers</option>
            {teachers.map((t) => (
              <option key={t.id} value={t.id}>
                {t.full_name || t.username}
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
                <tr className="border-b border-wi-line">
                  <th scope="col" className="text-left py-2 px-2 font-semibold">ID</th>
                  <th scope="col" className="text-left py-2 px-2 font-semibold">Name</th>
                  <th scope="col" className="text-left py-2 px-2 font-semibold"></th>
                </tr>
              </thead>
              <tbody>
                {teachers.map((t) => (
                  <tr key={t.id} className="border-b border-wi-line hover:bg-[var(--color-wi-row-alt)]">
                    <td className="py-2 px-2 font-mono text-xs text-[var(--color-wi-text-light)]">{t.id}</td>
                    <td className="py-2 px-2 text-xs text-[var(--color-wi-text)]">{t.full_name || t.username}</td>
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
              viewMonthKey={activeViewMonthKey}
              todayKey={todayKey}
              zone={zone}
              loadingNewMonth={loading && data != null}
              onPrevMonth={handlePrevMonth}
              onNextMonth={handleNextMonth}
              onToday={handleToday}
            />
          ) : loading ? (
            <LoadingSkeleton type="table" lines={8} />
          ) : null}
        </div>
      )}
    </div>
  );
}
