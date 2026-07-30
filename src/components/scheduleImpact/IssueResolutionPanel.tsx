import { useEffect, useMemo, useState } from "react";
import { AlertCircle, Bell, CheckCircle2, ChevronDown, RefreshCw } from "lucide-react";
import SlideOver from "../SlideOver";
import Button from "../ui/Button";
import { apiJson } from "../../api/client";
import { formatBangkokDateTime, issueConsequence, issueMessage } from "../../features/scheduleImpact/format";
import type { ImpactCandidate, ResolutionResponse, ScheduleImpactIssue } from "../../features/scheduleImpact/types";

type ResolutionAction = "reassign" | "keep" | "cancel" | "dismiss" | "mark_for_review";
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

export default function IssueResolutionPanel({ issue, initialAction = null, onClose, onResolve }: IssueResolutionPanelProps) {
  const candidatesQuery = `/api/v1/operations/schedule-issues/${issue.id}/candidates`;
  const [candidates, setCandidates] = useState<ImpactCandidate[]>([]);
  const [selectedID, setSelectedID] = useState("");
  const [showMore, setShowMore] = useState(false);
  const [pendingAction, setPendingAction] = useState<ResolutionAction | null>(null);
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const [candidateError, setCandidateError] = useState("");
  const [activity, setActivity] = useState<ActivityItem[]>([]);

  useEffect(() => {
    setCandidates([]);
    setSelectedID("");
    setReason("");
    setPendingAction(null);
    setCandidateError("");
    void apiJson<{ items: ImpactCandidate[] }>(candidatesQuery)
      .then((response) => {
        const fresh = response.items ?? [];
        setCandidates(fresh);
        setSelectedID(fresh[0]?.session_id ?? "");
      })
      .catch(() => setCandidateError("We could not refresh replacement options. Try again before reassigning."));
    void apiJson<{ items: ActivityItem[] }>(`/api/v1/operations/schedule-issues/${issue.id}/activity`)
      .then((response) => setActivity(response.items ?? []))
      .catch(() => setActivity([]));
  }, [candidatesQuery]);

  useEffect(() => {
    if (initialAction) setPendingAction(initialAction);
  }, [initialAction, issue.id]);

  const selectedCandidate = useMemo(() => candidates.find((candidate) => candidate.session_id === selectedID), [candidates, selectedID]);
  const requiresReason = pendingAction === "dismiss" || pendingAction === "mark_for_review";

  async function confirm(): Promise<void> {
    if (!pendingAction || (requiresReason && !reason.trim())) return;
    setBusy(true);
    try {
      const result = await onResolve(issue, pendingAction, pendingAction === "reassign" ? selectedCandidate : undefined, reason.trim());
      if (result) onClose();
      else {
        setPendingAction(null);
        refreshCandidates();
      }
    } finally {
      setBusy(false);
    }
  }

  function refreshCandidates(): void {
    setCandidateError("");
    void apiJson<{ items: ImpactCandidate[] }>(candidatesQuery)
      .then((response) => {
        const fresh = response.items ?? [];
        setCandidates(fresh);
        setSelectedID(fresh[0]?.session_id ?? "");
      })
      .catch(() => setCandidateError("Replacement options are still unavailable. Review the issue or cancel the sit-in."));
  }

  return (
    <SlideOver title="Resolve issue" onClose={onClose}>
      <div className="space-y-6">
        <section>
          <div className="flex flex-wrap items-center gap-2">
            <span className={`rounded-full px-2 py-0.5 text-[11px] font-semibold ${issue.severity === "critical" ? "bg-red-50 text-red-700" : "bg-amber-50 text-amber-800"}`}>{issue.severity === "critical" ? "Critical" : "Warning"}</span>
            <span className="text-xs font-medium text-gray-500">{(issue.assignment_context.current_session?.course_code ?? issue.assignment_context.original_session.snapshot?.course_code as string) ?? ""} · {issue.wcode}</span>
          </div>
          <h2 className="mt-2 text-lg font-semibold text-gray-900">{issue.student_name ?? issue.wcode}</h2>
          <p className="mt-2 text-sm font-medium text-gray-800">{issueMessage(issue)}</p>
          <p className="mt-1 text-sm text-gray-600">{issueConsequence(issue)}</p>
        </section>

        <section className="border-y border-gray-200 py-4">
          <h3 className="text-sm font-semibold text-gray-900">Current plan</h3>
          <p className="mt-1 text-sm text-gray-700">{formatBangkokDateTime(issue.start_at, issue.end_at)}</p>
          {issue.status === "needs_review" ? <p className="mt-2 text-sm text-amber-800">Marked for review</p> : null}
        </section>

        <section>
          <div className="flex items-center justify-between gap-3">
            <div><h3 className="text-sm font-semibold text-gray-900">Choose a replacement</h3><p className="mt-1 text-xs text-gray-500">Options are refreshed when this panel opens.</p></div>
            <Button variant="ghost" size="sm" onClick={refreshCandidates}><RefreshCw className="mr-1 h-3.5 w-3.5" aria-hidden="true" />Refresh</Button>
          </div>
          {candidateError ? <div className="mt-3 rounded-sm border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900"><AlertCircle className="mr-1 inline h-4 w-4" aria-hidden="true" />{candidateError}</div> : null}
          <div className="mt-3 space-y-2">
            {candidates.map((candidate, index) => (
              <label key={candidate.session_id} className={`block cursor-pointer rounded-sm border p-3 ${selectedID === candidate.session_id ? "border-[var(--color-wi-primary)] bg-blue-50/60" : "border-gray-200 hover:border-gray-300"}`}>
                <input type="radio" name="replacement" className="sr-only" checked={selectedID === candidate.session_id} onChange={() => setSelectedID(candidate.session_id)} />
                <div className="flex gap-3"><span className="mt-0.5 text-sm font-semibold text-[var(--color-wi-primary)]" aria-hidden="true">{selectedID === candidate.session_id ? "●" : "○"}</span><div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-2"><p className="font-medium text-gray-900">{formatBangkokDateTime(candidate.start_at, candidate.end_at)}</p>{index === 0 ? <span className="rounded-full bg-blue-100 px-2 py-0.5 text-[10px] font-semibold text-blue-800">Best match</span> : null}</div><p className="mt-1 text-sm text-gray-600">{candidate.room_name || "Room not assigned"} · Teacher: {candidate.teacher || "Not assigned"} · {capacityLabel(candidate)}</p><p className="mt-1 text-xs text-emerald-700">No student conflicts · Eligible</p></div></div>
              </label>
            ))}
            {!candidateError && candidates.length === 0 ? <p className="rounded-sm border border-gray-200 bg-gray-50 p-3 text-sm text-gray-600">No safe replacement is currently available. Cancel the sit-in or mark this issue for review.</p> : null}
          </div>
        </section>

        {pendingAction ? <section className="rounded-sm border border-blue-200 bg-blue-50 p-4"><h3 className="text-sm font-semibold text-blue-950">{pendingAction === "reassign" ? `Reassign ${issue.student_name ?? issue.wcode}?` : pendingAction === "cancel" ? "Cancel this sit-in?" : pendingAction === "keep" ? "Keep and notify?" : pendingAction === "dismiss" ? "Dismiss this issue?" : "Mark for review?"}</h3><p className="mt-1 text-sm text-blue-900">{pendingAction === "reassign" ? "The previous sit-in assignment will be removed and the current replacement will be revalidated." : "This decision will be recorded in the activity trail."}</p>{requiresReason ? <label className="mt-3 block text-sm font-medium text-blue-950">Reason<select value={reason} onChange={(event) => setReason(event.target.value)} className="mt-1 block w-full bg-white"><option value="">Select a reason</option><option value="Duplicate issue">Duplicate issue</option><option value="Student no longer needs a sit-in">Student no longer needs a sit-in</option><option value="Schedule data is incorrect">Schedule data is incorrect</option><option value="No action required">No action required</option><option value="Needs owner review">Needs owner review</option></select></label> : null}<div className="mt-4 flex justify-end gap-2"><Button variant="secondary" size="sm" onClick={() => setPendingAction(null)}>Back</Button><Button size="sm" loading={busy} disabled={requiresReason && !reason.trim()} onClick={() => void confirm()}>{pendingAction === "reassign" ? "Confirm reassignment" : "Confirm"}</Button></div></section> : null}

        {!pendingAction ? <section className="space-y-2"><div className="grid grid-cols-2 gap-2"><Button disabled={!selectedCandidate} onClick={() => setPendingAction("reassign")}>Reassign</Button><Button variant="secondary" onClick={() => setPendingAction("keep")}>Keep and notify</Button></div><Button variant="danger" className="w-full" onClick={() => setPendingAction("cancel")}>Cancel sit-in</Button><button type="button" className="flex w-full items-center justify-center gap-1 py-1 text-sm font-medium text-gray-700 hover:text-gray-950" onClick={() => setShowMore((current) => !current)}>More <ChevronDown className={`h-4 w-4 transition-transform ${showMore ? "rotate-180" : ""}`} aria-hidden="true" /></button>{showMore ? <div className="grid grid-cols-2 gap-2"><Button variant="ghost" onClick={() => setPendingAction("mark_for_review")}>Mark for review</Button><Button variant="ghost" onClick={() => setPendingAction("dismiss")}>Dismiss</Button></div> : null}</section> : null}

        <section className="border-t border-gray-200 pt-4"><h3 className="text-sm font-semibold text-gray-900">Activity</h3>{activity.length ? <ol className="mt-2 space-y-2">{activity.map((item, index) => <li key={`${item.created_at}-${index}`} className="text-sm text-gray-700"><span className="font-medium">{item.action.replace(/_/g, " ")}</span>{item.reason ? ` · ${item.reason}` : ""}<span className="block text-xs text-gray-500">{formatBangkokDateTime(item.created_at, null)}</span></li>)}</ol> : <p className="mt-2 text-sm text-gray-500">No previous activity is recorded for this arrangement.</p>}</section>

        <details className="border-t border-gray-200 pt-4"><summary className="cursor-pointer text-sm font-medium text-gray-700">Technical details</summary><dl className="mt-3 space-y-1 text-xs text-gray-500"><div><dt className="inline font-semibold">Issue ID: </dt><dd className="inline break-all">{issue.id}</dd></div><div><dt className="inline font-semibold">Issue version: </dt><dd className="inline">{issue.issue_version}</dd></div><div><dt className="inline font-semibold">Timezone: </dt><dd className="inline">Asia/Bangkok</dd></div></dl></details>

        <p className="flex items-center gap-1 text-xs text-gray-500"><Bell className="h-3.5 w-3.5" aria-hidden="true" />Notification status is confirmed after the decision is saved.</p>
        {issue.status === "open" ? null : <p className="flex items-center gap-1 text-sm text-emerald-700"><CheckCircle2 className="h-4 w-4" aria-hidden="true" />This issue is no longer open.</p>}
      </div>
    </SlideOver>
  );
}
