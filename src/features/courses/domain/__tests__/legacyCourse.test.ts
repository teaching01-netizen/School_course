import { describe, expect, it } from "vitest";
import { extractLegacyCourseId } from "../legacyCourse";

describe("legacy course id extraction", () => {
  it.each([
    ["https://warwick.azurewebsites.net/Admin/Courses/Detail?id=12345", "12345"],
    ["?id=678", "678"],
    ["  42  ", "42"],
  ])("extracts %s", (input, expected) => {
    expect(extractLegacyCourseId(input)).toBe(expected);
  });

  it.each(["", "abc", "https://example.test/Admin/Courses/Detail?id=abc"])("rejects %s", (input) => {
    expect(extractLegacyCourseId(input)).toBeNull();
  });
});
