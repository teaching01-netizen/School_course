import type { ImpactCandidate } from "../types";

/**
 * Determine if a candidate can be selected for reassignment.
 * A candidate is selectable when it is eligible, has capacity, and has no blocking reasons.
 */
export function isCandidateSelectable(c: ImpactCandidate): boolean {
  if (!c.eligible) return false;
  if (c.available_capacity === 0) return false;
  if (c.blocking_reasons && c.blocking_reasons.length > 0) return false;
  return true;
}

/**
 * Format a human-readable capacity label for a candidate.
 */
export function capacityLabel(c: ImpactCandidate): string {
  if (c.available_capacity < 0) return "Capacity not limited";
  if (c.available_capacity === 0) return "Full";
  return `${c.available_capacity} seats available`;
}
