import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { CourseInfoStrip } from "./CourseInfoStrip";
import type { Course } from "../types";

function makeCourse(overrides: Record<string, unknown> = {}): Course {
  return {
    id: "course-1",
    version: 3,
    code: "MATH-101",
    name: "Math",
    primary_teacher_id: "teacher-1",
    teachers: [
      { id: "teacher-1", username: "Teacher One", is_primary: true },
      { id: "teacher-2", username: "Teacher Two", is_primary: false },
    ],
    ...overrides,
  } as Course;
}

describe("CourseInfoStrip", () => {
  it("shows Teacher, Hour, Student and Type with the course values", () => {
    render(
      <CourseInfoStrip
        course={makeCourse({ hour: 120, student_count: 30, course_type: "Group" })}
      />,
    );

    expect(screen.getByText("Teacher")).toBeInTheDocument();
    expect(screen.getByText("Teacher One, Teacher Two")).toBeInTheDocument();
    expect(screen.getByText("Hour")).toBeInTheDocument();
    expect(screen.getByText("120")).toBeInTheDocument();
    expect(screen.getByText("Student")).toBeInTheDocument();
    expect(screen.getByText("30")).toBeInTheDocument();
    expect(screen.getByText("Type")).toBeInTheDocument();
    expect(screen.getByText("Group")).toBeInTheDocument();
  });

  it("falls back to teacher_name when the course has no teachers list", () => {
    render(
      <CourseInfoStrip
        course={makeCourse({ teachers: undefined, teacher_name: "Legacy Teacher", hour: 60, student_count: 15, course_type: "Private" })}
      />,
    );

    expect(screen.getByText("Legacy Teacher")).toBeInTheDocument();
  });

  it("prefers the teacher name over teacher usernames when both are present", () => {
    render(
      <CourseInfoStrip
        course={makeCourse({ teacher_name: "Jane Teacher" })}
      />,
    );

    expect(screen.getByText("Jane Teacher")).toBeInTheDocument();
    expect(screen.queryByText("Teacher One, Teacher Two")).not.toBeInTheDocument();
  });

  it("uses a resolved teacher directory name when supplied", () => {
    render(
      <CourseInfoStrip
        course={makeCourse({ teacher_name: "legacy:95" })}
        teacherName="AJ Bosch"
      />,
    );

    expect(screen.getByText("AJ Bosch")).toBeInTheDocument();
    expect(screen.queryByText("legacy:95")).not.toBeInTheDocument();
  });

  it("shows the last legacy sync time for a linked course", () => {
    render(
      <CourseInfoStrip
        course={makeCourse({
          legacy_course_id: "7090",
          legacy_last_synced_at: "2026-08-01T00:00:00Z",
        })}
      />,
    );

    expect(screen.getByText("Last sync")).toBeInTheDocument();
    expect(screen.getByText(/2026/)).toBeInTheDocument();
  });

  it("shows an explicit not-synced state for a linked course without a sync time", () => {
    render(<CourseInfoStrip course={makeCourse({ legacy_course_id: "7090", legacy_last_synced_at: null })} />);

    expect(screen.getByText("Last sync")).toBeInTheDocument();
    expect(screen.getByText("Not synced yet")).toBeInTheDocument();
  });

  it("renders an em dash for missing values", () => {
    render(<CourseInfoStrip course={makeCourse({ hour: null, student_count: null, course_type: null, teachers: [], teacher_name: null })} />);

    // Teacher, Hour, Remaining, Student, Type all have no value.
    const dashes = screen.getAllByText("—");
    expect(dashes).toHaveLength(5);
    expect(screen.queryByText("Teacher One, Teacher Two")).not.toBeInTheDocument();
  });

  describe("Remaining hours", () => {
    it("shows hours left with a green fill when usage is under the set hours", () => {
      render(<CourseInfoStrip course={makeCourse({ hour: 10 })} usedMinutes={240} />);

      const pill = screen.getByTestId("remaining-pill");
      expect(pill).toHaveTextContent("06:00");
      expect(pill).toHaveAttribute("data-usage", "remaining");
      expect(pill.className).toContain("green");
    });

    it("shows the deficit with a red fill when usage exceeds the set hours", () => {
      render(<CourseInfoStrip course={makeCourse({ hour: 2 })} usedMinutes={180} />);

      const pill = screen.getByTestId("remaining-pill");
      expect(pill).toHaveTextContent("-01:00");
      expect(pill).toHaveAttribute("data-usage", "over");
      expect(pill.className).toContain("danger");
    });

    it("shows a neutral pill at exactly zero hours left", () => {
      render(<CourseInfoStrip course={makeCourse({ hour: 3 })} usedMinutes={180} />);

      const pill = screen.getByTestId("remaining-pill");
      expect(pill).toHaveTextContent("00:00");
      expect(pill).toHaveAttribute("data-usage", "none");
    });

    it("renders an em dash with no pill when no hours are set", () => {
      render(<CourseInfoStrip course={makeCourse({ hour: null })} usedMinutes={180} />);

      expect(screen.queryByTestId("remaining-pill")).not.toBeInTheDocument();
      const remainingStat = screen.getByText("Remaining").closest("div");
      expect(remainingStat).toHaveTextContent("—");
    });
  });
});
