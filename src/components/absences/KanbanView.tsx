import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { apiJson } from "../../api/client";
import { useToast } from "../../hooks/useToast";
import type { AbsencePage, AbsenceStatus, ManagedAbsence } from "../../types";
import EmptyState from "../ui/EmptyState";
import Button from "../ui/Button";
import Modal from "../Modal";

export const COLUMNS: { key: AbsenceStatus; label: string }[] = [
  { key: "pending", label: "Pending" },
  { key: "reviewed", label: "Reviewed" },
  { key: "actioned", label: "Actioned" },
];

const COLUMN_STYLES: Record<string, string> = {
  pending: "border-blue-200 bg-blue-50/30",
  reviewed: "border-emerald-200 bg-emerald-50/30",
  actioned: "border-slate-200 bg-slate-50/30",
};

const PAGE_SIZE = 20;

function formatDate(value: string): string {
  return new Date(value + "T00:00:00").toLocaleDateString("en-GB", { day: "numeric", month: "short" });
}

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

function dateSpan(absence: ManagedAbsence): string {
  return `${formatDate(absence.date_from)} - ${formatDate(absence.date_to)}`;
}

function AbsenceCard({
  absence,
  reviewingId,
  onMarkReviewed,
  onCancelClick,
}: {
  absence: ManagedAbsence;
  reviewingId: string | null;
  onMarkReviewed: (a: ManagedAbsence) => void;
  onCancelClick: (a: ManagedAbsence) => void;
}) {
  const navigate = useNavigate();
  return (
    <div className="group relative rounded-sm border border-gray-200 bg-white p-3 text-sm shadow-sm transition-shadow hover:shadow-md cursor-pointer" onClick={() => navigate(`/absences/${absence.id}`)} onKeyDown={(e) => { if (e.key === "Enter") navigate(`/absences/${absence.id}`); }} tabIndex={0}>
      <div className="flex items-start gap-2.5">
        <span className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-[var(--color-wi-primary)] text-xs font-bold text-white">{initials(absence.student_name ?? absence.wcode)}</span>
        <div className="min-w-0 flex-1">
          <p className="truncate font-medium text-gray-900">{absence.student_name ?? "Unknown"}</p>
          <p className="font-mono text-xs text-gray-500">{absence.wcode}</p>
        </div>
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-1.5">
        <span className="rounded-sm bg-slate-100 px-1.5 py-0.5 text-xs font-semibold">{absence.subject_code ?? "-"}</span>
        <span className="text-xs text-gray-600">{dateSpan(absence)}</span>
      </div>
      <div className="mt-1.5 flex items-center justify-between gap-2">
        {absence.sit_in_method === "zoom" ? (
          <span className="rounded-sm bg-blue-50 px-2 py-0.5 text-xs text-blue-700">Zoom</span>
        ) : absence.sit_in_method === "physical" ? (
          <span className="rounded-sm bg-emerald-50 px-2 py-0.5 text-xs text-emerald-700">{absence.sit_in_course_code ?? "Physical"}</span>
        ) : (
          <span className="rounded-sm bg-gray-50 px-2 py-0.5 text-xs text-gray-500">Pending</span>
        )}
        <span className="text-xs text-gray-400">{submittedAgo(absence.created_at)}</span>
      </div>
      <div className="mt-2 flex gap-1.5 opacity-0 group-hover:opacity-100 transition-opacity" onClick={(e) => e.stopPropagation()} onKeyDown={(e) => e.stopPropagation()}>
        {absence.status === "pending" ? (
          <button disabled={reviewingId === absence.id} onClick={() => onMarkReviewed(absence)} className="rounded-sm bg-blue-600 px-2 py-1 text-xs font-medium text-white hover:bg-blue-700 disabled:opacity-50">{reviewingId === absence.id ? "..." : "Mark Reviewed"}</button>
        ) : null}
        {absence.status !== "cancelled" ? (
          <button onClick={() => onCancelClick(absence)} className="rounded-sm px-2 py-1 text-xs font-medium text-red-600 hover:bg-red-50">Cancel</button>
        ) : null}
      </div>
    </div>
  );
}

export default function KanbanView({ filters }: { filters: { query: string; subject: string; dateFrom: string; dateTo: string } }) {
  const { addToast } = useToast();
  const [columns, setColumns] = useState<Record<AbsenceStatus, ManagedAbsence[]>>(() => ({ pending: [], reviewed: [], actioned: [], cancelled: [] }));
  const [offsets, setOffsets] = useState<Record<string, number>>(() => ({ pending: 0, reviewed: 0, actioned: 0 }));
  const [loading, setLoading] = useState<Record<string, boolean>>({});
  const [initialLoadDone, setInitialLoadDone] = useState(false);
  const [reviewingId, setReviewingId] = useState<string | null>(null);
  const [cancelTarget, setCancelTarget] = useState<ManagedAbsence | null>(null);
  const [cancelReason, setCancelReason] = useState("");
  const [cancelling, setCancelling] = useState(false);
  const [showCancelled, setShowCancelled] = useState(false);
  const [cancelledPage, setCancelledPage] = useState<ManagedAbsence[]>([]);
  const [cancelledLoading, setCancelledLoading] = useState(false);
  const [cancelledOffset, setCancelledOffset] = useState(0);

  function buildQuery(status: AbsenceStatus, offset: number) {
    const params = new URLSearchParams();
    params.set("status", status);
    params.set("limit", String(PAGE_SIZE));
    params.set("offset", String(offset));
    if (filters.query) params.set("query", filters.query);
    if (filters.subject) params.set("subject_id", filters.subject);
    if (filters.dateFrom) params.set("date_from", filters.dateFrom);
    if (filters.dateTo) params.set("date_to", filters.dateTo);
    return params.toString();
  }

  const loadColumn = async (status: AbsenceStatus, offset: number, append = false) => {
    setLoading((prev) => ({ ...prev, [status]: true }));
    try {
      const result = await apiJson<AbsencePage>(`/api/v1/absences?${buildQuery(status, offset)}`, { method: "GET" });
      setColumns((prev) => ({ ...prev, [status]: append ? [...prev[status], ...result.items] : result.items }));
      setOffsets((prev) => ({ ...prev, [status]: offset + PAGE_SIZE }));
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to load absences");
    } finally {
      setLoading((prev) => ({ ...prev, [status]: false }));
    }
  };

  useEffect(() => {
    const init = async () => {
      await Promise.all([loadColumn("pending", 0), loadColumn("reviewed", 0), loadColumn("actioned", 0)]);
      setInitialLoadDone(true);
    };
    init();
  }, []);

  function loadMore(status: AbsenceStatus) {
    void loadColumn(status, offsets[status], true);
  }

  const loadCancelled = async (offset: number) => {
    setCancelledLoading(true);
    try {
      const result = await apiJson<AbsencePage>(`/api/v1/absences?${buildQuery("cancelled", offset)}`, { method: "GET" });
      setCancelledPage((prev) => offset === 0 ? result.items : [...prev, ...result.items]);
      setCancelledOffset(offset + PAGE_SIZE);
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to load cancelled absences");
    } finally {
      setCancelledLoading(false);
    }
  };

  async function handleMarkReviewed(absence: ManagedAbsence) {
    setReviewingId(absence.id);
    try {
      await apiJson(`/api/v1/absences/${absence.id}/status`, { method: "PUT", body: JSON.stringify({ status: "reviewed", expected_version: absence.version }) });
      setColumns((prev) => ({ ...prev, pending: prev.pending.filter((a) => a.id !== absence.id), reviewed: [{ ...absence, status: "reviewed" }, ...prev.reviewed] }));
      addToast("success", "Absence marked reviewed");
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Update failed");
    } finally {
      setReviewingId(null);
    }
  }

  async function handleCancel() {
    if (!cancelTarget || !cancelReason.trim()) return;
    setCancelling(true);
    try {
      await apiJson(`/api/v1/absences/${cancelTarget.id}/status`, { method: "PUT", body: JSON.stringify({ status: "cancelled", expected_version: cancelTarget.version, reason: cancelReason.trim() }) });
      setColumns((prev) => ({ ...prev, [cancelTarget.status]: prev[cancelTarget.status].filter((a) => a.id !== cancelTarget.id) }));
      setCancelTarget(null);
      setCancelReason("");
      addToast("success", "Absence cancelled");
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Cancel failed");
    } finally {
      setCancelling(false);
    }
  }

  if (!initialLoadDone && Object.values(loading).some(Boolean)) return null;

  return (
    <>
      <div className="grid gap-4 md:grid-cols-3">
        {COLUMNS.map(({ key, label }) => (
          <div key={key} className={`rounded-sm border p-3 ${COLUMN_STYLES[key] ?? "border-gray-200 bg-gray-50/30"}`}>
            <div className="mb-3 flex items-center justify-between">
              <h3 className="text-sm font-semibold text-gray-800">{label}</h3>
              <span className="rounded-full bg-white px-2 py-0.5 text-xs font-medium text-gray-600">{columns[key].length}</span>
            </div>
            <div className="space-y-2 max-h-[70vh] overflow-y-auto pr-1">
              {columns[key].map((absence) => (
                <AbsenceCard key={absence.id} absence={absence} reviewingId={reviewingId} onMarkReviewed={handleMarkReviewed} onCancelClick={setCancelTarget} />
              ))}
              {columns[key].length === 0 && !loading[key] ? <EmptyState message={`No ${label.toLowerCase()} absences.`} /> : null}
              {loading[key] && columns[key].length > 0 ? <div className="py-2 text-center text-xs text-gray-400">Loading...</div> : null}
              <button onClick={() => loadMore(key)} disabled={loading[key]} className="w-full py-2 text-xs font-medium text-[var(--color-wi-primary)] hover:underline disabled:text-gray-300">{loading[key] ? "Loading..." : "Load more"}</button>
            </div>
          </div>
        ))}
      </div>

      <div className="mt-4">
        <button onClick={() => setShowCancelled(!showCancelled)} className="flex items-center gap-2 text-sm font-medium text-gray-600 hover:text-gray-900">
          <span className={`transition-transform ${showCancelled ? "rotate-90" : ""}`}>&#9654;</span>
          Cancelled ({cancelledPage.length}{showCancelled ? "" : "..."})
        </button>
        {showCancelled && (
          <div className="mt-2 grid gap-2 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4">
            {cancelledPage.map((absence) => (
              <div key={absence.id} className="cursor-pointer rounded-sm border border-red-100 bg-red-50/30 p-3 text-sm opacity-70 hover:opacity-100" onClick={() => window.location.href = `/absences/${absence.id}`} role="button" tabIndex={0} onKeyDown={(e) => { if (e.key === "Enter") window.location.href = `/absences/${absence.id}`; }}>
                <div className="flex items-center gap-2">
                  <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-gray-400 text-[10px] font-bold text-white">{initials(absence.student_name ?? absence.wcode)}</span>
                  <span className="truncate font-medium text-gray-700">{absence.student_name ?? absence.wcode}</span>
                </div>
                <div className="mt-1 flex gap-1.5">
                  <span className="rounded-sm bg-slate-100 px-1 py-0.5 text-xs">{absence.subject_code}</span>
                  <span className="text-xs text-gray-500">{dateSpan(absence)}</span>
                </div>
              </div>
            ))}
            {cancelledLoading ? <p className="text-xs text-gray-400 col-span-full text-center py-2">Loading...</p> : null}
            <button onClick={() => loadCancelled(cancelledOffset)} disabled={cancelledLoading} className="col-span-full py-2 text-xs font-medium text-[var(--color-wi-primary)] hover:underline disabled:text-gray-300">Load more</button>
          </div>
        )}
      </div>

      {cancelTarget ? (
        <Modal title="Cancel absence" onClose={() => { setCancelTarget(null); setCancelReason(""); }}
          footer={<><Button variant="secondary" onClick={() => { setCancelTarget(null); setCancelReason(""); }}>Back</Button><Button variant="danger" disabled={!cancelReason.trim()} loading={cancelling} onClick={() => void handleCancel()}>Cancel Absence</Button></>}>
          <p className="mb-3 text-sm text-gray-600">Assigned sit-in sessions will be released.</p>
          <label className="block text-sm font-medium text-gray-700" htmlFor="kanban-cancel-reason">Cancellation reason</label>
          <textarea id="kanban-cancel-reason" className="mt-2 w-full rounded-sm border border-gray-300 p-2 text-sm" rows={3} value={cancelReason} onChange={(event) => setCancelReason(event.target.value)} />
        </Modal>
      ) : null}
    </>
  );
}