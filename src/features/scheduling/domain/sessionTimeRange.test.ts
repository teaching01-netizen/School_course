import { describe, expect, it } from "vitest";
import {
  isSessionTimeFilterActive,
  sessionMatchesTimeFilter,
  validateSessionTimeFilter,
  type SessionTimeFilter,
} from "./sessionTimeRange";

const MORNING_SESSION = {
  start_at: "2026-06-01T02:00:00Z",
  end_at: "2026-06-01T04:00:00Z",
};

describe("session time range filter", () => {
  it("matches a session whose local start and end are inside the selected range", () => {
    const filter: SessionTimeFilter = { from: "09:00", to: "11:00" };

    expect(sessionMatchesTimeFilter(MORNING_SESSION, "Asia/Bangkok", filter)).toBe(true);
  });

  it("rejects sessions that start before the selected range", () => {
    const filter: SessionTimeFilter = { from: "09:30", to: "11:00" };

    expect(sessionMatchesTimeFilter(MORNING_SESSION, "Asia/Bangkok", filter)).toBe(false);
  });

  it("treats an empty filter as no restriction", () => {
    const filter: SessionTimeFilter = { from: "", to: "" };

    expect(isSessionTimeFilterActive(filter)).toBe(false);
    expect(sessionMatchesTimeFilter(MORNING_SESSION, "Asia/Bangkok", filter)).toBe(true);
  });

  it("reports reversed ranges instead of silently applying them", () => {
    const filter: SessionTimeFilter = { from: "12:00", to: "09:00" };

    expect(validateSessionTimeFilter(filter)).toBe("From time must be earlier than or equal to To time.");
    expect(sessionMatchesTimeFilter(MORNING_SESSION, "Asia/Bangkok", filter)).toBe(false);
  });
});
