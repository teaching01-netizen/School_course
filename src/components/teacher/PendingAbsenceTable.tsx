import { Link } from 'react-router-dom';
import type { PendingAbsenceRequest } from '../../types';

function initials(name: string): string {
  return name.split(' ').map((part) => part.charAt(0)).join('').toUpperCase().slice(0, 2);
}

function submittedAgo(value: string): string {
  const elapsed = Date.now() - new Date(value).getTime();
  const hours = Math.floor(elapsed / 3_600_000);
  if (hours < 1) return 'Just now';
  if (hours < 24) return `${hours}h ago`;
  if (hours < 48) return 'Yesterday';
  return new Date(value).toLocaleDateString('en-GB', { day: 'numeric', month: 'short' });
}

type PendingAbsenceTableProps = {
  requests: PendingAbsenceRequest[];
};

export default function PendingAbsenceTable({ requests }: PendingAbsenceTableProps) {
  if (requests.length === 0) {
    return <p className="text-[12px] text-[var(--color-wi-text-light)]">No pending requests.</p>;
  }

  return (
    <div className="overflow-x-auto rounded-sm border border-[var(--color-wi-line)] bg-white">
      <table className="w-full text-sm">
        <thead className="text-left text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">
          <tr>
            <th className="px-3 py-2 w-[180px]">Student</th>
            <th className="px-3 py-2">Course / Subject</th>
            <th className="px-3 py-2 w-[160px]">Date Range</th>
            <th className="px-3 py-2 w-[100px]">Submitted</th>
            <th className="px-3 py-2 w-[80px] text-right">Actions</th>
          </tr>
        </thead>
        <tbody>
          {requests.map((req) => {
            const displayName = req.nickname ?? req.student_name ?? req.wcode;
            return (
              <tr key={req.id} className="group align-middle hover:bg-blue-50/40">
                <td className="px-3 py-3">
                  <div className="flex items-center gap-2">
                    <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-[var(--color-wi-primary)] text-[10px] font-bold text-white">
                      {initials(displayName)}
                    </span>
                    <div className="min-w-0">
                      <Link
                        to={`/teacher-dashboard/absences/${req.id}`}
                        className="font-medium text-[var(--color-wi-primary)] hover:underline"
                      >
                        {displayName}
                      </Link>
                      <div className="font-mono text-xs text-[var(--color-wi-text-light)]">{req.wcode}</div>
                    </div>
                  </div>
                </td>
                <td className="px-3 py-3">
                  <div className="max-w-[200px] truncate font-medium text-[var(--color-wi-text)]" title={`${req.course_code} — ${req.course_name}`}>
                    {req.course_code}
                  </div>
                  {req.subject_name ? (
                    <div className="text-xs text-[var(--color-wi-text-light)]">{req.subject_name}</div>
                  ) : null}
                </td>
                <td className="px-3 py-3 whitespace-nowrap text-[var(--color-wi-text-light)]">
                  {req.date_from === req.date_to ? req.date_from : `${req.date_from} – ${req.date_to}`}
                </td>
                <td className="px-3 py-3 whitespace-nowrap text-[var(--color-wi-text-light)]">{submittedAgo(req.created_at)}</td>
                <td className="px-3 py-3 text-right">
                  <Link
                    to={`/teacher-dashboard/absences/${req.id}`}
                    className="inline-flex items-center rounded-sm px-2 py-1 text-xs font-medium text-[var(--color-wi-primary)] hover:bg-blue-50"
                  >
                    Review →
                  </Link>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
