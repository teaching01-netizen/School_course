import { useState, useEffect, useCallback } from "react";
import type { AssignmentSummary, AssignmentListResponse } from "../../types/crossStudy";
import { apiJson } from "../../api/client";

type Props = {
  refreshKey: number;
  onSelectWCode: (wcode: string) => void;
  onReviewCountChange?: (count: number) => void;
};

const statusOptions = [
  { value: "", label: "All Statuses" },
  { value: "active", label: "✅ Active" },
  { value: "notes_changed", label: "⚠️ Notes Changed" },
  { value: "orphaned", label: "🔄 Orphaned" },
  { value: "pending", label: "⏳ Pending" },
];

export default function CrossStudyAssignmentList({ refreshKey, onSelectWCode, onReviewCountChange }: Props) {
  const [assignments, setAssignments] = useState<AssignmentSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [statusFilter, setStatusFilter] = useState("");
  const [searchQuery, setSearchQuery] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const params = new URLSearchParams();
      if (statusFilter) params.set("status", statusFilter);
      if (searchQuery) params.set("q", searchQuery);
      const res = await apiJson<AssignmentListResponse>(
        `/api/v1/cross-study/assignments?${params.toString()}`,
      );
      setAssignments(res.assignments);
      onReviewCountChange?.(res.review_count);
    } catch {
      setAssignments([]);
      onReviewCountChange?.(0);
    } finally {
      setLoading(false);
    }
  }, [onReviewCountChange, statusFilter, searchQuery]);

  useEffect(() => {
    load();
  }, [load, refreshKey]);

  const statusBadge = (status: string) => {
    const map: Record<string, string> = {
      active: "bg-green-50 text-green-700",
      notes_changed: "bg-amber-50 text-amber-700",
      orphaned: "bg-red-50 text-red-700",
      pending: "bg-blue-50 text-blue-700",
    };
    const icons: Record<string, string> = {
      active: "✅",
      notes_changed: "⚠️",
      orphaned: "🔄",
      pending: "⏳",
    };
    return (
      <span className={`inline-block px-2 py-0.5 rounded-sm text-xs font-medium ${map[status] ?? "bg-gray-50 text-gray-700"}`}>
        {icons[status] ?? "❓"} {status}
      </span>
    );
  };

  return (
    <div>
      <div className="flex items-end gap-2 mb-3">
        <div>
          <label className="block text-xs font-semibold text-gray-500 uppercase tracking-wider mb-1">Filter</label>
          <select
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value)}
            className="px-2 py-1.5 text-sm border border-gray-300 rounded-sm"
          >
            {statusOptions.map((o) => (
              <option key={o.value} value={o.value}>{o.label}</option>
            ))}
          </select>
        </div>
        <div className="flex-1">
          <label className="block text-xs font-semibold text-gray-500 uppercase tracking-wider mb-1">Search</label>
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="WCode or name…"
            className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded-sm"
          />
        </div>
      </div>

      {loading ? (
        <div className="text-sm text-gray-400 py-4">Loading assignments...</div>
      ) : assignments.length === 0 ? (
        <div className="text-sm text-gray-400 py-4">
          No cross-study assignments yet. Search a student's WCode above to create the first assignment.
        </div>
      ) : (
        <div className="border border-gray-200 rounded-sm overflow-hidden">
          <table className="w-full text-[12px]">
            <thead className="bg-gray-50">
              <tr className="border-b border-gray-200">
                <th className="text-left py-2 px-2 font-semibold text-gray-700">WCode</th>
                <th className="text-left py-2 px-2 font-semibold text-gray-700">Name</th>
                <th className="text-left py-2 px-2 font-semibold text-gray-700">Course A</th>
                <th className="text-left py-2 px-2 font-semibold text-gray-700">Course B</th>
                <th className="text-left py-2 px-2 font-semibold text-gray-700">Status</th>
              </tr>
            </thead>
            <tbody>
              {assignments.map((a) => (
                <tr
                  key={a.id}
                  className="border-b border-gray-100 hover:bg-blue-50 cursor-pointer"
                  onClick={() => onSelectWCode(a.wcode)}
                >
                  <td className="py-2 px-2 font-mono text-blue-600">{a.wcode}</td>
                  <td className="py-2 px-2">{a.full_name}</td>
                  <td className="py-2 px-2 text-gray-600 max-w-48 truncate" title={a.dest_course_a_name}>
                    {a.dest_course_a_name}
                  </td>
                  <td className="py-2 px-2 text-gray-600 max-w-48 truncate" title={a.dest_course_b_name}>
                    {a.dest_course_b_name}
                  </td>
                  <td className="py-2 px-2">{statusBadge(a.status)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
