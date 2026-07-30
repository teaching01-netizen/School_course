import type { ScheduleImpactIssue } from "./types";

const messages: Record<string, string> = {
  sit_in_overlap: "This sit-in overlaps with another sit-in.",
  regular_session_overlap: "This sit-in overlaps with the student's regular class.",
  sit_in_ineligible: "The student is no longer eligible for this session.",
  past_time_change: "This session has already started or passed.",
  short_notice_change: "The student has limited notice of this change.",
  sit_in_session_changed: "The assigned sit-in session was changed.",
  missed_session_changed: "The missed session was changed.",
};

export function issueMessage(issue: ScheduleImpactIssue): string {
  return messages[issue.issue_type] ?? "This student arrangement needs attention.";
}

export function issueConsequence(issue: ScheduleImpactIssue): string {
  if (issue.issue_type === "short_notice_change") return "The student needs a clear update before the session begins.";
  if (issue.issue_type === "past_time_change") return "The original arrangement can no longer be used.";
  return "The student cannot safely attend the current arrangement.";
}

export function formatBangkokDateTime(start: string | null, end?: string | null): string {
  if (!start) return "Session unavailable";
  const startDate = new Date(start);
  const date = new Intl.DateTimeFormat("en-GB", {
    weekday: "short", day: "numeric", month: "short", timeZone: "Asia/Bangkok",
  }).format(startDate);
  const time = new Intl.DateTimeFormat("en-GB", {
    hour: "2-digit", minute: "2-digit", hour12: false, timeZone: "Asia/Bangkok",
  }).format(startDate);
  if (!end) return `${date} · ${time}`;
  const endTime = new Intl.DateTimeFormat("en-GB", {
    hour: "2-digit", minute: "2-digit", hour12: false, timeZone: "Asia/Bangkok",
  }).format(new Date(end));
  return `${date} · ${time}–${endTime}`;
}

export function formatBangkokDate(iso: string | null): string {
  if (!iso) return "";
  return new Intl.DateTimeFormat("en-GB", {
    weekday: "long", day: "numeric", month: "long", timeZone: "Asia/Bangkok",
  }).format(new Date(iso));
}

export function formatBangkokDateShort(iso: string | null): string {
  if (!iso) return "";
  return new Intl.DateTimeFormat("en-GB", {
    day: "numeric", month: "short", timeZone: "Asia/Bangkok",
  }).format(new Date(iso));
}

export function formatBangkokTime(start: string | null, end?: string | null): string {
  if (!start) return "Not set";
  const startDate = new Date(start);
  const timeStart = new Intl.DateTimeFormat("en-GB", {
    hour: "2-digit", minute: "2-digit", hour12: false, timeZone: "Asia/Bangkok",
  }).format(startDate);
  if (!end) return timeStart;
  const timeEnd = new Intl.DateTimeFormat("en-GB", {
    hour: "2-digit", minute: "2-digit", hour12: false, timeZone: "Asia/Bangkok",
  }).format(new Date(end));
  return `${timeStart}–${timeEnd}`;
}

export function urgencyFor(issue: ScheduleImpactIssue): string {
  const hours = issue.details.notice_hours;
  if (typeof hours === "number" && hours >= 0) {
    if (hours < 1) return "Starts within an hour";
    if (hours < 24) return `Starts in ${Math.ceil(hours)}h`;
    return `Starts in ${Math.ceil(hours / 24)} days`;
  }
  const originalSnapshot = issue.assignment_context.original_session.snapshot;
  const before = issue.change_context.before;
  const startAt = (originalSnapshot?.start_at as string) ?? (before?.start_at as string) ?? null;
  const start = startAt ? new Date(startAt).getTime() : 0;
  const hoursUntil = (start - Date.now()) / 3_600_000;
  if (hoursUntil > 0 && hoursUntil < 24) return `Starts in ${Math.ceil(hoursUntil)}h`;
  return issue.severity === "critical" ? "Needs urgent review" : "Review soon";
}

export function notificationMessage(status: string): string {
  if (status === "queued") return "Student notification queued";
  if (status === "not_configured") return "Assignment updated, but SMS and email templates are not configured.";
  if (status === "no_recipient") return "Assignment updated, but no contact method is available for this student.";
  return "Issue updated";
}
