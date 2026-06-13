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
      <div className="px-3 py-4 text-sm text-gray-400">
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
      <div className="px-3 py-4 text-sm text-gray-400">
        No students enrolled
      </div>
    );
  }

  const sorted = [...students].sort((a, b) => a.wcode.localeCompare(b.wcode));

  return (
    <div className="border-t border-gray-100 bg-gray-50/50">
      <table className="w-full text-[13px]">
        <thead>
          <tr className="border-b border-gray-200">
            <th className="w-32 py-2 pl-10 pr-2 text-left text-[11px] font-semibold uppercase tracking-wider text-gray-500">
              W-code
            </th>
            <th className="py-2 px-2 text-left text-[11px] font-semibold uppercase tracking-wider text-gray-500">
              Name
            </th>
            <th className="w-24 py-2 px-2 text-left text-[11px] font-semibold uppercase tracking-wider text-gray-500">
              Status
            </th>
          </tr>
        </thead>
        <tbody>
          {sorted.map((student) => (
            <tr
              key={student.id}
              className="border-b border-gray-100 last:border-b-0 hover:bg-gray-100/50"
            >
              <td className="py-1.5 pl-10 pr-2 font-mono text-xs text-gray-700">
                {student.wcode}
              </td>
              <td className="py-1.5 px-2 text-gray-800">{student.full_name}</td>
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
