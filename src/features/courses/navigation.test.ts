import { describe, expect, it } from "vitest";
import {
  COURSES_LIST_PATH,
  createCourseDetailNavigationState,
  getCoursesReturnPath,
} from "./navigation";

describe("course navigation", () => {
  it("keeps the originating Courses list URL in detail navigation state", () => {
    const returnTo = "/courses?offset=50&q=math&type=private";

    expect(createCourseDetailNavigationState(returnTo)).toEqual({ returnTo });
    expect(getCoursesReturnPath({ returnTo })).toBe(returnTo);
  });

  it("falls back to the Courses list for invalid or missing navigation state", () => {
    expect(getCoursesReturnPath(undefined)).toBe(COURSES_LIST_PATH);
    expect(getCoursesReturnPath({ returnTo: "/students" })).toBe(COURSES_LIST_PATH);
    expect(getCoursesReturnPath({ returnTo: 42 })).toBe(COURSES_LIST_PATH);
  });
});
