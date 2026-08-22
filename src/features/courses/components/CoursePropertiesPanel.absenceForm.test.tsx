import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { CoursePropertiesPanel } from "./CoursePropertiesPanel";
import type { Course } from "../types";

function makeCourse(overrides: Partial<Course> = {}): Course {
  return {
    id: "course-1",
    version: 1,
    code: "MATH-101",
    name: "Math",
    primary_teacher_id: null,
    ...overrides,
  };
}

function renderPanel(course: Course) {
  const onSave = vi.fn().mockResolvedValue(true);
  render(
    <CoursePropertiesPanel
      course={course}
      teacherOptions={[]}
      savingField={null}
      onSave={onSave}
    />,
  );
  return onSave;
}

describe("CoursePropertiesPanel absence form visibility", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows Visible to students by default and when explicitly true", () => {
    renderPanel(makeCourse());
    expect(screen.getByText("Visible to students")).toBeInTheDocument();

    renderPanel(makeCourse({ absence_form_visible: true }));
    expect(screen.getAllByText("Visible to students").length).toBeGreaterThan(0);
  });

  it("shows Hidden from students when the course is hidden", () => {
    renderPanel(makeCourse({ absence_form_visible: false }));
    expect(screen.getByText("Hidden from students")).toBeInTheDocument();
  });

  it("saves the picked visibility as a boolean change set", async () => {
    const onSave = renderPanel(makeCourse());
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: "Visible to students" }));
    await user.click(await screen.findByRole("option", { name: "Hidden from students" }));

    expect(onSave).toHaveBeenCalledWith("absence_form_visible", { absence_form_visible: false });
  });
});
