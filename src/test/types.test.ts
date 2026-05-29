import { describe, it, expect } from "vitest";
import {
  conflictKindLabel,
  yyyyMmDd,
  formatTimeRange,
  minutesBetween,
  fmtDuration,
  localDateTimeToUTCISO,
  getRequestedLabel,
  type Course,
  type User,
} from "@/types";

describe("conflictKindLabel", () => {
  const cases: [string, string, string][] = [
    ["room_overlap", "Room already booked", "The requested room is occupied"],
    ["teacher_overlap", "Teacher has another session", "Teacher is busy with another class"],
    ["student_overlap", "Student scheduling conflict", "One or more students have a scheduling clash"],
    ["teacher_availability", "Teacher not available", "Teacher is not available at this time"],
    ["room_availability", "Room not available", "Room is not available at this time"],
  ];
  for (const [kind, expLabel, expDetail] of cases) {
    it(`maps ${kind}`, () => {
      const r = conflictKindLabel(kind);
      expect(r.label).toBe(expLabel);
      expect(r.detail).toBe(expDetail);
    });
  }

  it("handles unknown kind", () => {
    const r = conflictKindLabel("foo_bar");
    expect(r.label).toBe("foo bar");
    expect(r.detail).toBe("");
  });

  it("returns label and detail without icon field", () => {
    const r = conflictKindLabel("room_overlap");
    expect(r).not.toHaveProperty("icon");
    expect(typeof r.label).toBe("string");
    expect(typeof r.detail).toBe("string");
  });
});

describe("yyyyMmDd", () => {
  it("formats date to YYYY-MM-DD", () => {
    expect(yyyyMmDd(new Date("2025-01-15T12:00:00Z"))).toBe("2025-01-15");
  });

  it("handles month/day boundary", () => {
    expect(yyyyMmDd(new Date("2025-12-31T23:59:00Z"))).toBe("2025-12-31");
  });
});

describe("formatTimeRange", () => {
  it("formats valid ISO range", () => {
    const r = formatTimeRange("2025-01-15T03:00:00.000Z", "2025-01-15T05:00:00.000Z");
    expect(r).toContain("Jan");
    expect(r).toContain("15");
    expect(r).toMatch(/\d{2}:\d{2}/);
  });

  it("returns fallback for invalid input", () => {
    const r = formatTimeRange("bad-date", "also-bad");
    expect(r).toContain("Invalid Date");
  });
});

describe("minutesBetween", () => {
  it("computes difference in minutes", () => {
    expect(minutesBetween("2025-01-15T03:00:00.000Z", "2025-01-15T05:30:00.000Z")).toBe(150);
  });

  it("returns null for invalid dates", () => {
    expect(minutesBetween("bad", "2025-01-15T05:00:00.000Z")).toBeNull();
    expect(minutesBetween("2025-01-15T03:00:00.000Z", "bad")).toBeNull();
  });
});

describe("fmtDuration", () => {
  it("formats 0 minutes", () => expect(fmtDuration(0)).toBe("00:00"));
  it("formats 60 minutes", () => expect(fmtDuration(60)).toBe("01:00"));
  it("formats 90 minutes", () => expect(fmtDuration(90)).toBe("01:30"));
  it("formats 150 minutes", () => expect(fmtDuration(150)).toBe("02:30"));
});

describe("localDateTimeToUTCISO", () => {
  it("returns null for empty input", () => {
    expect(localDateTimeToUTCISO("", "Asia/Bangkok")).toBeNull();
  });

  it("converts valid local datetime to UTC ISO", () => {
    const r = localDateTimeToUTCISO("2025-01-15T10:00", "Asia/Bangkok");
    expect(r).toBe("2025-01-15T03:00:00.000Z");
  });

  it("handles datetime-local input with seconds suffix", () => {
    const r = localDateTimeToUTCISO("2025-01-15T10:00:00", "Asia/Bangkok");
    expect(r).toBe("2025-01-15T03:00:00.000Z");
  });

  it("handles datetime-local input with milliseconds", () => {
    const r = localDateTimeToUTCISO("2025-01-15T10:00:00.000", "Asia/Bangkok");
    expect(r).toBe("2025-01-15T03:00:00.000Z");
  });

  it("handles different timezone offset", () => {
    const r = localDateTimeToUTCISO("2025-06-15T10:00", "America/New_York");
    expect(r).toBe("2025-06-15T14:00:00.000Z");
  });

  it("returns null for garbage input", () => {
    expect(localDateTimeToUTCISO("not-a-date", "Asia/Bangkok")).toBeNull();
  });
});

describe("getRequestedLabel", () => {
  const courses = new Map<string, Course>([["c1", { id: "c1", code: "MATH101", name: "Math" }]]);
  const teachers = new Map<string, User>([["t1", { id: "t1", username: "jdoe", role: "Admin" }]]);

  it("returns label when both found", () => {
    expect(getRequestedLabel({ course_id: "c1", teacher_id: "t1" }, courses, teachers)).toBe("jdoe – MATH101");
  });

  it("truncates IDs when course not found", () => {
    const r = getRequestedLabel({ course_id: "abcdef123456", teacher_id: "t1" }, courses, teachers);
    expect(r).toContain("jdoe");
    expect(r).toContain("abcdef12");
  });

  it("truncates IDs when teacher not found", () => {
    const r = getRequestedLabel({ course_id: "c1", teacher_id: "abcdef123456" }, courses, teachers);
    expect(r).toContain("MATH101");
    expect(r).toContain("abcdef12");
  });
});
