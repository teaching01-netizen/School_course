import type { SubjectSessions } from "../types";

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

export function instituteDateKey(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value.slice(0, 10);
  const parts = new Intl.DateTimeFormat("en-GB", {
    timeZone: INSTITUTE_TIME_ZONE,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(date);
  const part = (type: string) => parts.find((p) => p.type === type)?.value ?? "";
  return `${part("year")}-${part("month")}-${part("day")}`;
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

export function groupByDay<T extends TimeRanged>(items: T[]): DayRangeGroup<T>[] {
  const byDay = new Map<string, T[]>();
  for (const item of sortByStart(items)) {
    const key = dayKey(item);
    byDay.set(key, [...(byDay.get(key) ?? []), item]);
  }
  return [...byDay.entries()].sort(([a], [b]) => a.localeCompare(b)).map(([date, dayItems]) => {
    const sorted = sortByStart(dayItems);
    const merged = mergeRanges(sorted);
    const id = sorted.map((item) => item.id ?? `${item.start_at}-${item.end_at}`).join(MERGED_SESSION_ID_SEPARATOR);
    return { id, date, start_at: merged.start_at, end_at: merged.end_at, items: sorted };
  });
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

export function countSelectedAbsenceDays(groups: SubjectSessions[], selected: Set<string>): number {
  return groups.reduce(
    (total, group) => total + countSelectedAbsenceDaysForGroup(group, selected),
    0,
  );
}

export function getSelectedSessionsForGroup(group: SubjectSessions, selected: Set<string>) {
  return group.sessions
    .filter((session) => selected.has(session.id) && !session.already_absent)
    .slice()
    .sort((a, b) => a.start_at.localeCompare(b.start_at));
}
