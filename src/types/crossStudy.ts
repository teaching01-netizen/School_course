export interface CourseRef {
  id: string;
  code: string;
  name: string;
  subject_name: string;
}

export interface CrmRowInfo {
  snapshot_id: string;
  course_name: string;
  course_id: string;
  extra_note: string;
  imported_at: string;
}

export interface StudentInfo {
  id: string;
  wcode: string;
  full_name: string;
}

export interface AssignmentDTO {
  id: string;
  dest_course_a: CourseRef | null;
  dest_course_b: CourseRef | null;
  assigned_course_id: string;
  status: string;
  extra_note_snapshot: string;
  source_valid: boolean;
  updated_at: string;
}

export interface StudentLookupResponse {
  student: StudentInfo;
  crm_row: CrmRowInfo | null;
  current_assignment: AssignmentDTO | null;
}

export interface AssignmentSummary {
  id: string;
  wcode: string;
  full_name: string;
  source_course_name: string;
  source_course_id: string;
  assigned_course_name: string;
  assigned_course_id: string;
  status: string;
  updated_at: string;
}

export interface AssignmentListResponse {
  assignments: AssignmentSummary[];
  total: number;
}

export interface SaveAssignmentInput {
  wcode: string;
  source_course_id: string;
  snapshot_id: string;
  dest_course_a_id: string;
  dest_course_b_id: string;
  assigned_course_id: string;
  extra_note_text: string;
}
