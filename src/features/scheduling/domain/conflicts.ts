import type { Course } from "@/features/courses/types";
import type { User } from "@/types/shared";
import type { RequestedSessionInfo } from "../types";

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

export function getRequestedLabel(
  requested: RequestedSessionInfo,
  coursesById: Map<string, Course>,
  teachersById: Map<string, User>,
): string {
  const course = coursesById.get(requested.course_id);
  const teacher = teachersById.get(requested.teacher_id);
  const courseStr = course ? `${course.code}` : requested.course_id.slice(0, 8) + "…";
  const teacherStr = teacher ? teacher.username : requested.teacher_id.slice(0, 8) + "…";
  return `${teacherStr} – ${courseStr}`;
}
