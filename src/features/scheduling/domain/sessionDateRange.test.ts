import { describe, expect, it } from "vitest";
import {
  isSessionDateFilterActive,
  sessionMatchesDateFilter,
  validateSessionDateFilter,
  type SessionDateFilter,
} from "./sessionDateRange";

const JUNE_FIRST_SESSION = {
  start_at: "2026-05-31T17:00:00Z",
};

describe("session date range filter", () => {
  it("matches a session whose local calendar date is inside the selected range", () => {
    const filter: SessionDateFilter = { from: "2026-06-01", to: "2026-06-03" };

    expect(sessionMatchesDateFilter(JUNE_FIRST_SESSION, "Asia/Bangkok", filter)).toBe(true);
  });

  it("rejects sessions whose local calendar date is outside the selected range", () => {
    const filter: SessionDateFilter = { from: "2026-06-02", to: "2026-06-03" };
    const session = JUNE_FIRST_SESSION;

    expect(sessionMatchesDateFilter(session, "Asia/Bangkok", filter)).toBe(false);
  });

  it("treats an empty filter as no restriction", () => {
    const filter: SessionDateFilter = { from: "", to: "" };

    expect(isSessionDateFilterActive(filter)).toBe(false);
    expect(sessionMatchesDateFilter(JUNE_FIRST_SESSION, "Asia/Bangkok", filter)).toBe(true);
  });

  it("reports reversed ranges instead of silently applying them", () => {
    const filter: SessionDateFilter = { from: "2026-06-03", to: "2026-06-01" };

    expect(validateSessionDateFilter(filter)).toBe("From date must be earlier than or equal to To date.");
    expect(sessionMatchesDateFilter(JUNE_FIRST_SESSION, "Asia/Bangkok", filter)).toBe(false);
  });
});
