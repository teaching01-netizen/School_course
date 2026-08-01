export const IMPACT_EVENTS = {
  QUEUE_LOADED: "schedule_impact.queue_loaded",
  ISSUE_OPENED: "schedule_impact.issue_opened",
  PREVIEW_GENERATED: "schedule_impact.preview_generated",
  RESOLUTION_SUCCEEDED: "schedule_impact.resolution_succeeded",
  RESOLUTION_CONFLICT: "schedule_impact.resolution_conflict",
  RESOLUTION_FAILED: "schedule_impact.resolution_failed",
  ANALYSIS_RETRIED: "schedule_impact.analysis_retried",
  NOTIFICATION_FAILED: "schedule_impact.notification_failed",
} as const;

export function emitImpactEvent(eventName: string, data?: Record<string, unknown>): void {
  if (import.meta.env.DEV) {
    console.log(`[Impact Analytics] ${eventName}`, data ?? "");
  }
  window.dispatchEvent(
    new CustomEvent("schedule-impact", {
      detail: { event: eventName, ...data },
    }),
  );
}
