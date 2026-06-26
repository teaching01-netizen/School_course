import type { SubjectSessions } from "../types";
import { daysBetween } from "./dateRange";
import { getSelectedSessionsForGroup, splitMergedSessionValue, uniqueValues } from "./sessionGrouping";
import { sitInForMissedSession } from "./sitInResolution";

export type AbsenceBatchCreateItem = {
  subject_id: string;
  course_id: string;
  date_from: string;
  date_to: string;
  reason?: string;
  sit_in_method?: string;
  sit_in_course_id?: string;
  missed_session_ids: string[];
  sit_in_session_ids: string[];
};

export type BuildSubmissionPayloadsInput = {
  lookupWcode: string | null;
  sessions: SubjectSessions[];
  selectedSubjectIds: string[];
  selectedSessionIds: Set<string>;
  sitInSelections: Record<string, string>;
  reason: string;
  maxDateRangeDays: number;
};

export type BuildSubmissionPayloadsResult =
  | { ok: true; payloads: AbsenceBatchCreateItem[] }
  | { ok: false; error: string };

export function selectedSitInCourseIDForGroup(
  group: SubjectSessions,
  selectedMissedSessionIds: string[],
  sitInSelections: Record<string, string>,
): string | null {
  if (group.sit_in?.sit_in_method !== "physical" && !group.sit_in?.sit_in_by_missed_session) {
    return group.sit_in?.sit_in_course?.id?.trim() || group.course_id.trim() || null;
  }
  const courseIDs = new Set<string>();
  for (const missedSessionID of selectedMissedSessionIds) {
    const sitIn = sitInForMissedSession(group, missedSessionID);
    if (sitIn?.sit_in_method !== "physical") {
      const courseID = sitIn?.sit_in_course?.id?.trim() || group.course_id.trim();
      if (courseID) courseIDs.add(courseID);
      continue;
    }
    const priorities = sitIn.priorities ?? [];
    if (priorities.length === 0) {
      const courseID = sitIn.sit_in_course?.id?.trim() || group.course_id.trim();
      if (courseID) courseIDs.add(courseID);
      continue;
    }
    const sitInSessionIDs = splitMergedSessionValue(sitInSelections[missedSessionID]);
    if (sitInSessionIDs.length === 0) continue;
    for (const priority of priorities) {
      const hasSession = (priority.available_sessions ?? []).some((session) => sitInSessionIDs.includes(session.id));
      const courseID = priority.sit_in_course?.id?.trim();
      if (hasSession && courseID) {
        courseIDs.add(courseID);
        break;
      }
    }
  }
  if (courseIDs.size === 1) return [...courseIDs][0];
  if (courseIDs.size === 0) return group.sit_in?.sit_in_course?.id?.trim() || group.course_id.trim() || null;
  return null;
}

export function buildSubmissionPayloads(input: BuildSubmissionPayloadsInput): BuildSubmissionPayloadsResult {
  if (!input.lookupWcode) return { ok: true, payloads: [] };
  const payloads: AbsenceBatchCreateItem[] = [];
  for (const group of input.sessions) {
    if (!input.selectedSubjectIds.includes(group.subject_id)) continue;
    if (group.absence_rate_exceeded) continue;
    const selectedGroupSessions = getSelectedSessionsForGroup(group, input.selectedSessionIds);
    if (selectedGroupSessions.length === 0) continue;
    const selectedDates = [...new Set(selectedGroupSessions.map((session) => session.date))].sort();
    const dateFrom = selectedDates[0];
    const dateTo = selectedDates[selectedDates.length - 1];
    if (daysBetween(dateFrom, dateTo) > input.maxDateRangeDays) {
      return {
        ok: false,
        error: `${group.subject_name || group.course_name} spans more than ${input.maxDateRangeDays} days. Split it into separate submissions.`,
      };
    }
    const selectedSessIds = selectedGroupSessions.map((session) => session.id);
    const sitInSessionIds = uniqueValues(selectedSessIds.flatMap((id) => splitMergedSessionValue(input.sitInSelections[id])));
    const sitInMethod = group.sit_in?.sit_in_method;
    const payload: AbsenceBatchCreateItem = {
      subject_id: group.subject_id,
      course_id: group.course_id,
      date_from: dateFrom,
      date_to: dateTo,
      reason: input.reason.trim() || undefined,
      missed_session_ids: selectedSessIds,
      sit_in_session_ids: sitInSessionIds,
    };
    if (sitInMethod === "physical" || sitInMethod === "zoom") payload.sit_in_method = sitInMethod;
    const sitInCourseID = selectedSitInCourseIDForGroup(group, selectedSessIds, input.sitInSelections);
    if (sitInCourseID === null) {
      return {
        ok: false,
        error: `${group.subject_name || group.course_name} has sit-in selections from more than one priority class. Split them into separate submissions.`,
      };
    }
    if (sitInCourseID) payload.sit_in_course_id = sitInCourseID;
    payloads.push(payload);
  }
  return { ok: true, payloads };
}
