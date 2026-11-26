import type { SubjectSessions } from "../types";
import { daysBetween } from "./dateRange";
import {
  absenceScopeKey,
  combineSubjectGroups,
  getSelectedSessionsForGroup,
  groupByDay,
  instituteDateKey,
  splitMergedSessionValue,
  uniqueValues,
} from "./sessionGrouping";
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

export function duplicateSitInSessionIds(
  items: Array<Pick<AbsenceBatchCreateItem, "sit_in_session_ids">>,
): string[] {
  const seen = new Set<string>();
  const duplicates = new Set<string>();
  for (const item of items) {
    for (const sessionID of item.sit_in_session_ids) {
      if (seen.has(sessionID)) duplicates.add(sessionID);
      seen.add(sessionID);
    }
  }
  return [...duplicates];
}

type MergeableAbsenceItem = Pick<
  AbsenceBatchCreateItem,
  | "date_from"
  | "date_to"
  | "missed_session_ids"
  | "sit_in_session_ids"
  | "sit_in_course_id"
  | "sit_in_method"
> & { special_sit_in_session_ids?: string[] };

export function mergeAbsenceBatchItemsByScope<T extends MergeableAbsenceItem>(
  entries: Array<{ scopeKey: string; targetMergeGroupId?: string; item: T }>,
): T[] {
  const merged = new Map<string, T>();
  const sharedTargetCourses = new Map<string, string>();
  for (const entry of entries) {
    const item = entry.item;
    let courseId = item.sit_in_course_id;
    if (entry.targetMergeGroupId && courseId && item.sit_in_session_ids.length > 0) {
      // A merged-day selection can be attached to more than one source class.
      const sessions = [...item.sit_in_session_ids].sort().join(",");
      const selection = `${entry.scopeKey}|${entry.targetMergeGroupId}|${sessions}`;
      courseId = sharedTargetCourses.get(selection) ?? courseId;
      sharedTargetCourses.set(selection, courseId);
    }
    const key = `${entry.scopeKey}|${courseId ?? ""}|${item.sit_in_method ?? ""}`;
    const current = merged.get(key);
    if (!current) {
      merged.set(key, {
        ...item,
        missed_session_ids: [...item.missed_session_ids],
        sit_in_session_ids: [...item.sit_in_session_ids],
      } as T);
      continue;
    }
    current.date_from = current.date_from < item.date_from ? current.date_from : item.date_from;
    current.date_to = current.date_to > item.date_to ? current.date_to : item.date_to;
    current.missed_session_ids = [
      ...new Set([...current.missed_session_ids, ...item.missed_session_ids]),
    ];
    current.sit_in_session_ids = [
      ...new Set([...current.sit_in_session_ids, ...item.sit_in_session_ids]),
    ];
    if (current.special_sit_in_session_ids || item.special_sit_in_session_ids) {
      current.special_sit_in_session_ids = [
        ...new Set([
          ...(current.special_sit_in_session_ids ?? []),
          ...(item.special_sit_in_session_ids ?? []),
        ]),
      ];
    }
  }
  return [...merged.values()];
}

export type BuildSubmissionPayloadsInput = {
  lookupWcode: string | null;
  sessions: SubjectSessions[];
  selectedSubjectIds: string[];
  selectedSessionIds: Set<string>;
  sitInSelections: Record<string, string>;
  reason: string;
  maxDateRangeDays: number;
  sitInPriorityLevels?: Record<string, number>;
  sitInPriorityHistory?: Record<string, Record<number, SubjectSessions>>;
};

export type BuildSubmissionPayloadsResult =
  | { ok: true; payloads: AbsenceBatchCreateItem[] }
  | { ok: false; error: string };

type SessionTime = { start_at: string; end_at: string };

type SessionWithCourse = { id: string; course_id?: string | null };

function overlaps(a: SessionTime, b: SessionTime): boolean {
  return (
    new Date(a.start_at) < new Date(b.end_at) &&
    new Date(a.end_at) > new Date(b.start_at)
  );
}

function findSitInSessionTime(
  group: SubjectSessions,
  sitInSessionId: string,
): SessionTime | null {
  const sitIn = group.sit_in;
  if (!sitIn) return null;
  const inTop = (sitIn.available_sessions ?? []).find(
    (s) => s.id === sitInSessionId,
  );
  if (inTop) return { start_at: inTop.start_at, end_at: inTop.end_at };
  for (const p of sitIn.priorities ?? []) {
    const inP = (p.available_sessions ?? []).find(
      (s) => s.id === sitInSessionId,
    );
    if (inP) return { start_at: inP.start_at, end_at: inP.end_at };
  }
  if (sitIn.sit_in_by_missed_session) {
    for (const entry of Object.values(sitIn.sit_in_by_missed_session)) {
      const inEntry = (entry.available_sessions ?? []).find(
        (s) => s.id === sitInSessionId,
      );
      if (inEntry)
        return { start_at: inEntry.start_at, end_at: inEntry.end_at };
      for (const p of entry.priorities ?? []) {
        const inP = (p.available_sessions ?? []).find(
          (s) => s.id === sitInSessionId,
        );
        if (inP) return { start_at: inP.start_at, end_at: inP.end_at };
      }
    }
  }
  return null;
}

function findSitInSessionTimeInGroups(
  groups: SubjectSessions[],
  sitInSessionId: string,
): SessionTime | null {
  for (const group of groups) {
    const time = findSitInSessionTime(group, sitInSessionId);
    if (time) return time;
  }
  return null;
}

function findSitInSessionMergeGroupIDInGroups(
  groups: SubjectSessions[],
  sitInSessionId: string,
): string | null {
  for (const group of groups) {
    const sitIn = group.sit_in;
    if (!sitIn) continue;
    if ((sitIn.available_sessions ?? []).some((session) => session.id === sitInSessionId)) {
      return sitIn.sit_in_course?.merge_group_id || null;
    }
    for (const priority of sitIn.priorities ?? []) {
      if ((priority.available_sessions ?? []).some((session) => session.id === sitInSessionId)) {
        return priority.sit_in_course?.merge_group_id || null;
      }
    }
    for (const nested of Object.values(sitIn.sit_in_by_missed_session ?? {})) {
      if ((nested.available_sessions ?? []).some((session) => session.id === sitInSessionId)) {
        return nested.sit_in_course?.merge_group_id || null;
      }
      for (const priority of nested.priorities ?? []) {
        if ((priority.available_sessions ?? []).some((session) => session.id === sitInSessionId)) {
          return priority.sit_in_course?.merge_group_id || null;
        }
      }
    }
  }
  return null;
}

function firstSessionCourseID(
  sessions: SessionWithCourse[] | undefined | null,
): string | null {
  for (const s of sessions ?? []) {
    if (s.course_id) return s.course_id;
  }
  return null;
}

function findSelectedSitInSessionCourseID(
  sitIn: SubjectSessions["sit_in"] | undefined | null,
  sitInSessionIDs: string[],
): string | null {
  if (!sitIn || sitInSessionIDs.length === 0) return null;
  const selected = new Set(sitInSessionIDs);
  const fromSessions = (sessions: SessionWithCourse[] | undefined | null) => {
    for (const session of sessions ?? []) {
      if (selected.has(session.id) && session.course_id)
        return session.course_id;
    }
    return null;
  };
  const fromTop = fromSessions(sitIn.available_sessions);
  if (fromTop) return fromTop;
  for (const p of sitIn.priorities ?? []) {
    const fromPriority = fromSessions(p.available_sessions);
    if (fromPriority) return fromPriority;
  }
  if (sitIn.sit_in_by_missed_session) {
    for (const entry of Object.values(sitIn.sit_in_by_missed_session)) {
      const fromEntry = fromSessions(entry.available_sessions);
      if (fromEntry) return fromEntry;
      for (const p of entry.priorities ?? []) {
        const fromPriority = fromSessions(p.available_sessions);
        if (fromPriority) return fromPriority;
      }
    }
  }
  return null;
}

function findSitInCourseFromSessions(group: SubjectSessions): string | null {
  const top = group.sit_in;
  if (!top) return null;
  const fromTop = firstSessionCourseID(top.available_sessions);
  if (fromTop) return fromTop;
  for (const p of top.priorities ?? []) {
    const fromPriority = firstSessionCourseID(p.available_sessions);
    if (fromPriority) return fromPriority;
  }
  if (top.sit_in_by_missed_session) {
    for (const entry of Object.values(top.sit_in_by_missed_session)) {
      const fromEntry = firstSessionCourseID(entry.available_sessions);
      if (fromEntry) return fromEntry;
      for (const p of entry.priorities ?? []) {
        const fromPriority = firstSessionCourseID(p.available_sessions);
        if (fromPriority) return fromPriority;
      }
    }
  }
  return null;
}

export function selectedSitInCourseIDForGroup(
  group: SubjectSessions,
  selectedMissedSessionIds: string[],
  sitInSelections: Record<string, string>,
  priorityLevels?: Record<string, number>,
  priorityHistory?: Record<string, Record<number, SubjectSessions>>,
): string | null {
  if (!group.sit_in) return group.course_id.trim() || null;
  if (group.sit_in.sit_in_method !== "physical" && group.sit_in.sit_in_method !== "zoom") return null;
  if (
    group.sit_in?.sit_in_method !== "physical" &&
    !group.sit_in?.sit_in_by_missed_session
  ) {
    return (
      group.sit_in?.sit_in_course?.id?.trim() ||
      findSitInCourseFromSessions(group) ||
      group.course_id.trim() ||
      null
    );
  }
  const courseIDs = new Set<string>();
  for (const missedSessionID of selectedMissedSessionIds) {
    let effectiveGroup = group;
    if (priorityLevels && priorityHistory) {
      const level = priorityLevels[missedSessionID];
      if (level !== undefined) {
        const historyGroup = priorityHistory[missedSessionID]?.[level];
        if (historyGroup) effectiveGroup = historyGroup;
      }
    }
    const sitIn = sitInForMissedSession(effectiveGroup, missedSessionID);
    const sitInSessionIDs = splitMergedSessionValue(
      sitInSelections[missedSessionID],
    );
    if (sitIn?.sit_in_method !== "physical") {
      const courseID =
        sitIn?.sit_in_course?.id?.trim() ||
        findSitInCourseFromSessions(effectiveGroup) ||
        effectiveGroup.course_id.trim();
      if (courseID) courseIDs.add(courseID);
      continue;
    }
    const priorities = sitIn.priorities ?? [];
    if (priorities.length === 0) {
      const courseID =
        sitIn.sit_in_course?.id?.trim() ||
        findSelectedSitInSessionCourseID(sitIn, sitInSessionIDs) ||
        findSitInCourseFromSessions(effectiveGroup) ||
        effectiveGroup.course_id.trim();
      if (courseID) courseIDs.add(courseID);
      continue;
    }
    if (sitInSessionIDs.length === 0) continue;
    for (const priority of priorities) {
      const hasSession = (priority.available_sessions ?? []).some((session) =>
        sitInSessionIDs.includes(session.id),
      );
      const courseID = priority.sit_in_course?.id?.trim();
      if (hasSession && courseID) {
        courseIDs.add(courseID);
        break;
      }
    }
  }
  if (courseIDs.size === 1) return [...courseIDs][0];
  if (courseIDs.size === 0)
    return (
      group.sit_in?.sit_in_course?.id?.trim() ||
      findSitInCourseFromSessions(group) ||
      group.course_id.trim() ||
      null
    );
  return null;
}

export function suggestedSitInForSubmission(
  group: SubjectSessions,
  missedSessionId: string,
  sitInSelections: Record<string, string>,
  priorityLevels?: Record<string, number>,
  priorityHistory?: Record<string, Record<number, SubjectSessions>>,
) {
  const level = priorityLevels?.[missedSessionId];
  const effectiveGroup = level === undefined
    ? group
    : priorityHistory?.[missedSessionId]?.[level] ?? group;
  const method = sitInForMissedSession(effectiveGroup, missedSessionId)?.sit_in_method;
  const sessionIds = splitMergedSessionValue(sitInSelections[missedSessionId]);
  const courseId = method === "none" ? undefined : selectedSitInCourseIDForGroup(
    effectiveGroup,
    [missedSessionId],
    sitInSelections,
  ) ?? undefined;
  const targetMergeGroupId = sessionIds
    .map((sessionId) => findSitInSessionMergeGroupIDInGroups([effectiveGroup], sessionId))
    .find((mergeGroupId) => mergeGroupId !== null);
  return {
    method: method === "physical" || method === "zoom" ? method : undefined,
    sessionIds,
    courseId,
    targetMergeGroupId,
  };
}

function collectAttendingSessions(
  sessions: SubjectSessions[],
): Map<string, SessionTime[]> {
  const byDate = new Map<string, SessionTime[]>();
  for (const group of sessions) {
    for (const session of group.sessions) {
      if (session.already_absent) continue;
      const date = session.date ?? instituteDateKey(session.start_at);
      const ranges = byDate.get(date) ?? [];
      ranges.push({ start_at: session.start_at, end_at: session.end_at });
      byDate.set(date, ranges);
    }
  }
  return byDate;
}

export function buildSubmissionPayloads(
  input: BuildSubmissionPayloadsInput,
): BuildSubmissionPayloadsResult {
  if (!input.lookupWcode) return { ok: true, payloads: [] };
  const selectedGroups = input.sessions.filter((group) => input.selectedSubjectIds.includes(group.subject_id));
  const groupBySessionId = new Map(selectedGroups.flatMap((group) => group.sessions.map((session) => [session.id, group] as const)));
  const pickerSessionByMember = new Map<string, string>();
  for (const block of combineSubjectGroups(selectedGroups)) {
    if (!block.isMerged) continue;
    for (const day of groupByDay(block.sessions)) {
      const first = day.items.find((session) => !session.already_absent);
      if (!first || !input.selectedSessionIds.has(first.id)) continue;
      const selection = input.sitInSelections[first.id];
      if (!selection) continue;
      // The merged date has one picker, so only its first session stores the revealed priority.
      for (const member of day.items) {
        if (member.id !== first.id && input.sitInSelections[member.id] === selection) {
          pickerSessionByMember.set(member.id, first.id);
        }
      }
    }
  }
  const payloadEntries: Array<{ scopeKey: string; targetMergeGroupId?: string; item: AbsenceBatchCreateItem }> = [];
  const attendingByDate = collectAttendingSessions(input.sessions);
  const selectedSitInIntervals: Array<{
    scopeKey: string;
    id: string;
    mergeGroupID: string | null;
    time: SessionTime;
  }> = [];
  for (const group of selectedGroups) {
    if (group.absence_limit_reached) continue;
    const selectedGroupSessions = getSelectedSessionsForGroup(
      group,
      input.selectedSessionIds,
    );
    if (selectedGroupSessions.length === 0) continue;
    const selectedSessIds = selectedGroupSessions.map((session) => session.id);
    const scopeGroups = input.sessions.filter(
      (candidate) => absenceScopeKey(candidate) === absenceScopeKey(group),
    );
    const selectedScopeSessionIds = uniqueValues(
      scopeGroups.flatMap((candidate) =>
        getSelectedSessionsForGroup(
          candidate,
          input.selectedSessionIds,
        ).map((session) => session.id),
      ),
    );
    const sitInCandidateGroups = [
      ...scopeGroups,
      ...selectedScopeSessionIds.flatMap((missedSessionID) =>
        Object.values(input.sitInPriorityHistory?.[missedSessionID] ?? {}),
      ),
    ];
    const sitInSessionIds = uniqueValues(
      selectedSessIds.flatMap((id) =>
        splitMergedSessionValue(input.sitInSelections[id]),
      ),
    );
    const conflictingSitInIds = sitInSessionIds.filter((sid) => {
      const time = findSitInSessionTimeInGroups(sitInCandidateGroups, sid);
      if (!time) return false;
      const date = instituteDateKey(time.start_at);
      const enrolledRanges = attendingByDate.get(date) ?? [];
      return enrolledRanges.some((r) => overlaps(time, r));
    });
    if (conflictingSitInIds.length > 0) {
      return {
        ok: false,
        error: `${group.subject_name || group.course_name} sit-in session conflicts with another class. Please select a different make-up time.`,
      };
    }
    for (const sid of sitInSessionIds) {
      if (selectedSitInIntervals.some((selected) => selected.scopeKey === absenceScopeKey(group) && selected.id === sid)) {
        continue;
      }
      const time = findSitInSessionTimeInGroups(sitInCandidateGroups, sid);
      if (!time) continue;
      const mergeGroupID = findSitInSessionMergeGroupIDInGroups(sitInCandidateGroups, sid);
      const scopeKey = absenceScopeKey(group);
      const overlapsSelected = selectedSitInIntervals.some(
        (selected) => {
          if (selected.id === sid) return true;
          if (!overlaps(selected.time, time)) return false;
          return !(
            selected.scopeKey === scopeKey &&
            mergeGroupID &&
            selected.mergeGroupID === mergeGroupID
          );
        },
      );
      if (overlapsSelected) {
        return {
          ok: false,
          error: `${group.subject_name || group.course_name} sit-in session overlaps another selected make-up. Please select a different time.`,
        };
      }
      selectedSitInIntervals.push({ scopeKey, id: sid, mergeGroupID, time });
    }
    for (const session of selectedGroupSessions) {
      const pickerSessionId = pickerSessionByMember.get(session.id) ?? session.id;
      const pickerGroup = groupBySessionId.get(pickerSessionId) ?? group;
      const assignment = suggestedSitInForSubmission(
        pickerGroup,
        pickerSessionId,
        input.sitInSelections,
        input.sitInPriorityLevels,
        input.sitInPriorityHistory,
      );
      payloadEntries.push({
        scopeKey: absenceScopeKey(group),
        targetMergeGroupId: assignment.targetMergeGroupId,
        item: {
          subject_id: pickerGroup.subject_id,
          course_id: pickerGroup.course_id,
          date_from: session.date,
          date_to: session.date,
          reason: input.reason.trim() || undefined,
          sit_in_method: assignment.method,
          sit_in_course_id: assignment.courseId,
          missed_session_ids: [session.id],
          sit_in_session_ids: assignment.sessionIds,
        },
      });
    }
  }
  const payloads = mergeAbsenceBatchItemsByScope(payloadEntries);
  const outOfRange = payloads.find((item) =>
    daysBetween(item.date_from, item.date_to) > input.maxDateRangeDays,
  );
  if (outOfRange) {
    const group = input.sessions.find((sessionGroup) => sessionGroup.course_id === outOfRange.course_id);
    return {
      ok: false,
      error: `${group?.subject_name || group?.course_name || "The selected class"} spans more than ${input.maxDateRangeDays} days. Split it into separate submissions.`,
    };
  }
  const duplicateSitInIDs = duplicateSitInSessionIds(payloads);
  if (duplicateSitInIDs.length > 0) {
    return {
      ok: false,
      error: "The same sit-in session is assigned to more than one absence. Please choose a different make-up session.",
    };
  }
  return { ok: true, payloads };
}
