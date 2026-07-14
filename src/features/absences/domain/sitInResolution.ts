import { formatDate, formatTime } from "@/utils/date";
import type { SubjectSessions } from "../types";
import { dayKey, groupByDay, splitMergedSessionValue } from "./sessionGrouping";

export type SitInAvailableSession = NonNullable<NonNullable<SubjectSessions["sit_in"]>["available_sessions"]>[number];
export type SitInCourse = NonNullable<SubjectSessions["sit_in"]>["sit_in_course"];

export function resolveSitInSubjectName(sitInCourse: SitInCourse, allSubjects: SubjectSessions[]): string | undefined {
  return sitInCourse?.subject_name?.trim() || allSubjects.find(s => s.course_id === sitInCourse?.id)?.subject_name?.trim();
}

export function getSitInCourseDisplayName(
  sitInCourse: SitInCourse,
  fallbackSubjectName: string,
  allSubjects: SubjectSessions[],
) {
  return (
    resolveSitInSubjectName(sitInCourse, allSubjects) ||
    sitInCourse?.name?.trim() ||
    fallbackSubjectName ||
    ""
  );
}

export function getPriorityTargetDisplayName(
  priority: NonNullable<NonNullable<SubjectSessions["sit_in"]>["priorities"]>[number],
  fallbackSubjectName: string,
  allSubjects: SubjectSessions[],
) {
  const courseName = getSitInCourseDisplayName(priority.sit_in_course, "", allSubjects);
  if (courseName) return courseName;
  const firstSession = priority.available_sessions?.[0];
  return (
    firstSession?.class_name?.trim() ||
    firstSession?.subject_name?.trim() ||
    firstSession?.course_name?.trim() ||
    fallbackSubjectName
  );
}

export function getCurrentSitInDisplayName(
  sitIn: SubjectSessions["sit_in"],
  currentPriorities: NonNullable<NonNullable<SubjectSessions["sit_in"]>["priorities"]>,
  fallbackSubjectName: string,
  allSubjects: SubjectSessions[],
) {
  if (sitIn?.sit_in_method !== "physical") {
    return sitIn?.sit_in_method === "zoom" ? "Zoom" : "To arrange";
  }
  if (currentPriorities.length > 0) {
    const labels = [
      ...new Set(
        currentPriorities
          .map((priority) => {
            if (!priority.sit_in_course && (priority.available_sessions ?? []).length === 0) return "";
            return getPriorityTargetDisplayName(priority, fallbackSubjectName, allSubjects).trim();
          })
          .filter(Boolean),
      ),
    ];
    if (labels.length > 0) return labels.join(", ");
    return "Not available";
  }
  return getSitInCourseDisplayName(sitIn.sit_in_course, fallbackSubjectName, allSubjects);
}

export function sitInForMissedSession(group: SubjectSessions, missedSessionId: string) {
  return group.sit_in?.sit_in_by_missed_session?.[missedSessionId] ?? group.sit_in;
}

export function groupWithSitInForMissedSession(group: SubjectSessions, missedSessionId: string): SubjectSessions {
  const sitIn = sitInForMissedSession(group, missedSessionId);
  if (!sitIn || sitIn === group.sit_in) return group;
  return { ...group, sit_in: sitIn };
}

export function availableSessionsForMissedSession(
  priority: NonNullable<NonNullable<SubjectSessions["sit_in"]>["priorities"]>[number],
  missedSessionId: string,
) {
  return availableSessionsForMissedSessions(priority, [missedSessionId]);
}

export function availableSessionsForMissedSessions(
  priority: NonNullable<NonNullable<SubjectSessions["sit_in"]>["priorities"]>[number],
  missedSessionIds: string[],
) {
  const available = priority.available_sessions ?? [];
  if (!available.some((session) => session.missed_session_id)) return available;
  return available.filter((session) => session.missed_session_id ? missedSessionIds.includes(session.missed_session_id) : false);
}

export function unavailableSessionsForMissedSession(
  priority: NonNullable<NonNullable<SubjectSessions["sit_in"]>["priorities"]>[number],
  missedSessionId: string,
) {
  const unavailable = priority.unavailable_sessions ?? [];
  if (!unavailable.some((session) => session.missed_session_id)) return unavailable;
  return unavailable.filter((session) => session.missed_session_id === missedSessionId);
}

export function rootAvailableSessionsForMissedSession(
  sitIn: SubjectSessions["sit_in"],
  missedSessionId: string,
) {
  return rootAvailableSessionsForMissedSessions(sitIn, [missedSessionId]);
}

export function rootAvailableSessionsForMissedSessions(
  sitIn: SubjectSessions["sit_in"],
  missedSessionIds: string[],
) {
  const available = sitIn?.available_sessions ?? [];
  if (!available.some((session) => session.missed_session_id)) return available;
  return available.filter((session) => session.missed_session_id ? missedSessionIds.includes(session.missed_session_id) : false);
}

export function hasServerPriorityReveal(group: SubjectSessions): boolean {
  return group.sit_in?.current_priority_level !== undefined || group.sit_in?.has_next_priority !== undefined;
}

export function firstPriorityLevel(group: SubjectSessions): number {
  const priorities = group.sit_in?.priorities ?? [];
  if (priorities.length === 0) return 1;
  return Math.min(...priorities.map((priority) => priority.level));
}

export function hasPriorityLevel(group: SubjectSessions, level: number): boolean {
  return (group.sit_in?.priorities ?? []).some((priority) => priority.level === level);
}

export function nextPriorityLevel(group: SubjectSessions, currentLevel: number): number | null {
  const levels = [...new Set((group.sit_in?.priorities ?? []).map((priority) => priority.level))]
    .filter((level) => level > currentLevel)
    .sort((a, b) => a - b);
  return levels[0] ?? null;
}

export function previousPriorityLevel(group: SubjectSessions, currentLevel: number): number | null {
  const levels = [...new Set((group.sit_in?.priorities ?? []).map((priority) => priority.level))]
    .filter((level) => level < currentLevel)
    .sort((a, b) => b - a);
  return levels[0] ?? null;
}

export function prioritiesForLevel(group: SubjectSessions, level: number) {
  return (group.sit_in?.priorities ?? []).filter((priority) => priority.level === level);
}

export function getReviewSitInLabel(
  missedSession: { id: string },
  group: SubjectSessions,
  sitInSelections: Record<string, string>,
  priorityLevels: Record<string, number>,
  priorityHistory: Record<string, Record<number, SubjectSessions>>,
  allSubjects: SubjectSessions[],
): string {
  const requestedLevel = priorityLevels[missedSession.id];
  const sitInGroup = requestedLevel
    ? priorityHistory[missedSession.id]?.[requestedLevel] ?? groupWithSitInForMissedSession(group, missedSession.id)
    : groupWithSitInForMissedSession(group, missedSession.id);
  const sitIn = sitInGroup.sit_in;
  if (!sitIn) return "To arrange";
  if (sitIn.sit_in_method === "zoom") return "Zoom";
  if (sitIn.sit_in_method === "teacher_case") return "To arrange";
  if (sitIn.sit_in_method !== "physical") return "To arrange";
  const sitInSessionIds = splitMergedSessionValue(sitInSelections[missedSession.id]);
  if (sitInSessionIds.length === 0) return "Not yet selected";
  const priorities = sitIn.priorities ?? [];
  const groupLabel = group.subject_name?.trim() || group.course_name?.trim();
  const rootMatches = rootAvailableSessionsForMissedSession(sitIn, missedSession.id).filter((s) => sitInSessionIds.includes(s.id));
  if (rootMatches.length > 0) {
    return getSitInSessionGroupLabel(rootMatches, sitIn.sit_in_course, groupLabel, allSubjects);
  }
  for (const p of priorities) {
    const available = availableSessionsForMissedSession(p, missedSession.id);
    const matches = available.filter((s) => sitInSessionIds.includes(s.id));
    if (matches.length > 0) {
      return getSitInSessionGroupLabel(matches, p.sit_in_course, groupLabel, allSubjects);
    }
  }
  return "Make-up class selected";
}

export function getSitInSessionLabel(
  session: SitInAvailableSession,
  sitInCourse: SitInCourse,
  fallbackSubjectName: string,
  allSubjects: SubjectSessions[],
) {
  const className =
    resolveSitInSubjectName(sitInCourse, allSubjects) ||
    sitInCourse?.name?.trim() ||
    session.class_name?.trim() ||
    session.subject_name?.trim() ||
    session.course_name?.trim() ||
    fallbackSubjectName;
  return `${className} — ${formatDate(dayKey(session))} ${formatTime(session.start_at)}-${formatTime(session.end_at)}`;
}

export function getSitInSessionGroupLabel(
  sessions: SitInAvailableSession[],
  sitInCourse: SitInCourse,
  fallbackSubjectName: string,
  allSubjects: SubjectSessions[],
) {
  if (sessions.length === 1) return getSitInSessionLabel(sessions[0], sitInCourse, fallbackSubjectName, allSubjects);
  const first = sessions[0];
  const className =
    resolveSitInSubjectName(sitInCourse, allSubjects) ||
    sitInCourse?.name?.trim() ||
    first.class_name?.trim() ||
    first.subject_name?.trim() ||
    first.course_name?.trim() ||
    fallbackSubjectName;
  const range = groupByDay(sessions)[0];
  return `${className} — ${formatDate(range.date)} ${formatTime(range.start_at)}-${formatTime(range.end_at)}`;
}
