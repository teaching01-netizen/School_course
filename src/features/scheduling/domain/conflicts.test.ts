import { describe, expect, it } from "vitest";
import { conflictHeadline, conflictSharedResourceName } from "./conflicts";
import type { ConflictDetails } from "../types";
import type { Course } from "@/features/courses/types";
import type { User } from "@/types/shared";
import type { Room } from "@/features/scheduling/types";

const ROOMS = new Map<string, Room>([["room-1", { id: "room-1", name: "Room 101", capacity: 20 }]]);
const TEACHERS = new Map<string, User>([["teacher-1", { id: "teacher-1", username: "Teacher One", role: "Teacher" }]]);
const COURSES = new Map<string, Course>([["course-1", { id: "course-1", code: "MATH-101", name: "Math", version: 1, primary_teacher_id: "teacher-1" }]]);
const CTX = { roomsById: ROOMS, teachersById: TEACHERS, coursesById: COURSES };

function details(kind: string, overrides: Partial<ConflictDetails> = {}): ConflictDetails {
  return {
    kind,
    requested: {
      start_at: "2026-06-01T02:00:00Z",
      end_at: "2026-06-01T04:00:00Z",
      course_id: "course-1",
      room_id: "room-1",
      teacher_id: "teacher-1",
    },
    conflicts: [],
    ...overrides,
  };
}

describe("conflictHeadline", () => {
  it("names the room that is already booked", () => {
    expect(conflictHeadline(details("room_overlap"), CTX)).toBe("Room 101 is already booked at this time");
  });

  it("falls back to a generic room phrasing when the room is unknown", () => {
    expect(conflictHeadline(details("room_overlap"), {})).toBe("That room is already booked at this time");
  });

  it("names the teacher who is teaching another class", () => {
    expect(conflictHeadline(details("teacher_overlap"), CTX)).toBe("Teacher One is teaching another class at this time");
  });

  it("counts the students who would clash", () => {
    const base = details("student_overlap", {
      conflicts: [
        { session_id: "s1", course_id: "course-2", room_id: "room-1", teacher_id: "teacher-2", start_at: "2026-06-01T02:00:00Z", end_at: "2026-06-01T04:00:00Z" },
        { session_id: "s2", course_id: "course-2", room_id: "room-1", teacher_id: "teacher-2", start_at: "2026-06-02T02:00:00Z", end_at: "2026-06-02T04:00:00Z" },
      ],
      conflicting_students: [
        { student_id: "st1", full_name: "A Student", status: "confirmed" },
        { student_id: "st2", full_name: "B Student", status: "confirmed" },
        { student_id: "st3", full_name: "C Student", status: "draft" },
      ],
    });
    expect(conflictHeadline(base, CTX)).toBe("3 students would clash with another class");
    expect(conflictHeadline({ ...base, conflicting_students: [base.conflicting_students![0]] }, CTX)).toBe("1 student would clash with another class");
  });

  it("names the teacher when they are simply unavailable", () => {
    expect(conflictHeadline(details("teacher_availability"), CTX)).toBe("Teacher One isn't available at this time");
    expect(conflictHeadline(details("teacher_availability"), {})).toBe("Teacher isn't available at this time");
  });

  it("names the room when it is unavailable", () => {
    expect(conflictHeadline(details("room_availability"), CTX)).toBe("Room 101 isn't available at this time");
  });

  it("explains a teacher who is not assigned to the course", () => {
    expect(conflictHeadline(details("teacher_not_assigned_to_course"), CTX)).toBe("Teacher One isn't assigned to MATH-101");
  });

  it("explains a course without assigned teachers", () => {
    expect(conflictHeadline(details("course_has_no_assigned_teachers"), CTX)).toBe("This course has no teachers assigned");
  });

  it("names an inactive teacher", () => {
    expect(conflictHeadline(details("teacher_inactive"), CTX)).toBe("Teacher One is no longer active");
  });

  it("falls back to the kind label for unknown kinds", () => {
    expect(conflictHeadline(details("mystery_kind"), CTX)).toBe("mystery kind");
  });
});

describe("conflictSharedResourceName", () => {
  const item = { course_id: "course-1", room_id: "room-1", teacher_id: "teacher-1" };

  it("points at the room for room conflicts", () => {
    expect(conflictSharedResourceName("room_overlap", item, CTX)).toBe("Room 101");
  });

  it("points at the teacher for teacher conflicts", () => {
    expect(conflictSharedResourceName("teacher_overlap", item, CTX)).toBe("Teacher One");
    expect(conflictSharedResourceName("teacher_availability", item, CTX)).toBe("Teacher One");
  });

  it("returns null when there is no specific shared resource", () => {
    expect(conflictSharedResourceName("student_overlap", item, CTX)).toBeNull();
    expect(conflictSharedResourceName("course_has_no_assigned_teachers", item, CTX)).toBeNull();
  });

  it("returns null when the resource cannot be resolved", () => {
    expect(conflictSharedResourceName("room_overlap", item, {})).toBeNull();
  });
});