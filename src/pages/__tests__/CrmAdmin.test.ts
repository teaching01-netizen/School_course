import { describe, expect, it } from "vitest";
import { parseBusyRangeConflict } from "@/pages/CrmAdmin";

describe("CrmAdmin conflict parsing", () => {
  it("extracts student and course details from structured CRM conflict details", () => {
    const parsed = parseBusyRangeConflict("Student schedule conflict", {
      kind: "crm_student_schedule_conflict",
      student: { wcode: "W250001", full_name: "Jane Student" },
      target_course: { id: "course-1", code: "SAT", name: "SAT Math Course", subject_name: "SAT Math" },
      conflicts: [
        {
          course: { code: "ALG", name: "Algebra Course", subject_name: "Algebra" },
          start_at: "2026-05-20T10:00:00Z",
          end_at: "2026-05-20T11:00:00Z",
        },
      ],
      target_sessions: [
        {
          session_id: "session-1",
          start_at: "2026-05-20T10:00:00Z",
          end_at: "2026-05-20T11:00:00Z",
        },
      ],
    });

    expect(parsed?.studentWCode).toBe("W250001");
    expect(parsed?.studentName).toBe("Jane Student");
    expect(parsed?.targetCourse).toBe("SAT Math");
    expect(parsed?.targetCourseID).toBe("course-1");
    expect(parsed?.conflictingCourse).toBe("Algebra");
    expect(parsed?.conflictTime).toBe("20 May, 17:00-18:00");
    expect(parsed?.targetSessions?.[0]?.session_id).toBe("session-1");
    expect(parsed?.detail).toBe(
      "Student schedule conflict: Jane Student (W250001) cannot be added to SAT Math because they already have Algebra at 20 May, 17:00-18:00",
    );
  });

  it("falls back to course name and then code when subject names are unavailable", () => {
    const parsed = parseBusyRangeConflict("Student schedule conflict", {
      kind: "crm_student_schedule_conflict",
      student: { wcode: "W250003", full_name: "Fallback Student" },
      target_course: { code: "SAT", name: "SAT Math Course" },
      conflicts: [
        {
          course: { code: "ALG" },
          start_at: "2026-05-20T10:00:00Z",
          end_at: "2026-05-20T11:00:00Z",
        },
      ],
    });

    expect(parsed?.targetCourse).toBe("SAT Math Course");
    expect(parsed?.conflictingCourse).toBe("ALG");
  });

  it("extracts nested structured CRM conflict details", () => {
    const parsed = parseBusyRangeConflict("Student schedule conflict", {
      details: {
        kind: "crm_student_schedule_conflict",
        student: { wcode: "W250002", full_name: "Nested Student" },
        target_course: { code: "BIO" },
        conflicts: [],
      },
    });

    expect(parsed?.studentWCode).toBe("W250002");
    expect(parsed?.targetCourse).toBe("BIO");
  });
});
