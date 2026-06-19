import { Fragment, useMemo } from 'react';
import { Link } from 'react-router-dom';
import type { TeacherDashboardSession, PendingAbsenceRequest } from '../../types';

type AbsenceRequestTableProps = {
  sessions: TeacherDashboardSession[];
  pendingRequests: PendingAbsenceRequest[];
};

function yyyyMmDd(iso: string): string {
  const d = new Date(iso);
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${y}-${m}-${day}`;
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString('en-GB', { weekday: 'short', day: 'numeric', month: 'short' });
}

function formatSubmitted(iso: string): string {
  const d = new Date(iso);
  const date = d.toLocaleDateString('en-GB', { day: 'numeric', month: 'short' });
  const time = d.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' });
  return `${date}, ${time}`;
}

function initials(name: string): string {
  return name.split(' ').map((part) => part.charAt(0)).join('').toUpperCase().slice(0, 2);
}

type AbsenceRow = {
  dateKey: string;
  dateLabel: string;
  id: string;
  studentName: string;
  wcode: string;
  courseCode: string;
  courseName: string;
  subjectName: string | null;
  submittedLabel: string;
  isPending: boolean;
  absenceId: string;
};

function useAbsenceRows(sessions: TeacherDashboardSession[], pendingRequests: PendingAbsenceRequest[]): Map<string, AbsenceRow[]> {
  return useMemo(() => {
    const rows: AbsenceRow[] = [];

    for (const s of sessions) {
      for (const a of s.absent_students ?? []) {
        const createdAt = a.created_at ?? s.start_at;
        rows.push({
          dateKey: yyyyMmDd(createdAt),
          dateLabel: formatDate(createdAt),
          id: `abs-${a.absence_id}`,
          studentName: a.nickname ?? a.student_name ?? a.wcode,
          wcode: a.wcode,
          courseCode: s.course_code,
          courseName: s.course_name,
          subjectName: s.subject_name,
          submittedLabel: formatSubmitted(createdAt),
          isPending: false,
          absenceId: a.absence_id,
        });
      }
    }

    for (const r of pendingRequests) {
      rows.push({
        dateKey: yyyyMmDd(r.created_at),
        dateLabel: formatDate(r.created_at),
        id: `pend-${r.id}`,
        studentName: r.nickname ?? r.student_name ?? r.wcode,
        wcode: r.wcode,
        courseCode: r.course_code,
        courseName: r.course_name,
        subjectName: r.subject_name,
        submittedLabel: formatSubmitted(r.created_at),
        isPending: true,
        absenceId: r.id,
      });
    }

    rows.sort((a, b) => {
      if (a.dateKey !== b.dateKey) return a.dateKey.localeCompare(b.dateKey);
      return a.studentName.localeCompare(b.studentName);
    });

    const grouped = new Map<string, AbsenceRow[]>();
    for (const row of rows) {
      const group = grouped.get(row.dateKey);
      if (group) {
        group.push(row);
      } else {
        grouped.set(row.dateKey, [row]);
      }
    }

    return grouped;
  }, [sessions, pendingRequests]);
}

export default function AbsenceRequestTable({ sessions, pendingRequests }: AbsenceRequestTableProps) {
  const grouped = useAbsenceRows(sessions, pendingRequests);
  const total = [...grouped.values()].reduce((sum, g) => sum + g.length, 0);

  if (total === 0) {
    return <p className="text-[12px] text-gray-400">No absence requests in this period.</p>;
  }

  return (
    <div className="overflow-x-auto rounded-sm border border-gray-200 bg-white">
      <table className="w-full text-sm">
        <thead className="text-left text-xs font-semibold uppercase tracking-wide text-gray-500">
          <tr>
            <th className="px-3 py-2 w-[200px]">Student</th>
            <th className="px-3 py-2 w-[100px]">Wcode</th>
            <th className="px-3 py-2">Course</th>
            <th className="px-3 py-2 w-[160px]">Submitted</th>
            <th className="px-3 py-2 w-[90px]">Status</th>
            <th className="px-3 py-2 w-[70px] text-right">Action</th>
          </tr>
        </thead>
        <tbody>
          {Array.from(grouped.entries()).map(([dateKey, rows]) => (
            <Fragment key={dateKey}>
              <tr className="bg-gray-50">
                <td colSpan={6} className="px-3 py-2 text-[11px] font-semibold uppercase tracking-wider text-gray-500">
                  {rows[0].dateLabel}
                </td>
              </tr>
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
                    <div className="max-w-[200px] truncate font-medium text-gray-900" title={`${row.courseCode} — ${row.courseName}`}>
                      {row.courseCode}
                    </div>
                    {row.subjectName ? (
                      <div className="text-xs text-gray-500">{row.subjectName}</div>
                    ) : null}
                  </td>
                  <td className="px-3 py-3 whitespace-nowrap text-[13px] text-gray-700">{row.submittedLabel}</td>
                  <td className="px-3 py-3 whitespace-nowrap">
                    {row.isPending ? (
                      <span className="inline-flex items-center rounded-full border border-amber-200 bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-700">
                        Pending
                      </span>
                    ) : (
                      <span className="inline-flex items-center rounded-full border border-red-200 bg-red-50 px-2 py-0.5 text-xs font-medium text-red-700">
                        Absent
                      </span>
                    )}
                  </td>
                  <td className="px-3 py-3 text-right">
                    <Link
                      to={`/absences/${row.absenceId}`}
                      className="inline-flex items-center rounded-sm px-2 py-1 text-xs font-medium text-[var(--color-wi-primary)] hover:bg-blue-50"
                    >
                      View →
                    </Link>
                  </td>
                </tr>
              ))}
            </Fragment>
          ))}
        </tbody>
      </table>
    </div>
  );
}
