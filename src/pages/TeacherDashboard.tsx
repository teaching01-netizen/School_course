import { useEffect, useMemo, useRef, useState } from 'react';
import { useApiQuery } from '../hooks/useApiQuery';
import type { TeacherDashboardResponse } from '../types';
import DashboardView from '../components/teacher/DashboardView';
import PageHeading from '../components/ui/PageHeading';
import LoadingSkeleton from '../components/ui/LoadingSkeleton';
import useInstituteMeta from '../hooks/useInstituteMeta';
import { shiftZoneMonthKey, startOfZoneMonthKey, utcISOToZoneDate } from '../utils/timezone';

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

export default function TeacherDashboard() {
  const { serverNow, instituteTZ, loaded: instituteMetaLoaded } = useInstituteMeta();
  const zone = instituteTZ ?? 'Asia/Bangkok';
  const fallbackNowIso = useMemo(() => new Date().toISOString(), []);
  const [viewMonthKey, setViewMonthKey] = useState<string | null>(null);
  const [showUpdated, setShowUpdated] = useState(false);
  const hasSeenDataRef = useRef(false);
  const lastUpdatedRef = useRef<number | null>(null);

  const todayKey = useMemo(
    () => utcISOToZoneDate(serverNow ?? fallbackNowIso, zone),
    [fallbackNowIso, serverNow, zone],
  );
  const todayMonthKey = useMemo(
    () => (todayKey ? startOfZoneMonthKey(todayKey, zone) : null),
    [todayKey, zone],
  );

  useEffect(() => {
    if (!instituteMetaLoaded || viewMonthKey !== null) return;
    const initialDayKey = todayKey ?? utcISOToZoneDate(fallbackNowIso, zone);
    const initialMonthKey = initialDayKey ? startOfZoneMonthKey(initialDayKey, zone) : null;
    if (initialMonthKey) {
      setViewMonthKey(initialMonthKey);
    }
  }, [fallbackNowIso, instituteMetaLoaded, todayKey, viewMonthKey, zone]);

  const apiPath = viewMonthKey ? `/api/v1/teacher/dashboard?month_start=${viewMonthKey}` : null;
  const { data, loading, refreshing, error, refetch } = useApiQuery<TeacherDashboardResponse>(apiPath, [apiPath]);

  useEffect(() => {
    if (!data) return;
    const now = Date.now();
    if (hasSeenDataRef.current && lastUpdatedRef.current !== null && now - lastUpdatedRef.current > 500) {
      setShowUpdated(true);
      const timer = setTimeout(() => setShowUpdated(false), 3000);
      return () => clearTimeout(timer);
    }
    hasSeenDataRef.current = true;
    lastUpdatedRef.current = now;
  }, [data]);

  const handlePrevMonth = () => {
    setViewMonthKey((current) => (current ? shiftZoneMonthKey(current, zone, -1) ?? current : current));
  };

  const handleNextMonth = () => {
    setViewMonthKey((current) => (current ? shiftZoneMonthKey(current, zone, 1) ?? current : current));
  };

  const handleToday = () => {
    setViewMonthKey((current) => todayMonthKey ?? current);
  };

  if (!viewMonthKey || (loading && !data)) return <LoadingSkeleton type="table" lines={10} />;

  return (
    <div>
      <div className="mb-4 flex items-center gap-3">
        <PageHeading>Dashboard</PageHeading>
        {showUpdated ? (
          <span className="rounded-sm bg-green-50 px-2 py-0.5 text-[11px] font-medium text-green-700 animate-fade-in">
            Dashboard updated
          </span>
        ) : null}
      </div>

      {error && !loading ? (
        <div className="rounded-sm border border-red-200 bg-red-50 p-4 text-sm text-red-700">
          {friendlyDashboardError(error)}
          <button onClick={() => void refetch()} className="ml-3 underline hover:no-underline">Retry</button>
        </div>
      ) : data ? (
        <DashboardView
          data={data}
          viewMonthKey={viewMonthKey}
          todayKey={todayKey}
          zone={zone}
          loadingNewMonth={refreshing && data != null}
          onPrevMonth={handlePrevMonth}
          onNextMonth={handleNextMonth}
          onToday={handleToday}
        />
      ) : loading ? (
        <LoadingSkeleton type="table" lines={8} />
      ) : null}
    </div>
  );
}
