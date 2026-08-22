import { useEffect, useMemo, useRef, useState } from "react";
import SearchableSelect from "@/components/ui/SearchableSelect";
import { Link, useSearchParams } from "react-router-dom";
import { LayoutList, RefreshCw, Rows3, Search } from "lucide-react";
import PageHeading from "../components/ui/PageHeading";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";
import Button from "../components/ui/Button";
import ImpactWorkQueue from "../components/scheduleImpact/ImpactWorkQueue";
import IssueResolutionPanel from "../components/scheduleImpact/IssueResolutionPanel";
import { ApiRequestError, apiJson } from "../api/client";
import { useApiQuery } from "../hooks/useApiQuery";
import { useToast } from "../hooks/useToast";
import { formatBangkokDateTime, notificationMessage } from "../features/scheduleImpact/format";
import { emitImpactEvent, IMPACT_EVENTS } from "../features/scheduleImpact/observability";
import type { ImpactCandidate, ImpactProcessingChange, ResolutionResponse, ScheduleImpactIssue, ScheduleImpactQueueResponse, HistoryItem } from "../features/scheduleImpact/types";

type Density = "comfortable" | "compact";
type View = "queue" | "processing" | "history";

function getStoredDensity(): Density {
  return localStorage.getItem("schedule-impact-density") === "compact" ? "compact" : "comfortable";
}

function viewFrom(value: string | null): View {
  return value === "processing" || value === "history" ? value : "queue";
}

export default function SessionChanges() {
  const { addToast } = useToast();
  const [searchParams, setSearchParams] = useSearchParams();
  const [density, setDensity] = useState<Density>(getStoredDensity);
  const [selected, setSelected] = useState<ScheduleImpactIssue | null>(null);
  const [focusedID, setFocusedID] = useState<string | null>(null);
  const [shortcutAction, setShortcutAction] = useState<"reassign" | "keep" | null>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  const view = viewFrom(searchParams.get("view"));
  const queryText = searchParams.get("q") ?? "";
  const severity = searchParams.get("severity") ?? "";
  const status = searchParams.get("status") ?? "all";
  const offset = parseInt(searchParams.get("offset") ?? "0", 10);
  const rawLimit = parseInt(searchParams.get("limit") ?? "25", 10);
  const limit = (rawLimit === 50 || rawLimit === 100 ? rawLimit : 25) as 25 | 50 | 100;
  const [debouncedQuery, setDebouncedQuery] = useState(queryText);

  useEffect(() => {
    const timer = setTimeout(() => setDebouncedQuery(queryText), 350);
    return () => clearTimeout(timer);
  }, [queryText]);
  const queueURL = useMemo(() => {
    const params = new URLSearchParams({ status, offset: String(offset), limit: String(limit) });
    if (debouncedQuery) params.set("q", debouncedQuery);
    if (severity) params.set("severity", severity);
    return `/api/v1/operations/schedule-impact?${params.toString()}`;
  }, [debouncedQuery, severity, status, offset, limit]);
  const queue = useApiQuery<ScheduleImpactQueueResponse>(view === "queue" ? queueURL : null, [view, queueURL], { keepPreviousData: true });
  const processing = useApiQuery<{ items: ImpactProcessingChange[] }>(view === "processing" ? "/api/v1/operations/schedule-impact/processing" : null, [view]);
  const history = useApiQuery<{ items: HistoryItem[] }>(view === "history" ? "/api/v1/operations/session-changes?limit=100" : null, [view]);
  const items = queue.data?.items ?? [];
  const groupedItems = useMemo(() => {
    const critical = items.filter((item) => item.severity === "critical");
    const review = items.filter((item) => item.status === "needs_review" && item.severity !== "critical");
    const warning = items.filter((item) => item.severity === "warning" && item.status !== "needs_review");
    return [...critical, ...review, ...warning];
  }, [items]);

  useEffect(() => {
    if (queue.data) {
      const itemCount = queue.data.items.length;
      const criticalCount = queue.data.items.filter((i) => i.severity === "critical").length;
      emitImpactEvent(IMPACT_EVENTS.QUEUE_LOADED, {
        item_count: itemCount,
        critical_count: criticalCount,
        has_pagination: !!queue.data.pagination,
      });
    }
  }, [queue.data]);

  useEffect(() => { localStorage.setItem("schedule-impact-density", density); }, [density]);
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.target instanceof HTMLInputElement || event.target instanceof HTMLTextAreaElement || event.target instanceof HTMLSelectElement) return;
      if (event.key === "/") { event.preventDefault(); searchRef.current?.focus(); return; }
      if (event.key === "Escape") { setSelected(null); setShortcutAction(null); return; }
      if (items.length === 0) return;
      const index = focusedID ? items.findIndex((item) => item.id === focusedID) : -1;
      if (event.key.toLowerCase() === "j") { event.preventDefault(); setFocusedID(items[Math.min(index + 1, items.length - 1)].id); }
      if (event.key.toLowerCase() === "k") { event.preventDefault(); setFocusedID(items[Math.max(index - 1, 0)].id); }
      const focused = items.find((item) => item.id === focusedID);
      if (event.key === "Enter" && focused) { event.preventDefault(); setShortcutAction(null); setSelected(focused); }
      if (event.key.toLowerCase() === "r" && focused) { event.preventDefault(); setShortcutAction("reassign"); setSelected(focused); }
      if (event.key.toLowerCase() === "n" && focused) { event.preventDefault(); setShortcutAction("keep"); setSelected(focused); }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [items, focusedID]);

  function updateParams(next: Record<string, string>): void {
    const params = new URLSearchParams(searchParams);
    Object.entries(next).forEach(([key, value]) => { if (value) params.set(key, value); else params.delete(key); });
    if (!("offset" in next) && ("q" in next || "severity" in next || "status" in next)) {
      params.delete("offset");
    }
    setSearchParams(params, { replace: true });
  }

  function setView(nextView: View): void {
    setSelected(null);
    setShortcutAction(null);
    updateParams({ view: nextView === "queue" ? "" : nextView });
  }

  async function resolve(issue: ScheduleImpactIssue, action: "reassign" | "keep" | "cancel" | "dismiss" | "mark_for_review", candidate?: ImpactCandidate, reason?: string): Promise<ResolutionResponse | null> {
    try {
      const result = await apiJson<ResolutionResponse>(`/api/v1/operations/schedule-issues/${issue.id}/resolve`, {
        method: "POST",
        body: JSON.stringify({ action, reason, expected_issue_version: issue.issue_version, ...(candidate ? { candidate_session_id: candidate.session_id, expected_session_version: candidate.session_version } : {}) }),
      });
      addToast(result.notification_status === "not_configured" || result.notification_status === "no_recipient" ? "warning" : "success", notificationMessage(result.notification_status));
      emitImpactEvent(IMPACT_EVENTS.RESOLUTION_SUCCEEDED, {
        issue_id: issue.id,
        action,
        notification_status: result.notification_status,
      });
      await queue.refetch();
      // Keep the panel mounted so its success screen stays visible; the panel's
      // Close button clears selection via onClose.
      setFocusedID(items.find((item) => item.id !== issue.id)?.id ?? null);
      return result;
    } catch (error) {
      if (error instanceof ApiRequestError && error.code === "resolution_conflict") {
        emitImpactEvent(IMPACT_EVENTS.RESOLUTION_CONFLICT, { issue_id: issue.id });
        addToast("warning", "This issue changed while you were reviewing it. The queue has been refreshed.");
      } else {
        emitImpactEvent(IMPACT_EVENTS.RESOLUTION_FAILED, {
          issue_id: issue.id,
          error_type: error instanceof Error ? error.constructor.name : "unknown",
        });
        addToast("error", error instanceof Error ? error.message : "Could not update this issue");
      }
      await queue.refetch();
      return null;
    }
  }

  const tabs: Array<{ id: View; label: string }> = [{ id: "queue", label: "Needs attention" }, { id: "processing", label: "Processing" }, { id: "history", label: "History" }];

  return (
    <div className="mx-auto w-full max-w-6xl">
      <div className="mb-5 flex flex-wrap items-end justify-between gap-3">
        <div><PageHeading>Schedule Impact</PageHeading><p className="mt-1 text-sm text-[var(--color-wi-text-light)]">Resolve student arrangements affected by a schedule change. Times shown in Asia/Bangkok.</p></div>
        <Button variant="secondary" size="sm" loading={queue.refreshing || processing.refreshing || history.refreshing} onClick={() => { void queue.refetch(); void processing.refetch(); void history.refetch(); }}><RefreshCw className="mr-1 h-3.5 w-3.5" aria-hidden="true" />Refresh</Button>
      </div>

      <nav className="mb-4 flex flex-wrap gap-1 border-b border-wi-line" aria-label="Schedule impact views">
        {tabs.map((tab) => <button key={tab.id} type="button" onClick={() => setView(tab.id)} className={`border-b px-3 py-2 text-sm font-medium ${view === tab.id ? "border-[var(--color-wi-primary)] text-[var(--color-wi-primary)]" : "border-transparent text-[var(--color-wi-text-light)] hover:text-[var(--color-wi-text)]"}`}>{tab.label}{tab.id === "queue" && queue.data?.summary.need_attention ? <span className="ml-2 rounded-full bg-red-50 px-1.5 py-0.5 text-xs text-red-700">{queue.data.summary.need_attention}</span> : null}</button>)}
      </nav>

      {view === "queue" ? <>
        <section className="mb-4 flex flex-wrap items-center gap-3 text-sm">
          <span className="font-medium text-[var(--color-wi-text)]">
            {queue.data?.summary.critical ?? 0} critical
          </span>
          <span className="text-[var(--color-wi-text-light)]">·</span>
          <span className="text-[var(--color-wi-text-light)]">
            {queue.data?.summary.need_attention ?? 0} total
          </span>
          <span className="text-[var(--color-wi-text-light)]">·</span>
          <span className="text-amber-700">
            {queue.data?.summary.warnings ?? 0} warnings
          </span>
          {(queue.data?.summary.notification_failures ?? 0) > 0 ? (
            <>
              <span className="text-[var(--color-wi-text-light)]">·</span>
              <span className="text-red-700">
                {queue.data?.summary.notification_failures} notification failures
              </span>
            </>
          ) : null}
        </section>
        {queue.data && !queue.data.summary.notifications_configured ? <section className="mb-4 flex flex-wrap items-center justify-between gap-3 rounded-sm border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-950"><span>SMS and email templates are not configured. Decisions can be recorded, but students will not be notified automatically.</span><Link to="/admin/absence-settings" className="font-medium text-[var(--color-wi-primary)] hover:underline">Open notification settings</Link></section> : null}
        <section className="mb-4 flex flex-wrap items-center gap-2 rounded-sm border border-wi-line bg-white p-3">
          <label className="relative min-w-[220px] flex-1"><Search className="pointer-events-none absolute left-3 top-2 h-4 w-4 text-[var(--color-wi-text-light)]" aria-hidden="true" /><input ref={searchRef} value={queryText} onChange={(event) => updateParams({ q: event.target.value })} placeholder="Search student, course, session" className="w-full pl-9" aria-label="Search student, course, or session" /></label>
          <SearchableSelect value={severity} onChange={(event) => updateParams({ severity: event.target.value })} aria-label="Filter severity"><option value="">All severities</option><option value="critical">Critical</option><option value="warning">Warnings</option></SearchableSelect>
          <SearchableSelect value={status} onChange={(event) => updateParams({ status: event.target.value })} aria-label="Filter issue status"><option value="all">All unresolved</option><option value="open">Open issues</option><option value="needs_review">Needs review</option></SearchableSelect>
          <Button variant="ghost" size="sm" onClick={() => setDensity((current) => current === "comfortable" ? "compact" : "comfortable")} aria-label={`Switch to ${density === "comfortable" ? "compact" : "comfortable"} queue`}><>{density === "comfortable" ? <Rows3 className="h-4 w-4" aria-hidden="true" /> : <LayoutList className="h-4 w-4" aria-hidden="true" />}</></Button>
        </section>
        {queue.loading ? <LoadingSkeleton type="table" lines={6} /> : queue.error ? <p className="rounded-sm border border-red-200 bg-red-50 p-4 text-sm text-red-800">Could not load Schedule Impact: {queue.error.message}</p> : <ImpactWorkQueue items={groupedItems} density={density} selectedID={focusedID} onOpen={(issue) => { setFocusedID(issue.id); setShortcutAction(null); setSelected(issue); }} />}
        {queue.data?.pagination && queue.data.pagination.total > limit ? (
          <nav className="mt-4 flex items-center justify-between text-sm" aria-label="Queue pagination">
            <span className="text-[var(--color-wi-text-light)]">
              Showing {offset + 1}–{Math.min(offset + limit, queue.data.pagination.total)} of {queue.data.pagination.total}
            </span>
            <div className="flex gap-2">
              <Button
                variant="secondary"
                size="sm"
                disabled={offset === 0}
                onClick={() => updateParams({ offset: String(Math.max(0, offset - limit)) })}
              >
                Previous
              </Button>
              <Button
                variant="secondary"
                size="sm"
                disabled={!queue.data.pagination.has_more}
                onClick={() => updateParams({ offset: String(offset + limit) })}
              >
                Next
              </Button>
            </div>
          </nav>
        ) : null}
      </> : null}

      {view === "processing" ? <ProcessingView loading={processing.loading} items={processing.data?.items ?? []} error={processing.error?.message ?? null} /> : null}
      {view === "history" ? <HistoryView loading={history.loading} items={history.data?.items ?? []} error={history.error?.message ?? null} /> : null}
      {selected ? <IssueResolutionPanel issue={selected} initialAction={shortcutAction} onClose={() => { setShortcutAction(null); setSelected(null); }} onResolve={resolve} /> : null}
    </div>
  );
}

function ProcessingView({ loading, items, error }: { loading: boolean; items: ImpactProcessingChange[]; error: string | null }) {
  const { addToast } = useToast();
  const [retrying, setRetrying] = useState<string | null>(null);

  async function retryAnalysis(id: string) {
    setRetrying(id);
    try {
      await apiJson(`/api/v1/operations/session-changes/${id}/reprocess`, { method: "POST", body: JSON.stringify({}) });
      emitImpactEvent(IMPACT_EVENTS.ANALYSIS_RETRIED, { change_id: id });
      addToast("success", "Impact analysis queued for retry");
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Could not retry analysis");
    } finally {
      setRetrying(null);
    }
  }

  if (loading) return <LoadingSkeleton type="table" lines={4} />;
  if (error) return <p className="rounded-sm border border-red-200 bg-red-50 p-4 text-sm text-red-800">Could not load processing changes: {error}</p>;
  if (items.length === 0) return (
    <section className="rounded-sm border border-wi-line bg-white p-8 text-center">
      <h2 className="font-semibold text-[var(--color-wi-text)]">No impact analyses are processing</h2>
      <p className="mt-1 text-sm text-[var(--color-wi-text-light)]">Completed changes are available in History.</p>
    </section>
  );

  return (
    <section className="overflow-hidden rounded-sm border border-wi-line bg-white">
      <div className="divide-y divide-wi-line">
        {items.map((item) => {
          const isFailed = item.status === "failed";
          return (
            <div key={item.id} className="px-4 py-4">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <p className="font-medium text-[var(--color-wi-text)]">
                  {item.subject_name || "Schedule change"}
                </p>
                <span className={`rounded-full px-2 py-0.5 text-xs font-medium ${
                  isFailed ? "bg-red-50 text-red-700"
                  : item.status === "processing" ? "bg-blue-50 text-blue-700"
                  : "bg-[var(--color-wi-row-alt)] text-[var(--color-wi-text-light)]"
                }`}>
                  {item.status === "delayed_by_batch" ? "Waiting for batch" : item.status}
                </span>
              </div>
              <p className="mt-1 text-sm text-[var(--color-wi-text-light)]">
                {isFailed
                  ? "Student arrangements may not have been checked."
                  : "Checking affected arrangements…"}
              </p>
              {item.last_error ? (
                <p className="mt-1 text-xs text-red-700">{item.last_error}</p>
              ) : null}
              {isFailed ? (
                <div className="mt-3 flex gap-2">
                  <Button
                    size="sm"
                    variant="secondary"
                    loading={retrying === item.id}
                    onClick={() => void retryAnalysis(item.id)}
                  >
                    Retry analysis
                  </Button>
                </div>
              ) : null}
            </div>
          );
        })}
      </div>
    </section>
  );
}

function HistoryView({ loading, items, error }: { loading: boolean; items: HistoryItem[]; error: string | null }) {
  if (loading) return <LoadingSkeleton type="table" lines={6} />;
  if (error) return <p className="rounded-sm border border-red-200 bg-red-50 p-4 text-sm text-red-800">Could not load history: {error}</p>;

  return (
    <section className="overflow-hidden rounded-sm border border-wi-line bg-white">
      {items.length === 0 ? (
        <p className="p-6 text-sm text-[var(--color-wi-text-light)]">No completed schedule changes have been recorded.</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="min-w-full text-sm">
            <thead className="bg-[var(--color-wi-row-alt)] text-xs text-[var(--color-wi-text-light)]">
              <tr>
                <th className="px-4 py-3 text-left">Changed</th>
                <th className="px-4 py-3 text-left">Course</th>
                <th className="px-4 py-3 text-left">Change</th>
                <th className="px-4 py-3 text-right">Affected</th>
                <th className="px-4 py-3 text-left">Result</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-wi-line">
              {items.map((item) => (
                <tr key={item.id} className="hover:bg-[var(--color-wi-row-alt)]">
                  <td className="px-4 py-3 text-[var(--color-wi-text-light)] whitespace-nowrap">
                    {formatBangkokDateTime(item.created_at, null)}
                  </td>
                  <td className="px-4 py-3 font-medium text-[var(--color-wi-text)]">
                    <Link to={`/operations/session-changes/${item.id}`} className="text-[var(--color-wi-primary)] hover:underline">
                      {item.new_course_subject || "Schedule change"}
                    </Link>
                  </td>
                  <td className="px-4 py-3 text-[var(--color-wi-text-light)] whitespace-nowrap">
                    {formatBangkokDateTime(item.old_start_at, item.old_end_at)}
                    <span className="px-1 text-[var(--color-wi-text-light)]">→</span>
                    {formatBangkokDateTime(item.new_start_at, item.new_end_at)}
                  </td>
                  <td className="px-4 py-3 text-right text-[var(--color-wi-text-light)]">
                    {item.open_issue_count + item.critical_issue_count > 0
                      ? item.open_issue_count + item.critical_issue_count
                      : "—"}
                  </td>
                  <td className="px-4 py-3">
                    {item.open_issue_count > 0 || item.critical_issue_count > 0 ? (
                      <span className="rounded-full bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-800">
                        {item.open_issue_count + item.critical_issue_count} unresolved
                      </span>
                    ) : (
                      <span className="rounded-full bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700">
                        Completed
                      </span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
