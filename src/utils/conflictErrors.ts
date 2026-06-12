import { ApiRequestError } from "@/api/client";
import { conflictKindLabel, formatTimeRange, type ConflictDetails } from "@/types";

export function isConflictDetails(value: unknown): value is ConflictDetails {
  if (!value || typeof value !== "object") return false;
  const details = value as Record<string, unknown>;
  if (typeof details.kind !== "string") return false;
  if (!details.requested || typeof details.requested !== "object") return false;
  return Array.isArray(details.conflicts) || details.conflicts === null;
}

export function getConflictDetails(error: unknown): ConflictDetails | null {
  if (!(error instanceof ApiRequestError)) return null;
  return isConflictDetails(error.details) ? error.details : null;
}

export function formatConflictToastMessage(error: unknown, fallback: string): string {
  const details = getConflictDetails(error);
  if (!details) {
    return error instanceof Error ? error.message : fallback;
  }

  const label = conflictKindLabel(details.kind).label;
  const names = details.conflicting_students?.map((student) => student.full_name).filter(Boolean) ?? [];
  const range = details.requested?.start_at && details.requested?.end_at
    ? formatTimeRange(details.requested.start_at, details.requested.end_at)
    : "";

  if (names.length > 0 && range) return `${label}: ${names.join(", ")} conflict at ${range}`;
  if (names.length > 0) return `${label}: ${names.join(", ")}`;
  if (range) return `${label} at ${range}`;
  return label;
}
