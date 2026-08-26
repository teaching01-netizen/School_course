import { conflictOverviewResponseSchema, conflictSummarySchema } from "../types/conflictOverview";
import type { ConflictOverviewResponse, ConflictSummaryResponse } from "../types/conflictOverview";

export function parseConflictOverviewResponse(value: unknown): ConflictOverviewResponse {
  return conflictOverviewResponseSchema.parse(value);
}

export function scheduleConflictsURL(searchParams: URLSearchParams): string {
  const query = new URLSearchParams(searchParams);
  query.set("limit", "50");
  return `/api/v1/schedule-conflicts?${query.toString()}`;
}

export function parseConflictSummaryResponse(value: unknown): ConflictSummaryResponse {
  return conflictSummarySchema.parse(value);
}

export function scheduleConflictsSummaryURL(searchParams: URLSearchParams): string {
  const query = new URLSearchParams(searchParams);
  query.delete("cursor");
  query.delete("limit");
  return `/api/v1/schedule-conflicts/summary?${query.toString()}`;
}
