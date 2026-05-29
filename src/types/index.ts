import { zoneLocalInputToUTCISO } from "@/utils/timezone";

// === Sit-in Rule types ===

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

export type SitInRuleListResponse = {
  sit_in_rules: SitInRule[];
};

export type SitInRuleCreateInput = {
  name: string;
  type: SitInRuleType;
  predicate: Record<string, unknown>;
  description: string;
};

// === Canonical API types ===

export type Session = {
  id: string;
  series_id?: string | null;
  course_id: string;
  room_id: string | null;
  teacher_id: string;
  start_at: string;
  end_at: string;
  version: number;
};

export type Course = { id: string; code: string; name: string; deleted_at?: string | null };
export type Room = { id: string; name: string; capacity: number | null };
export type User = { id: string; username: string; role: "Admin" | "Teacher" };
export type Student = { id: string; wcode: string; full_name: string; notes: string; status?: string };
export type AttendanceOverride = { student_id: string; status: "included" | "excluded"; created_at: string };

export type ConflictDetails = {
  kind: string;
  requested: {
    start_at: string;
    end_at: string;
    course_id: string;
    room_id: string | null;
    teacher_id: string;
    series_id?: string | null;
  };
  conflicts: Array<{
    session_id: string;
    series_id?: string | null;
    course_id: string;
    room_id: string | null;
    teacher_id: string;
    start_at: string;
    end_at: string;
  }> | null;
  conflicting_students?: Array<{
    student_id: string;
    full_name: string;
    status: string;
  }>;
};

export type StaleEditDetails = {
  current?: {
    id: string;
    course_id: string;
    room_id: string | null;
    teacher_id: string;
    weekdays: number[];
    start_local_time?: string;
    duration_minutes: number;
    start_date: string;
    end_date: string;
    count: number | null;
    version: number;
  };
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
  created_at: string;
  subject_id?: string | null;
  subject_code?: string | null;
  subject_name?: string | null;
  sit_in_method?: string | null;
  sit_ins?: Array<{ id: string; session_id: string }>;
  student_name?: string | null;
  student_email?: string | null;
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
  version: number;
  updated_at: string;
};

export type AbsenceStatus = "pending" | "reviewed" | "actioned" | "cancelled";

export type AbsenceSitInSession = {
  id: string;
  session_id: string;
  course_id: string;
  course_code: string;
  course_name: string;
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
  sit_ins?: AbsenceSitInSession[];
  timeline?: AbsenceTimelineEntry[];
};

export type AbsencePage = {
  items: ManagedAbsence[];
  subjects?: Array<{ id: string; code: string; name: string }>;
  total_count: number;
  offset: number;
  limit: number;
};

export type AbsenceStats = {
  total_count: number;
  pending_count: number;
  reviewed_count: number;
  actioned_count: number;
  cancelled_count: number;
  today_count: number;
};

export type ReasonCategory = { value: string; label: string };

export type AbsenceSettings = {
  form: {
    max_date_range_days: number;
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
  student_self_service?: {
    can_view_own: boolean;
    can_cancel_own: boolean;
  };
};

export type StaffAbsencePolicies = {
  notify_admin_on_teacher_absence: boolean;
  notify_substitute_teachers: boolean;
  auto_assign_cover_enabled: boolean;
  cover_threshold_days: number;
  default_cover_duration_minutes: number;
};

export type RequestedSessionInfo = { course_id: string; teacher_id: string };

export type CalendarSessionBrief = {
  id: string;
  course_id: string;
  course_code: string;
  course_name?: string;
  start_at: string;
  end_at: string;
  room_name?: string | null;
  teacher_name?: string;
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
  subject_code?: string | null;
  date_from: string;
  date_to: string;
  sit_in_method: string | null;
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

// === Legacy types kept for other pages ===

export interface Teacher {
  id: string;
  name: string;
  username: string;
  email: string;
  status: 'active' | 'inactive';
}

export interface Subject {
  id: string;
  name: string;
  code: string;
}

export type SubjectWithActiveCourse = Subject & {
  active_course_id?: string | null;
  active_course_code?: string | null;
  active_cycle_label?: string | null;
};

export type ActiveCourseSubject = {
  subject_id: string;
  subject_code: string;
  subject_name: string;
  courses: Array<{
    course_id: string;
    course_code: string;
    course_name: string;
    cycle_id: string;
    cycle_label: string;
    is_active: boolean;
  }>;
};

export type ActiveCoursePayload = {
  subject_id: string;
  course_id: string;
};

export interface Classroom {
  id: string;
  name: string;
  location: string;
  capacity: number;
  type: 'physical' | 'online';
}

export interface Attendee {
  pcode: string;
  altCode: string;
  wcode: string;
  name: string;
  nickname: string;
  school: string;
  enrolled: string;
}

export interface AttendanceRecord {
  wcode: string;
  name: string;
  school: string;
  status: 'present' | 'absent' | 'pending';
}

export interface ScheduleItem {
  courseId: string;
  subject: string;
  teacher: string;
  teacherId: string;
  timeFrom: string;
  timeTo: string;
  duration: string;
  room: string;
  roomId: string;
  status: 'confirmed' | 'pending' | 'conflict';
  studentCount: number;
  type: 'General' | 'Private';
}

export interface DailySchedule {
  date: string;
  rooms: {
    roomId: string;
    roomName: string;
    items: ScheduleItem[];
  }[];
  unassigned: ScheduleItem[];
}

export type ToastType = 'success' | 'warning' | 'error' | 'info';

export interface ToastMessage {
  id: string;
  type: ToastType;
  message: string;
}

// === Shared helper functions ===

export function conflictKindLabel(kind: string): { label: string; detail: string } {
  switch (kind) {
    case "room_overlap":
      return { label: "Room already booked", detail: "The requested room is occupied" };
    case "teacher_overlap":
      return { label: "Teacher has another session", detail: "Teacher is busy with another class" };
    case "student_overlap":
      return { label: "Student scheduling conflict", detail: "One or more students have a scheduling clash" };
    case "teacher_availability":
      return { label: "Teacher not available", detail: "Teacher is not available at this time" };
    case "room_availability":
      return { label: "Room not available", detail: "Room is not available at this time" };
    default:
      return { label: kind.replace(/_/g, " "), detail: "" };
  }
}

export function yyyyMmDd(d: Date) {
  return d.toISOString().slice(0, 10);
}

export function formatTimeRange(startAt: string, endAt: string): string {
  try {
    const start = new Date(startAt);
    const end = new Date(endAt);
    const dateStr = start.toLocaleDateString("en-GB", { day: "numeric", month: "short" });
    const startTime = start.toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit" });
    const endTime = end.toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit" });
    return `${dateStr}, ${startTime}–${endTime}`;
  } catch {
    return `${startAt} → ${endAt}`;
  }
}

export function minutesBetween(startUTCISO: string, endUTCISO: string): number | null {
  const s = new Date(startUTCISO);
  const e = new Date(endUTCISO);
  if (Number.isNaN(s.getTime()) || Number.isNaN(e.getTime())) return null;
  return Math.round((e.getTime() - s.getTime()) / 60000);
}

export function fmtDuration(mins: number): string {
  const h = Math.floor(mins / 60);
  const m = mins % 60;
  return `${String(h).padStart(2, "0")}:${String(m).padStart(2, "0")}`;
}

export function localDateTimeToUTCISO(local: string, zone: string): string | null {
  if (!local) return null;
  return zoneLocalInputToUTCISO(local, zone);
}

export function getRequestedLabel(requested: RequestedSessionInfo, coursesById: Map<string, Course>, teachersById: Map<string, User>): string {
  const course = coursesById.get(requested.course_id);
  const teacher = teachersById.get(requested.teacher_id);
  const courseStr = course ? `${course.code}` : requested.course_id.slice(0, 8) + "…";
  const teacherStr = teacher ? teacher.username : requested.teacher_id.slice(0, 8) + "…";
  return `${teacherStr} – ${courseStr}`;
}
