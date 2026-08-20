import { describe, expect, it } from "vitest";
import {
  formatRemainingHours,
  remainingMinutes,
  remainingStatus,
  sumSessionMinutes,
} from "./sessionUsage";
import type { Session } from "@/features/scheduling/types";

function session(startAt: string, endAt: string): Session {
  return {
    id: "s",
    course_id: "course-1",
    room_id: null,
    teacher_id: "t",
    start_at: startAt,
    end_at: endAt,
    version: 1,
  };
}

describe("sumSessionMinutes", () => {
  it("sums the duration of every session", () => {
    const sessions = [
      session("2026-05-01T09:00:00Z", "2026-05-01T10:30:00Z"), // 90
      session("2026-05-02T09:00:00Z", "2026-05-02T10:00:00Z"), // 60
      session("2026-05-03T09:00:00Z", "2026-05-03T09:30:00Z"), // 30
    ];
    expect(sumSessionMinutes(sessions)).toBe(180);
  });

  it("returns zero for an empty schedule", () => {
    expect(sumSessionMinutes([])).toBe(0);
  });

  it("skips sessions with unparseable timestamps instead of crashing", () => {
    const sessions = [
      session("2026-05-01T09:00:00Z", "2026-05-01T10:00:00Z"), // 60
      session("not-a-date", "2026-05-01T10:00:00Z"),
    ];
    expect(sumSessionMinutes(sessions)).toBe(60);
  });
});

describe("remainingMinutes", () => {
  it("subtracts used minutes from the hours the user set", () => {
    expect(remainingMinutes(10, 240)).toBe(360);
  });

  it("is negative when usage exceeds the set hours", () => {
    expect(remainingMinutes(2, 180)).toBe(-60);
  });

  it("is null when no hours are set on the course", () => {
    expect(remainingMinutes(null, 240)).toBeNull();
    expect(remainingMinutes(undefined, 240)).toBeNull();
  });
});

describe("remainingStatus", () => {
  it("is remaining while minutes are left", () => {
    expect(remainingStatus(1)).toBe("remaining");
  });

  it("is over once usage exceeds the set hours", () => {
    expect(remainingStatus(-1)).toBe("over");
  });

  it("is none at exactly zero left", () => {
    expect(remainingStatus(0)).toBe("none");
  });
});

describe("formatRemainingHours", () => {
  it("formats positive minutes as hours:minutes", () => {
    expect(formatRemainingHours(390)).toBe("06:30");
  });

  it("prefixes negatives with a minus sign", () => {
    expect(formatRemainingHours(-90)).toBe("-01:30");
  });

  it("formats zero as 00:00", () => {
    expect(formatRemainingHours(0)).toBe("00:00");
  });
});