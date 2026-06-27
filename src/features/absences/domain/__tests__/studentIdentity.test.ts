import { describe, expect, it } from "vitest";
import { normalizeLookupWcode, maskPhone, getStudentDisplayName } from "../studentIdentity";
import type { StudentLookupResponse } from "../../types";

describe("normalizeLookupWcode", () => {
  it('lowercases "w" prefix to uppercase W', () => {
    expect(normalizeLookupWcode("w250389")).toBe("W250389");
  });

  it("leaves already uppercase W intact", () => {
    expect(normalizeLookupWcode("W250389")).toBe("W250389");
  });

  it("trims whitespace before normalizing", () => {
    expect(normalizeLookupWcode("  w250389  ")).toBe("W250389");
  });

  it("returns empty string for empty input", () => {
    expect(normalizeLookupWcode("")).toBe("");
    expect(normalizeLookupWcode("   ")).toBe("");
  });

  it("returns input unchanged when no w prefix", () => {
    expect(normalizeLookupWcode("STU250389")).toBe("STU250389");
  });
});

describe("maskPhone", () => {
  it("masks middle digits with ***", () => {
    expect(maskPhone("0812345678")).toBe("081 *** 678");
  });

  it("returns as-is for 4 or fewer digits", () => {
    expect(maskPhone("1234")).toBe("1234");
    expect(maskPhone("12")).toBe("12");
  });

  it("returns empty string for null/undefined/empty", () => {
    expect(maskPhone(null)).toBe("");
    expect(maskPhone(undefined)).toBe("");
    expect(maskPhone("")).toBe("");
  });
});

describe("getStudentDisplayName", () => {
  const base: StudentLookupResponse = {
    student_id: "s1",
    wcode: "W250389",
    full_name: "John Doe",
    display_name: "Johnny",
    nickname: "JD",
    subjects: [],
  };

  it("prefers display_name over nickname over full_name", () => {
    expect(getStudentDisplayName(base)).toBe("Johnny");
  });

  it("falls back to nickname when display_name is null", () => {
    expect(getStudentDisplayName({ ...base, display_name: null })).toBe("JD");
  });

  it("falls back to full_name when nickname is null", () => {
    expect(getStudentDisplayName({ ...base, display_name: null, nickname: null })).toBe("John Doe");
  });

  it("returns empty string when all names are null", () => {
    expect(getStudentDisplayName({ ...base, display_name: null, nickname: null, full_name: null } as any)).toBe("");
  });

  it("returns empty string for null lookup", () => {
    expect(getStudentDisplayName(null)).toBe("");
  });
});
