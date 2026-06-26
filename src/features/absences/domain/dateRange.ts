export function dateToLocalISO(date: Date): string {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, "0");
  const d = String(date.getDate()).padStart(2, "0");
  return `${y}-${m}-${d}`;
}

export function postSessionLookbackDays(maxHoursAfterSession: number): number {
  if (!Number.isFinite(maxHoursAfterSession) || maxHoursAfterSession <= 0) return 0;
  return Math.ceil(maxHoursAfterSession / 24);
}

export function daysBetween(from: string, to: string): number {
  return Math.round(
    (new Date(`${to}T00:00:00`).getTime() - new Date(`${from}T00:00:00`).getTime()) /
      (1000 * 60 * 60 * 24),
  );
}
