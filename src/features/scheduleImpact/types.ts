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
  start_at: string | null;
  end_at: string | null;
  details: { reasons?: string[]; notice_hours?: number; old_start_at?: string; new_start_at?: string };
  suggested_resolutions: ImpactCandidate[];
  resolution_action: string | null;
  assignment_context: AssignmentContext;
  change_context: ChangeContext;
  impact_context: ImpactContext;
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
  limit: number;
  offset: number;
};

export type ImpactProcessingChange = {
  id: string;
  course_code: string;
  course_name: string;
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
