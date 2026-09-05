import { formatDate, formatTime } from "@/utils/date";
import type { SubjectSessions } from "../types";
import { dayKey, groupByDay, splitMergedSessionValue } from "./sessionGrouping";

export type SitInAvailableSession = NonNullable<NonNullable<SubjectSessions["sit_in"]>["available_sessions"]>[number];
export type SitInCourse = NonNullable<SubjectSessions["sit_in"]>["sit_in_course"];
type SitInPriority = NonNullable<NonNullable<SubjectSessions["sit_in"]>["priorities"]>[number];

function sessionTimeRange(session: SitInAvailableSession): string {
  const start = session.merged_start_at?.trim() || session.start_at;
  const end = session.merged_end_at?.trim() || session.end_at;
  return `${formatTime(start)}-${formatTime(end)}`;
}

export function resolveSitInSubjectName(sitInCourse: SitInCourse, allSubjects: SubjectSessions[]): string | undefined {
  return sitInCourse?.merge_group_name?.trim() || sitInCourse?.subject_name?.trim() || allSubjects.find(s => s.course_id === sitInCourse?.id)?.subject_name?.trim();
}

/** Teacher shown after a subject name everywhere in the absence form:
 *  "Subject (Teacher)". No teacher on file → the label is unchanged. */
export function appendTeacher(label: string, teacher?: string | null): string {
  const trimmed = teacher?.trim();
  return trimmed ? `${label} (${trimmed})` : label;
}

function resolveSitInCourseTeacher(sitInCourse: SitInCourse, allSubjects: SubjectSessions[]): string | undefined {
  return allSubjects.find(s => s.course_id === sitInCourse?.id)?.teacher_name?.trim() || undefined;
}

export function getSitInCourseDisplayName(
  sitInCourse: SitInCourse,
  fallbackSubjectName: string,
  allSubjects: SubjectSessions[],
) {
  const base = (
    resolveSitInSubjectName(sitInCourse, allSubjects) ||
    sitInCourse?.name?.trim() ||
    fallbackSubjectName ||
    ""
  );
  return appendTeacher(base, resolveSitInCourseTeacher(sitInCourse, allSubjects));
}

export function getPriorityTargetDisplayName(
  priority: NonNullable<NonNullable<SubjectSessions["sit_in"]>["priorities"]>[number],
  fallbackSubjectName: string,
  allSubjects: SubjectSessions[],
) {
  const courseName = getSitInCourseDisplayName(priority.sit_in_course, "", allSubjects);
  if (courseName) return courseName;
  const firstSession = priority.available_sessions?.[0];
  const fallback = (
    firstSession?.class_name?.trim() ||
    firstSession?.subject_name?.trim() ||
    firstSession?.course_name?.trim() ||
    fallbackSubjectName
  );
  return appendTeacher(fallback, firstSession?.teacher_name);
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
  priority: SitInPriority,
  missedSessionIds: string[],
) {
  const available = priority.available_sessions ?? [];
  if (!available.some((session) => session.missed_session_id)) return available;
  return available.filter((session) => session.missed_session_id ? missedSessionIds.includes(session.missed_session_id) : false);
}

export type SitInOptionGroup = {
  items: SitInAvailableSession[];
  sitInCourse?: SitInCourse;
};

export function sitInOptionGroupsBySession(
  sessions: SitInAvailableSession[],
  sitInCourse?: SitInCourse,
): SitInOptionGroup[] {
  return sessions.map((session) => ({ items: [session], sitInCourse }));
}

export type SitInSessionConflict = {
  group: SubjectSessions;
  session: SubjectSessions["sessions"][number];
};

export function findSitInSessionConflicts(
  sitInSessions: Array<{ start_at: string; end_at: string }>,
  enrolledGroups: SubjectSessions[],
  selectedSubjectIds: string[],
): SitInSessionConflict[] {
  const selectedSubjects = new Set(selectedSubjectIds);
  const conflicts: SitInSessionConflict[] = [];
  const seen = new Set<string>();
  for (const group of enrolledGroups) {
    if (selectedSubjects.has(group.subject_id)) continue;
    for (const session of group.sessions) {
      if (session.already_absent) continue;
      const overlaps = sitInSessions.some((sitInSession) =>
        new Date(sitInSession.start_at) < new Date(session.end_at) &&
        new Date(sitInSession.end_at) > new Date(session.start_at),
      );
      if (!overlaps) continue;
      const key = `${group.course_id}:${session.id}`;
      if (seen.has(key)) continue;
      seen.add(key);
      conflicts.push({ group, session });
    }
  }
  return conflicts;
}

export function formatSitInSessionConflictDescription(conflicts: SitInSessionConflict[]): string | undefined {
  if (conflicts.length === 0) return undefined;

  type ConflictBucket = {
    label: string;
    teachers: string[];
    sessions: SubjectSessions["sessions"];
  };

  const buckets = new Map<string, ConflictBucket>();
  for (const { group, session } of conflicts) {
    const mergeKey = group.merge_group_id?.trim() || group.merge_group_name?.trim();
    const key = mergeKey ? `merge:${mergeKey}` : `course:${group.course_id}`;
    const bucket = buckets.get(key) ?? {
      label: group.merge_group_name?.trim()
        || group.subject_name?.trim()
        || group.course_name?.trim()
        || group.course_code,
      teachers: [],
      sessions: [],
    };
    const teacher = group.teacher_name?.trim();
    if (teacher && !bucket.teachers.includes(teacher)) bucket.teachers.push(teacher);
    bucket.sessions.push(session);
    buckets.set(key, bucket);
  }

  const descriptions = [...buckets.values()].flatMap((bucket) => {
    const label = appendTeacher(bucket.label, bucket.teachers.join(", "));
    return groupByDay(bucket.sessions).map((day) =>
      `${label} — ${formatDate(day.date)} ${formatTime(day.start_at)}-${formatTime(day.end_at)}`,
    );
  });
  return descriptions.length > 0 ? `Overlaps with ${descriptions.join("; ")}` : undefined;
}

export function formatHistoricalSitInConflictDescription(session: SitInAvailableSession): string | undefined {
  const conflict = session.conflict;
  if (!conflict) return undefined;
  const subject = conflict.sit_in_subject_name || conflict.sit_in_course_name || "This sit-in session";
  const time = conflict.sit_in_start_at && conflict.sit_in_end_at
    ? `${formatDate(conflict.sit_in_start_at)} ${formatTime(conflict.sit_in_start_at)}-${formatTime(conflict.sit_in_end_at)}`
    : "the displayed time";
  const absence = conflict.absence_subject_name
    ? `absence for ${conflict.absence_subject_name}${conflict.absence_date_from ? ` on ${conflict.absence_date_from}` : ""}`
    : `absence ${conflict.absence_id}`;
  return `${subject} at ${time} is already assigned to an ${absence}. Submission will be blocked.`;
}

export function formatSitInSubmissionConflictDetails(details: unknown): string | undefined {
  if (!details || typeof details !== "object" || !("conflicts" in details) || !Array.isArray(details.conflicts)) return undefined;
  const conflict = details.conflicts.find((item): item is Record<string, unknown> => Boolean(item) && typeof item === "object");
  if (!conflict) return undefined;
  const subject = typeof conflict.sit_in_subject_name === "string" && conflict.sit_in_subject_name ? conflict.sit_in_subject_name : typeof conflict.sit_in_course_name === "string" && conflict.sit_in_course_name ? conflict.sit_in_course_name : "This sit-in session";
  const absence = typeof conflict.absence_subject_name === "string" && conflict.absence_subject_name ? `the ${conflict.absence_subject_name} absence` : "another absence";
  return `${subject} is already assigned to ${absence}. Please choose another session.`;
}

function sitInTargetKey(
  sitInCourse: SitInCourse | undefined,
  sessions: SitInAvailableSession[],
  fallbackKey: string,
): string {
  const mergeGroupID = sitInCourse?.merge_group_id?.trim();
  if (mergeGroupID) return `merge:${mergeGroupID}`;
  const courseID = sitInCourse?.id?.trim() || sessions.find((session) => session.course_id?.trim())?.course_id?.trim();
  return courseID ? `course:${courseID}` : fallbackKey;
}

export function sitInOptionsByTargetAndSession(
  priorities: SitInPriority[],
  missedSessionIds: string[],
): SitInOptionGroup[] {
  const buckets = new Map<string, SitInOptionGroup>();
  const seenSessionIDs = new Set<string>();
  priorities.forEach((priority, index) => {
    const sessions = availableSessionsForMissedSessions(priority, missedSessionIds);
    if (sessions.length === 0) return;
    const key = sitInTargetKey(priority.sit_in_course, sessions, `priority:${index}`);
    const bucket = buckets.get(key);
    if (bucket) {
      bucket.items.push(...sessions.filter((session) => !seenSessionIDs.has(session.id)));
      sessions.forEach((session) => seenSessionIDs.add(session.id));
      return;
    }
    const uniqueSessions = sessions.filter((session) => !seenSessionIDs.has(session.id));
    uniqueSessions.forEach((session) => seenSessionIDs.add(session.id));
    buckets.set(key, { items: uniqueSessions, sitInCourse: priority.sit_in_course });
  });

  return [...buckets.values()].flatMap((bucket) => {
    const mergeGroupID = bucket.sitInCourse?.merge_group_id?.trim();
    // A merged target is one logical sit-in per institute day. Ordinary
    // targets keep distinct same-day times as separate choices.
    if (!mergeGroupID) return sitInOptionGroupsBySession(bucket.items, bucket.sitInCourse);
    return groupByDay(bucket.items).map((dayGroup) => ({
      items: dayGroup.items,
      sitInCourse: bucket.sitInCourse,
    }));
  });
}

export function unavailableSessionsForMissedSession(
  priority: NonNullable<NonNullable<SubjectSessions["sit_in"]>["priorities"]>[number],
  missedSessionId: string,
) {
  return unavailableSessionsForMissedSessions(priority, [missedSessionId]);
}

export function unavailableSessionsForMissedSessions(
  priority: NonNullable<NonNullable<SubjectSessions["sit_in"]>["priorities"]>[number],
  missedSessionIds: string[],
) {
  const unavailable = priority.unavailable_sessions ?? [];
  if (!unavailable.some((session) => session.missed_session_id)) return unavailable;
  return unavailable.filter((session) =>
    session.missed_session_id ? missedSessionIds.includes(session.missed_session_id) : false,
  );
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

export function blockedSitInSessionIds(groups: SubjectSessions[]): Set<string> {
  const blocked = new Set<string>();
  const visibleConflicts = new Set<string>();

  const collect = (sitIn: SubjectSessions["sit_in"] | undefined) => {
    for (const session of sitIn?.available_sessions ?? []) {
      if (session.conflict?.absence_id) visibleConflicts.add(session.id);
    }
    for (const unavailable of sitIn?.unavailable_sessions ?? []) {
      if (unavailable.reason_code === "sit_in_session_already_used" && unavailable.session?.id) {
        if (!visibleConflicts.has(unavailable.session.id)) blocked.add(unavailable.session.id);
      }
    }
    for (const priority of sitIn?.priorities ?? []) {
      for (const unavailable of priority.unavailable_sessions ?? []) {
        if (unavailable.reason_code === "sit_in_session_already_used" && unavailable.session?.id) {
          if (!visibleConflicts.has(unavailable.session.id)) blocked.add(unavailable.session.id);
        }
      }
    }
    for (const missedSitIn of Object.values(sitIn?.sit_in_by_missed_session ?? {})) {
      collect(missedSitIn);
    }
  };

  for (const group of groups) collect(group.sit_in);
  return blocked;
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
  const groupLabel = appendTeacher(group.subject_name?.trim() || group.course_name?.trim(), group.teacher_name);
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
  const subjectName =
    session.subject_name?.trim() ||
    sitInCourse?.subject_name?.trim() ||
    allSubjects.find((subject) => subject.course_id === sitInCourse?.id)?.subject_name?.trim();
  const className =
    sitInCourse?.merge_group_name?.trim() ||
    sitInCourse?.name?.trim() ||
    sitInCourse?.subject_name?.trim() ||
    session.class_name?.trim() ||
    session.course_name?.trim();
  const label = subjectName && className && subjectName.toLowerCase() !== className.toLowerCase()
    ? `${subjectName} — ${className}`
    : subjectName || className || fallbackSubjectName;
  const teacher = resolveSitInCourseTeacher(sitInCourse, allSubjects) ?? session.teacher_name;
  return `${appendTeacher(label, teacher)} — ${formatDate(dayKey(session))} ${sessionTimeRange(session)}`;
}

export function getSitInSessionGroupLabel(
  sessions: SitInAvailableSession[],
  sitInCourse: SitInCourse,
  fallbackSubjectName: string,
  allSubjects: SubjectSessions[],
) {
  if (sessions.length === 1) return getSitInSessionLabel(sessions[0], sitInCourse, fallbackSubjectName, allSubjects);
  const first = sessions[0];
  const subjectName =
    first.subject_name?.trim() ||
    sitInCourse?.subject_name?.trim() ||
    allSubjects.find((subject) => subject.course_id === sitInCourse?.id)?.subject_name?.trim();
  const className =
    sitInCourse?.merge_group_name?.trim() ||
    sitInCourse?.name?.trim() ||
    sitInCourse?.subject_name?.trim() ||
    first.class_name?.trim() ||
    first.course_name?.trim();
  const label = subjectName && className && subjectName.toLowerCase() !== className.toLowerCase()
    ? `${subjectName} — ${className}`
    : subjectName || className || fallbackSubjectName;
  const teacher = resolveSitInCourseTeacher(sitInCourse, allSubjects) ?? first.teacher_name;
  const range = groupByDay(sessions)[0];
  return `${appendTeacher(label, teacher)} — ${formatDate(range.date)} ${formatTime(range.start_at)}-${formatTime(range.end_at)}`;
}

export function getSitInSessionSubjectTimeLabel(
  sessions: SitInAvailableSession[],
  sitInCourse: SitInCourse,
  fallbackSubjectName: string,
  allSubjects: SubjectSessions[],
): string {
  const first = sessions[0];
  const range = groupByDay(sessions)[0];
  if (!first || !range) return "Session time unavailable";
  const subjectName =
    first.subject_name?.trim() ||
    sitInCourse?.subject_name?.trim() ||
    allSubjects.find((subject) => subject.course_id === sitInCourse?.id)?.subject_name?.trim() ||
    fallbackSubjectName;
  return `${subjectName} — ${formatDate(range.date)} ${formatTime(range.start_at)}-${formatTime(range.end_at)}`;
}

export type ChosenSitInRow = {
  /** Stable identity of the missed class-day (its first missed session id). */
  sessionKey: string;
  missedSessionId: string;
  group: SubjectSessions;
  value: string;
};

export type ChosenSitInOverlap = {
  sessionKey: string;
  /** Human-readable reason, ready to show on that plan row. */
  message: string;
};

type Interval = { start: number; end: number };

type ResolvedChosen = {
  row: ChosenSitInRow;
  records: SitInAvailableSession[];
  range: Interval;
};

function availableSessionRecords(sitIn: SubjectSessions["sit_in"]): SitInAvailableSession[] {
  const records: SitInAvailableSession[] = [];
  const collect = (node: SubjectSessions["sit_in"] | undefined) => {
    if (!node) return;
    records.push(...(node.available_sessions ?? []));
    for (const priority of node.priorities ?? []) records.push(...(priority.available_sessions ?? []));
    for (const nested of Object.values(node.sit_in_by_missed_session ?? {})) collect(nested);
  };
  collect(sitIn);
  return records;
}

function overlap(a: Interval, b: Interval): boolean {
  return a.start < b.end && a.end > b.start;
}

function recordInterval(record: SitInAvailableSession): Interval | null {
  const start = record.merged_start_at?.trim() || record.start_at;
  const end = record.merged_end_at?.trim() || record.end_at;
  if (!start || !end) return null;
  const startMs = new Date(start).getTime();
  const endMs = new Date(end).getTime();
  if (Number.isNaN(startMs) || Number.isNaN(endMs)) return null;
  return { start: startMs, end: endMs };
}

function dayTimeLabel(record: SitInAvailableSession): string {
  return `${formatDate(dayKey(record))} ${sessionTimeRange(record)}`;
}

/**
 * Client-side make-up validation (the server enforces the same rules at
 * submission): a chosen make-up must not overlap a class the student still
 * attends, nor another chosen make-up. Returns one entry per affected row.
 */
export function findChosenSitInOverlaps(
  rows: ChosenSitInRow[],
  allSubjects: SubjectSessions[],
  selectedSessionIds: Set<string>,
): ChosenSitInOverlap[] {
  const overlapsByKey = new Map<string, string[]>();

  // Resolve each chosen value to its session records and union time range.
  const resolved: ResolvedChosen[] = rows.map((row) => {
    const sitIn = groupWithSitInForMissedSession(row.group, row.missedSessionId).sit_in;
    const byId = new Map(availableSessionRecords(sitIn).map((record) => [record.id, record]));
    const parts = splitMergedSessionValue(row.value)
      .map((id) => byId.get(id))
      .filter((record): record is SitInAvailableSession => Boolean(record));
    if (parts.length === 0) return null;
    const ranges = parts.map((record) => recordInterval(record)).filter((interval): interval is Interval => Boolean(interval));
    if (ranges.length === 0) return null;
    return {
      row,
      records: parts,
      range: {
        start: Math.min(...ranges.map((range) => range.start)),
        end: Math.max(...ranges.map((range) => range.end)),
      },
    };
  }).filter((entry): entry is ResolvedChosen => Boolean(entry));

  // Attended classes: sessions the student still has on their timetable.
  const attended: Array<{ group: SubjectSessions; session: SubjectSessions["sessions"][number] }> = [];
  for (const group of allSubjects) {
    for (const session of group.sessions) {
      if (session.already_absent) continue;
      if (selectedSessionIds.has(session.id)) continue;
      const interval = recordInterval(session);
      if (interval) attended.push({ group, session: { ...session, start_at: session.start_at, end_at: session.end_at } });
    }
  }

  const pushMessage = (sessionKey: string, message: string) => {
    const existing = overlapsByKey.get(sessionKey) ?? [];
    if (!existing.includes(message)) existing.push(message);
    overlapsByKey.set(sessionKey, existing);
  };

  for (const chosen of resolved) {
    const conflicts = attended.filter(({ session }) => {
      const interval = recordInterval(session);
      return interval ? overlap(chosen.range, interval) : false;
    }).map(({ group, session }) => ({ group, session }));
    const description = formatSitInSessionConflictDescription(conflicts);
    if (description) pushMessage(chosen.row.sessionKey, `${description}. Choose another time.`);
  }

  for (let i = 0; i < resolved.length; i += 1) {
    for (let j = i + 1; j < resolved.length; j += 1) {
      const a = resolved[i];
      const b = resolved[j];
      if (!overlap(a.range, b.range)) continue;
      // Each row's message names the OTHER make-up it clashes with.
      const aLabel = a.row.group.merge_group_name?.trim() || a.row.group.subject_name?.trim() || a.row.group.course_name?.trim() || a.row.group.course_code || "that class";
      const bLabel = b.row.group.merge_group_name?.trim() || b.row.group.subject_name?.trim() || b.row.group.course_name?.trim() || b.row.group.course_code || "that class";
      const bTime = dayTimeLabel(b.records[0]);
      const aTime = dayTimeLabel(a.records[0]);
      pushMessage(a.row.sessionKey, `Overlaps with your ${bLabel} make-up (${bTime}). Choose another time.`);
      pushMessage(b.row.sessionKey, `Overlaps with your ${aLabel} make-up (${aTime}). Choose another time.`);
    }
  }

  return [...overlapsByKey.entries()].map(([sessionKey, messages]) => ({ sessionKey, message: messages.join(" ") }));
}
