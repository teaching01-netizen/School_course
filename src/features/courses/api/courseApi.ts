import { apiJson } from "@/api/client";
import type { Room, Session } from "@/features/scheduling/types";
import type { Student, User, Subject } from "@/types/shared";
import type { Course, CourseGroup, CourseMergeCandidate, EditableTeacher, LegacyCourseConflict } from "../types";

export type CourseCrmFilter = {
  enabled: boolean;
  locked: boolean;
  filter: unknown;
};

export type InstituteTimeMeta = {
  institute_tz: string;
  server_now: string;
};

export type CourseCycle = {
  id: string;
  label: string;
  display_name?: string | null;
  start_date?: string | null;
  end_date?: string | null;
};

export function getCourseCycles(): Promise<CourseCycle[]> {
  return apiJson<CourseCycle[]>("/api/v1/crm/cycles", { method: "GET" });
}

export function getCourse(courseId: string): Promise<Course> {
  return apiJson<Course>(`/api/v1/courses/${courseId}`, { method: "GET" });
}

export function updateCourse(courseId: string, body: unknown): Promise<Course> {
  return apiJson<Course>(`/api/v1/courses/${courseId}`, {
    method: "PUT",
    body: JSON.stringify(body),
  });
}

export function patchCourse(
  courseId: string,
  body: {
    expected_version: number;
    code: string;
    name: string;
    legacy_course_id?: string | null;
    teachers: EditableTeacher[];
    subject_id?: string | null;
    course_type?: string | null;
    year?: number | null;
    hour?: number | null;
    student_count?: number | null;
    cycle_id?: string | null;
    expiry_days?: number | null;
  },
): Promise<Course> {
  return apiJson<Course>(`/api/v1/courses/${courseId}`, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
}

export function deleteCourse(courseId: string): Promise<void> {
  return apiJson(`/api/v1/courses/${courseId}`, { method: "DELETE" });
}

export function getCourseCrmFilter(courseId: string): Promise<CourseCrmFilter> {
  return apiJson<CourseCrmFilter>(`/api/v1/courses/${courseId}/crm-filter`, { method: "GET" });
}

export function getCourseStudents(courseId: string): Promise<Student[]> {
  return apiJson<Student[]>(`/api/v1/courses/${courseId}/students`, { method: "GET" });
}

export function addCourseStudent(courseId: string, studentId: string): Promise<void> {
  return apiJson(`/api/v1/courses/${courseId}/students`, {
    method: "POST",
    body: JSON.stringify({ student_id: studentId }),
  });
}

export function removeCourseStudent(courseId: string, studentId: string): Promise<void> {
  return apiJson(`/api/v1/courses/${courseId}/students/${studentId}`, { method: "DELETE" });
}

export function getCourseSessions(courseId: string): Promise<Session[]> {
  return apiJson<Session[]>(`/api/v1/courses/${courseId}/sessions`, { method: "GET" });
}

export function getRooms(): Promise<Room[]> {
  return apiJson<Room[]>("/api/v1/rooms", { method: "GET" });
}

export function getTeacherUsers(): Promise<User[]> {
  return apiJson<User[]>("/api/v1/users?role=Teacher", { method: "GET" });
}

export function getSubjects(): Promise<Subject[]> {
  return apiJson<Subject[]>("/api/v1/subjects", { method: "GET" });
}

export function getInstituteTimeMeta(): Promise<InstituteTimeMeta> {
  return apiJson<InstituteTimeMeta>("/api/v1/meta/time", { method: "GET" });
}

export function getStudentByWcode(wcode: string): Promise<Student> {
  return apiJson<Student>(`/api/v1/students/${encodeURIComponent(wcode)}`, { method: "GET" });
}

export function syncLegacyCourse(courseId: string): Promise<void> {
  return apiJson(`/api/v1/courses/${courseId}/legacy-sync`, { method: "POST" });
}

export type LegacyConflictsResponse = {
  course_id: string;
  legacy_course_id: string | null;
  open_conflicts: LegacyCourseConflict[];
};

export function getCourseLegacyConflicts(courseId: string): Promise<LegacyConflictsResponse> {
  return apiJson<LegacyConflictsResponse>(`/api/v1/courses/${courseId}/legacy-conflicts`);
}

export type CourseGroupSessions = {
  id: string;
  course_id: string;
  course_code: string;
  course_name: string;
  room_id: string | null;
  teacher_id: string;
  teacher_name: string;
  start_at: string;
  end_at: string;
  version: number;
};

export function createCourseGroup(body: { name: string; course_ids: string[] }): Promise<{ id: string; name: string; course_ids: string[] }> {
  return apiJson<{ id: string; name: string; course_ids: string[] }>("/api/v1/course-groups", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function getCourseGroup(groupId: string): Promise<CourseGroup> {
  return apiJson<CourseGroup>(`/api/v1/course-groups/${groupId}`, { method: "GET" });
}

export function updateCourseGroup(groupId: string, body: { name: string }): Promise<{ id: string; name: string }> {
  return apiJson<{ id: string; name: string }>(`/api/v1/course-groups/${groupId}`, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
}

export function deleteCourseGroup(groupId: string): Promise<void> {
  return apiJson(`/api/v1/course-groups/${groupId}`, { method: "DELETE" });
}

export function getCourseGroupSessions(groupId: string): Promise<CourseGroupSessions[]> {
  return apiJson<CourseGroupSessions[]>(`/api/v1/course-groups/${groupId}/sessions`, { method: "GET" });
}

export function getCourseMergeCandidates(): Promise<{ items: CourseMergeCandidate[] }> {
  return apiJson<{ items: CourseMergeCandidate[] }>("/api/v1/courses?limit=200&offset=0", { method: "GET" });
}
