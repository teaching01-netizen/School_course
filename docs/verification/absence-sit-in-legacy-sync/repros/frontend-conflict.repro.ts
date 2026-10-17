import { describe, it, expect } from "vitest";
import { findSitInSessionConflicts } from "@/features/absences/domain/sitInResolution";
import { buildSubmissionPayloads } from "@/features/absences/domain/submissionPayload";
import type { SubjectSessions } from "@/features/absences/types";

const group = (id: string, makeUp: string): SubjectSessions => ({
  subject_id: id, subject_code: id, subject_name: id, course_id: id, course_code: id, course_name: id,
  total_course_days: 10, used_absence_days: 0, absence_limit_reached: false,
  sessions: [{ id: `missed-${id}`, start_at: "2026-10-10T03:00:00Z", end_at: "2026-10-10T04:00:00Z", date: "2026-10-10", already_absent: false }],
  sit_in: { sit_in_method: "physical", sit_in_course: { id: `target-${id}`, code: id, name: id }, available_sessions: [{ id: makeUp, course_id: `target-${id}`, start_at: "2026-10-13T06:00:00Z", end_at: "2026-10-13T07:00:00Z" }] },
});

describe("policy audit safety assertions", () => {
  it("checks an expected class even when its subject is selected", () => {
    const source = group("selected", "candidate");
    source.sessions[0] = { ...source.sessions[0], start_at: "2026-10-13T06:00:00Z", end_at: "2026-10-13T07:00:00Z", date: "2026-10-13" };
    expect(findSitInSessionConflicts(source.sit_in!.available_sessions!, [source], ["selected"])).toHaveLength(1);
  });

  it("rejects two distinct make-up IDs that overlap each other", () => {
    const result = buildSubmissionPayloads({ lookupWcode: "audit", sessions: [group("a", "makeup-a"), group("b", "makeup-b")], selectedSubjectIds: ["a", "b"], selectedSessionIds: new Set(["missed-a", "missed-b"]), sitInSelections: { "missed-a": "makeup-a", "missed-b": "makeup-b" }, reason: "audit", maxDateRangeDays: 30 });
    expect(result.ok).toBe(false);
  });
});
