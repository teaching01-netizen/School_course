import type { Student } from "@/types";

type Props = {
  students: Student[];
  loading: boolean;
  error: string | null;
  onRetry?: () => void;
};

function StatusBadge({ status }: { status?: string }) {
  const isDraft = status === "draft";
  return isDraft ? (
    <span className="inline-flex items-center rounded-sm border border-amber-200 bg-amber-100 px-1.5 py-0.5 text-[10px] font-medium text-amber-800">
      DRAFT
    </span>
  ) : (
    <span className="inline-flex items-center rounded-sm border border-green-200 bg-green-100 px-1.5 py-0.5 text-[10px] font-medium text-green-800">
      ENROLLED
    </span>
  );
}

const ATTENDEE_SKELETON_KEYS = ["a", "b", "c"] as const;

export default function CourseAttendeeRow({
  students,
  loading,
  error,
  onRetry,
}: Props) {
  if (loading) {
    return (
      <div className="border-t border-wi-line-soft bg-[var(--color-wi-row-alt)]/50 animate-fade-in motion-reduce:animate-none" aria-label="Loading attendees">
        {ATTENDEE_SKELETON_KEYS.map((key) => (
          <div key={key} className="flex items-center gap-3 px-10 py-2">
            <div className="h-3 w-20 animate-pulse rounded-sm bg-[var(--color-wi-row-alt)]" />
            <div className="h-3 w-36 animate-pulse rounded-sm bg-[var(--color-wi-row-alt)]" />
            <div className="h-3 w-16 animate-pulse rounded-sm bg-[var(--color-wi-row-alt)]" />
          </div>
        ))}
      </div>
    );
  }

  if (error) {
    return (
      <div className="border-t border-wi-line-soft bg-[var(--color-wi-row-alt)]/50 px-10 py-4 text-sm animate-fade-in motion-reduce:animate-none" role="alert">
        <p className="text-red-600">Couldn&apos;t load attendees: {error}</p>
        {onRetry ? (
          <button
            type="button"
            onClick={onRetry}
            className="mt-2 cursor-pointer rounded-sm border border-wi-line bg-white px-2.5 py-1 text-xs font-medium text-[var(--color-wi-text)] transition-colors duration-150 hover:bg-[var(--color-wi-row-alt)] focus-visible:outline-none focus-visible:shadow-[inset_0_0_0_2px_var(--color-wi-primary)] motion-reduce:transition-none"
          >
            Retry
          </button>
        ) : null}
      </div>
    );
  }

  if (students.length === 0) {
    return (
      <div className="border-t border-wi-line-soft bg-[var(--color-wi-row-alt)]/50 px-10 py-4 text-sm text-[var(--color-wi-text-light)] animate-fade-in motion-reduce:animate-none">
        No students enrolled
      </div>
    );
  }

  const sorted = [...students].sort((a, b) => a.wcode.localeCompare(b.wcode));

  return (
    <div className="border-t border-wi-line-soft bg-[var(--color-wi-row-alt)]/50 animate-fade-in motion-reduce:animate-none">
      <table className="w-full text-[13px]">
        <thead>
          <tr className="border-b border-wi-line">
            <th className="w-28 py-2 pl-10 pr-2 text-left text-[11px] font-semibold uppercase tracking-wider text-[var(--color-wi-text-light)]">
              W-code
            </th>
            <th className="py-2 px-2 text-left text-[11px] font-semibold uppercase tracking-wider text-[var(--color-wi-text-light)]">
              Name
            </th>
            <th className="w-20 py-2 px-2 text-left text-[11px] font-semibold uppercase tracking-wider text-[var(--color-wi-text-light)]">
              Nick
            </th>
            <th className="w-28 py-2 px-2 text-left text-[11px] font-semibold uppercase tracking-wider text-[var(--color-wi-text-light)]">
              School
            </th>
            <th className="w-14 py-2 px-2 text-left text-[11px] font-semibold uppercase tracking-wider text-[var(--color-wi-text-light)]">
              Level
            </th>
            <th className="w-14 py-2 px-2 text-left text-[11px] font-semibold uppercase tracking-wider text-[var(--color-wi-text-light)]">
              Year
            </th>
            <th className="w-24 py-2 px-2 text-left text-[11px] font-semibold uppercase tracking-wider text-[var(--color-wi-text-light)]">
              Phone
            </th>
            <th className="w-20 py-2 px-2 text-left text-[11px] font-semibold uppercase tracking-wider text-[var(--color-wi-text-light)]">
              Status
            </th>
          </tr>
        </thead>
        <tbody>
          {sorted.map((student) => (
            <tr
              key={student.id}
              className="border-b border-wi-line-soft last:border-b-0 hover:bg-[var(--color-wi-row-alt)]/50"
            >
              <td className="py-1.5 pl-10 pr-2 font-mono text-xs text-[var(--color-wi-text-light)]">
                {student.wcode}
              </td>
              <td className="py-1.5 px-2 text-[var(--color-wi-text)]">{student.full_name}</td>
              <td className="py-1.5 px-2 text-[var(--color-wi-text-light)]">{student.nickname || '—'}</td>
              <td className="py-1.5 px-2 text-[var(--color-wi-text-light)]">{student.school || '—'}</td>
              <td className="py-1.5 px-2 text-[var(--color-wi-text-light)]">{student.level || '—'}</td>
              <td className="py-1.5 px-2 text-[var(--color-wi-text-light)]">{student.year || '—'}</td>
              <td className="py-1.5 px-2 text-[var(--color-wi-text-light)]">{student.student_phone || '—'}</td>
              <td className="py-1.5 px-2">
                <StatusBadge status={student.status} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
