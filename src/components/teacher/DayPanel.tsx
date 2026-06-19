import { format } from 'date-fns';
import { ExternalLink } from 'lucide-react';
import { Link } from 'react-router-dom';
import type { TeacherDashboardSession } from '../../types';

type DayPanelProps = {
  date: Date;
  sessions: TeacherDashboardSession[];
  onClose: () => void;
};

export default function DayPanel({ date, sessions, onClose }: DayPanelProps) {
  const sorted = [...sessions].sort(
    (a, b) => new Date(a.start_at).getTime() - new Date(b.start_at).getTime(),
  );

  const totalAbsences = sorted.reduce((s, sess) => s + (sess.absent_students?.length ?? 0), 0);
  const totalVisitors = sorted.reduce((s, sess) => s + (sess.sit_in_visitors?.length ?? 0), 0);

  return (
    <>
      {/* Backdrop */}
      <div className="fixed inset-0 z-40 bg-black/15" onClick={onClose} />

      {/* Modal */}
      <div className="fixed inset-0 m-auto z-50 flex max-h-[70vh] w-[92vw] max-w-[560px] flex-col rounded-lg bg-white shadow-lg">

        {/* Header */}
        <div className="flex items-center justify-between border-b border-gray-100 px-4 py-3">
          <div>
            <h3 className="text-[14px] font-bold text-gray-900">{format(date, 'EEEE, d MMMM yyyy')}</h3>
            {sessions.length > 0 ? (
              <p className="text-[11px] text-gray-500">
                {sessions.length} {sessions.length === 1 ? 'session' : 'sessions'}
                {totalAbsences > 0 ? ` · ${totalAbsences} ${totalAbsences === 1 ? 'absence' : 'absences'}` : ''}
                {totalVisitors > 0 ? ` · ${totalVisitors} ${totalVisitors === 1 ? 'visitor' : 'visitors'}` : ''}
              </p>
            ) : null}
          </div>
          <button
            type="button"
            onClick={onClose}
            className="flex h-7 w-7 items-center justify-center rounded-sm text-gray-400 hover:bg-gray-100 hover:text-gray-600"
          >
            ×
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
                            <div className="flex items-center gap-1.5 min-w-0">
                              <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-red-500" />
                              <span className="truncate text-[13px] text-gray-800">
                                {a.nickname ?? a.student_name ?? a.wcode}
                              </span>
                              <span className="shrink-0 text-[11px] text-gray-400">({a.wcode})</span>
                              <span className="shrink-0 text-[11px] text-red-600">absent</span>
                            </div>
                            <Link
                              to={`/absences/${a.absence_id}`}
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
                            <div className="flex items-center gap-1.5 min-w-0">
                              <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-amber-500" />
                              <span className="truncate text-[13px] text-gray-800">
                                {v.nickname ?? v.student_name ?? v.wcode}
                              </span>
                              <span className="shrink-0 text-[11px] text-amber-600">
                                sit-in from {v.absent_subject_name ?? v.from_course_code}
                              </span>
                            </div>
                            <Link
                              to={`/absences/${v.absence_id}`}
                              className="shrink-0 text-[12px] font-medium text-[var(--color-wi-primary)] hover:underline"
                            >
                              View <ExternalLink className="inline h-2.5 w-2.5" />
                            </Link>
                          </div>
                        ))}
                      </div>
                    ) : null}

                    {/* All clear */}
                    {absences.length === 0 && visitors.length === 0 ? (
                      <div className="mt-1 flex items-center gap-1.5">
                        <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-green-500" />
                        <span className="text-[12px] text-green-600">All clear</span>
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
