import { describe, expect, it } from "vitest";
import { formatBatchAbsenceSummary, getAbsenceSessionDateLabels, formatBatchSitInSummary } from "../resultSummaries";
import type { ManagedAbsence } from "../../types";
import type { AbsenceSitInSession } from "../../types";

const baseAbsence = {
  id: "a1",
  wcode: "W250389",
  course_id: "c1",
  created_at: "2026-06-01T00:00:00Z",
  status: "pending" as const,
  version: 1,
  updated_at: "2026-06-01T00:00:00Z",
} as ManagedAbsence;

describe("formatBatchAbsenceSummary", () => {
  it("shows className with dates when both present", () => {
    const absence: ManagedAbsence = {
      ...baseAbsence,
      subject_name: "Mathematics",
      missed_sessions: [
        { id: "ms1", start_at: "2026-06-01T09:00:00+07:00", end_at: "2026-06-01T10:00:00+07:00", session_id: "s1", course_id: "c1", course_code: "MATH101", course_name: "Math 101" } as AbsenceSitInSession,
      ],
    };
    expect(formatBatchAbsenceSummary(absence)).toContain("Mathematics");
  });

  it("returns className only when no dates", () => {
    const absence: ManagedAbsence = { ...baseAbsence, subject_name: "Mathematics", missed_sessions: [] };
    expect(formatBatchAbsenceSummary(absence)).toBe("Mathematics");
  });

  it("returns dates only when no className", () => {
    const absence: ManagedAbsence = {
      ...baseAbsence,
      missed_sessions: [
        { id: "ms1", start_at: "2026-06-01T09:00:00+07:00", end_at: "2026-06-01T10:00:00+07:00", session_id: "s1", course_id: "c1", course_code: "MATH101", course_name: "Math 101" } as AbsenceSitInSession,
      ],
    };
    const result = formatBatchAbsenceSummary(absence);
    expect(result).not.toContain("Mathematics");
    expect(result).toBeTruthy();
  });

  it("returns 'To arrange' when neither className nor dates", () => {
    const absence: ManagedAbsence = { ...baseAbsence };
    expect(formatBatchAbsenceSummary(absence)).toBe("To arrange");
  });
});

describe("getAbsenceSessionDateLabels", () => {
  it("returns formatted single date from missed_sessions", () => {
    const absence: ManagedAbsence = {
      ...baseAbsence,
      missed_sessions: [
        { id: "ms1", start_at: "2026-06-01T09:00:00+07:00", end_at: "2026-06-01T10:00:00+07:00", session_id: "s1", course_id: "c1", course_code: "MATH101", course_name: "Math 101" } as AbsenceSitInSession,
      ],
    };
    expect(getAbsenceSessionDateLabels(absence)).toBeTruthy();
  });

  it("falls back to date_from/date_to range when no missed_sessions", () => {
    const absence: ManagedAbsence = { ...baseAbsence, date_from: "2026-06-01", date_to: "2026-06-05" };
    const labels = getAbsenceSessionDateLabels(absence);
    expect(labels).toContain(" - ");
  });

  it("returns single date when date_from equals date_to", () => {
    const absence: ManagedAbsence = { ...baseAbsence, date_from: "2026-06-01", date_to: "2026-06-01" };
    const labels = getAbsenceSessionDateLabels(absence);
    expect(labels).not.toContain(" - ");
    expect(labels).toBeTruthy();
  });

  it("returns empty string when nothing available", () => {
    const absence: ManagedAbsence = { ...baseAbsence, date_from: undefined, date_to: undefined } as unknown as ManagedAbsence;
    expect(getAbsenceSessionDateLabels(absence)).toBe("");
  });
});

describe("formatBatchSitInSummary", () => {
  it("returns 'Zoom' for zoom method", () => {
    const absence: ManagedAbsence = { ...baseAbsence, sit_in_method: "zoom" };
    expect(formatBatchSitInSummary(absence)).toBe("Zoom");
  });

  it("returns 'To arrange' for non-physical with no session labels", () => {
    const absence: ManagedAbsence = { ...baseAbsence, sit_in_method: "teacher_case" };
    expect(formatBatchSitInSummary(absence)).toBe("To arrange");
  });

  it("returns className with labels for physical method with sessions", () => {
    const absence: ManagedAbsence = {
      ...baseAbsence,
      sit_in_method: "physical",
      sit_in_subject_name: "Calculus",
      sit_ins: [
        { id: "sis1", start_at: "2026-06-03T09:00:00+07:00", end_at: "2026-06-03T10:00:00+07:00", session_id: "s1", course_id: "c1", course_code: "MATH301", course_name: "Calc III" } as AbsenceSitInSession,
      ],
    };
    const result = formatBatchSitInSummary(absence);
    expect(result).toContain("Calculus");
    expect(result).not.toContain("To arrange");
  });

  it("returns className only for physical with no session times", () => {
    const absence: ManagedAbsence = {
      ...baseAbsence,
      sit_in_method: "physical",
      sit_in_subject_name: "Calculus",
      sit_ins: [{ id: "sis1", session_id: "s1", course_id: "c1", course_code: "MATH301", course_name: "Calc III" } as AbsenceSitInSession],
    };
    expect(formatBatchSitInSummary(absence)).toBe("Calculus");
  });

  it("returns 'To arrange' when physical method with nothing resolved", () => {
    const absence: ManagedAbsence = { ...baseAbsence, sit_in_method: "physical" };
    expect(formatBatchSitInSummary(absence)).toBe("To arrange");
  });
});
