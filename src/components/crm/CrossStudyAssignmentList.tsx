import { useCallback, useEffect, useRef, useState } from "react";
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

const PAGE_SIZE = 25;
const MIN_SEARCH_LENGTH = 2;
const SEARCH_DEBOUNCE_MS = 300;

function isAbortError(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError";
}

export default function CrossStudyAssignmentList({ refreshKey, onSelectWCode, onReviewCountChange }: Props) {
  const [assignments, setAssignments] = useState<AssignmentSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [statusFilter, setStatusFilter] = useState("");
  const [searchQuery, setSearchQuery] = useState("");
  const [debouncedQuery, setDebouncedQuery] = useState("");
  const [offset, setOffset] = useState(0);
  const [total, setTotal] = useState(0);
  const [reloadKey, setReloadKey] = useState(0);
  const searchInputRef = useRef<HTMLInputElement | null>(null);

  // Debounce the raw input so typing settles before a request fires.
  useEffect(() => {
    const t = setTimeout(() => setDebouncedQuery(searchQuery.trim()), SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(t);
  }, [searchQuery]);

  // Short queries are not sent; the list falls back to the full set rather
  // than firing a broad request on the first keystroke.
  const effectiveQuery = debouncedQuery.length >= MIN_SEARCH_LENGTH ? debouncedQuery : "";
  const showMinLengthHint = searchQuery.trim().length > 0 && debouncedQuery.length < MIN_SEARCH_LENGTH;

  const load = useCallback(
    async (signal: AbortSignal) => {
      setLoading(true);
      setError(null);
      try {
        const params = new URLSearchParams();
        if (statusFilter) params.set("status", statusFilter);
        if (effectiveQuery) params.set("q", effectiveQuery);
        params.set("limit", String(PAGE_SIZE));
        params.set("offset", String(offset));
        const res = await apiJson<AssignmentListResponse>(
          `/api/v1/cross-study/assignments?${params.toString()}`,
          { signal },
        );
        setAssignments(res.assignments);
        setTotal(res.total);
        onReviewCountChange?.(res.review_count);
      } catch (err) {
        if (isAbortError(err)) return; // superseded request; newer one owns state
        setAssignments([]);
        setTotal(0);
        setError(err instanceof Error ? err.message : "Failed to load assignments");
        onReviewCountChange?.(0);
      } finally {
        setLoading(false);
      }
    },
    [statusFilter, effectiveQuery, offset, onReviewCountChange],
  );

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load, refreshKey, reloadKey]);

  const hasPrevious = offset > 0;
  const hasNext = offset + assignments.length < total;

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
      <span className={`inline-block px-2 py-0.5 rounded-sm text-xs font-medium ${map[status] ?? "bg-[var(--color-wi-row-alt)] text-[var(--color-wi-text-light)]"}`}>
        {icons[status] ?? "❓"} {status}
      </span>
    );
  };

  return (
    <div>
      <div className="flex items-end gap-2 mb-3">
        <div>
          <label htmlFor="cross-study-status-filter" className="block text-xs font-semibold text-[var(--color-wi-text-light)] uppercase tracking-wider mb-1">
            Filter
          </label>
          <select
            id="cross-study-status-filter"
            value={statusFilter}
            onChange={(e) => {
              setStatusFilter(e.target.value);
              setOffset(0);
            }}
            className="px-2 py-1.5 text-sm border border-wi-line rounded-sm"
          >
            {statusOptions.map((o) => (
              <option key={o.value} value={o.value}>{o.label}</option>
            ))}
          </select>
        </div>
        <div className="flex-1">
          <label htmlFor="cross-study-search" className="block text-xs font-semibold text-[var(--color-wi-text-light)] uppercase tracking-wider mb-1">
            Search
          </label>
          <div className="relative">
            <input
              id="cross-study-search"
              ref={searchInputRef}
              type="search"
              value={searchQuery}
              onChange={(e) => {
                setSearchQuery(e.target.value);
                setOffset(0);
              }}
              placeholder="WCode or name…"
              className="w-full px-2 py-1.5 text-sm border border-wi-line rounded-sm"
            />
            {showMinLengthHint ? (
              <p className="absolute right-2 top-1/2 -translate-y-1/2 text-[11px] text-[var(--color-wi-text-light)] pointer-events-none">
                Type at least {MIN_SEARCH_LENGTH} characters
              </p>
            ) : null}
          </div>
        </div>
      </div>

      {loading && assignments.length === 0 ? (
        <div className="text-sm text-[var(--color-wi-text-light)] py-4">Loading assignments...</div>
      ) : error ? (
        <div className="text-sm text-red-600 py-4" role="alert">
          Could not load assignments: {error}.{" "}
          <button
            type="button"
            className="underline text-blue-600"
            onClick={() => {
              setError(null);
              setReloadKey((k) => k + 1);
              searchInputRef.current?.focus();
            }}
          >
            Retry
          </button>
        </div>
      ) : assignments.length === 0 ? (
        <div className="text-sm text-[var(--color-wi-text-light)] py-4">
          {effectiveQuery
            ? `No assignments match "${effectiveQuery}".`
            : "No cross-study assignments yet. Search a student's WCode above to create the first assignment."}
        </div>
      ) : (
        <div className="border border-wi-line rounded-sm overflow-hidden">
          <table className="w-full text-[12px]">
            <thead className="bg-[var(--color-wi-row-alt)]">
              <tr className="border-b border-wi-line">
                <th scope="col" className="text-left py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">WCode</th>
                <th scope="col" className="text-left py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">Name</th>
                <th scope="col" className="text-left py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">Course A</th>
                <th scope="col" className="text-left py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">Course B</th>
                <th scope="col" className="text-left py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">Status</th>
              </tr>
            </thead>
            <tbody>
              {assignments.map((a) => (
                <tr
                  key={a.id}
                  className="border-b border-wi-line-soft hover:bg-blue-50 cursor-pointer"
                  onClick={() => onSelectWCode(a.wcode)}
                >
                  <td className="py-2 px-2 font-mono text-blue-600">{a.wcode}</td>
                  <td className="py-2 px-2">{a.full_name}</td>
                  <td className="py-2 px-2 text-[var(--color-wi-text-light)] max-w-48 truncate" title={a.dest_course_a_name}>
                    {a.dest_course_a_name}
                  </td>
                  <td className="py-2 px-2 text-[var(--color-wi-text-light)] max-w-48 truncate" title={a.dest_course_b_name}>
                    {a.dest_course_b_name}
                  </td>
                  <td className="py-2 px-2">{statusBadge(a.status)}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <div className="flex items-center justify-between gap-2 border-t border-wi-line px-2 py-1.5">
            <span className="text-xs text-[var(--color-wi-text-light)]">
              {total} assignment{total === 1 ? "" : "s"}
            </span>
            <div className="flex items-center gap-2">
              <button
                type="button"
                className="px-2 py-1 text-xs border border-wi-line rounded-sm disabled:opacity-40"
                disabled={!hasPrevious || loading}
                onClick={() => setOffset((o) => Math.max(0, o - PAGE_SIZE))}
              >
                Previous
              </button>
              <button
                type="button"
                className="px-2 py-1 text-xs border border-wi-line rounded-sm disabled:opacity-40"
                disabled={!hasNext || loading}
                onClick={() => setOffset((o) => o + PAGE_SIZE)}
              >
                Next
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}