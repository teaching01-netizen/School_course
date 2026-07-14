import { formatDate, formatTime } from "@/utils/date";
import type { ManagedAbsence } from "../types";
import { dayKey, groupByDay } from "./sessionGrouping";

export function formatBatchAbsenceSummary(absence: ManagedAbsence) {
  const className = absence.subject_name?.trim() || absence.course_name?.trim() || "";
  const dates = getAbsenceSessionDateLabels(absence);
  if (!className && !dates) return "To arrange";
  if (!dates) return className || "To arrange";
  if (!className) return dates;
  return `${className} (${dates})`;
}

export function getAbsenceSessionDateLabels(absence: ManagedAbsence) {
  const sessions = absence.missed_sessions ?? [];
  const dates = new Set<string>();
  for (const session of sessions) {
    if (session.start_at) dates.add(dayKey(session));
  }
  const labels = [...dates].sort().map((date) => formatDate(date));
  if (labels.length > 0) return labels.join(", ");
  if (absence.date_from && absence.date_to) {
    if (absence.date_from === absence.date_to) return formatDate(absence.date_from);
    return `${formatDate(absence.date_from)} - ${formatDate(absence.date_to)}`;
  }
  return "";
}

export function formatBatchSitInSummary(absence: ManagedAbsence) {
  const method = absence.sit_in_method?.trim();
  if (method === "zoom") return "Zoom";
  const sessions = absence.sit_ins ?? [];
  const sessionLabels = (() => {
    const withTimes = sessions.filter((session) => session.start_at);
    return groupByDay(withTimes).map((group) => `${formatDate(group.date)} ${formatTime(group.start_at)}-${formatTime(group.end_at)}`);
  })();
  if (method !== "physical") {
    return sessionLabels.length > 0 ? `To arrange (${sessionLabels.join(", ")})` : "To arrange";
  }
  if (sessionLabels.length > 0) {
    const className = absence.sit_in_subject_name?.trim() || absence.sit_in_course_name?.trim() || "Make-up class";
    return `${className} (${sessionLabels.join(", ")})`;
  }
  const label = absence.sit_in_subject_name?.trim() || absence.sit_in_course_name?.trim();
  if (label) return label;
  return "To arrange";
}
