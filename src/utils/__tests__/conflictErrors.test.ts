import { describe, expect, it } from "vitest";
import { ApiRequestError } from "@/api/client";
import { formatConflictToastMessage, getConflictDetails, isConflictDetails } from "@/utils/conflictErrors";
import type { ConflictDetails } from "@/types";

const conflictDetails: ConflictDetails = {
  kind: "student_overlap",
  requested: {
    start_at: "2026-05-20T10:30:00Z",
    end_at: "2026-05-20T11:30:00Z",
    course_id: "course-a",
    room_id: null,
    teacher_id: "teacher-a",
  },
  conflicts: [
    {
      session_id: "session-b",
      course_id: "course-b",
      room_id: null,
      teacher_id: "teacher-b",
      start_at: "2026-05-20T10:00:00Z",
      end_at: "2026-05-20T11:00:00Z",
    },
  ],
  conflicting_students: [{ student_id: "student-1", full_name: "Preflight Student", status: "enrolled" }],
};

describe("conflictErrors", () => {
  it("parses conflict details from ApiRequestError.details", () => {
    const err = new ApiRequestError("Schedule conflict", { status: 409, code: "schedule_conflict" });
    err.details = conflictDetails;

    expect(isConflictDetails(conflictDetails)).toBe(true);
    expect(getConflictDetails(err)).toEqual(conflictDetails);
  });

  it("rejects malformed details and falls back to the error message", () => {
    const err = new ApiRequestError("Bad request", { status: 400, code: "bad_request" });
    err.details = { kind: "student_overlap" };

    expect(isConflictDetails(err.details)).toBe(false);
    expect(getConflictDetails(err)).toBeNull();
    expect(formatConflictToastMessage(err, "Fallback")).toBe("Bad request");
  });

  it("formats student conflict messages with affected names", () => {
    const err = new ApiRequestError("Schedule conflict", { status: 409, code: "schedule_conflict" });
    err.details = conflictDetails;

    expect(formatConflictToastMessage(err, "Fallback")).toContain("Student scheduling conflict: Preflight Student");
  });
});
