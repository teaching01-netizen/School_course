import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  loadAbsenceFormConfig,
  loadSessionsInRange,
  lookupStudentByWcode,
  sessionsInRangePath,
  normalizeAbsenceFormConfig,
  submitAbsenceBatch,
} from "../absenceFormApi";
import type { AbsenceFormConfig } from "../../types";
import { ApiRequestError } from "@/api/client";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

const BATCH_INPUT = {
  idempotencyKey: "absence-submit-key",
  wcode: "W250389",
  email: "student@example.com",
  reason: "Medical appointment",
  verificationToken: "verified-token",
  items: [
    {
      subject_id: "subject-1",
      course_id: "course-1",
      date_from: "2026-07-14",
      date_to: "2026-07-14",
      sit_in_method: "zoom" as const,
      missed_session_ids: ["session-1"],
      sit_in_session_ids: [],
    },
  ],
};

describe("submitAbsenceBatch", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
  });

  it("retries one interrupted request with the identical body and idempotency key", async () => {
    const response = { items: [] };
    mockApiJson
      .mockRejectedValueOnce(new TypeError("Failed to fetch"))
      .mockResolvedValueOnce(response);

    await expect(submitAbsenceBatch(BATCH_INPUT)).resolves.toBe(response);

    expect(mockApiJson).toHaveBeenCalledTimes(2);
    const firstInit = mockApiJson.mock.calls[0][1] as RequestInit;
    const secondInit = mockApiJson.mock.calls[1][1] as RequestInit;
    expect(secondInit.body).toBe(firstInit.body);
    expect(secondInit.headers).toEqual(firstInit.headers);
    expect(firstInit.headers).toEqual({
      "Idempotency-Key": BATCH_INPUT.idempotencyKey,
    });
  });

  it("does not retry a readable API response error", async () => {
    const error = new ApiRequestError("Invalid absence", {
      code: "invalid_absence",
      status: 400,
    });
    mockApiJson.mockRejectedValueOnce(error);

    await expect(submitAbsenceBatch(BATCH_INPUT)).rejects.toBe(error);

    expect(mockApiJson).toHaveBeenCalledTimes(1);
  });

  it("does not retry an aborted request", async () => {
    const error = new DOMException("The operation was aborted", "AbortError");
    mockApiJson.mockRejectedValueOnce(error);

    await expect(submitAbsenceBatch(BATCH_INPUT)).rejects.toBe(error);

    expect(mockApiJson).toHaveBeenCalledTimes(1);
  });

  it("does not retry other errors", async () => {
    const error = new Error("Unexpected client failure");
    mockApiJson.mockRejectedValueOnce(error);

    await expect(submitAbsenceBatch(BATCH_INPUT)).rejects.toBe(error);

    expect(mockApiJson).toHaveBeenCalledTimes(1);
  });

  it("propagates a second interrupted request after exactly one retry", async () => {
    const secondError = new TypeError("Failed to fetch again");
    mockApiJson
      .mockRejectedValueOnce(new TypeError("Failed to fetch"))
      .mockRejectedValueOnce(secondError);

    await expect(submitAbsenceBatch(BATCH_INPUT)).rejects.toBe(secondError);

    expect(mockApiJson).toHaveBeenCalledTimes(2);
  });

  it("omits optional identity fields instead of sending null placeholders", async () => {
    mockApiJson.mockResolvedValueOnce({ items: [] });

    await submitAbsenceBatch({
      ...BATCH_INPUT,
      email: undefined,
      verificationToken: undefined,
    });

    const init = mockApiJson.mock.calls[0][1] as RequestInit;
    expect(JSON.parse(String(init.body))).toEqual({
      wcode: BATCH_INPUT.wcode,
      reason: BATCH_INPUT.reason,
      items: BATCH_INPUT.items,
    });
  });
});

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

  it("encodes reserved characters in the student identifier", () => {
    expect(sessionsInRangePath("W25 0389&admin=true")).toBe(
      "/api/v1/absences/sessions-in-range?wcode=W25+0389%26admin%3Dtrue",
    );
  });

  it("keeps priority level zero because it is a valid boundary value", () => {
    expect(
      sessionsInRangePath("W250389", undefined, undefined, {
        satVerbalAfterPriority: 0,
      }),
    ).toContain("sat_verbal_after_priority=0");
  });
});

describe("public absence API requests", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
  });

  it("normalizes configuration returned by the public endpoint", async () => {
    mockApiJson.mockResolvedValueOnce({
      form: { max_date_range_days: 7 },
      sit_in: {},
    });

    const config = await loadAbsenceFormConfig();

    expect(config.form.max_date_range_days).toBe(7);
    expect(config.notifications?.allow_submit_without_otp).toBe(false);
    expect(mockApiJson).toHaveBeenCalledWith("/api/v1/absence-form-config", {
      method: "GET",
    });
  });

  it("URL-encodes W-codes used for student lookup", async () => {
    mockApiJson.mockResolvedValueOnce({ wcode: "W250389", subjects: [] });

    await lookupStudentByWcode(" W250389&include=private ");

    expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/absences/student-lookup?wcode=%20W250389%26include%3Dprivate%20",
      { method: "GET" },
    );
  });

  it("forwards cancellation and range options when loading sessions", async () => {
    mockApiJson.mockResolvedValueOnce({ subjects: [] });
    const controller = new AbortController();

    await loadSessionsInRange(
      "W250389",
      "2026-07-01",
      "2026-07-31",
      { signal: controller.signal },
      { courseIds: ["course-1"] },
    );

    const [path, init] = mockApiJson.mock.calls[0] as [string, RequestInit];
    expect(path).toContain("date_from=2026-07-01");
    expect(path).toContain("date_to=2026-07-31");
    expect(path).toContain("course_ids=course-1");
    expect(init).toEqual({ method: "GET", signal: controller.signal });
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
