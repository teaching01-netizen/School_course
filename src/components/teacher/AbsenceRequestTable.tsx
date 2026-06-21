import { useMemo } from 'react';
import { Link } from 'react-router-dom';
import { CalendarX } from 'lucide-react';
import type { TeacherDashboardSession, TeacherDashboardSitInVisitor } from '../../types';
import EmptyState from '../ui/EmptyState';

type AbsenceRequestTableProps = {
  sessions: TeacherDashboardSession[];
};

function formatTime(iso: string): string {
  return new Date(iso).toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' });
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString('en-GB', { day: 'numeric', month: 'short' });
}

function formatSubmitted(iso: string): string {
  const d = new Date(iso);
  const date = d.toLocaleDateString('en-GB', { day: 'numeric', month: 'short' });
  const time = d.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' });
  return `${date}, ${time}`;
}

function formatSessionTime(iso: string): string {
  return `${formatDate(iso)}, ${formatTime(iso)}`;
}

function initials(name: string): string {
  return name.split(' ').map((part) => part.charAt(0)).join('').toUpperCase().slice(0, 2);
}

type AbsenceRow = {
  id: string;
  studentName: string;
  wcode: string;
  courseName: string;
  subjectName: string | null;
  submittedLabel: string;
  submittedAt: string;
  absenceId: string;
  missedSubject: string | null;
  missedTimeLabel: string;
  sitInSubject: string | null;
  sitInTimeLabel: string | null;
};

function useAbsenceRows(sessions: TeacherDashboardSession[]): AbsenceRow[] {
  return useMemo(() => {
    const sitInLookup = new Map<string, { visitor: TeacherDashboardSitInVisitor; visitedSession: TeacherDashboardSession }>();
    for (const s of sessions) {
      for (const v of s.sit_in_visitors ?? []) {
        sitInLookup.set(v.absence_id, { visitor: v, visitedSession: s });
      }
    }

    const rows: AbsenceRow[] = [];

    for (const s of sessions) {
      for (const a of s.absent_students ?? []) {
        const createdAt = a.created_at ?? s.start_at;
        const sitIn = sitInLookup.get(a.absence_id);

        rows.push({
          id: `abs-${a.absence_id}`,
          studentName: a.nickname ?? a.student_name ?? a.wcode,
          wcode: a.wcode,
          courseName: s.course_name,
          subjectName: s.subject_name,
          submittedLabel: formatSubmitted(createdAt),
          submittedAt: createdAt,
          absenceId: a.absence_id,
          missedSubject: s.subject_name ?? s.course_name,
          missedTimeLabel: formatSessionTime(s.start_at),
          sitInSubject: sitIn ? (sitIn.visitedSession.subject_name ?? sitIn.visitedSession.course_name) : null,
          sitInTimeLabel: sitIn ? formatSessionTime(sitIn.visitor.session_start_at) : null,
        });
      }
    }

    rows.sort((a, b) => new Date(b.submittedAt).getTime() - new Date(a.submittedAt).getTime());

    return rows;
  }, [sessions]);
}

export default function AbsenceRequestTable({ sessions }: AbsenceRequestTableProps) {
  const rows = useAbsenceRows(sessions);

  if (rows.length === 0) {
    return (
      <EmptyState
        message="No absence requests in this period."
        icon={<CalendarX className="h-10 w-10" />}
      />
    );
  }

  return (
    <>
      <div className="space-y-2 sm:hidden" data-testid="mobile-absence-list">
      {rows.map((row) => (
        <article key={row.id} className="rounded-sm border border-gray-200 bg-white p-3">
          <div className="flex items-start gap-2">
            <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-[var(--color-wi-primary)] text-[10px] font-bold text-white">
              {initials(row.studentName)}
            </span>
            <div className="min-w-0 flex-1">
              <h3 className="truncate text-sm font-semibold text-gray-900">{row.studentName}</h3>
              <p className="font-mono text-[11px] text-gray-500">{row.wcode}</p>
            </div>
            <Link to={`/teacher-dashboard/absences/${row.absenceId}`} className="inline-flex min-h-11 shrink-0 items-center px-1 text-xs font-medium text-[var(--color-wi-primary)]">
              View →
            </Link>
          </div>
          <dl className="mt-2 grid gap-2 border-t border-gray-100 pt-2 text-xs">
            <div><dt className="font-medium text-gray-500">Course</dt><dd className="break-words text-gray-900">{row.subjectName?.trim() || row.courseName}</dd></div>
            <div><dt className="font-medium text-gray-500">Missed</dt><dd className="break-words text-gray-900">{row.missedSubject} · {row.missedTimeLabel}</dd></div>
            <div><dt className="font-medium text-gray-500">Sit-in</dt><dd className="break-words text-gray-900">{row.sitInSubject ? `${row.sitInSubject} · ${row.sitInTimeLabel}` : '—'}</dd></div>
            <div><dt className="font-medium text-gray-500">Submitted</dt><dd className="text-gray-900">{row.submittedLabel}</dd></div>
          </dl>
        </article>
      ))}
      </div>
      <div className="hidden overflow-x-auto rounded-sm border border-gray-200 bg-white sm:block">
      <table className="w-full text-sm">
        <thead className="text-left text-xs font-semibold uppercase tracking-wide text-gray-500">
          <tr>
            <th className="px-3 py-2 w-[180px]">Student</th>
            <th className="px-3 py-2 w-[90px]">Wcode</th>
            <th className="px-3 py-2 w-[120px]">Course</th>
            <th className="px-3 py-2 w-[160px]">Missed</th>
            <th className="px-3 py-2 w-[160px]">Sit-in</th>
            <th className="px-3 py-2 w-[140px]">Submitted</th>
            <th className="px-3 py-2 w-[70px] text-right">Action</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.id} className="group align-middle hover:bg-blue-50/40">
              <td className="px-3 py-3">
                <div className="flex items-center gap-2">
                  <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-[var(--color-wi-primary)] text-[10px] font-bold text-white">
                    {initials(row.studentName)}
                  </span>
                  <span className="truncate font-medium text-gray-900">{row.studentName}</span>
                </div>
              </td>
              <td className="px-3 py-3 font-mono text-xs text-gray-500">{row.wcode}</td>
              <td className="px-3 py-3">
                <div className="max-w-[120px] truncate font-medium text-gray-900" title={row.subjectName?.trim() || row.courseName}>
                  {row.subjectName?.trim() || row.courseName}
                </div>
              </td>
              <td className="px-3 py-3">
                <div className="max-w-[160px] truncate font-medium text-gray-900" title={row.missedSubject ?? ''}>
                  {row.missedSubject}
                </div>
                <div className="text-xs text-gray-500">{row.missedTimeLabel}</div>
              </td>
              <td className="px-3 py-3">
                {row.sitInSubject ? (
                  <>
                    <div className="max-w-[160px] truncate font-medium text-gray-900" title={row.sitInSubject}>
                      {row.sitInSubject}
                    </div>
                    <div className="text-xs text-gray-500">{row.sitInTimeLabel}</div>
                  </>
                ) : (
                  <span className="text-xs text-gray-400">&mdash;</span>
                )}
              </td>
              <td className="px-3 py-3 whitespace-nowrap text-[13px] text-gray-700">{row.submittedLabel}</td>
              <td className="px-3 py-3 text-right">
                <Link
                  to={`/teacher-dashboard/absences/${row.absenceId}`}
                  className="inline-flex items-center rounded-sm px-2 py-1 text-xs font-medium text-[var(--color-wi-primary)] hover:bg-blue-50"
                >
                  View →
                </Link>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      </div>
    </>
  );
}
