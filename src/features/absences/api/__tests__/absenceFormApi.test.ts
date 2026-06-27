import { describe, expect, it } from "vitest";
import { sessionsInRangePath, normalizeAbsenceFormConfig } from "../absenceFormApi";
import type { AbsenceFormConfig } from "../../types";

describe("sessionsInRangePath", () => {
  it("builds basic path with wcode", () => {
    const path = sessionsInRangePath("W250389");
    expect(path).toBe("/api/v1/absences/sessions-in-range?wcode=W250389");
  });

  it("adds date_from and date_to", () => {
    const path = sessionsInRangePath("W250389", "2026-06-01", "2026-06-30");
    expect(path).toContain("date_from=2026-06-01");
    expect(path).toContain("date_to=2026-06-30");
  });

  it("adds courseIds when provided", () => {
    const path = sessionsInRangePath("W250389", undefined, undefined, { courseIds: ["c1", "c2"] });
    expect(path).toContain("course_ids=c1%2Cc2");
  });

  it("adds satVerbalAfterPriority when provided", () => {
    const path = sessionsInRangePath("W250389", undefined, undefined, { satVerbalAfterPriority: 3 });
    expect(path).toContain("sat_verbal_after_priority=3");
  });
});

describe("normalizeAbsenceFormConfig", () => {
  const partial = {
    form: { max_date_range_days: 14 },
    sit_in: { auto_resolve_enabled: false },
  } as AbsenceFormConfig;

  it("merges partial input with defaults", () => {
    const result = normalizeAbsenceFormConfig(partial);
    expect(result.form.max_date_range_days).toBe(14);
    expect(result.sit_in.auto_resolve_enabled).toBe(false);
    expect(result.notifications).toBeDefined();
    expect(result.admin_contact).toBeDefined();
  });

  it("fills missing fields with defaults", () => {
    const result = normalizeAbsenceFormConfig(partial);
    expect(result.form.reason_categories).toEqual([]);
    expect(result.notifications?.sms_parent_enabled).toBe(true);
    expect(result.admin_contact?.email).toBe("");
  });
});
