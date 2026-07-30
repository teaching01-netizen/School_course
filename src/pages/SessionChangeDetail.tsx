import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import PageHeading from "../components/ui/PageHeading";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";
import Button from "../components/ui/Button";
import { apiJson } from "../api/client";
import { useApiQuery } from "../hooks/useApiQuery";
import { useToast } from "../hooks/useToast";

type ScheduleIssue = {
  id: string;
  absence_id: string;
  issue_type: string;
  severity: "warning" | "critical";
  status: string;
  details: Record<string, unknown>;
  wcode: string;
  student_name: string | null;
  start_at: string | null;
  end_at: string | null;
  resolution_action: string | null;
};

type NotificationStatus = {
  id: string;
  absence_id: string;
  message_type: string;
  channel: "sms" | "email";
  status: string;
  attempt_count: number;
  failure_reason: string | null;
  provider_message_id: string | null;
  created_at: string;
  sent_at: string | null;
};

type SessionChangeDetailResponse = {
  change: {
    id: string;
    session_id: string;
    session_version: number;
    change_source: string;
    changed_fields: Record<string, unknown>;
    before_snapshot: Record<string, unknown>;
    after_snapshot: Record<string, unknown>;
    old_start_at: string;
    old_end_at: string;
    new_start_at: string;
    new_end_at: string;
    old_course: { code: string; name: string };
    new_course: { code: string; name: string };
    open_issue_count: number;
    critical_issue_count: number;
    created_at: string;
  };
  issues: ScheduleIssue[];
  notifications: NotificationStatus[];
};

function formatDateTime(value: string | null): string {
  if (!value) return "-";
  return new Date(value).toLocaleString("en-GB", { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit", timeZone: "Asia/Bangkok" });
}

function titleCase(value: string): string {
  return value.replace(/_/g, " ").replace(/\b\w/g, (letter: string) => letter.toUpperCase());
}

export default function SessionChangeDetail() {
  const { id = "" } = useParams();
  const { addToast } = useToast();
  const query = useApiQuery<SessionChangeDetailResponse>(id ? `/api/v1/operations/session-changes/${id}` : null, [id]);
  const [busyIssue, setBusyIssue] = useState<string | null>(null);
  const change = query.data?.change;
  const issues = query.data?.issues ?? [];
  const notifications = query.data?.notifications ?? [];
  const visibleNotifications = notifications.filter((notification) => notification.status !== "delivered");
  const deliveredCount = notifications.length - visibleNotifications.length;

  async function reprocess() {
    setBusyIssue("reprocess");
    try {
      await apiJson(`/api/v1/operations/session-changes/${id}/reprocess`, { method: "POST", body: JSON.stringify({}) });
      addToast("success", "Impact analysis refreshed");
      await query.refetch();
    } catch (error) {
      addToast("error", error instanceof Error ? error.message : "Could not reprocess impact analysis");
    } finally {
      setBusyIssue(null);
    }
  }

  async function updateNotification(notification: NotificationStatus, action: "retry" | "cancel") {
    setBusyIssue(notification.id);
    try {
      await apiJson(`/api/v1/operations/notifications/${notification.id}/${action}`, { method: "POST", body: JSON.stringify({}) });
      addToast("success", action === "retry" ? "Notification queued for retry" : "Notification cancelled");
      await query.refetch();
    } catch (error) {
      addToast("error", error instanceof Error ? error.message : "Could not update notification");
    } finally {
      setBusyIssue(null);
    }
  }

  if (query.loading) return <LoadingSkeleton type="table" lines={6} />;
  if (query.error || !change) return <p className="text-sm text-red-600">Could not load this session change.</p>;

  return (
    <div className="mx-auto w-full max-w-6xl">
      <Link to="/operations/schedule-impact?view=history" className="text-sm text-[var(--color-wi-primary)] hover:underline">Back to Schedule Impact</Link>
      <div className="mt-2 flex flex-wrap items-end justify-between gap-3">
        <div>
          <PageHeading>Session Change Impact</PageHeading>
          <p className="mt-1 text-sm text-gray-500">{change.new_course.code} {change.new_course.name} · {Object.keys(change.changed_fields).map(titleCase).join(", ") || titleCase(change.change_source)} changed</p>
        </div>
        <div className="flex items-center gap-2">
          <Button size="sm" variant="secondary" loading={busyIssue === "reprocess"} onClick={() => void reprocess()}>Reprocess impact</Button>
          <Link to="/schedule" className="text-sm font-medium text-[var(--color-wi-primary)] hover:underline">Open schedule</Link>
        </div>
      </div>

      <section className="mt-5 rounded-sm border border-gray-200 bg-white p-4">
        <div className="grid gap-4 md:grid-cols-3">
          <div><p className="text-xs uppercase tracking-wide text-gray-400">Previous schedule</p><p className="mt-1 font-medium text-gray-900">{formatDateTime(change.old_start_at)} - {formatDateTime(change.old_end_at)}</p><p className="text-sm text-gray-600">{change.old_course.code} {change.old_course.name}</p></div>
          <div><p className="text-xs uppercase tracking-wide text-gray-400">Current schedule</p><p className="mt-1 font-medium text-gray-900">{formatDateTime(change.new_start_at)} - {formatDateTime(change.new_end_at)}</p><p className="text-sm text-gray-600">{change.new_course.code} {change.new_course.name}</p></div>
          <div className="md:text-right"><p className="text-xs uppercase tracking-wide text-gray-400">Impact status</p><p className="mt-1 text-2xl font-semibold text-gray-900">{change.open_issue_count}</p><p className="text-sm text-gray-500">open issues {change.critical_issue_count > 0 ? `· ${change.critical_issue_count} critical` : ""}</p></div>
        </div>
      </section>

      <section className="mt-4 rounded-sm border border-gray-200 bg-white">
        <div className="border-b border-gray-100 bg-gray-50/70 px-4 py-3 text-sm font-semibold text-gray-800">Affected absence plans</div>
        {issues.length === 0 ? <p className="p-6 text-sm text-gray-500">No affected absence issues were found.</p> : (
          <div className="divide-y divide-gray-100">
            {issues.map((issue) => {
              const unresolved = issue.status === "open" || issue.status === "needs_review";
              return (
                <article key={issue.id} className="p-4">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div>
                      <div className="flex flex-wrap items-center gap-2">
                        <Link to={`/absences/${issue.absence_id}`} className="font-medium text-[var(--color-wi-primary)] hover:underline">{issue.student_name || issue.wcode}</Link>
                        <span className={`rounded-full px-2 py-0.5 text-[11px] font-medium ${issue.severity === "critical" ? "bg-red-50 text-red-700" : "bg-amber-50 text-amber-700"}`}>{issue.severity}</span>
                        <span className="rounded-full bg-gray-100 px-2 py-0.5 text-[11px] text-gray-600">{titleCase(issue.issue_type)}</span>
                        <span className={`rounded-full px-2 py-0.5 text-[11px] ${unresolved ? "bg-amber-50 text-amber-800" : "bg-emerald-50 text-emerald-700"}`}>{titleCase(issue.status)}</span>
                      </div>
                      <p className="mt-1 text-sm text-gray-500">{issue.start_at ? `Affected time: ${formatDateTime(issue.start_at)} - ${formatDateTime(issue.end_at)}` : "The referenced session is no longer available."}</p>
                      {Array.isArray(issue.details?.reasons) && issue.details.reasons.length > 0 ? <p className="mt-1 text-sm text-gray-700">{(issue.details.reasons as string[]).map(titleCase).join(" · ")}</p> : null}
                    </div>
                    {unresolved ? <Link to="/operations/schedule-impact" className="text-sm font-medium text-[var(--color-wi-primary)] hover:underline">Review in work queue</Link> : null}
                  </div>
                </article>
              );
            })}
          </div>
        )}
      </section>

      <section className="mt-4 rounded-sm border border-gray-200 bg-white">
        <div className="border-b border-gray-100 bg-gray-50/70 px-4 py-3 text-sm font-semibold text-gray-800">Notification delivery</div>
        {visibleNotifications.length === 0 ? <p className="p-4 text-sm text-gray-500">{deliveredCount ? `${deliveredCount} successful notification${deliveredCount === 1 ? "" : "s"} collapsed. No delivery needs attention.` : "No notifications have been queued for this change."}</p> : (
          <div className="divide-y divide-gray-100">
            {visibleNotifications.map((notification) => (
              <div key={notification.id} className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
                <div>
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium text-gray-900">{titleCase(notification.message_type)}</span>
                    <span className="rounded-full bg-gray-100 px-2 py-0.5 text-[11px] text-gray-600">{notification.channel}</span>
                    <span className={`rounded-full px-2 py-0.5 text-[11px] ${notification.status === "delivered" ? "bg-emerald-50 text-emerald-700" : notification.status === "failed" || notification.status === "dead_letter" ? "bg-red-50 text-red-700" : "bg-blue-50 text-blue-700"}`}>{titleCase(notification.status)}</span>
                  </div>
                  <p className="mt-1 text-xs text-gray-500">{formatDateTime(notification.created_at)}{notification.failure_reason ? ` · ${notification.failure_reason}` : ""}</p>
                </div>
                <div className="flex gap-2">
                  {notification.status === "failed" || notification.status === "dead_letter" ? <Button size="sm" variant="secondary" loading={busyIssue === notification.id} onClick={() => void updateNotification(notification, "retry")}>Retry</Button> : null}
                  {notification.status === "queued" || notification.status === "failed" || notification.status === "dead_letter" ? <Button size="sm" variant="ghost" loading={busyIssue === notification.id} onClick={() => void updateNotification(notification, "cancel")}>Cancel</Button> : null}
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      <details className="mt-4 rounded-sm border border-gray-200 bg-white p-4">
        <summary className="cursor-pointer text-sm font-medium text-gray-700">Technical details</summary>
        <dl className="mt-3 space-y-1 text-xs text-gray-500"><div><dt className="inline font-semibold">Session ID: </dt><dd className="inline break-all">{change.session_id}</dd></div><div><dt className="inline font-semibold">Session version: </dt><dd className="inline">{change.session_version}</dd></div><div><dt className="inline font-semibold">Source: </dt><dd className="inline">{titleCase(change.change_source)}</dd></div><div><dt className="inline font-semibold">Change ID: </dt><dd className="inline break-all">{change.id}</dd></div></dl>
      </details>
    </div>
  );
}
