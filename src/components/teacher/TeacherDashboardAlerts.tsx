import { useMemo } from 'react';
import { format } from 'date-fns';
import type { TeacherDashboardSession, TeacherDashboardSitInVisitor } from '../../types';

type TeacherDashboardAlertsProps = {
  sessions: TeacherDashboardSession[];
};

function yyyyMmDd(d: Date): string {
  return d.toISOString().slice(0, 10);
}

type AnnotatedSession = TeacherDashboardSession & {
  sitInVisitors: TeacherDashboardSitInVisitor[];
};

export default function TeacherDashboardAlerts({ sessions }: TeacherDashboardAlertsProps) {
  const today = useMemo(() => yyyyMmDd(new Date()), []);

  const todaySessions = useMemo(() => {
    const filtered = sessions.filter((s) => {
      const sessionDate = yyyyMmDd(new Date(s.start_at));
      return sessionDate === today;
    });

    const annotated: AnnotatedSession[] = filtered.map((s) => ({
      ...s,
      sitInVisitors: s.sit_in_visitors ?? [],
    }));

    annotated.sort((a, b) => {
      const aScore = (a.absent_count > 0 ? 2 : 0) + (a.sitInVisitors.length > 0 ? 1 : 0);
      const bScore = (b.absent_count > 0 ? 2 : 0) + (b.sitInVisitors.length > 0 ? 1 : 0);
      return bScore - aScore;
    });

    return annotated;
  }, [sessions, today]);

  if (todaySessions.length === 0) {
    return (
      <div className="rounded-sm border border-gray-200 bg-white p-8 text-center text-sm text-gray-400">
        No sessions today.
      </div>
    );
  }

  return (
    <div className="space-y-2">
      {todaySessions.map((session) => {
        const hasAbsences = session.absent_count > 0;
        const hasSitIns = session.sitInVisitors.length > 0;
        const hasAnomalies = hasAbsences || hasSitIns;

        return (
          <div
            key={session.id}
            className={`rounded-sm border p-3 ${
              hasAnomalies
                ? 'border-amber-200 bg-amber-50/50'
                : 'border-gray-100 bg-white'
            }`}
          >
            <div className="flex items-start justify-between">
              <div>
                <p className="font-semibold text-gray-800">
                  {session.course_code} — {session.course_name}
                </p>
                <p className="text-xs text-gray-500">
                  {format(new Date(session.start_at), 'HH:mm')}–{format(new Date(session.end_at), 'HH:mm')}
                  {session.room_name ? ` · ${session.room_name}` : null}
                </p>
              </div>
              <div className="flex items-center gap-1.5 shrink-0">
                {hasAbsences ? (
                  <span className="inline-flex items-center rounded-sm bg-red-100 px-1.5 py-0.5 text-[11px] font-semibold text-red-700">
                    {session.absent_count} absent
                  </span>
                ) : null}
                {hasSitIns ? (
                  <span className="inline-flex items-center rounded-sm bg-blue-100 px-1.5 py-0.5 text-[11px] font-semibold text-blue-700">
                    {session.sitInVisitors.length} sit-in
                  </span>
                ) : null}
              </div>
            </div>
            {hasSitIns ? (
              <div className="mt-2 border-t border-amber-200/50 pt-2 space-y-1">
                {session.sitInVisitors.map((v) => (
                  <p key={v.absence_id} className="text-xs text-amber-800">
                    Sittin in: {v.student_name ?? v.wcode} (from {v.from_course_code})
                  </p>
                ))}
              </div>
            ) : null}
            {!hasAnomalies ? (
              <p className="mt-1 text-xs text-gray-400">All clear — no absences or visitors.</p>
            ) : null}
          </div>
        );
      })}
    </div>
  );
}
