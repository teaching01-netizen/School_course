import { describe, expect, it } from "vitest";
import { parseBusyRangeConflict } from "@/pages/CrmAdmin";

describe("CrmAdmin conflict parsing", () => {
  it("extracts student and course details from structured CRM conflict details", () => {
    const parsed = parseBusyRangeConflict("Student schedule conflict", {
      kind: "crm_student_schedule_conflict",
      student: { wcode: "W250001", full_name: "Jane Student" },
      target_course: { code: "SAT", name: "SAT Math" },
      conflicts: [
        {
          course: { code: "ALG", name: "Algebra" },
          start_at: "2026-05-20T10:00:00Z",
          end_at: "2026-05-20T11:00:00Z",
        },
      ],
    });

    expect(parsed?.studentWCode).toBe("W250001");
    expect(parsed?.studentName).toBe("Jane Student");
    expect(parsed?.targetCourse).toBe("SAT · SAT Math");
    expect(parsed?.conflictingCourse).toBe("ALG · Algebra");
    expect(parsed?.conflictTime).toContain("20 May");
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
