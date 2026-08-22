import type { SubjectSessions, VerifiedStudentSubject } from "../types";

export const INSTITUTE_TIME_ZONE = "Asia/Bangkok";
export const MERGED_SESSION_ID_SEPARATOR = "|";

export type TimeRanged = { id?: string; start_at: string; end_at: string; date?: string };

export type MergedDayRange = {
  date: string;
  start_at: string;
  end_at: string;
};

export type DayRangeGroup<T extends TimeRanged> = MergedDayRange & {
  id: string;
  items: T[];
};

// Hot path: instituteDateKey runs per session on every groupByDay call and
// groupByDay runs O(groups) times per render. Two invariants keep it cheap:
// the formatter is constructed once, and results are memoized by input
// identity — server-state arrays are never mutated, so a WeakMap is safe.
let instituteDateFormatter: Intl.DateTimeFormat | null = null;
const dateKeyCache = new Map<string, string>();
const DATE_KEY_CACHE_LIMIT = 2000;

export function instituteDateKey(value: string): string {
  const cached = dateKeyCache.get(value);
  if (cached !== undefined) return cached;
  instituteDateFormatter ??= new Intl.DateTimeFormat("en-GB", {
    timeZone: INSTITUTE_TIME_ZONE,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  });
  const date = new Date(value);
  const key = Number.isNaN(date.getTime())
    ? value.slice(0, 10)
    : (() => {
        const parts = instituteDateFormatter!.formatToParts(date);
        const part = (type: string) => parts.find((p) => p.type === type)?.value ?? "";
        return `${part("year")}-${part("month")}-${part("day")}`;
      })();
  if (dateKeyCache.size >= DATE_KEY_CACHE_LIMIT) dateKeyCache.clear();
  dateKeyCache.set(value, key);
  return key;
}

export function dayKey(item: TimeRanged): string {
  return item.date ?? instituteDateKey(item.start_at);
}

function mergeRanges(ranges: TimeRanged[]): { start_at: string; end_at: string } {
  let start = ranges[0].start_at;
  let end = ranges[0].end_at;
  for (const r of ranges) {
    if (new Date(r.start_at).getTime() < new Date(start).getTime()) start = r.start_at;
    if (new Date(r.end_at).getTime() > new Date(end).getTime()) end = r.end_at;
  }
  return { start_at: start, end_at: end };
}

function sortByStart<T extends TimeRanged>(items: T[]): T[] {
  return items.slice().sort((a, b) => new Date(a.start_at).getTime() - new Date(b.start_at).getTime());
}

const groupByDayCache = new WeakMap<object, unknown>();

export function groupByDay<T extends TimeRanged>(items: T[]): DayRangeGroup<T>[] {
  const cached = groupByDayCache.get(items);
  if (cached !== undefined) return cached as DayRangeGroup<T>[];
  // One global sort by start time; per-day buckets preserve that order, so no
  // re-sort inside a day is needed.
  const sorted = sortByStart(items);
  const byDay = new Map<string, T[]>();
  for (const item of sorted) {
    const key = dayKey(item);
    const bucket = byDay.get(key);
    if (bucket) bucket.push(item);
    else byDay.set(key, [item]);
  }
  const result = [...byDay.entries()].sort(([a], [b]) => a.localeCompare(b)).map(([date, dayItems]) => {
    const merged = mergeRanges(dayItems);
    const id = dayItems.map((item) => item.id ?? `${item.start_at}-${item.end_at}`).join(MERGED_SESSION_ID_SEPARATOR);
    return { id, date, start_at: merged.start_at, end_at: merged.end_at, items: dayItems };
  });
  groupByDayCache.set(items, result);
  return result;
}

export function mergedSessionValue(items: Array<{ id: string }>): string {
  return items.map((item) => item.id).join(MERGED_SESSION_ID_SEPARATOR);
}

export function splitMergedSessionValue(value: string | undefined): string[] {
  return (value ?? "").split(MERGED_SESSION_ID_SEPARATOR).filter(Boolean);
}

export function uniqueValues(values: string[]): string[] {
  return [...new Set(values)];
}

export function isDayGroupSelected(group: DayRangeGroup<{ id: string; start_at: string; end_at: string; date?: string }>, selected: Set<string>): boolean {
  return group.items.every((session) => selected.has(session.id));
}

export function countSelectedAbsenceDaysForGroup(group: SubjectSessions, selected: Set<string>): number {
  return groupByDay(group.sessions)
    .map((sessionGroup) => ({
      ...sessionGroup,
      items: sessionGroup.items.filter((session) => !session.already_absent),
    }))
    .filter((sessionGroup) => sessionGroup.items.length > 0 && isDayGroupSelected(sessionGroup, selected)).length;
}

export function absenceScopeKey(group: SubjectSessions): string {
  return group.merge_group_id ? `merge:${group.merge_group_id}` : `course:${group.course_id}`;
}

function joinTeachers(names: Array<string | null | undefined>): string | undefined {
  const distinct: string[] = [];
  for (const name of names) {
    const trimmed = name?.trim();
    if (trimmed && !distinct.includes(trimmed)) distinct.push(trimmed);
  }
  return distinct.length > 0 ? distinct.join(", ") : undefined;
}

/** One renderable class block. A merged course combines its source courses'
 *  groups into a single block keyed by the shared absence scope, so students
 *  see and book one merged class with one quota; `groups` keeps the source
 *  courses for per-session sit-in resolution and submission truth. */
export type SubjectBlock = {
  key: string;
  subjectIds: string[];
  isMerged: boolean;
  label: string;
  teacherName?: string;
  groups: SubjectSessions[];
  sessions: SubjectSessions["sessions"];
};

/** Combine session groups into one block per absence scope (merged courses
 *  collapse to a single block). Single pass; block order follows first
 *  appearance. */
export function combineSubjectGroups(groups: SubjectSessions[]): SubjectBlock[] {
  const byKey = new Map<string, SubjectBlock>();
  const order: SubjectBlock[] = [];
  for (const group of groups) {
    const key = absenceScopeKey(group);
    let block = byKey.get(key);
    if (!block) {
      block = {
        key,
        subjectIds: [],
        isMerged: Boolean(group.merge_group_id),
        label: "",
        groups: [],
        sessions: [],
      };
      byKey.set(key, block);
      order.push(block);
    }
    block.groups.push(group);
    block.subjectIds.push(group.subject_id);
    block.sessions.push(...group.sessions);
  }
  for (const block of order) {
    block.sessions.sort((a, b) => a.start_at.localeCompare(b.start_at));
    const primary = block.groups[0];
    block.label =
      (block.isMerged ? block.groups.find((g) => g.merge_group_name?.trim())?.merge_group_name?.trim() : "") ||
      primary.subject_name?.trim() ||
      primary.course_name?.trim() ||
      primary.course_code;
    block.teacherName = joinTeachers(block.groups.map((g) => g.teacher_name));
  }
  return order;
}

/** One picker entry: merged subjects collapse into a single entry keyed by
 *  their merge group, carrying every source subject id so selecting the
 *  entry selects all of them. */
export type SubjectPickerEntry = {
  key: string;
  subjectIds: string[];
  isMerged: boolean;
  label: string;
  teacherName?: string;
};

export function combineSubjectPickerEntries(subjects: VerifiedStudentSubject[]): SubjectPickerEntry[] {
  const byKey = new Map<string, { entry: SubjectPickerEntry; teacherNames: Array<string | null | undefined> }>();
  const order: SubjectPickerEntry[] = [];
  for (const subject of subjects) {
    const mergeID = subject.merge_group_id?.trim();
    const key = mergeID ? `merge:${mergeID}` : `subject:${subject.id}`;
    let bucket = byKey.get(key);
    if (!bucket) {
      bucket = {
        entry: {
          key,
          subjectIds: [],
          isMerged: Boolean(mergeID),
          label: (mergeID ? subject.merge_group_name?.trim() : "") || subject.name,
        },
        teacherNames: [],
      };
      byKey.set(key, bucket);
      order.push(bucket.entry);
    }
    bucket.entry.subjectIds.push(subject.id);
    bucket.teacherNames.push(subject.teacher_name);
  }
  for (const bucket of byKey.values()) {
    bucket.entry.teacherName = joinTeachers(bucket.teacherNames);
  }
  return order;
}

export function countSelectedAbsenceDaysForScope(
  groups: SubjectSessions[],
  selected: Set<string>,
  scopeKey: string,
): number {
  const selectedDays = new Set<string>();
  for (const group of groups) {
    if (absenceScopeKey(group) !== scopeKey) continue;
    for (const sessionGroup of groupByDay(group.sessions)) {
      const activeItems = sessionGroup.items.filter((session) => !session.already_absent);
      if (activeItems.length > 0 && isDayGroupSelected({ ...sessionGroup, items: activeItems }, selected)) {
        selectedDays.add(sessionGroup.date);
      }
    }
  }
  return selectedDays.size;
}

export function countSelectedAbsenceDays(groups: SubjectSessions[], selected: Set<string>): number {
  const scopeKeys = new Set(groups.map(absenceScopeKey));
  return [...scopeKeys].reduce(
    (total, scopeKey) => total + countSelectedAbsenceDaysForScope(groups, selected, scopeKey),
    0,
  );
}

export function getSelectedSessionsForGroup(group: SubjectSessions, selected: Set<string>) {
  return group.sessions
    .filter((session) => selected.has(session.id) && !session.already_absent)
    .slice()
    .sort((a, b) => a.start_at.localeCompare(b.start_at));
}
