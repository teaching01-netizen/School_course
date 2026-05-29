import { describe, it, expect } from "vitest";
import {
  getGapWarning,
  detectGaps,
  computeLevelCompletion,
  buildRootGroupHierarchy,
} from "../levels";
import type { CourseLevelItem } from "../levels";

const makeCourse = (overrides: Partial<CourseLevelItem>): CourseLevelItem => ({
  id: "c1",
  code: "MATH-101",
  name: "Math 101",
  subject_id: "subj-1",
  subject_code: "MATH",
  subject_name: "Mathematics",
  cycle_id: "cy2025a",
  cycle_label: "Cycle 2025-01",
  level: null,
  root_course_group_id: null,
  root_course_group_name: null,
  ...overrides,
});

describe("getGapWarning", () => {
  it("returns null for fewer than 2 levels", () => {
    expect(getGapWarning([makeCourse({ level: 1 })])).toBeNull();
    expect(getGapWarning([makeCourse({ level: null })])).toBeNull();
    expect(getGapWarning([])).toBeNull();
  });

  it("returns null for consecutive levels", () => {
    const courses = [
      makeCourse({ id: "c1", level: 1 }),
      makeCourse({ id: "c2", level: 2 }),
      makeCourse({ id: "c3", level: 3 }),
    ];
    expect(getGapWarning(courses)).toBeNull();
  });

  it("detects a gap between levels", () => {
    const courses = [
      makeCourse({ id: "c1", level: 1 }),
      makeCourse({ id: "c2", level: 3 }),
    ];
    expect(getGapWarning(courses)).toBe("No Level 2 — Level 1 students will skip to Level 3");
  });

  it("detects first gap only", () => {
    const courses = [
      makeCourse({ id: "c1", level: 1 }),
      makeCourse({ id: "c2", level: 4 }),
    ];
    expect(getGapWarning(courses)).toBe("No Level 2 — Level 1 students will skip to Level 4");
  });

  it("ignores null levels in gap detection (only non-null levels count)", () => {
    const courses = [
      makeCourse({ id: "c1", level: 1 }),
      makeCourse({ id: "c2", level: null }),
      makeCourse({ id: "c3", level: 3 }),
    ];
    // Null level doesn't fill the gap between 1 and 3
    expect(getGapWarning(courses)).toBe("No Level 2 — Level 1 students will skip to Level 3");
  });
});

describe("detectGaps", () => {
  it("returns empty array for consecutive levels", () => {
    const courses = [
      makeCourse({ id: "c1", level: 1 }),
      makeCourse({ id: "c2", level: 2 }),
      makeCourse({ id: "c3", level: 3 }),
    ];
    expect(detectGaps(courses)).toEqual([]);
  });

  it("identifies all gaps", () => {
    const courses = [
      makeCourse({ id: "c1", level: 1 }),
      makeCourse({ id: "c2", level: 4 }),
    ];
    const gaps = detectGaps(courses);
    expect(gaps).toHaveLength(2);
    expect(gaps[0]).toEqual({ level: 2, message: "Level 2 is missing" });
    expect(gaps[1]).toEqual({ level: 3, message: "Level 3 is missing" });
  });
});

describe("computeLevelCompletion", () => {
  it("counts assigned vs total", () => {
    const courses = [
      makeCourse({ id: "c1", level: 1 }),
      makeCourse({ id: "c2", level: null }),
      makeCourse({ id: "c3", level: 3 }),
    ];
    expect(computeLevelCompletion(courses)).toEqual({ assigned: 2, total: 3 });
  });

  it("returns zero for empty", () => {
    expect(computeLevelCompletion([])).toEqual({ assigned: 0, total: 0 });
  });
});

describe("buildRootGroupHierarchy", () => {
  it("returns empty object for empty courses array", () => {
    expect(buildRootGroupHierarchy([])).toEqual({});
  });

  it("groups courses by root_course_group_id first", () => {
    const courses = [
      makeCourse({
        id: "c1",
        root_course_group_id: "g1",
        root_course_group_name: "SAT Math",
        subject_id: "subj-1",
        subject_code: "MATH",
        subject_name: "Mathematics",
      }),
      makeCourse({
        id: "c2",
        root_course_group_id: "g1",
        root_course_group_name: "SAT Math",
        subject_id: "subj-1",
        subject_code: "MATH",
        subject_name: "Mathematics",
      }),
    ];
    const result = buildRootGroupHierarchy(courses);
    expect(Object.keys(result)).toEqual(["g1"]);
    expect(result["g1"].label).toBe("SAT Math");
    expect(result["g1"].subjects).toHaveLength(1);
    expect(result["g1"].subjects[0].subjectCode).toBe("MATH");
  });

  it("groups ungrouped courses (null root) under __none__ key with label '(none — ungrouped)'", () => {
    const courses = [
      makeCourse({
        id: "c1",
        root_course_group_id: null,
        root_course_group_name: null,
      }),
    ];
    const result = buildRootGroupHierarchy(courses);
    expect(result["__none__"]).toBeDefined();
    expect(result["__none__"].label).toBe("(none — ungrouped)");
    expect(result["__none__"].rootCourseGroupId).toBeNull();
  });

  it("separates courses from different root groups", () => {
    const courses = [
      makeCourse({
        id: "c1",
        root_course_group_id: "g1",
        root_course_group_name: "SAT Math",
        subject_id: "subj-1",
        subject_code: "MATH",
        subject_name: "Mathematics",
      }),
      makeCourse({
        id: "c2",
        root_course_group_id: "g2",
        root_course_group_name: "SAT Physics",
        subject_id: "subj-2",
        subject_code: "PHYS",
        subject_name: "Physics",
      }),
    ];
    const result = buildRootGroupHierarchy(courses);
    expect(Object.keys(result)).toHaveLength(2);
    expect(result["g1"].label).toBe("SAT Math");
    expect(result["g2"].label).toBe("SAT Physics");
  });

  it("creates multiple subjects within one root group", () => {
    const courses = [
      makeCourse({
        id: "c1",
        root_course_group_id: "g1",
        root_course_group_name: "SAT",
        subject_id: "subj-1",
        subject_code: "MATH",
        subject_name: "Mathematics",
      }),
      makeCourse({
        id: "c2",
        root_course_group_id: "g1",
        root_course_group_name: "SAT",
        subject_id: "subj-2",
        subject_code: "PHYS",
        subject_name: "Physics",
      }),
    ];
    const result = buildRootGroupHierarchy(courses);
    expect(result["g1"].subjects).toHaveLength(2);
    const subjectCodes = result["g1"].subjects.map((s) => s.subjectCode).sort();
    expect(subjectCodes).toEqual(["MATH", "PHYS"]);
  });

  it("groups cycles within a subject correctly", () => {
    const courses = [
      makeCourse({
        id: "c1",
        root_course_group_id: "g1",
        root_course_group_name: "SAT",
        subject_id: "subj-1",
        subject_code: "MATH",
        subject_name: "Mathematics",
        cycle_id: "cyA",
        cycle_label: "Cycle A",
      }),
      makeCourse({
        id: "c2",
        root_course_group_id: "g1",
        root_course_group_name: "SAT",
        subject_id: "subj-1",
        subject_code: "MATH",
        subject_name: "Mathematics",
        cycle_id: "cyB",
        cycle_label: "Cycle B",
      }),
    ];
    const result = buildRootGroupHierarchy(courses);
    const mathSubject = result["g1"].subjects.find((s) => s.subjectCode === "MATH");
    expect(mathSubject).toBeDefined();
    expect(Object.keys(mathSubject!.cycles)).toHaveLength(2);
    expect(mathSubject!.cycles["cyA"].cycleLabel).toBe("Cycle A");
    expect(mathSubject!.cycles["cyB"].cycleLabel).toBe("Cycle B");
    expect(mathSubject!.cycles["cyA"].courses).toHaveLength(1);
    expect(mathSubject!.cycles["cyB"].courses).toHaveLength(1);
  });

  it("populates all SubjectInRoot fields correctly", () => {
    const courses = [
      makeCourse({
        id: "c1",
        root_course_group_id: "g1",
        root_course_group_name: "SAT",
        subject_id: "subj-1",
        subject_code: "MATH",
        subject_name: "Mathematics",
      }),
    ];
    const result = buildRootGroupHierarchy(courses);
    const subject = result["g1"].subjects[0];
    expect(subject.subjectId).toBe("subj-1");
    expect(subject.subjectCode).toBe("MATH");
    expect(subject.subjectName).toBe("Mathematics");
  });

  it("handles mixed grouped and ungrouped courses", () => {
    const courses = [
      makeCourse({
        id: "c1",
        root_course_group_id: "g1",
        root_course_group_name: "SAT",
        subject_id: "subj-1",
        subject_code: "MATH",
        subject_name: "Mathematics",
      }),
      makeCourse({
        id: "c2",
        root_course_group_id: null,
        root_course_group_name: null,
        subject_id: "subj-2",
        subject_code: "PHYS",
        subject_name: "Physics",
      }),
    ];
    const result = buildRootGroupHierarchy(courses);
    expect(Object.keys(result)).toHaveLength(2);
    expect(result["g1"].label).toBe("SAT");
    expect(result["__none__"].label).toBe("(none — ungrouped)");
    expect(result["__none__"].subjects).toHaveLength(1);
    expect(result["__none__"].subjects[0].subjectCode).toBe("PHYS");
  });
});
