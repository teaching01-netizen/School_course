export function startOfLocalDay(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate(), 0, 0, 0, 0);
}

export function endOfLocalDay(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate(), 23, 59, 59, 999);
}

export function localDayRangeRFC3339(d: Date): { start: string; end: string } {
  return { start: startOfLocalDay(d).toISOString(), end: endOfLocalDay(d).toISOString() };
}

export function clampDateRange(
  startDateStr: string,
  endDateStr: string,
  maxDays = 13,
): { endDate: string; clamped: boolean } {
  const start = new Date(startDateStr);
  const end = new Date(endDateStr);
  const diffDays = (end.getTime() - start.getTime()) / (1000 * 60 * 60 * 24);
  if (diffDays <= maxDays) return { endDate: endDateStr, clamped: false };
  const clamped = new Date(start.getTime() + maxDays * 24 * 60 * 60 * 1000);
  return { endDate: clamped.toISOString().slice(0, 10), clamped: true };
}

export function minutesBetweenRFC3339(start: string, end: string): number {
  const a = new Date(start).getTime();
  const b = new Date(end).getTime();
  if (Number.isNaN(a) || Number.isNaN(b)) return 0;
  return Math.max(0, Math.round((b - a) / 60000));
}

