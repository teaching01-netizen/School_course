import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AlertCircle, Bell, CheckCircle2, RefreshCw } from "lucide-react";
import SlideOver from "../SlideOver";
import Button from "../ui/Button";
import LoadingSkeleton from "../ui/LoadingSkeleton";
import ResolutionComparison from "./ResolutionComparison";
import { apiJson } from "../../api/client";
import { formatBangkokDateTime, subjectNameFor } from "../../features/scheduleImpact/format";
import { emitImpactEvent, IMPACT_EVENTS } from "../../features/scheduleImpact/observability";
import type { ImpactCandidate, ResolutionAction, ResolutionResponse, ScheduleImpactIssue } from "../../features/scheduleImpact/types";

type ActivityItem = { action: string; reason: string; created_at: string };

interface IssueResolutionPanelProps {
  issue: ScheduleImpactIssue;
  initialAction?: ResolutionAction | null;
  onClose: () => void;
  onResolve: (issue: ScheduleImpactIssue, action: ResolutionAction, candidate?: ImpactCandidate, reason?: string) => Promise<ResolutionResponse | null>;
}

function capacityLabel(candidate: ImpactCandidate): string {
  if (candidate.available_capacity < 0) return "Capacity not limited";
  if (candidate.available_capacity === 0) return "Full";
  return `${candidate.available_capacity} seats available`;
}

const isCandidateSelectable = (c: ImpactCandidate) => {
  if (!c.eligible) return false;
  if (c.available_capacity === 0) return false;
  if (c.blocking_reasons && c.blocking_reasons.length > 0) return false;
  return true;
};

export default function IssueResolutionPanel({ issue, initialAction = null, onClose, onResolve }: IssueResolutionPanelProps) {
  const candidatesQuery = `/api/v1/operations/schedule-issues/${issue.id}/candidates`;
  const [candidates, setCandidates] = useState<ImpactCandidate[]>([]);
  const [selectedID, setSelectedID] = useState("");
  const [pendingAction, setPendingAction] = useState<ResolutionAction | null>(null);
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const [candidateError, setCandidateError] = useState("");
  const [resolutionError, setResolutionError] = useState<string | null>(null);
  const [resolutionSuccess, setResolutionSuccess] = useState(false);
  const [activity, setActivity] = useState<ActivityItem[]>([]);
  const [loadingCandidates, setLoadingCandidates] = useState(true);
  const liveRegionRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    emitImpactEvent(IMPACT_EVENTS.ISSUE_OPENED, {
      issue_id: issue.id,
      issue_type: issue.issue_type,
      severity: issue.severity,
    });
  }, [issue.id, issue.issue_type, issue.severity]);

  useEffect(() => {
    setCandidates([]);
    setSelectedID("");
    setReason("");
    setPendingAction(null);
    setCandidateError("");
    setResolutionError(null);
    setResolutionSuccess(false);
    setLoadingCandidates(true);
    void apiJson<{ items: ImpactCandidate[] }>(candidatesQuery)
      .then((response) => {
        const fresh = response.items ?? [];
        setCandidates(fresh);
        setSelectedID("");
      })
      .catch(() => setCandidateError("We could not refresh replacement options. Try again before reassigning."))
      .finally(() => setLoadingCandidates(false));
    void apiJson<{ items: ActivityItem[] }>(`/api/v1/operations/schedule-issues/${issue.id}/activity`)
      .then((response) => setActivity(response.items ?? []))
      .catch(() => setActivity([]));
  }, [candidatesQuery]);

  useEffect(() => {
    if (initialAction) setPendingAction(initialAction);
  }, [initialAction, issue.id]);

  const selectedCandidate = useMemo(() => candidates.find((candidate) => candidate.session_id === selectedID), [candidates, selectedID]);
  const requiresReason = pendingAction === "dismiss" || pendingAction === "mark_for_review";

  const confirm = useCallback(async (): Promise<void> => {
    if (!pendingAction || (requiresReason && !reason.trim())) return;
    setBusy(true);
    setResolutionError(null);
    try {
      const result = await onResolve(issue, pendingAction, pendingAction === "reassign" ? selectedCandidate : undefined, reason.trim());
      if (result) {
        setResolutionSuccess(true);
        liveRegionRef.current?.focus();
      } else {
        setPendingAction(null);
        setResolutionError("This issue changed while you were reviewing it. The queue has been refreshed. Please review the updated issue and try again.");
        refreshCandidates();
      }
    } catch {
      setResolutionError("Could not save this resolution. Please try again.");
    } finally {
      setBusy(false);
    }
  }, [pendingAction, requiresReason, reason, issue, selectedCandidate, onResolve]);

  function refreshCandidates(): void {
    setCandidateError("");
    setLoadingCandidates(true);
    void apiJson<{ items: ImpactCandidate[] }>(candidatesQuery)
      .then((response) => {
        const fresh = response.items ?? [];
        setCandidates(fresh);
        // Keep the current selection only while the same session still exists;
        // a version bump is picked up automatically through selectedCandidate.
        setSelectedID((current) => (current && fresh.some((c) => c.session_id === current) ? current : ""));
      })
      .catch(() => setCandidateError("Replacement options are still unavailable. Review the issue or cancel the sit-in."))
      .finally(() => setLoadingCandidates(false));
  }

  const handleAction = useCallback((action: "reassign" | "keep" | "cancel" | "mark_for_review") => {
    setPendingAction(action);
    // A reason chosen for mark_for_review must not carry into a different action.
    if (action !== "mark_for_review") setReason("");
  }, []);

  return (
    <SlideOver title="Resolve issue" onClose={onClose}>
      <div className="space-y-6">
        {/* Accessible live region for resolution results */}
        <div
          ref={liveRegionRef}
          tabIndex={-1}
          aria-live="polite"
          aria-atomic="true"
          className="sr-only"
        >
          {resolutionSuccess ? "Resolution saved successfully." : ""}
          {resolutionError ? `Error: ${resolutionError}` : ""}
        </div>

        {/* Severity and student header */}
        <section>
          <div className="flex flex-wrap items-center gap-2">
            <span className={`rounded-full px-2 py-0.5 text-[11px] font-semibold ${issue.severity === "critical" ? "bg-red-50 text-red-700" : "bg-amber-50 text-amber-800"}`}>{issue.severity === "critical" ? "Critical" : "Warning"}</span>
            <span className="text-xs font-medium text-[var(--color-wi-text-light)]">{subjectNameFor(issue)} · {issue.wcode}</span>
          </div>
          <h2 className="mt-2 text-lg font-semibold text-[var(--color-wi-text)]">{issue.student_name ?? issue.wcode}</h2>
        </section>

        {/* 4-section Resolution Comparison */}
        {resolutionSuccess ? (
          <section className="rounded-sm border border-emerald-200 bg-emerald-50 p-4" role="status">
            <CheckCircle2 className="h-5 w-5 text-emerald-600" aria-hidden="true" />
            <p className="mt-2 text-sm font-medium text-emerald-900">Arrangement updated</p>
            <p className="mt-1 text-xs text-emerald-700">The resolution has been recorded. You can close this panel when ready.</p>
            <div className="mt-4">
              <Button size="sm" onClick={onClose}>Close</Button>
            </div>
          </section>
        ) : (
          <ResolutionComparison
            issue={issue}
            selectedCandidate={selectedCandidate}
            onAction={handleAction}
            busy={busy}
            resolutionError={resolutionError}
          />
        )}

        {/* Candidate selection (only for reassign action) */}
        {pendingAction === "reassign" && !resolutionSuccess ? (
          <section aria-labelledby="candidate-heading">
            <div className="flex items-center justify-between gap-3">
              <div>
                <h3 id="candidate-heading" className="text-sm font-semibold text-[var(--color-wi-text)]">Choose a replacement</h3>
                <p className="mt-1 text-xs text-[var(--color-wi-text-light)]">Options are refreshed when this panel opens.</p>
              </div>
              <Button variant="ghost" size="sm" onClick={refreshCandidates}><RefreshCw className="mr-1 h-3.5 w-3.5" aria-hidden="true" />Refresh</Button>
            </div>
            {candidateError ? (
              <div className="mt-3 rounded-sm border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900" role="alert">
                <AlertCircle className="mr-1 inline h-4 w-4" aria-hidden="true" />{candidateError}
              </div>
            ) : null}
            {loadingCandidates ? (
              <LoadingSkeleton type="text" lines={3} />
            ) : (
              <div className="mt-3 space-y-2" role="radiogroup" aria-label="Replacement session options">
                {candidates.map((candidate) => {
                  const selectable = isCandidateSelectable(candidate);
                  return (
                    <label key={candidate.session_id} className={`block rounded-sm border p-3 ${!selectable ? "cursor-not-allowed border-wi-line-soft bg-[var(--color-wi-row-alt)] text-[var(--color-wi-text-light)]" : selectedID === candidate.session_id ? "cursor-pointer border-[var(--color-wi-primary)] bg-blue-50/60" : "cursor-pointer border-wi-line hover:border-wi-line"}`}>
                      <input type="radio" name="replacement" className="sr-only" checked={selectedID === candidate.session_id} disabled={!selectable || busy} onChange={() => setSelectedID(candidate.session_id)} />
                      <div className="flex gap-3"><span className="mt-0.5 text-sm font-semibold text-[var(--color-wi-primary)]" aria-hidden="true">{selectedID === candidate.session_id ? "●" : "○"}</span><div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-2"><p className="font-medium text-[var(--color-wi-text)]">{formatBangkokDateTime(candidate.start_at, candidate.end_at)}</p>{!selectable ? <span className="rounded-full bg-[var(--color-wi-row-alt)] px-2 py-0.5 text-[10px] font-semibold text-[var(--color-wi-text-light)]">Cannot be selected</span> : null}</div><p className="mt-1 text-sm text-[var(--color-wi-text-light)]">{candidate.room_name || "Room not assigned"} · Teacher: {candidate.teacher || "Not assigned"}</p><div className="mt-1 flex flex-wrap gap-2 text-xs"><span className={candidate.eligible ? "text-emerald-700" : "text-red-700"}>{candidate.eligible ? "\u2713 Eligible" : "\u2717 Not eligible"}</span><span className={candidate.student_conflicts ? "text-red-700" : "text-emerald-700"}>{candidate.student_conflicts ? "\u2717 Conflicts with regular class" : "\u2713 No timetable conflicts"}</span><span className={candidate.available_capacity === 0 ? "text-red-700" : candidate.available_capacity < 0 ? "text-[var(--color-wi-text-light)]" : "text-[var(--color-wi-text-light)]"}>{capacityLabel(candidate)}</span></div></div></div>
                    </label>
                  );
                })}
                {candidates.length === 0 ? (
                  <p className="rounded-sm border border-wi-line bg-[var(--color-wi-row-alt)] p-3 text-sm text-[var(--color-wi-text-light)]">No safe replacement is currently available. Cancel the sit-in or mark this issue for review.</p>
                ) : null}
              </div>
            )}
          </section>
        ) : null}

        {/* Confirmation step */}
        {pendingAction && !resolutionSuccess ? (
          <section className="rounded-sm border border-blue-200 bg-blue-50 p-4" aria-labelledby="confirm-heading">
            <h3 id="confirm-heading" className="text-sm font-semibold text-blue-950">
              {pendingAction === "reassign" ? `Reassign ${issue.student_name ?? issue.wcode}?` : pendingAction === "cancel" ? "Cancel this sit-in?" : pendingAction === "keep" ? "Keep and notify?" : pendingAction === "dismiss" ? "Dismiss this issue?" : "Mark for review?"}
            </h3>
            <p className="mt-1 text-sm text-blue-900">
              {pendingAction === "reassign" ? "The previous sit-in assignment will be removed and the current replacement will be revalidated." : "This decision will be recorded in the activity trail."}
            </p>
            {requiresReason ? (
              <div className="mt-3">
                <label htmlFor="reason-select" className="block text-sm font-medium text-blue-950">Reason</label>
                <select
                  id="reason-select"
                  value={reason}
                  onChange={(event) => setReason(event.target.value)}
                  aria-required="true"
                  aria-describedby={reason ? undefined : "reason-error"}
                  className="mt-1 block w-full rounded-sm border border-blue-300 bg-white px-2 py-1.5 text-sm"
                >
                  <option value="">Select a reason</option>
                  <option value="Duplicate issue">Duplicate issue</option>
                  <option value="Student no longer needs a sit-in">Student no longer needs a sit-in</option>
                  <option value="Schedule data is incorrect">Schedule data is incorrect</option>
                  <option value="No action required">No action required</option>
                  <option value="Needs owner review">Needs owner review</option>
                </select>
                {requiresReason && !reason.trim() ? (
                  <p id="reason-error" className="mt-1 text-xs text-red-700" role="alert">A reason is required for this action.</p>
                ) : null}
              </div>
            ) : null}
            <div className="mt-4 flex justify-end gap-2">
              <Button variant="secondary" size="sm" onClick={() => { setPendingAction(null); setResolutionError(null); }} disabled={busy}>Back</Button>
              <Button
                size="sm"
                loading={busy}
                disabled={busy || (requiresReason && !reason.trim())}
                onClick={() => void confirm()}
              >
                {pendingAction === "reassign" ? "Confirm reassignment"
                  : pendingAction === "cancel" ? "Cancel arrangement"
                  : pendingAction === "keep" ? "Keep arrangement and notify"
                  : pendingAction === "mark_for_review" ? "Send for manual review"
                  : "Confirm"}
              </Button>
            </div>
          </section>
        ) : null}

        {/* Activity trail */}
        {!resolutionSuccess ? (
          <section className="border-t border-wi-line pt-4" aria-labelledby="activity-heading">
            <h3 id="activity-heading" className="text-sm font-semibold text-[var(--color-wi-text)]">Activity</h3>
            {activity.length ? (
              <ol className="mt-2 space-y-2">
                {activity.map((item, index) => (
                  <li key={`${item.created_at}-${index}`} className="text-sm text-[var(--color-wi-text-light)]">
                    <span className="font-medium">{item.action.replace(/_/g, " ")}</span>
                    {item.reason ? ` · ${item.reason}` : ""}
                    <span className="block text-xs text-[var(--color-wi-text-light)]">{formatBangkokDateTime(item.created_at, null)}</span>
                  </li>
                ))}
              </ol>
            ) : (
              <p className="mt-2 text-sm text-[var(--color-wi-text-light)]">No previous activity is recorded for this arrangement.</p>
            )}
          </section>
        ) : null}

        {/* Technical details */}
        {!resolutionSuccess ? (
          <details className="border-t border-wi-line pt-4">
            <summary className="cursor-pointer text-sm font-medium text-[var(--color-wi-text-light)]">Technical details</summary>
            <dl className="mt-3 space-y-1 text-xs text-[var(--color-wi-text-light)]">
              <div><dt className="inline font-semibold">Issue ID: </dt><dd className="inline break-all">{issue.id}</dd></div>
              <div><dt className="inline font-semibold">Issue version: </dt><dd className="inline">{issue.issue_version}</dd></div>
              <div><dt className="inline font-semibold">Timezone: </dt><dd className="inline">Asia/Bangkok</dd></div>
            </dl>
          </details>
        ) : null}

        {/* Notification status hint */}
        {!resolutionSuccess ? (
          <p className="flex items-center gap-1 text-xs text-[var(--color-wi-text-light)]">
            <Bell className="h-3.5 w-3.5" aria-hidden="true" />
            Notification status is confirmed after the decision is saved.
          </p>
        ) : null}
      </div>
    </SlideOver>
  );
}
