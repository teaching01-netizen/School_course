import { useEffect, useMemo, useRef } from 'react';
import { useQueries } from '@tanstack/react-query';
import { CheckCircle, ExternalLink, X } from 'lucide-react';
import { Link } from 'react-router-dom';
import { apiJson } from '../../api/client';
import type { TeacherAbsenceDetail as TeacherAbsenceDetailData, TeacherDashboardSession } from '../../types';
import { cachePolicies, queryClient, queryKeys } from '../../query/cache';
import { formatUTCToZone, formatZoneDateKey } from '../../utils/timezone';

type DayPanelProps = {
  dateKey: string;
  zone: string;
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
    return <span className="text-[var(--color-wi-text-light)]">Loading reason…</span>;
  }
  if (state.status === 'error') {
    return <span className="text-[var(--color-wi-text-light)]">Reason unavailable</span>;
  }

  const reason = trimReason(state.reason);
  return (
    <span className="min-w-0 truncate text-[var(--color-wi-text-light)]" title={reason ?? undefined}>
      {reason ?? 'No reason provided'}
    </span>
  );
}

export default function DayPanel({ dateKey, zone, sessions, onClose }: DayPanelProps) {
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

  const prevFocus = useRef<HTMLElement | null>(null);
  useEffect(() => {
    prevFocus.current = document.activeElement as HTMLElement;
    return () => { prevFocus.current?.focus(); };
  }, []);

  const dateLabel = formatZoneDateKey(dateKey, zone, 'EEEE, d MMMM yyyy') ?? dateKey;

  return (
    <>
      <div className="fixed inset-0 z-40 bg-black/15" onClick={onClose} aria-hidden="true" />

      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-label={`Session details for ${dateLabel}`}
        className="fixed inset-x-0 bottom-0 z-50 flex max-h-[90dvh] w-full flex-col rounded-t-xl bg-white shadow-lg sm:inset-0 sm:m-auto sm:max-h-[70vh] sm:w-[92vw] sm:max-w-[560px] sm:rounded-lg"
      >
        <div className="flex items-center justify-between border-b border-b-[var(--color-wi-line)] px-4 py-3">
          <div>
            <h3 className="text-[14px] font-bold text-[var(--color-wi-text)]">{dateLabel}</h3>
            {sessions.length > 0 ? (
              <p className="text-[11px] text-[var(--color-wi-text-light)]">
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
            className="flex h-11 w-11 items-center justify-center rounded-sm text-[var(--color-wi-text-light)] hover:bg-[var(--color-wi-row-alt)] hover:text-[var(--color-wi-text-light)] sm:h-7 sm:w-7"
            aria-label="Close panel"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="overflow-y-auto px-4 py-3">
          {sessions.length === 0 ? (
            <div className="py-8 text-center text-[13px] text-[var(--color-wi-text-light)]">No sessions on this day.</div>
          ) : (
            <div className="space-y-4">
              {sorted.map((s) => {
                const start = formatUTCToZone(s.start_at, zone, 'HH:mm');
                const end = formatUTCToZone(s.end_at, zone, 'HH:mm');
                const absences = s.absent_students ?? [];
                const visitors = s.sit_in_visitors ?? [];
                const label = s.subject_name ?? s.course_name;

                return (
                  <div key={s.id} className="rounded-sm border border-[var(--color-wi-line)] bg-white px-3 py-2.5">
                    <div className="flex items-center justify-between">
                      <div className="flex min-w-0 flex-col gap-0.5 sm:flex-row sm:items-baseline sm:gap-1.5">
                        <span className="text-[12px] font-semibold text-[var(--color-wi-text)] tabular-nums">
                          {start ?? '--:--'}–{end ?? '--:--'}
                        </span>
                        <span className="text-[14px] font-bold text-[var(--color-wi-text)]">{label}</span>
                      </div>
                    </div>

                    {absences.length === 0 && visitors.length === 0 ? (
                      <div className="mt-1.5 flex items-center gap-1.5 text-[12px] text-[var(--color-wi-green)]">
                        <CheckCircle className="h-3.5 w-3.5 shrink-0" />
                        No absences — No sit-ins
                      </div>
                    ) : null}

                    {absences.length > 0 ? (
                      <div className="mt-1.5 space-y-1">
                        {absences.map((a) => (
                          <div key={a.absence_id} className="flex flex-col gap-1 py-0.5 sm:flex-row sm:items-center sm:justify-between">
                            <div className="min-w-0 flex-1">
                              <div className="flex items-center gap-1.5 min-w-0">
                                <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-[var(--color-wi-red)]" />
                                <span className="truncate text-[13px] text-[var(--color-wi-text)]">
                                  {a.nickname ?? a.student_name ?? a.wcode}
                                </span>
                                <span className="shrink-0 text-[11px] text-[var(--color-wi-text-light)]">({a.wcode})</span>
                                <span className="shrink-0 text-[11px] text-[var(--color-wi-red)]">absent</span>
                              </div>
                              <div className="mt-1 flex min-w-0 items-start gap-1.5 text-[11px] text-[var(--color-wi-text-light)]">
                                <span className="shrink-0">Reason:</span>
                                <ReasonText absenceId={a.absence_id} reasonStates={reasonStates} />
                              </div>
                            </div>
                            <Link
                              to={`/teacher-dashboard/absences/${a.absence_id}`}
                              className="inline-flex min-h-11 shrink-0 items-center text-[12px] font-medium text-[var(--color-wi-primary)] hover:underline sm:min-h-0"
                            >
                              View <ExternalLink className="inline h-2.5 w-2.5" />
                            </Link>
                          </div>
                        ))}
                      </div>
                    ) : null}

                    {visitors.length > 0 ? (
                      <div className="mt-1.5 space-y-1">
                        {visitors.map((v) => (
                          <div key={`${v.absence_id}-${v.wcode}`} className="flex flex-col gap-1 py-0.5 sm:flex-row sm:items-center sm:justify-between">
                            <div className="min-w-0 flex-1">
                              <div className="flex items-center gap-1.5 min-w-0">
                                <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-[var(--color-wi-amber)]" />
                                <span className="truncate text-[13px] text-[var(--color-wi-text)]">
                                  {v.nickname ?? v.student_name ?? v.wcode}
                                </span>
                                <span className="shrink-0 text-[11px] text-[var(--color-wi-amber)]">
                                  sit-in from {v.absent_subject_name?.trim() || v.from_subject_name?.trim() || 'Subject unavailable'}
                                  {v.absence_date ? ` · ${formatZoneDateKey(v.absence_date, zone, 'd MMM')}` : ''}
                                </span>
                              </div>
                              <div className="mt-1 flex min-w-0 items-start gap-1.5 text-[11px] text-[var(--color-wi-text-light)]">
                                <span className="shrink-0">Reason:</span>
                                <ReasonText absenceId={v.absence_id} reasonStates={reasonStates} />
                              </div>
                            </div>
                            <Link
                              to={`/teacher-dashboard/absences/${v.absence_id}`}
                              className="inline-flex min-h-11 shrink-0 items-center text-[12px] font-medium text-[var(--color-wi-primary)] hover:underline sm:min-h-0"
                            >
                              View <ExternalLink className="inline h-2.5 w-2.5" />
                            </Link>
                          </div>
                        ))}
                      </div>
                    ) : null}
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
