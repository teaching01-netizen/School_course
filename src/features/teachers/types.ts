export type TeacherDashboardSitInVisitor = {
  wcode: string;
  nickname: string | null;
  student_name: string | null;
  from_course_code: string;
  from_subject_name: string | null;
  absence_id: string;
  session_start_at: string;
  session_end_at: string;
  absent_subject_name: string | null;
  absence_date: string;
};

export type AbsentStudent = {
  wcode: string;
  nickname: string | null;
  student_name: string | null;
  absence_id: string;
  created_at: string | null;
};

export type TeacherDashboardSession = {
  id: string;
  course_id: string;
  course_code: string;
  course_name: string;
  subject_name: string | null;
  start_at: string;
  end_at: string;
  room_name: string | null;
  absent_count: number;
  absent_students: AbsentStudent[];
  sit_in_visitors: TeacherDashboardSitInVisitor[];
};

export type PendingAbsenceRequest = {
  id: string;
  wcode: string;
  student_name: string | null;
  nickname: string | null;
  course_code: string;
  course_name: string;
  subject_name: string | null;
  date_from: string;
  date_to: string;
  reason: string | null;
  reason_category: string | null;
  created_at: string;
};

export type TeacherDashboardResponse = {
  week_start: string;
  week_end: string;
  teacher: { id: string; username: string };
  sessions: TeacherDashboardSession[];
  summary: {
    total_sessions: number;
    total_absences: number;
    total_sit_ins: number;
  };
  pending_absence_requests: PendingAbsenceRequest[];
};

export type TeacherAbsenceSession = {
  session_id: string;
  course_code: string;
  course_name: string;
  subject_name: string | null;
  room_name: string | null;
  start_at: string;
  end_at: string;
};

export type TeacherAbsenceDetail = {
  id: string;
  wcode: string;
  student_name: string | null;
  student_nickname: string | null;
  course_code: string;
  course_name: string;
  subject_name: string | null;
  date_from: string;
  date_to: string;
  reason_category: string | null;
  reason: string | null;
  status: string;
  version: number;
  sit_in_method?: string | null;
  sit_in_course_id?: string | null;
  missed_sessions: TeacherAbsenceSession[];
  sit_in_sessions: TeacherAbsenceSession[];
};
