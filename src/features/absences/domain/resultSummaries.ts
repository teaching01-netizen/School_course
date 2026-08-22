import { formatDate, formatTime } from "@/utils/date";
import type { ManagedAbsence, SubjectSessions } from "../types";
import { absenceScopeKey, combineSubjectGroups, dayKey, groupByDay } from "./sessionGrouping";

export type SubmittedAbsenceGroup = {
  key: string;
  label: string;
  absences: ManagedAbsence[];
};

export function groupSubmittedAbsences(
  absences: ManagedAbsence[],
  sessionGroups: SubjectSessions[],
): SubmittedAbsenceGroup[] {
  const sourceByCourseID = new Map(sessionGroups.map((group) => [group.course_id, group]));
  const blockByKey = new Map(
    combineSubjectGroups(sessionGroups).map((block) => [block.key, block]),
  );
  const groups = new Map<string, SubmittedAbsenceGroup>();

  for (const absence of absences) {
    const source = sourceByCourseID.get(absence.course_id);
    const mergeGroupID = source?.merge_group_id?.trim() || absence.merge_group_id?.trim();
    const key = mergeGroupID
      ? `merge:${mergeGroupID}`
      : source
        ? absenceScopeKey(source)
        : `absence:${absence.id}`;
    const block = blockByKey.get(key);
    const label =
      block?.label ||
      absence.merge_group_name?.trim() ||
      absence.subject_name?.trim() ||
      absence.course_name?.trim() ||
      "Submitted class";
    const existing = groups.get(key);
    if (existing) {
      existing.absences.push(absence);
    } else {
      groups.set(key, { key, label, absences: [absence] });
    }
  }

  return [...groups.values()];
}

export function formatSubmittedAbsenceSummary(group: SubmittedAbsenceGroup): string {
  const dates = [...new Set(group.absences.flatMap((absence) => {
    const summary = getAbsenceSessionDateLabels(absence);
    return summary ? [summary] : [];
  }))];
  return dates.length > 0 ? `${group.label} (${dates.join(", ")})` : group.label;
}

export function formatSubmittedSitInSummary(group: SubmittedAbsenceGroup): string {
  const summaries = [...new Set(group.absences.map(formatBatchSitInSummary))];
  return summaries.join("; ");
}

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
