export const MAX_SERIES_OCCURRENCES = 1000;
export const MAX_SERIES_HORIZON_YEARS = 5;
export const MAX_SESSION_DURATION_MINUTES = 24 * 60;

export function isFutureSession(startAt: string, nowMs = Date.now()): boolean {
  const startMs = Date.parse(startAt);
  return Number.isFinite(startMs) && startMs > nowMs;
}
