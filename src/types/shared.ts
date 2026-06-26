export type User = { id: string; username: string; role: "Admin" | "Teacher" };

export type Student = {
  id: string;
  wcode: string;
  full_name: string;
  notes: string;
  status?: string;
  student_phone?: string | null;
  parent_phone?: string | null;
};

export interface Teacher {
  id: string;
  name: string;
  username: string;
  email: string;
  status: "active" | "inactive";
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
  type: "physical" | "online";
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
  status: "present" | "absent" | "pending";
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
  status: "confirmed" | "pending" | "conflict";
  studentCount: number;
  type: "General" | "Private";
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
