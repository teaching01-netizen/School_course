import type { Student } from "@/types";

type Props = {
  students: Student[];
  loading: boolean;
  error: string | null;
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

export default function CourseAttendeeRow({
  students,
  loading,
  error,
}: Props) {
  if (loading) {
    return (
      <div className="px-3 py-4 text-sm text-[var(--color-wi-text-light)]">
        Loading attendees…
      </div>
    );
  }

  if (error) {
    return (
      <div className="px-3 py-4 text-sm text-red-500">
        Failed to load: {error}
      </div>
    );
  }

  if (students.length === 0) {
    return (
      <div className="px-3 py-4 text-sm text-[var(--color-wi-text-light)]">
        No students enrolled
      </div>
    );
  }

  const sorted = [...students].sort((a, b) => a.wcode.localeCompare(b.wcode));

  return (
    <div className="border-t border-t-[var(--color-wi-line)] bg-[var(--color-wi-row-alt)]/50">
      <table className="w-full text-[13px]">
        <thead>
          <tr className="border-b border-b-[var(--color-wi-line)]">
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
              className="border-b border-b-[var(--color-wi-line)] last:border-b-0 hover:bg-[var(--color-wi-row-alt)]/50"
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
