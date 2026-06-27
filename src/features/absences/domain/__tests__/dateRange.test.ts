import { describe, expect, it } from "vitest";
import { daysBetween, postSessionLookbackDays, dateToLocalISO } from "../dateRange";

describe("daysBetween", () => {
  it("returns 0 for same day", () => {
    expect(daysBetween("2026-06-01", "2026-06-01")).toBe(0);
  });

  it("returns 1 for consecutive days", () => {
    expect(daysBetween("2026-06-01", "2026-06-02")).toBe(1);
  });

  it("returns 30 for 30 days apart", () => {
    expect(daysBetween("2026-06-01", "2026-07-01")).toBe(30);
  });
});

describe("postSessionLookbackDays", () => {
  it("returns ceil(hours/24) for normal values", () => {
    expect(postSessionLookbackDays(48)).toBe(2);
    expect(postSessionLookbackDays(25)).toBe(2);
    expect(postSessionLookbackDays(24)).toBe(1);
  });

  it("returns 0 for zero hours", () => {
    expect(postSessionLookbackDays(0)).toBe(0);
  });

  it("returns 0 for negative hours", () => {
    expect(postSessionLookbackDays(-10)).toBe(0);
  });

  it("returns 0 for Infinity", () => {
    expect(postSessionLookbackDays(Infinity)).toBe(0);
  });

  it("returns 0 for NaN", () => {
    expect(postSessionLookbackDays(NaN)).toBe(0);
  });
});

describe("dateToLocalISO", () => {
  it("formats a date as YYYY-MM-DD", () => {
    expect(dateToLocalISO(new Date(2026, 5, 1))).toBe("2026-06-01");
  });

  it("pads single-digit month and day", () => {
    expect(dateToLocalISO(new Date(2026, 0, 5))).toBe("2026-01-05");
  });
});
