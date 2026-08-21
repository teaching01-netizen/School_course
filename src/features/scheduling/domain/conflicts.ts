import type { Course } from "@/features/courses/types";
import type { User } from "@/types/shared";
import type { ConflictDetails, RequestedSessionInfo, Room } from "../types";

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

export type ConflictDisplayCtx = {
  coursesById?: Map<string, Course>;
  teachersById?: Map<string, User>;
  roomsById?: Map<string, Room>;
};

/**
 * The one-line "what went wrong" for a blocked preflight. Instead of a generic
 * kind label ("Room already booked") the headline names the actual thing the
 * user cares about: the room, the teacher, or the students involved.
 */
export function conflictHeadline(details: ConflictDetails, ctx: ConflictDisplayCtx = {}): string {
  const { kind, requested } = details;
  const room = requested.room_id ? ctx.roomsById?.get(requested.room_id)?.name ?? null : null;
  const requestedTeacher = ctx.teachersById?.get(requested.teacher_id);
  const teacher = requestedTeacher ? (requestedTeacher.full_name || requestedTeacher.username) : null;
  const course = ctx.coursesById?.get(requested.course_id)?.code ?? null;
  switch (kind) {
    case "room_overlap":
      return room ? `${room} is already booked at this time` : "That room is already booked at this time";
    case "teacher_overlap":
      return teacher ? `${teacher} is teaching another class at this time` : "The teacher has another class at this time";
    case "student_overlap": {
      const n = details.student_count ?? details.conflicting_students?.length ?? details.conflicts?.length ?? 0;
      return n > 0 ? `${n} student${n === 1 ? "" : "s"} would clash with another class` : "Students would clash with another class";
    }
    case "teacher_availability":
      return teacher ? `${teacher} isn't available at this time` : "Teacher isn't available at this time";
    case "room_availability":
      return room ? `${room} isn't available at this time` : "Room isn't available at this time";
    case "teacher_not_assigned_to_course":
      return teacher && course ? `${teacher} isn't assigned to ${course}` : "Teacher isn't assigned to this course";
    case "course_has_no_assigned_teachers":
      return "This course has no teachers assigned";
    case "course_not_found":
      return "Course not found";
    case "teacher_not_found":
      return "Teacher not found";
    case "teacher_inactive":
      return teacher ? `${teacher} is no longer active` : "Teacher is no longer active";
    case "room_not_found":
      return "Room not found or inactive";
    default:
      return conflictKindLabel(kind).label;
  }
}

/**
 * The single resource two sessions share when they clash (the room for
 * room_overlap, the teacher for teacher_overlap...). Used to emphasize what
 * actually collides in each conflicting-session row.
 */
export function conflictSharedResourceName(
  kind: string,
  item: { course_id: string; teacher_id: string; room_id: string | null },
  ctx: ConflictDisplayCtx = {},
): string | null {
  switch (kind) {
    case "room_overlap":
    case "room_availability":
      return item.room_id ? ctx.roomsById?.get(item.room_id)?.name ?? null : null;
    case "teacher_overlap":
    case "teacher_availability":
    case "teacher_not_assigned_to_course": {
      const teacher = ctx.teachersById?.get(item.teacher_id);
      return teacher ? (teacher.full_name || teacher.username) : null;
    }
    default:
      return null;
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
  const teacherStr = teacher ? (teacher.full_name || teacher.username) : requested.teacher_id.slice(0, 8) + "…";
  return `${teacherStr} – ${courseStr}`;
}
