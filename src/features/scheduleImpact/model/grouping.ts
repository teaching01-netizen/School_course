import type { ScheduleImpactIssue } from "../types";

/**
 * Group unresolved issues by priority: critical first, then needs_review, then warning.
 * Excludes resolved, dismissed, and superseded issues.
 * Server ordering is preserved within each group.
 */
export function groupIssues(items: ScheduleImpactIssue[]): ScheduleImpactIssue[] {
  const unresolved = items.filter(
    (i) => i.status !== "resolved" && i.status !== "dismissed" && i.status !== "superseded",
  );
  const critical = unresolved.filter((i) => i.severity === "critical");
  const review = unresolved.filter((i) => i.status === "needs_review" && i.severity !== "critical");
  const warning = unresolved.filter((i) => i.severity === "warning" && i.status !== "needs_review");
  return [...critical, ...review, ...warning];
}
