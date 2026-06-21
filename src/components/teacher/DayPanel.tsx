import { useEffect, useMemo, useRef } from 'react';
import { useQueries } from '@tanstack/react-query';
import { format, parseISO } from 'date-fns';
import { ExternalLink, X } from 'lucide-react';
import { Link } from 'react-router-dom';
import { apiJson } from '../../api/client';
import type { TeacherAbsenceDetail as TeacherAbsenceDetailData, TeacherDashboardSession } from '../../types';
import { cachePolicies, queryClient, queryKeys } from '../../query/cache';

type DayPanelProps = {
  date: Date;
  sessions: TeacherDashboardSession[];
  onClose: () => void;
};

type ReasonPreviewState = {
  status: 'loading' | 'ready' | 'error';
  reason: string | null;
};

function trimReason(reason: string | null | undefined): string | null {
  const trimmed = reason?.trim() ?? '';
  return trimmed.length > 0 ? trimmed : null;
}

function ReasonText({ absenceId, reasonStates }: { absenceId: string; reasonStates: Record<string, ReasonPreviewState> }) {
  const state = reasonStates[absenceId];
  if (!state || state.status === 'loading') {
    return <span className="text-gray-400">Loading reason…</span>;
  }
  if (state.status === 'error') {
    return <span className="text-gray-400">Reason unavailable</span>;
  }

  const reason = trimReason(state.reason);
  return (
    <span className="min-w-0 truncate text-gray-700" title={reason ?? undefined}>
      {reason ?? 'No reason provided'}
    </span>
  );
}

export default function DayPanel({ date, sessions, onClose }: DayPanelProps) {
  const sorted = useMemo(
    () => [...sessions].sort((a, b) => new Date(a.start_at).getTime() - new Date(b.start_at).getTime()),
    [sessions],
  );

  const totalAbsences = sorted.reduce((s, sess) => s + (sess.absent_students?.length ?? 0), 0);
  const totalSitIns = sorted.reduce((s, sess) => s + (sess.sit_in_visitors?.length ?? 0), 0);
  const absenceIds = useMemo(() => {
    const ids = new Set<string>();
    for (const session of sorted) {
      for (const absent of session.absent_students ?? []) {
        ids.add(absent.absence_id);
      }
      for (const visitor of session.sit_in_visitors ?? []) {
        ids.add(visitor.absence_id);
      }
    }
    return [...ids];
  }, [sorted]);

  const panelRef = useRef<HTMLDivElement>(null);
  const reasonQueries = useQueries({
    queries: absenceIds.map((absenceId) => ({
      queryKey: queryKeys.absences.teacherDetail(absenceId),
      queryFn: () => apiJson<TeacherAbsenceDetailData>(`/api/v1/teacher/absences/${absenceId}`, { method: 'GET' }),
      ...cachePolicies.sensitiveDetail,
      retry: false,
    })),
  }, queryClient);
  const reasonStates: Record<string, ReasonPreviewState> = {};
  for (let index = 0; index < absenceIds.length; index += 1) {
    const query = reasonQueries[index];
    reasonStates[absenceIds[index]] = query.isError
      ? { status: 'error', reason: null }
      : query.data
        ? { status: 'ready', reason: trimReason(query.data.reason) }
        : { status: 'loading', reason: null };
  }

  // Trap focus inside modal on mount, restore on unmount
  useEffect(() => {
    const panel = panelRef.current;
    if (!panel) return;
    const closeBtn = panel.querySelector<HTMLButtonElement>('[data-panel-close]');
    closeBtn?.focus();

    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') { onClose(); return; }
      if (e.key !== 'Tab' || !panel) return;
      const focusable = panel.querySelectorAll<HTMLElement>(
        'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
      );
      if (focusable.length === 0) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    }

    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [onClose]);

  // Remember the previously focused element to restore on close
  const prevFocus = useRef<HTMLElement | null>(null);
  useEffect(() => {
    prevFocus.current = document.activeElement as HTMLElement;
    return () => { prevFocus.current?.focus(); };
  }, []);

  return (
    <>
      {/* Backdrop */}
      <div className="fixed inset-0 z-40 bg-black/15" onClick={onClose} aria-hidden="true" />

      {/* Modal */}
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-label={`Session details for ${format(date, 'EEEE, d MMMM yyyy')}`}
        className="fixed inset-0 m-auto z-50 flex max-h-[70vh] w-[92vw] max-w-[560px] flex-col rounded-lg bg-white shadow-lg"
      >

        {/* Header */}
        <div className="flex items-center justify-between border-b border-gray-100 px-4 py-3">
          <div>
            <h3 className="text-[14px] font-bold text-gray-900">{format(date, 'EEEE, d MMMM yyyy')}</h3>
            {sessions.length > 0 ? (
              <p className="text-[11px] text-gray-500">
                {sessions.length} {sessions.length === 1 ? 'session' : 'sessions'}
                {totalAbsences > 0 ? ` · ${totalAbsences} ${totalAbsences === 1 ? 'absence' : 'absences'}` : ''}
                {totalSitIns > 0 ? ` · ${totalSitIns} ${totalSitIns === 1 ? 'sit-in' : 'sit-ins'}` : ''}
              </p>
            ) : null}
          </div>
          <button
            type="button"
            data-panel-close
            onClick={onClose}
            className="flex h-7 w-7 items-center justify-center rounded-sm text-gray-400 hover:bg-gray-100 hover:text-gray-600"
            aria-label="Close panel"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* Sessions */}
        <div className="overflow-y-auto px-4 py-3">
          {sessions.length === 0 ? (
            <div className="py-8 text-center text-[13px] text-gray-400">No sessions on this day.</div>
          ) : (
            <div className="space-y-4">
              {sorted.map((s) => {
                const start = new Date(s.start_at);
                const end = new Date(s.end_at);
                const absences = s.absent_students ?? [];
                const visitors = s.sit_in_visitors ?? [];
                const label = s.subject_name ?? s.course_name;

                return (
                  <div key={s.id} className="rounded-sm border border-gray-200 bg-white px-3 py-2.5">
                    {/* Session header */}
                    <div className="flex items-center justify-between">
                      <div className="flex items-baseline gap-1.5">
                        <span className="text-[12px] font-semibold text-gray-900 tabular-nums">
                          {format(start, 'HH:mm')}–{format(end, 'HH:mm')}
                        </span>
                        <span className="text-[14px] font-bold text-gray-800">{label}</span>
                      </div>
                    </div>

                    {/* Absences */}
                    {absences.length > 0 ? (
                      <div className="mt-1.5 space-y-1">
                        {absences.map((a) => (
                          <div key={a.absence_id} className="flex items-center justify-between py-0.5">
                            <div className="min-w-0 flex-1">
                              <div className="flex items-center gap-1.5 min-w-0">
                                <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-red-500" />
                                <span className="truncate text-[13px] text-gray-800">
                                  {a.nickname ?? a.student_name ?? a.wcode}
                                </span>
                                <span className="shrink-0 text-[11px] text-gray-400">({a.wcode})</span>
                                <span className="shrink-0 text-[11px] text-red-600">absent</span>
                              </div>
                              <div className="mt-1 flex min-w-0 items-start gap-1.5 text-[11px] text-gray-500">
                                <span className="shrink-0">Reason:</span>
                                <ReasonText absenceId={a.absence_id} reasonStates={reasonStates} />
                              </div>
                            </div>
                            <Link
                              to={`/teacher-dashboard/absences/${a.absence_id}`}
                              className="shrink-0 text-[12px] font-medium text-[var(--color-wi-primary)] hover:underline"
                            >
                              View <ExternalLink className="inline h-2.5 w-2.5" />
                            </Link>
                          </div>
                        ))}
                      </div>
                    ) : null}

                    {/* Visitors */}
                    {visitors.length > 0 ? (
                      <div className="mt-1.5 space-y-1">
                        {visitors.map((v) => (
                          <div key={`${v.absence_id}-${v.wcode}`} className="flex items-center justify-between py-0.5">
                            <div className="min-w-0 flex-1">
                              <div className="flex items-center gap-1.5 min-w-0">
                                <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-amber-500" />
                                <span className="truncate text-[13px] text-gray-800">
                                  {v.nickname ?? v.student_name ?? v.wcode}
                                </span>
                                <span className="shrink-0 text-[11px] text-amber-600">
                                  sit-in from {v.absent_subject_name?.trim() || v.from_subject_name?.trim() || 'Subject unavailable'}
                                  {v.absence_date ? ` · ${format(parseISO(v.absence_date), 'd MMM')}` : ''}
                                </span>
                              </div>
                              <div className="mt-1 flex min-w-0 items-start gap-1.5 text-[11px] text-gray-500">
                                <span className="shrink-0">Reason:</span>
                                <ReasonText absenceId={v.absence_id} reasonStates={reasonStates} />
                              </div>
                            </div>
                            <Link
                              to={`/teacher-dashboard/absences/${v.absence_id}`}
                              className="shrink-0 text-[12px] font-medium text-[var(--color-wi-primary)] hover:underline"
                            >
                              View <ExternalLink className="inline h-2.5 w-2.5" />
                            </Link>
                          </div>
                        ))}
                      </div>
                    ) : null}

                    {/* Actions */}
                    <div className="mt-2 flex items-center gap-2 border-t border-gray-50 pt-2">
                      <Link
                        to={`/courses/${s.course_id}`}
                        className="rounded-sm border border-gray-200 px-2.5 py-1 text-[11px] font-medium text-gray-600 hover:bg-gray-50"
                      >
                        View Course
                      </Link>
                      <Link
                        to={`/attendance?session=${s.id}`}
                        className="rounded-sm bg-[var(--color-wi-primary)] px-2.5 py-1 text-[11px] font-medium text-white"
                      >
                        Take Attendance
                      </Link>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </div>
    </>
  );
}
