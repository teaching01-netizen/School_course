import { describe, it, expect } from "vitest";
import { groupSessionKey } from "@/utils/timezone";

describe("groupSessionKey", () => {
  it("returns null for invalid ISO string", () => {
    expect(groupSessionKey("not-a-date", "Asia/Bangkok")).toBeNull();
  });

  it("returns null for invalid zone", () => {
    expect(groupSessionKey("2026-01-15T02:00:00Z", "Invalid/Zone")).toBeNull();
  });

  it("maps 09:00 Bangkok (02:00 UTC Thursday) to key '4-09:00'", () => {
    // 2026-01-15 is Thursday → getDay() = 4
    expect(groupSessionKey("2026-01-15T02:00:00Z", "Asia/Bangkok")).toBe("4-09:00");
  });

  it("maps 00:00 Bangkok (17:00 UTC previous day) correctly", () => {
    // 2026-01-15T17:00:00Z = 2026-01-16T00:00:00 Bangkok (Friday)
    expect(groupSessionKey("2026-01-15T17:00:00Z", "Asia/Bangkok")).toBe("5-00:00");
  });

  it("maps 23:00 Bangkok (16:00 UTC same day) correctly", () => {
    // 2026-01-15T16:00:00Z = 2026-01-15T23:00:00 Bangkok (Thursday)
    expect(groupSessionKey("2026-01-15T16:00:00Z", "Asia/Bangkok")).toBe("4-23:00");
  });

  it("maps UTC session in UTC zone correctly", () => {
    // 2026-01-15T14:00:00Z in UTC → Thursday 14:00
    expect(groupSessionKey("2026-01-15T14:00:00Z", "UTC")).toBe("4-14:00");
  });

  it("maps midnight UTC in UTC zone correctly", () => {
    // 2026-01-15T00:00:00Z = Thursday 00:00 UTC
    expect(groupSessionKey("2026-01-15T00:00:00Z", "UTC")).toBe("4-00:00");
  });

  it("handles Monday correctly", () => {
    // 2026-01-12 is Monday → getDay() = 1
    expect(groupSessionKey("2026-01-12T09:00:00Z", "UTC")).toBe("1-09:00");
  });

  it("handles Sunday correctly", () => {
    // 2026-01-11 is Sunday → getDay() = 0
    expect(groupSessionKey("2026-01-11T09:00:00Z", "UTC")).toBe("0-09:00");
  });
});
