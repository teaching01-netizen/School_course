export type ImpactCandidate = {
  session_id: string;
  session_version: number;
  start_at: string;
  end_at: string;
  course_code: string;
  course_name: string;
  room_name: string;
  teacher: string;
  available_capacity: number;
  eligible: boolean;
  student_conflicts: boolean;
  generated_at: string;
  /** @deprecated Prefer selectable + blocking_reasons */
  selectable?: boolean;
  blocking_reasons?: CandidateBlockingReason[];
  recommendation_rank?: number | null;
  recommendation_reasons?: string[];
};

export type OriginalSessionView = {
  quality: "exact" | "reconstructed" | "unavailable";
  source: string;
  snapshot: Record<string, unknown> | null;
};

export type CurrentSessionView = {
  status: "active" | "deleted" | "unknown";
  session_id: string;
  version: number;
  start_at: string;
  end_at: string;
  course_code: string;
  course_name: string;
  /** Present for active sessions; absent for deleted sessions. */
  subject_name?: string;
  room_name: string | null;
  teacher_name: string;
};

export type AssignmentContext = {
  assigned_at: string | null;
  original_session: OriginalSessionView;
  current_session: CurrentSessionView | null;
};

export type ChangeContext = {
  change_id: string;
  before: Record<string, unknown> | null;
  after: Record<string, unknown> | null;
};

export type ImpactReason = {
  code: string;
  message: string;
};

export type ImpactContext = {
  issue_type: string;
  severity: string;
  reasons: ImpactReason[];
};

export type ScheduleImpactIssue = {
  id: string;
  absence_id: string;
  issue_type: string;
  severity: "critical" | "warning";
  status: "open" | "needs_review" | "resolved" | "dismissed" | "superseded";
  issue_version: number;
  wcode: string;
  student_name: string | null;
  /** @deprecated Use assignment_context.original_session.snapshot.start_at or change_context.before.start_at */
  start_at: string | null;
  /** @deprecated Use assignment_context.original_session.snapshot.end_at or change_context.before.end_at */
  end_at: string | null;
  details: { reasons?: string[]; notice_hours?: number; old_start_at?: string; new_start_at?: string };
  suggested_resolutions: ImpactCandidate[];
  resolution_action: string | null;
  assignment_context: AssignmentContext;
  change_context: ChangeContext;
  impact_context: ImpactContext;
  action_policy?: ResolutionActionPolicy[];
};

export type ScheduleImpactSummary = {
  need_attention: number;
  critical: number;
  warnings: number;
  notification_failures: number;
  notifications_configured: boolean;
};

export type ScheduleImpactQueueResponse = {
  items: ScheduleImpactIssue[];
  summary: ScheduleImpactSummary;
  pagination?: PaginationMeta;
  limit: number;
  offset: number;
};

export type ImpactProcessingChange = {
  id: string;
  course_code: string;
  course_name: string;
  subject_name: string;
  created_at: string;
  status: "pending" | "processing" | "failed" | "delayed_by_batch";
  last_error: string | null;
};

export type ResolutionResponse = {
  id: string;
  status: string;
  action: string;
  notification_status: "queued" | "not_configured" | "no_recipient" | "not_required";
};

/* ── PR-00: Extended contracts ────────────────────────────────────── */

export type ResolutionAction =
  | "reassign"
  | "keep"
  | "cancel"
  | "dismiss"
  | "mark_for_review";

export type ResolutionActionPolicy = {
  action: ResolutionAction;
  allowed: boolean;
  reason_required: boolean;
  disabled_reason: string | null;
  notification_expected: boolean;
};

export type CandidateBlockingReason = {
  code:
    | "student_conflict"
    | "not_eligible"
    | "full"
    | "session_changed"
    | "unavailable";
  message: string;
};

export type PaginationMeta = {
  limit: number;
  offset: number;
  total: number;
  has_more: boolean;
  next_offset: number | null;
};

export type NotificationChannelPreview = {
  channel: "sms" | "email";
  configured: boolean;
  recipient_masked: string | null;
  will_queue: boolean;
  unavailable_reason: string | null;
};

export type NotificationPreview = {
  action: ResolutionAction;
  message_type: string | null;
  channels: NotificationChannelPreview[];
  overall_status:
    | "will_queue"
    | "not_configured"
    | "no_recipient"
    | "not_required";
  generated_at: string;
  issue_version: number;
};

export type ImpactProcessingChangeExtended = ImpactProcessingChange & {
  updated_at: string;
  processing_attempt: number;
  retryable: boolean;
  error_category: string | null;
  trace_id: string | null;
};

export type HistoryItem = {
  id: string;
  new_course_code: string;
  new_course_name: string;
  new_course_subject: string;
  old_start_at: string;
  old_end_at: string;
  new_start_at: string;
  new_end_at: string;
  created_at: string;
  open_issue_count: number;
  critical_issue_count: number;
};

export type QueueURLState = {
  view: "queue" | "processing" | "history";
  query: string;
  severity: "" | "critical" | "warning";
  status: "all" | "open" | "needs_review";
  offset: number;
  limit: 25 | 50 | 100;
};
