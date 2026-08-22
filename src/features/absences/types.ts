export type SitInRuleType =
  | "level_ladder"
  | "cross_section"
  | "any_day_except_last"
  | "rank_chain"
  | "teacher_case_by_case";

export type SitInRule = {
  id: string;
  name: string;
  type: SitInRuleType;
  predicate: Record<string, unknown>;
  description: string;
  created_at: string;
  updated_at: string;
};

export type SitInRuleCreateInput = {
  name: string;
  type: SitInRuleType;
  predicate: Record<string, unknown>;
  description: string;
};

export type StudentAbsence = {
  id: string;
  wcode: string;
  course_id: string;
  date_from: string;
  date_to: string;
  reason?: string | null;
  sit_in_course_id?: string | null;
  course_code?: string;
  course_name?: string;
  sit_in_course_code?: string | null;
  sit_in_course_name?: string | null;
  sit_in_subject_name?: string | null;
  sit_in_merge_group_name?: string | null;
  created_at: string;
  subject_id?: string | null;
  subject_code?: string | null;
  subject_name?: string | null;
  merge_group_id?: string | null;
  merge_group_name?: string | null;
  sit_in_method?: string | null;
  sit_ins?: Array<{ id: string; session_id: string }>;
  student_name?: string | null;
  student_email?: string | null;
  student_nickname?: string | null;
  student_phone?: string | null;
  reason_category?: string | null;
  status: AbsenceStatus;
  admin_notes?: string | null;
  reviewed_by?: string | null;
  reviewed_at?: string | null;
  sit_in_rule_id?: string | null;
  sit_in_rule_name?: string | null;
  sit_in_overridden?: boolean;
  sit_in_overridden_by?: string | null;
  sit_in_override_reason?: string | null;
  open_schedule_issue_count?: number;
  critical_schedule_issue_count?: number;
  latest_session_change_id?: string | null;
  version: number;
  updated_at: string;
};

export const ABSENCE_STATUSES = ["pending", "reviewed", "actioned", "cancelled", "special_approved"] as const;
export type AbsenceStatus = (typeof ABSENCE_STATUSES)[number];

export type AbsenceSitInSession = {
  id: string;
  session_id: string;
  course_id: string;
  course_code: string;
  course_name: string;
  subject_name?: string | null;
  room_name?: string | null;
  start_at: string;
  end_at: string;
};

export type AbsenceTimelineEntry = {
  id: string;
  action: string;
  actor_id?: string | null;
  actor_name?: string | null;
  actor_role: "admin" | "student";
  details: Record<string, unknown>;
  created_at: string;
};

export type ManagedAbsence = Omit<StudentAbsence, "sit_ins"> & {
  missed_sessions?: AbsenceSitInSession[];
  sit_ins?: AbsenceSitInSession[];
  timeline?: AbsenceTimelineEntry[];
};

export type AbsencePage = {
  items: ManagedAbsence[];
  subjects?: Array<{ id: string; code: string; name: string }>;
  total_count: number;
  offset: number;
  limit: number;
  open_schedule_impact_count?: number;
  critical_schedule_impact_count?: number;
};

export type AbsenceStats = {
  total_count: number;
  pending_count: number;
  reviewed_count: number;
  actioned_count: number;
  cancelled_count: number;
  special_approved_count: number;
  today_count: number;
};

export type ReasonCategory = { value: string; label: string };

export type StaffCreateAbsenceRequest = {
  wcode: string;
  subject_id?: string;
  course_id?: string;
  date_from: string;
  date_to: string;
  missed_session_ids: string[];
  sit_in_method?: string;
  sit_in_course_id?: string;
  sit_in_session_ids: string[];
  reason?: string;
  reason_category?: string;
  status?: "pending" | "special_approved";
};

export type SmsPreview = {
  phones: string[];
  message: string;
};

export type AbsenceNotificationsSettings = {
  sms_parent_enabled: boolean;
  sms_parent_template: string;
  sms_success_template?: string;
  sms_special_approved_template?: string;
  email_success_enabled: boolean;
  email_success_subject: string;
  email_success_body: string;
};

export type AdminContactSettings = {
  email: string;
  phone: string;
  hours: string;
};

export type AbsenceSettings = {
  form: {
    max_date_range_days: number;
    min_hours_before_session: number;
    max_hours_after_session: number;
    require_reason: boolean;
    reason_categories: ReasonCategory[];
    allow_free_text_reason: boolean;
    intro_text: string;
    confirmation_text: string;
  };
  sit_in: {
    auto_resolve_enabled: boolean;
    zoom_description: string;
    max_sessions_per_absence: number;
  };
  notifications?: AbsenceNotificationsSettings;
  admin_contact?: AdminContactSettings;
  student_self_service?: {
    can_view_own: boolean;
    can_cancel_own: boolean;
  };
};

export type AbsenceFormConfig = {
  form: AbsenceSettings["form"];
  sit_in: AbsenceSettings["sit_in"];
  notifications?: AbsenceNotificationsSettings;
  admin_contact?: AdminContactSettings;
};

export type StudentLookupSubject = {
  id: string;
  code: string;
  name: string;
  active_course_id?: string | null;
  teacher_name?: string | null;
  merge_group_id?: string | null;
  merge_group_name?: string | null;
};

/** Legacy staff lookup shape. Public student lookup uses PublicStudentLookupResponse. */
export type StudentLookupResponse = {
  student_id: string;
  wcode: string;
  full_name: string;
  display_name?: string | null;
  nickname?: string | null;
  school?: string | null;
  email?: string | null;
  email_crm?: string | null;
  email_system?: string | null;
  parent_phone?: string | null;
  subjects: StudentLookupSubject[];
};

export type PublicStudentLookupResponse = {
  wcode: string;
  lookup_token: string;
  email_input_required: boolean;
  parent_verification_available: boolean;
  /** Masked server-side (e.g. "B***"); the raw name stays behind the OTP. */
  nickname_hint?: string;
  /** Masked server-side (e.g. "••••5678"); omitted when no phone exists. */
  parent_phone_hint?: string;
};

export type VerifiedStudentSubject = {
  id: string;
  code: string;
  name: string;
  /** Teacher of the subject's active class, shown as "Subject (Teacher)". */
  teacher_name?: string | null;
  /** Merged-course link: set when the subject's active classes all belong to
   *  the same merged course, so the form can offer that merged course as one
   *  entry spanning its source subjects. */
  merge_group_id?: string | null;
  merge_group_name?: string | null;
};

export type VerifiedStudentProfile = {
  wcode: string;
  display_name: string;
  email_on_file: boolean;
  /** Whether a nickname exists on file — never the value itself. */
  nickname_set?: boolean;
  subjects: VerifiedStudentSubject[];
};

export type ParentVerificationResponse = {
  token: string;
  status: "pending" | "verified" | "consumed";
  wcode: string;
  parent_phone?: string | null;
  otp_last_sent_at?: string | null;
  otp_code_expires_at?: string | null;
  verified_at?: string | null;
  consumed_at?: string | null;
  consumed_absence_id?: string | null;
  expires_at?: string | null;
  delivery_id?: string | null;
  delivery_status?:
    | "queued"
    | "preparing"
    | "submitting"
    | "accepted"
    | "retryable"
    | "failed"
    | "uncertain"
    | "expired"
    | null;
  delivery_retry_after_seconds?: number;
};

export type StaffAbsencePolicies = {
  notify_admin_on_teacher_absence: boolean;
  notify_substitute_teachers: boolean;
  auto_assign_cover_enabled: boolean;
  cover_threshold_days: number;
  default_cover_duration_minutes: number;
};

export type CalendarSitInStudent = {
  wcode: string;
  nickname?: string | null;
  student_name: string | null;
  absence_id: string;
  from_course_code: string;
  from_course_name: string | null;
};

export type CalendarSessionBrief = {
  id: string;
  course_id: string;
  course_code: string;
  course_name?: string;
  subject_name?: string | null;
  start_at: string;
  end_at: string;
  room_name?: string | null;
  teacher_name?: string;
  sit_in_students?: CalendarSitInStudent[];
};

export type CalendarAbsenceDay = {
  date: string;
  absences: CalendarAbsence[];
};

export type CalendarAbsence = {
  id: string;
  wcode: string;
  student_name: string | null;
  status: AbsenceStatus;
  subject_name?: string | null;
  subject_code?: string | null;
  date_from: string;
  date_to: string;
  sit_in_method: string | null;
  sit_in_course_code?: string | null;
  sit_in_course_name?: string | null;
  sit_in_subject_name?: string | null;
  missed_sessions?: CalendarSessionBrief[];
  sit_in_sessions?: CalendarSessionBrief[];
};

export type CalendarResponse = {
  sessions: CalendarSessionBrief[];
  absence_days: CalendarAbsenceDay[];
};

export type AbsenceTrends = {
  period: string;
  total_count: number;
  pending_count: number;
  reviewed_count: number;
  actioned_count: number;
  cancelled_count: number;
  prev_total_count: number;
  prev_pending_count: number;
  prev_reviewed_count: number;
  prev_actioned_count: number;
  prev_cancelled_count: number;
};

export type SessionInSubject = {
  id: string;
  start_at: string;
  end_at: string;
  date: string;
  already_absent: boolean;
};

export type SitInPriority = {
  level: number;
  label: string;
  sit_in_course?: {
    id: string;
    code: string;
    name: string;
    subject_code?: string | null;
    subject_name?: string | null;
    merge_group_id?: string | null;
    merge_group_name?: string | null;
  };
  available_sessions?: Array<{
    id: string;
    start_at: string;
    end_at: string;
    course_id?: string | null;
    missed_session_id?: string | null;
    class_name?: string | null;
    subject_name?: string | null;
    subject_code?: string | null;
    course_name?: string | null;
    course_code?: string | null;
    teacher_name?: string | null;
  }>;
  pre_selected?: Array<{ id: string; start_at: string; end_at: string; course_id?: string | null }>;
  unavailable_sessions?: Array<{
    session?: {
      id: string;
      start_at: string;
      end_at: string;
      missed_session_id?: string | null;
      class_name?: string | null;
      subject_name?: string | null;
      subject_code?: string | null;
      course_name?: string | null;
      course_code?: string | null;
      teacher_name?: string | null;
    } | null;
    reason: string;
    reason_code: string;
    missed_session_id?: string | null;
    occurrence_number?: number | null;
  }>;
};

export type SitInSessionInfo = {
  rule_name?: string;
  rule_type?: string;
  sit_in_method: "physical" | "zoom" | "teacher_case" | "none";
  priorities?: SitInPriority[];
  current_priority_level?: number;
  has_next_priority?: boolean;
  sit_in_course?: {
    id: string;
    code: string;
    name: string;
    subject_code?: string | null;
    subject_name?: string | null;
    merge_group_id?: string | null;
    merge_group_name?: string | null;
  };
  available_sessions?: Array<{
    id: string;
    start_at: string;
    end_at: string;
    course_id?: string | null;
    missed_session_id?: string | null;
    class_name?: string | null;
    subject_name?: string | null;
    subject_code?: string | null;
    course_name?: string | null;
    course_code?: string | null;
    teacher_name?: string | null;
  }>;
  missed_sessions?: Array<{ id: string; start_at: string; end_at: string }>;
  missed_occurrence_number?: number;
};

export type SitInInfo = SitInSessionInfo & {
  sit_in_by_missed_session?: Record<string, SitInSessionInfo>;
};

export type SubjectSessions = {
  subject_id: string;
  subject_code: string;
  subject_name: string;
  /** Teacher of this class, shown as "Subject (Teacher)" in the form. */
  teacher_name?: string | null;
  course_id: string;
  course_code: string;
  course_name: string;
  merge_group_id?: string | null;
  merge_group_name?: string | null;
  sessions: SessionInSubject[];
  sit_in?: SitInInfo;
  total_course_days?: number;
  used_absence_days?: number;
  maximum_absence_days?: number;
  remaining_absence_days?: number;
  absence_limit_reached?: boolean;
};

export type SessionsInRangeResponse = {
  subjects: SubjectSessions[];
};
