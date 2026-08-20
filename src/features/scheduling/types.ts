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

export type Room = { id: string; name: string; capacity: number | null };

export type AttendanceOverride = {
  student_id: string;
  status: "included" | "excluded";
  created_at: string;
};

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
  total_conflicts?: number;
  conflicts_truncated?: boolean;
  conflicting_students?: Array<{
    student_id: string;
    full_name: string;
    status: string;
  }>;
  /** Number of students who would clash; travels with a conflict that is
   *  carried across pages (the student list itself is not serialized). */
  student_count?: number;
  resource?: string;
  session_ids?: string[];
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

export type RequestedSessionInfo = { course_id: string; teacher_id: string };
