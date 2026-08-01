import type {
  ScheduleImpactIssue,
  ImpactCandidate,
  ResolutionActionPolicy,
  ScheduleImpactQueueResponse,
  ScheduleImpactSummary,
  NotificationPreview,
  ImpactProcessingChangeExtended,
  HistoryItem,
  ResolutionResponse,
  PaginationMeta,
} from "../types";

/* ------------------------------------------------------------------ */
/*  Deep merge helper                                                  */
/* ------------------------------------------------------------------ */

export type DeepPartial<T> = {
  [P in keyof T]?: T[P] extends object ? DeepPartial<T[P]> : T[P];
};

/** Deep merge helper for builders. Arrays in overrides replace entirely (not merged). */
function deepMerge<T>(base: T, overrides: DeepPartial<T>): T {
  const result = { ...base };
  for (const key of Object.keys(overrides) as Array<keyof T>) {
    const overrideVal = overrides[key];
    if (
      overrideVal !== null &&
      overrideVal !== undefined &&
      typeof overrideVal === "object" &&
      !Array.isArray(overrideVal) &&
      typeof result[key] === "object" &&
      result[key] !== null &&
      !Array.isArray(result[key])
    ) {
      (result as Record<string, unknown>)[key as string] = deepMerge(
        result[key] as Record<string, unknown>,
        overrideVal as Record<string, unknown>,
      );
    } else if (overrideVal !== undefined) {
      (result as Record<string, unknown>)[key as string] = overrideVal;
    }
  }
  return result;
}

/* ------------------------------------------------------------------ */
/*  Issue builder                                                      */
/* ------------------------------------------------------------------ */

export function buildImpactIssue(
  overrides: DeepPartial<ScheduleImpactIssue> = {},
): ScheduleImpactIssue {
  return deepMerge(
    {
      id: "issue-1",
      absence_id: "absence-1",
      issue_type: "regular_session_overlap",
      severity: "critical" as const,
      status: "open" as const,
      issue_version: 4,
      wcode: "STU001",
      student_name: "Alice Johnson",
      start_at: null,
      end_at: null,
      details: {},
      suggested_resolutions: [],
      resolution_action: null,
      assignment_context: {
        assigned_at: "2026-07-30T03:00:00Z",
        original_session: {
          quality: "exact" as const,
          source: "assignment_snapshot",
          snapshot: {
            start_at: "2026-07-31T03:00:00Z",
            end_at: "2026-07-31T04:00:00Z",
            course_code: "MATH101",
          },
        },
        current_session: {
          status: "active" as const,
          session_id: "session-1",
          version: 7,
          start_at: "2026-07-31T06:00:00Z",
          end_at: "2026-07-31T07:00:00Z",
          course_code: "MATH101",
          course_name: "Mathematics",
          room_name: "Room 5",
          teacher_name: "Dr Jones",
        },
      },
      change_context: {
        change_id: "change-1",
        before: {},
        after: {},
      },
      impact_context: {
        issue_type: "regular_session_overlap",
        severity: "critical",
        reasons: [
          {
            code: "regular_session_overlap",
            message: "Overlaps the student's regular class.",
          },
        ],
      },
      action_policy: [],
    },
    overrides,
  );
}

/* ------------------------------------------------------------------ */
/*  Candidate builder                                                  */
/* ------------------------------------------------------------------ */

export function buildImpactCandidate(
  overrides: DeepPartial<ImpactCandidate> = {},
): ImpactCandidate {
  return deepMerge(
    {
      session_id: "cand-1",
      session_version: 3,
      start_at: "2026-08-14T03:00:00Z",
      end_at: "2026-08-14T04:30:00Z",
      course_code: "MATH101",
      course_name: "Mathematics",
      room_name: "Room 9",
      teacher: "Dr Jones",
      available_capacity: 5,
      eligible: true,
      student_conflicts: false,
      generated_at: "2026-07-31T00:00:00Z",
      recommendation_rank: null,
    },
    overrides,
  );
}

/* ------------------------------------------------------------------ */
/*  Action policy builder                                              */
/* ------------------------------------------------------------------ */

export function buildActionPolicy(
  overrides: DeepPartial<ResolutionActionPolicy> = {},
): ResolutionActionPolicy {
  return deepMerge(
    {
      action: "reassign" as const,
      allowed: true,
      reason_required: false,
      disabled_reason: null,
      notification_expected: true,
    },
    overrides,
  );
}

/* ------------------------------------------------------------------ */
/*  Queue response builder                                             */
/* ------------------------------------------------------------------ */

export function buildQueueResponse(
  items: ScheduleImpactIssue[] = [],
  overrides: DeepPartial<ScheduleImpactQueueResponse> = {},
): ScheduleImpactQueueResponse {
  return deepMerge(
    {
      items,
      summary: buildQueueSummary({
        need_attention: items.length,
        critical: items.filter((i) => i.severity === "critical").length,
        warnings: items.filter((i) => i.severity === "warning").length,
      }),
      limit: 25,
      offset: 0,
    },
    overrides,
  );
}

export function buildQueueSummary(
  overrides: DeepPartial<ScheduleImpactSummary> = {},
): ScheduleImpactSummary {
  return deepMerge(
    {
      need_attention: 0,
      critical: 0,
      warnings: 0,
      notification_failures: 0,
      notifications_configured: true,
    },
    overrides,
  );
}

/* ------------------------------------------------------------------ */
/*  Pagination builder                                                 */
/* ------------------------------------------------------------------ */

export function buildPagination(
  overrides: DeepPartial<PaginationMeta> = {},
): PaginationMeta {
  return deepMerge(
    {
      limit: 25,
      offset: 0,
      total: 0,
      has_more: false,
      next_offset: null,
    },
    overrides,
  );
}

/* ------------------------------------------------------------------ */
/*  Notification preview builder                                       */
/* ------------------------------------------------------------------ */

export function buildNotificationPreview(
  overrides: DeepPartial<NotificationPreview> = {},
): NotificationPreview {
  return deepMerge(
    {
      action: "keep" as const,
      message_type: "sit_in_change",
      channels: [
        {
          channel: "sms" as const,
          configured: true,
          recipient_masked: "081****5678",
          will_queue: true,
          unavailable_reason: null,
        },
      ],
      overall_status: "will_queue" as const,
      generated_at: "2026-07-31T04:00:00Z",
      issue_version: 4,
    },
    overrides,
  );
}

/* ------------------------------------------------------------------ */
/*  Processing change builder                                          */
/* ------------------------------------------------------------------ */

export function buildProcessingChange(
  overrides: DeepPartial<ImpactProcessingChangeExtended> = {},
): ImpactProcessingChangeExtended {
  return deepMerge(
    {
      id: "change-1",
      course_code: "MATH101",
      course_name: "Mathematics",
      created_at: "2026-07-31T03:00:00Z",
      status: "pending" as const,
      last_error: null,
      updated_at: "2026-07-31T03:00:00Z",
      processing_attempt: 0,
      retryable: true,
      error_category: null,
      trace_id: null,
    },
    overrides,
  );
}

/* ------------------------------------------------------------------ */
/*  History item builder                                               */
/* ------------------------------------------------------------------ */

export function buildHistoryItem(
  overrides: DeepPartial<HistoryItem> = {},
): HistoryItem {
  return deepMerge(
    {
      id: "change-1",
      new_course_code: "MATH101",
      new_course_name: "Mathematics",
      old_start_at: "2026-07-31T03:00:00Z",
      old_end_at: "2026-07-31T04:00:00Z",
      new_start_at: "2026-07-31T06:00:00Z",
      new_end_at: "2026-07-31T07:00:00Z",
      created_at: "2026-07-31T02:00:00Z",
      open_issue_count: 1,
      critical_issue_count: 0,
    },
    overrides,
  );
}

/* ------------------------------------------------------------------ */
/*  Resolution response builder                                        */
/* ------------------------------------------------------------------ */

export function buildResolutionResponse(
  overrides: DeepPartial<ResolutionResponse> = {},
): ResolutionResponse {
  return deepMerge(
    {
      id: "issue-1",
      status: "resolved",
      action: "keep",
      notification_status: "queued",
    },
    overrides,
  );
}
