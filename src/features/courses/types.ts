export type CourseTeacher = {
  id: string;
  username: string;
  is_primary: boolean;
};

export type EditableTeacher = {
  teacher_id: string;
  is_primary: boolean;
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
  course_type?: string | null;
  deleted_at?: string | null;
  legacy_course_id?: string | null;
  legacy_last_synced_at?: string | null;
  teachers?: CourseTeacher[];
};
