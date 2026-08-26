import { conflictOverviewResponseSchema } from "../types/conflictOverview";
import type { ConflictOverviewResponse } from "../types/conflictOverview";

export function parseConflictOverviewResponse(value: unknown): ConflictOverviewResponse {
  return conflictOverviewResponseSchema.parse(value);
}

export function scheduleConflictsURL(searchParams: URLSearchParams): string {
  const query = new URLSearchParams(searchParams);
  query.set("limit", "50");
  return `/api/v1/schedule-conflicts?${query.toString()}`;
}
