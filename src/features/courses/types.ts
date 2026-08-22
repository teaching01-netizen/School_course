export type CourseTeacher = {
  id: string;
  username: string;
  full_name?: string | null;
  is_primary: boolean;
};

export type EditableTeacher = {
  teacher_id: string;
  is_primary: boolean;
};

/** The per-field change set a single course save persists. Absent keys keep
 *  the current value; a null value clears the property (not offered by every
 *  editor). Absence-form visibility is intentionally absent: it is managed
 *  only in Operations → Active courses. */
export type CourseEditChanges = {
  name?: string;
  teachers?: EditableTeacher[];
  subject_id?: string | null;
  course_type?: string | null;
  year?: number | null;
  hour?: number | null;
  student_count?: number | null;
  cycle_id?: string | null;
  expiry_days?: number | null;
};

export type Course = {
  id: string;
  version: number;
  code: string;
  name: string;
  primary_teacher_id: string | null;
  course_no?: number | null;
  year?: number | null;
  teacher_id?: string | null;
  teacher_name?: string | null;
  subject_id?: string | null;
  subject_code?: string | null;
  subject_name?: string | null;
  hour?: number | null;
  student_count?: number | null;
  cycle_id?: string | null;
  cycle_label?: string | null;
  expiry_days?: number | null;
  absence_form_visible?: boolean;
  /** True when this course is one of its subject's active classes (open in
   *  the student absence form and eligible for sit-ins). Managed only in
   *  Operations; a subject may have several active classes at once. */
  is_active_course?: boolean;
  last_session_at?: string | null;
  expires_at?: string | null;
  expiry_status?: "active" | "expired" | "not_configured";
  course_type?: string | null;
  deleted_at?: string | null;
  legacy_course_id?: string | null;
  legacy_last_synced_at?: string | null;
  teachers?: CourseTeacher[];
};

export type LegacyCourseConflict = {
  id: string;
  conflict_type: string;
  category: string;
  message: string | null;
  source_payload: Record<string, unknown> | null;
  local_payload: Record<string, unknown> | null;
  created_at: string;
};

export type CourseMergeCandidate = {
  id: string;
  code: string;
  name: string;
  subject_code: string;
  subject_name: string;
  teacher_name: string;
  legacy_course_id?: string | null;
};

export type CourseGroupTeacher = {
  id: string;
  username: string;
  full_name: string | null;
  course_ids: string[];
  course_codes: string[];
};

export type CourseGroupMember = CourseMergeCandidate & {
  year: number | null;
  hour: number | null;
  student_count: number | null;
  course_type: string | null;
  cycle_id: string | null;
  root_course_group_id: string | null;
  legacy_archived: boolean;
  teachers: CourseTeacher[];
};

export type CourseGroup = {
  id: string;
  name: string;
  members: CourseGroupMember[];
  teachers: CourseGroupTeacher[];
};

export type CourseGroupSummary = {
  id: string;
  name: string;
  member_count: number;
  course_codes: string[];
};
