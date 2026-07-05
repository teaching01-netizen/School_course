import { describe, expect, it } from "vitest";
import {
  sessionsInRangePath,
  normalizeAbsenceFormConfig,
} from "../absenceFormApi";
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
    const path = sessionsInRangePath("W250389", undefined, undefined, {
      courseIds: ["c1", "c2"],
    });
    expect(path).toContain("course_ids=c1%2Cc2");
  });

  it("adds subject-wide staff options when provided", () => {
    const path = sessionsInRangePath("W250389", undefined, undefined, {
      subjectIds: ["sub1", "sub2"],
      includeAllSubjects: true,
    });
    expect(path).toContain("subject_ids=sub1%2Csub2");
    expect(path).toContain("include_all_subjects=true");
  });

  it("adds satVerbalAfterPriority when provided", () => {
    const path = sessionsInRangePath("W250389", undefined, undefined, {
      satVerbalAfterPriority: 3,
    });
    expect(path).toContain("sat_verbal_after_priority=3");
  });

  it("adds bypass_timing when bypassTiming is true", () => {
    const path = sessionsInRangePath("W250389", undefined, undefined, {
      bypassTiming: true,
    });
    expect(path).toContain("bypass_timing=true");
  });

  it("omits bypass_timing when bypassTiming is false", () => {
    const path = sessionsInRangePath("W250389", undefined, undefined, {
      bypassTiming: false,
    });
    expect(path).not.toContain("bypass_timing");
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

  it("fills sms_special_approved_template from defaults when absent", () => {
    const input = {
      form: { max_date_range_days: 14 },
      sit_in: { auto_resolve_enabled: false },
      notifications: { sms_parent_enabled: true },
    } as AbsenceFormConfig;
    const result = normalizeAbsenceFormConfig(input);
    expect(result.notifications?.sms_special_approved_template).toBe("");
  });

  it("preserves existing sms_special_approved_template", () => {
    const input = {
      form: { max_date_range_days: 14 },
      sit_in: { auto_resolve_enabled: false },
      notifications: { sms_special_approved_template: "Custom special" },
    } as AbsenceFormConfig;
    const result = normalizeAbsenceFormConfig(input);
    expect(result.notifications?.sms_special_approved_template).toBe("Custom special");
  });

  it("handles completely empty notifications", () => {
    const input = {
      form: { max_date_range_days: 14 },
      sit_in: { auto_resolve_enabled: false },
      notifications: {},
    } as AbsenceFormConfig;
    const result = normalizeAbsenceFormConfig(input);
    expect(result.notifications?.sms_parent_enabled).toBe(true);
    expect(result.notifications?.sms_special_approved_template).toBe("");
    expect(result.notifications?.allow_submit_without_otp).toBe(false);
  });

  it("preserves email fields when present", () => {
    const input = {
      form: { max_date_range_days: 14 },
      sit_in: { auto_resolve_enabled: false },
      notifications: {
        email_success_enabled: true,
        email_success_subject: "Custom Subject {{student_name}}",
        email_success_body: "<p>Custom body</p>",
      },
    } as AbsenceFormConfig;
    const result = normalizeAbsenceFormConfig(input);
    expect(result.notifications?.email_success_enabled).toBe(true);
    expect(result.notifications?.email_success_subject).toBe("Custom Subject {{student_name}}");
    expect(result.notifications?.email_success_body).toBe("<p>Custom body</p>");
  });

  it("defaults email fields when missing", () => {
    const result = normalizeAbsenceFormConfig(partial);
    expect(result.notifications?.email_success_enabled).toBe(false);
    expect(result.notifications?.email_success_subject).toBe("");
    expect(result.notifications?.email_success_body).toBe("");
  });

  it("defaults partial email fields", () => {
    const input = {
      form: { max_date_range_days: 14 },
      sit_in: { auto_resolve_enabled: false },
      notifications: {
        email_success_enabled: true,
      },
    } as AbsenceFormConfig;
    const result = normalizeAbsenceFormConfig(input);
    expect(result.notifications?.email_success_enabled).toBe(true);
    expect(result.notifications?.email_success_subject).toBe("");
    expect(result.notifications?.email_success_body).toBe("");
  });
});
