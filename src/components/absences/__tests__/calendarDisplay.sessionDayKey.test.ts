import { describe, expect, it } from "vitest";
import { sessionDayKey } from "../calendarDisplay";
import type { CalendarSessionBrief } from "../../../features/absences/types";

function sessionWith(startAt: string): CalendarSessionBrief {
  return {
    id: "s-1",
    course_id: "c-1",
    course_code: "C1",
    start_at: startAt,
    end_at: startAt,
  };
}

describe("sessionDayKey", () => {
  it("buckets a late-evening UTC session onto the next Bangkok calendar day", () => {
    expect(sessionDayKey(sessionWith("2026-06-02T23:30:00Z"), "Asia/Bangkok")).toBe("2026-06-03");
  });

  it("keeps the same calendar day when the zone is UTC", () => {
    expect(sessionDayKey(sessionWith("2026-06-02T23:30:00Z"), "UTC")).toBe("2026-06-02");
  });

  it("buckets an early-morning UTC session onto the same Bangkok day", () => {
    expect(sessionDayKey(sessionWith("2026-06-03T00:30:00Z"), "Asia/Bangkok")).toBe("2026-06-03");
  });

  it("falls back to the UTC date when the zone cannot be resolved", () => {
    expect(sessionDayKey(sessionWith("2026-06-02T12:00:00Z"), "Mars/Olympus")).toBe("2026-06-02");
  });
});