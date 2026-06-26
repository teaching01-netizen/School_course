import { apiJson } from "@/api/client";
import type { Room, Session } from "@/features/scheduling/types";
import type { Student, User } from "@/types/shared";
import type { Course } from "../types";

export type CourseCrmFilter = {
  enabled: boolean;
  locked: boolean;
  filter: unknown;
};

export type InstituteTimeMeta = {
  institute_tz: string;
  server_now: string;
};

export function getCourse(courseId: string): Promise<Course> {
  return apiJson<Course>(`/api/v1/courses/${courseId}`, { method: "GET" });
}

export function updateCourse(courseId: string, body: unknown): Promise<Course> {
  return apiJson<Course>(`/api/v1/courses/${courseId}`, {
    method: "PUT",
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

export function getInstituteTimeMeta(): Promise<InstituteTimeMeta> {
  return apiJson<InstituteTimeMeta>("/api/v1/meta/time", { method: "GET" });
}

export function getStudentByWcode(wcode: string): Promise<Student> {
  return apiJson<Student>(`/api/v1/students/${encodeURIComponent(wcode)}`, { method: "GET" });
}

export function syncLegacyCourse(courseId: string): Promise<void> {
  return apiJson(`/api/v1/courses/${courseId}/legacy-sync`, { method: "POST" });
}
