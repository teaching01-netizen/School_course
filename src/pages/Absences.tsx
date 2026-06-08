import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { Download, Eye, LayoutGrid, RefreshCcw, Table2 } from "lucide-react";
import { apiJson, downloadApiFile } from "../api/client";
import { useToast } from "../hooks/useToast";
import type { AbsencePage, AbsenceStatus, ManagedAbsence } from "../types";
import PageHeading from "../components/ui/PageHeading";
import SearchInput from "../components/ui/SearchInput";
import EmptyState from "../components/ui/EmptyState";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";
import Button from "../components/ui/Button";
import Modal from "../components/Modal";
import KanbanView from "../components/absences/KanbanView";
import { formatAbsenceSummaryDates } from "../components/absences/dateSummary";
import { formatSitInLabel } from "../components/absences/sitInLabel";

const PAGE_SIZE = 25;

function submittedAgo(value: string): string {
  const elapsed = Date.now() - new Date(value).getTime();
  const hours = Math.floor(elapsed / 3_600_000);
  if (hours < 1) return "Just now";
  if (hours < 24) return `${hours}h ago`;
  if (hours < 48) return "Yesterday";
  return new Date(value).toLocaleDateString("en-GB", { day: "numeric", month: "short" });
}

function initials(name: string): string {
  return name.split(" ").map((part) => part.charAt(0)).join("").toUpperCase().slice(0, 2);
}

const statusPresentation: Record<AbsenceStatus, { label: string; classes: string }> = {
  pending: { label: "Awaiting review", classes: "bg-blue-50 text-blue-700 border-blue-200" },
  reviewed: { label: "Reviewed", classes: "bg-emerald-50 text-emerald-700 border-emerald-200" },
  actioned: { label: "Actioned", classes: "bg-slate-100 text-slate-600 border-slate-200" },
  cancelled: { label: "Cancelled", classes: "bg-red-50 text-red-700 border-red-200 line-through" },
};

function StatusBadge({ status }: { status: AbsenceStatus }) {
  const presentation = statusPresentation[status];
  return (
    <span className={`inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-xs font-medium ${presentation.classes}`}>
      <span className="h-1.5 w-1.5 rounded-full bg-current" aria-hidden="true" />
      {presentation.label}
    </span>
  );
}

const CANCEL_REASON_OPTIONS = [
  { value: "duplicate", label: "Duplicate submission" },
  { value: "student_requested", label: "Student requested cancellation" },
  { value: "admin_error", label: "Admin error" },
  { value: "incorrect_dates", label: "Incorrect dates" },
  { value: "other", label: "Other" },
];

export default function Absences() {
  const { addToast } = useToast();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const [page, setPage] = useState<AbsencePage | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshToken, setRefreshToken] = useState(0);
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [reviewing, setReviewing] = useState<string | null>(null);
  const [cancelTargets, setCancelTargets] = useState<ManagedAbsence[]>([]);
  const [cancelReasonCategory, setCancelReasonCategory] = useState("");
  const [cancelReasonDetail, setCancelReasonDetail] = useState("");
  const [cancelling, setCancelling] = useState(false);
  const [batchProcessing, setBatchProcessing] = useState(false);
  const [batchProgress, setBatchProgress] = useState({ done: 0, total: 0 });
  const [batchFailed, setBatchFailed] = useState<Array<{ id: string; error: string }>>([]);
  const [deleteTarget, setDeleteTarget] = useState<ManagedAbsence | null>(null);
  const [deleting, setDeleting] = useState(false);

  const viewMode = searchParams.get("view") === "board" ? "board" : "table";

  const filters = {
    query: searchParams.get("query") ?? "",
    subject: searchParams.get("subject_id") ?? "",
    status: searchParams.get("status") ?? "",
    dateFrom: searchParams.get("date_from") ?? "",
    dateTo: searchParams.get("date_to") ?? "",
    offset: Math.max(0, Number(searchParams.get("offset") ?? 0) || 0),
  };

  const requestQuery = useMemo(() => {
    const params = new URLSearchParams(searchParams);
    params.set("limit", String(PAGE_SIZE));
    params.set("offset", String(filters.offset));
    return params.toString();
  }, [searchParams, filters.offset]);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const result = await apiJson<AbsencePage>(`/api/v1/absences?${requestQuery}`, { method: "GET" });
      setPage(result);
      setSelected(new Set());
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to load absences");
    } finally {
      setLoading(false);
    }
  }, [addToast, requestQuery]);

  useEffect(() => {
    void load();
  }, [load, refreshToken]);

  function updateFilter(key: string, value: string) {
    const params = new URLSearchParams(searchParams);
    if (value) params.set(key, value);
    else params.delete(key);
    if (key !== "view") params.delete("offset");
    setSearchParams(params);
  }

  async function setStatus(absence: ManagedAbsence, status: AbsenceStatus, reason?: string, reload = true) {
    setReviewing(absence.id);
    const previousStatus = absence.status;
    absence.status = status;
    try {
      await apiJson(`/api/v1/absences/${absence.id}/status`, {
        method: "PUT",
        body: JSON.stringify({ status, expected_version: absence.version, ...(reason ? { reason } : {}) }),
      });
      addToast("success", status === "reviewed" ? "Absence marked reviewed" : "Absence updated");
      if (reload) await load();
    } catch (err) {
      absence.status = previousStatus;
      addToast("error", err instanceof Error ? err.message : "Update failed");
    } finally {
      setReviewing(null);
    }
  }

  async function cancelAbsences() {
    if (cancelTargets.length === 0) return;
    setCancelling(true);
    setBatchFailed([]);
    try {
      const expectedVersions: Record<string, number> = {};
      for (const target of cancelTargets) {
        expectedVersions[target.id] = target.version;
        target.status = "cancelled";
      }
      const result = await apiJson<{ succeeded: string[]; failed: Array<{ id: string; error: string }>; total_processed: number }>(
        "/api/v1/absences/batch-status",
        {
          method: "POST",
          body: JSON.stringify({
            ids: cancelTargets.map((t) => t.id),
            status: "cancelled",
            reason: JSON.stringify({ category: cancelReasonCategory, detail: cancelReasonDetail }),
            expected_versions: expectedVersions,
          }),
        }
      );
      if (result.failed.length > 0) {
        addToast("error", `${result.succeeded.length} cancelled, ${result.failed.length} failed`);
        setBatchFailed(result.failed);
        for (const f of result.failed) {
          const item = cancelTargets.find((t) => t.id === f.id);
          if (item) item.status = "pending";
        }
      } else {
        addToast("success", `${result.succeeded.length} absences cancelled`);
        setBatchFailed([]);
      }
      await load();
      setCancelTargets([]);
      setCancelReasonCategory("");
      setCancelReasonDetail("");
    } catch (err) {
      for (const target of cancelTargets) {
        target.status = "pending";
      }
      addToast("error", err instanceof Error ? err.message : "Batch cancel failed");
      await load();
    } finally {
      setCancelling(false);
    }
  }

  async function deleteAbsence() {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await apiJson(`/api/v1/absences/${deleteTarget.id}`, { method: "DELETE", body: JSON.stringify({ expected_version: deleteTarget.version }) });
      addToast("success", "Absence permanently deleted");
      setDeleteTarget(null);
      await load();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Delete failed");
    } finally {
      setDeleting(false);
    }
  }

  async function exportCsv() {
    try {
      const params = new URLSearchParams(searchParams);
      params.delete("offset");
      await downloadApiFile(`/api/v1/absences/export?${params.toString()}`);
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Export failed");
    }
  }

  async function exportSelected() {
    try {
      const params = new URLSearchParams(searchParams);
      params.delete("offset");
      params.set("ids", [...selected].join(","));
      await downloadApiFile(`/api/v1/absences/export?${params.toString()}`);
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Export failed");
    }
  }

  async function markSelectedReviewed() {
    const records = (page?.items ?? []).filter((item) => selected.has(item.id) && item.status === "pending");
    if (records.length === 0) return;
    setBatchProcessing(true);
    setBatchFailed([]);
    setBatchProgress({ done: 0, total: records.length });
    for (const item of records) {
      item.status = "reviewed";
    }
    try {
      const expectedVersions: Record<string, number> = {};
      for (const item of records) {
        expectedVersions[item.id] = item.version;
      }
      const result = await apiJson<{ succeeded: string[]; failed: Array<{ id: string; error: string }>; total_processed: number }>(
        "/api/v1/absences/batch-status",
        {
          method: "POST",
          body: JSON.stringify({
            ids: records.map((r) => r.id),
            status: "reviewed",
            expected_versions: expectedVersions,
          }),
        }
      );
      setBatchProgress({ done: result.succeeded.length, total: result.total_processed });
      if (result.failed.length > 0) {
        addToast("error", `${result.succeeded.length} succeeded, ${result.failed.length} failed`);
        setBatchFailed(result.failed);
        for (const f of result.failed) {
          const item = records.find((r) => r.id === f.id);
          if (item) item.status = "pending";
        }
      } else {
        addToast("success", `${result.succeeded.length} absences marked reviewed`);
        setBatchFailed([]);
      }
      await load();
    } catch (err) {
      for (const item of records) {
        item.status = "pending";
      }
      addToast("error", err instanceof Error ? err.message : "Batch update failed");
      await load();
    } finally {
      setBatchProcessing(false);
      setBatchProgress({ done: 0, total: 0 });
    }
  }

  async function retryFailed() {
    if (batchFailed.length === 0) return;
    const previousSelected = new Set(selected);
    setSelected(new Set(batchFailed.map((f) => f.id)));
    await markSelectedReviewed();
    setSelected(previousSelected);
  }

  const subjects = useMemo(() => {
    if (page?.subjects?.length) {
      return page.subjects.map((subject) => [subject.id, subject.name ? `${subject.code} — ${subject.name}` : subject.code] as const);
    }
    const map = new Map<string, string>();
    for (const item of page?.items ?? []) {
      if (item.subject_id && item.subject_code) map.set(item.subject_id, item.subject_code);
    }
    return [...map.entries()];
  }, [page?.items, page?.subjects]);

  function setViewMode(mode: "table" | "board") {
    const params = new URLSearchParams(searchParams);
    if (mode === "board") params.set("view", "board");
    else params.delete("view");
    setSearchParams(params);
  }

  if (viewMode === "board") {
    return (
      <div className="w-full">
        <div className="mb-5 flex flex-wrap items-end justify-between gap-3">
          <div>
            <PageHeading>Absence Board</PageHeading>
            <p className="text-sm text-gray-500">Kanban-style triage for student absences.</p>
          </div>
          <div className="flex items-center gap-2">
            <div className="flex rounded-sm border border-gray-300 bg-white text-sm">
              <button onClick={() => setViewMode("board")} className="flex items-center gap-1 px-3 py-1.5 bg-gray-100 text-gray-900 font-medium"><LayoutGrid className="h-4 w-4" /> Board</button>
              <button onClick={() => setViewMode("table")} className="flex items-center gap-1 px-3 py-1.5 text-gray-500 hover:text-gray-900"><Table2 className="h-4 w-4" /> Table</button>
            </div>
          </div>
        </div>
        <section className="mb-4 rounded-sm border border-gray-200 bg-white p-3" aria-label="Absence filters">
          <div className="grid gap-3 md:grid-cols-[minmax(200px,2fr)_1fr_1fr_1fr]">
            <SearchInput value={filters.query} onChange={(value) => updateFilter("query", value)} placeholder="Search W-Code or name..." />
            <select aria-label="Subject" value={filters.subject} onChange={(event) => updateFilter("subject_id", event.target.value)}>
              <option value="">All subjects</option>
              {subjects.map(([id, label]) => <option key={id} value={id}>{label}</option>)}
            </select>
            <input aria-label="From date" type="date" value={filters.dateFrom} onChange={(event) => updateFilter("date_from", event.target.value)} />
            <input aria-label="To date" type="date" value={filters.dateTo} onChange={(event) => updateFilter("date_to", event.target.value)} />
          </div>
        </section>
        <KanbanView filters={filters} />
      </div>
    );
  }

  if (loading && page === null) {
    return <LoadingSkeleton type="table" lines={5} />;
  }

  const items = page?.items ?? [];
  const allSelected = items.length > 0 && items.every((item) => selected.has(item.id));
  const hasPrevious = filters.offset > 0;
  const hasNext = filters.offset + PAGE_SIZE < (page?.total_count ?? 0);
  const totalPages = Math.ceil((page?.total_count ?? 0) / PAGE_SIZE);
  const currentPage = Math.floor(filters.offset / PAGE_SIZE) + 1;

  function jumpToPage(event: React.ChangeEvent<HTMLInputElement>) {
    const next = Math.max(1, Math.min(totalPages, Number(event.target.value) || 1));
    updateFilter("offset", String((next - 1) * PAGE_SIZE));
  }

  return (
    <div className="w-full">
      <div className="mb-5 flex flex-wrap items-end justify-between gap-3">
        <div>
          <PageHeading>Absence Inbox</PageHeading>
          <p className="text-sm text-gray-500">Review submitted absences and resolve sit-in arrangements.</p>
        </div>
        <div className="flex gap-2">
          <div className="flex rounded-sm border border-gray-300 bg-white text-sm">
            <button onClick={() => setViewMode("table")} className="flex items-center gap-1 px-3 py-1.5 bg-gray-100 text-gray-900 font-medium"><Table2 className="h-4 w-4" /> Table</button>
            <button onClick={() => setViewMode("board")} className="flex items-center gap-1 px-3 py-1.5 text-gray-500 hover:text-gray-900"><LayoutGrid className="h-4 w-4" /> Board</button>
          </div>
          <Link to="/absences/dashboard" className="inline-flex min-h-[34px] items-center rounded-sm border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium hover:bg-gray-50">Dashboard</Link>
          <Link
            to="/admin/operations?tab=form-settings"
            className="inline-flex items-center gap-1 rounded-sm border border-gray-300 bg-white px-2.5 py-1.5 text-xs text-gray-500 hover:text-gray-700"
            title="Configure absence form settings"
          >
            ⚙️ Settings
          </Link>
          <Button variant="secondary" onClick={exportCsv}><Download className="mr-1.5 h-4 w-4" />Export CSV</Button>
          <Button variant="secondary" onClick={() => setRefreshToken((value) => value + 1)}><RefreshCcw className="mr-1.5 h-4 w-4" /> Refresh</Button>
        </div>
      </div>

      <section className="mb-4 rounded-sm border border-gray-200 bg-white p-3" aria-label="Absence filters">
        <div className="grid gap-3 md:grid-cols-[minmax(200px,2fr)_1fr_1fr_1fr_1fr]">
          <SearchInput value={filters.query} onChange={(value) => updateFilter("query", value)} placeholder="Search W-Code or name..." />
          <select aria-label="Subject" value={filters.subject} onChange={(event) => updateFilter("subject_id", event.target.value)}>
            <option value="">All subjects</option>
            {subjects.map(([id, label]) => <option key={id} value={id}>{label}</option>)}
          </select>
          <select aria-label="Status" value={filters.status} onChange={(event) => updateFilter("status", event.target.value)}>
            <option value="">All statuses</option>
            <option value="pending">Pending</option>
            <option value="reviewed">Reviewed</option>
            <option value="actioned">Actioned</option>
            <option value="cancelled">Cancelled</option>
          </select>
          <input aria-label="From date" type="date" value={filters.dateFrom} onChange={(event) => updateFilter("date_from", event.target.value)} />
          <input aria-label="To date" type="date" value={filters.dateTo} onChange={(event) => updateFilter("date_to", event.target.value)} />
        </div>
      </section>

      <div className={`overflow-hidden transition-all duration-300 ease-in-out ${selected.size > 0 ? 'max-h-16 mb-3' : 'max-h-0'}`}>
        <div className="flex items-center gap-3 rounded-sm border border-blue-100 bg-blue-50 px-3 py-2 text-sm">
          <span className="font-medium text-blue-800">{selected.size} selected</span>
          <Button size="sm" onClick={() => void markSelectedReviewed()} loading={batchProcessing}>
            {batchProcessing ? `Processing ${batchProgress.done}/${batchProgress.total}...` : "Mark Reviewed"}
          </Button>
          <Button size="sm" variant="secondary" onClick={() => void exportSelected()}>Export Selected</Button>
          <Button size="sm" variant="danger" onClick={() => {
            setCancelTargets(items.filter((item) => selected.has(item.id) && item.status !== "cancelled"));
            setCancelReasonCategory("");
            setCancelReasonDetail("");
          }}>Cancel Selected</Button>
        </div>
      </div>

      {batchProcessing ? (
        <div className="mb-3 overflow-hidden rounded-sm bg-gray-100">
          <div
            className="h-1.5 rounded-sm bg-blue-500 transition-all duration-300"
            style={{ width: `${batchProgress.total > 0 ? (batchProgress.done / batchProgress.total) * 100 : 0}%` }}
          />
        </div>
      ) : null}

      {!batchProcessing && batchFailed.length > 0 ? (
        <div className="mb-3 flex items-center gap-3 rounded-sm border border-amber-200 bg-amber-50 px-3 py-2 text-sm">
          <span className="text-amber-700">{batchFailed.length} failed</span>
          <Button size="sm" variant="secondary" onClick={() => void retryFailed()}>Retry failed</Button>
        </div>
      ) : null}

      <div className="overflow-x-auto rounded-sm border border-gray-200 bg-white">
        <table className="min-w-[1060px] text-sm absence-inbox-table">
          <thead>
            <tr className="text-left text-gray-500">
              <th className="w-8">
                <input aria-label="Select all absences" type="checkbox" checked={allSelected} onChange={(event) => setSelected(event.target.checked ? new Set(items.map((item) => item.id)) : new Set())} />
              </th>
              <th>Status</th>
              <th>Student</th>
              <th>Subject</th>
              <th>Dates</th>
              <th>Sit-in</th>
              <th>Submitted</th>
              <th className="text-right">Actions</th>
            </tr>
          </thead>
          <tbody>
            {items.map((absence) => (
              <tr key={absence.id} className="group cursor-pointer" onClick={() => navigate(`/absences/${absence.id}`)}>
                <td onClick={(event) => event.stopPropagation()}>
                  <input aria-label={`Select ${absence.wcode}`} type="checkbox" checked={selected.has(absence.id)} onChange={(event) => setSelected((current) => {
                    const next = new Set(current);
                    if (event.target.checked) next.add(absence.id);
                    else next.delete(absence.id);
                    return next;
                  })} />
                </td>
                <td><StatusBadge status={absence.status} /></td>
                <td>
                  <div className="flex items-center gap-2">
                    <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-[var(--color-wi-primary)] text-[10px] font-bold text-white">{initials(absence.student_name ?? absence.wcode)}</span>
                    <div>
                      <Link className="font-medium text-[var(--color-wi-primary)] hover:underline" to={`/absences/${absence.id}`} aria-label={`View ${absence.student_name ?? absence.wcode} absence`} onClick={(event) => event.stopPropagation()}>{absence.student_name ?? "Unknown student"}</Link>
                      <div className="font-mono text-xs text-gray-500">{absence.wcode}</div>
                    </div>
                  </div>
                </td>
                <td><span className="rounded-sm bg-slate-100 px-1.5 py-0.5 text-xs font-semibold">{absence.subject_name ?? absence.subject_code ?? "-"}</span></td>
                <td className="whitespace-pre-line align-top text-gray-700">{formatAbsenceSummaryDates(absence)}</td>
                <td>
                  {absence.sit_in_method === "zoom" ? (
                    <span className="rounded-sm bg-blue-50 px-2 py-1 text-xs text-blue-700">Zoom</span>
                  ) : (
                    <span className="rounded-sm bg-emerald-50 px-2 py-1 text-xs text-emerald-700">{formatSitInLabel(absence)}{absence.sit_ins?.length ? ` (${absence.sit_ins.length})` : ""}</span>
                  )}
                </td>
                <td className="whitespace-nowrap text-gray-500">{submittedAgo(absence.created_at)}</td>
                <td onClick={(event) => event.stopPropagation()}>
                  <div className="flex justify-end gap-1">
                    <Link to={`/absences/${absence.id}`} aria-label={`Open details for ${absence.wcode}`} className="inline-flex min-h-[28px] items-center rounded-sm px-2 text-xs text-gray-700 hover:bg-gray-100"><Eye className="mr-1 h-3.5 w-3.5" /> View</Link>
                    {absence.status === "pending" ? <Button size="sm" loading={reviewing === absence.id} onClick={() => void setStatus(absence, "reviewed")}>Mark Reviewed</Button> : null}
                    {absence.status === "reviewed" ? <Button size="sm" loading={reviewing === absence.id} onClick={() => void setStatus(absence, "actioned")}>Actioned</Button> : null}
                    <div className="flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity duration-150">
                      {absence.status !== "cancelled" ? <Button size="sm" variant="ghost" onClick={() => { setCancelTargets([absence]); setCancelReasonCategory(""); setCancelReasonDetail(""); }}>Cancel</Button> : null}
                      <Button size="sm" variant="ghost" className="text-red-600 hover:bg-red-50" onClick={() => setDeleteTarget(absence)}>Delete</Button>
                    </div>
                  </div>
                </td>
              </tr>
            ))}
            {items.length === 0 ? (
              <tr>
                <td colSpan={8}><EmptyState message="All caught up! No absences match these filters." action={
                  <div className="flex justify-center gap-2">
                    <Link to="/absences" className="text-sm text-[var(--color-wi-primary)] hover:underline">View all</Link>
                    <Link to="/absences/dashboard" className="text-sm text-[var(--color-wi-primary)] hover:underline">View dashboard</Link>
                  </div>
                } /></td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>

      <div className="mt-3 flex items-center justify-between text-sm text-gray-500">
        <span>{page?.total_count ?? 0} records</span>
        <div className="flex items-center gap-2">
          <Button variant="secondary" size="sm" disabled={!hasPrevious} onClick={() => updateFilter("offset", String(Math.max(0, filters.offset - PAGE_SIZE)))}>Previous</Button>
          <div className="flex items-center gap-1">
            <input aria-label="Go to page" type="number" min={1} max={totalPages} value={currentPage} onChange={jumpToPage} className="w-14 rounded-sm border border-gray-300 px-2 py-1 text-sm text-center" />
            <span>of {totalPages}</span>
          </div>
          <Button variant="secondary" size="sm" disabled={!hasNext} onClick={() => updateFilter("offset", String(filters.offset + PAGE_SIZE))}>Next</Button>
        </div>
      </div>

      {cancelTargets.length > 0 ? (
        <Modal
          title={cancelTargets.length === 1 ? "Cancel absence" : `Cancel ${cancelTargets.length} absences`}
          onClose={() => { setCancelTargets([]); setCancelReasonCategory(""); setCancelReasonDetail(""); }}
          footer={(
            <>
              {cancelTargets.length === 1 ? (
                <button type="button" className="text-sm text-red-600 hover:text-red-800 hover:underline" onClick={() => { setDeleteTarget(cancelTargets[0]); setCancelTargets([]); setCancelReasonCategory(""); setCancelReasonDetail(""); }}>Delete Permanently</button>
              ) : null}
              <Button variant="secondary" onClick={() => { setCancelTargets([]); setCancelReasonCategory(""); setCancelReasonDetail(""); }}>Back</Button>
              <Button variant="danger" disabled={!cancelReasonCategory} loading={cancelling} onClick={() => void cancelAbsences()}>Cancel Absence</Button>
            </>
          )}
        >
          <p className="mb-3 text-sm text-gray-600">Assigned sit-in sessions will be released. This action is retained in the audit timeline.</p>
          <label className="block text-sm font-medium text-gray-700" htmlFor="inbox-cancel-category">Cancellation reason</label>
          <select id="inbox-cancel-category" className="mt-1 w-full rounded-sm border border-gray-300 p-2 text-sm" value={cancelReasonCategory} onChange={(event) => setCancelReasonCategory(event.target.value)}>
            <option value="">Select a reason...</option>
            {CANCEL_REASON_OPTIONS.map((opt) => <option key={opt.value} value={opt.value}>{opt.label}</option>)}
          </select>
          <label className="mt-3 block text-sm font-medium text-gray-700" htmlFor="inbox-cancel-detail">Additional details (optional)</label>
          <textarea id="inbox-cancel-detail" className="mt-1 w-full rounded-sm border border-gray-300 p-2 text-sm" rows={3} value={cancelReasonDetail} onChange={(event) => setCancelReasonDetail(event.target.value)} />
        </Modal>
      ) : null}

      {deleteTarget ? (
        <Modal
          title="Permanently delete absence"
          onClose={() => setDeleteTarget(null)}
          footer={(
            <>
              <Button variant="secondary" onClick={() => setDeleteTarget(null)}>Back</Button>
              <Button variant="danger" loading={deleting} onClick={() => void deleteAbsence()}>Delete Permanently</Button>
            </>
          )}
        >
          <p className="mb-3 text-sm text-gray-600">
            This will permanently remove the absence record for <strong>{deleteTarget.student_name ?? deleteTarget.wcode}</strong>.
            This action cannot be undone.
          </p>
          <p className="text-sm text-red-600 font-medium">Warning: All associated data will be lost.</p>
        </Modal>
      ) : null}
    </div>
  );
}
