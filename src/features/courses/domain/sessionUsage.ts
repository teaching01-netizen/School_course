import type { Session } from "@/features/scheduling/types";
import { fmtDuration, minutesBetween } from "@/features/scheduling/domain/time";

/** Total minutes of scheduled teaching across a course's sessions. Sessions
 *  with unparseable timestamps are skipped rather than treated as zero. */
export function sumSessionMinutes(sessions: Session[]): number {
  return sessions.reduce((total, session) => total + (minutesBetween(session.start_at, session.end_at) ?? 0), 0);
}

/** Minutes left from the user-set hours after the scheduled sessions are
 *  consumed. Returns null (undefined) when the course has no hours set. */
export function remainingMinutes(hour: number | null | undefined, usedMinutes: number): number | null {
  if (hour == null) return null;
  return hour * 60 - usedMinutes;
}

export type RemainingStatus = "remaining" | "over" | "none";

/** Only three states exist on the boundary: still some left (> 0), exactly
 *  consumed (= 0), or over the set hours (< 0). */
export function remainingStatus(minutes: number): RemainingStatus {
  if (minutes > 0) return "remaining";
  if (minutes < 0) return "over";
  return "none";
}

/** Hours:minutes for display, with the deficit drawn as a leading minus. */
export function formatRemainingHours(minutes: number): string {
  return (minutes < 0 ? "-" : "") + fmtDuration(Math.abs(minutes));
}