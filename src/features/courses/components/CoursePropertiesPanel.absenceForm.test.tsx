import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
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
    <MemoryRouter>
      <CoursePropertiesPanel
        course={course}
        teacherOptions={[]}
        savingField={null}
        onSave={onSave}
      />
    </MemoryRouter>,
  );
  return onSave;
}

describe("CoursePropertiesPanel absence form visibility (read-only)", () => {
  it("shows Shown to students by default and when explicitly true", () => {
    renderPanel(makeCourse());
    expect(screen.getByText("Shown to students")).toBeInTheDocument();

    renderPanel(makeCourse({ absence_form_visible: true }));
    expect(screen.getAllByText("Shown to students").length).toBeGreaterThan(0);
  });

  it("shows the Hidden chip when the course is hidden", () => {
    renderPanel(makeCourse({ absence_form_visible: false }));
    expect(screen.getByText("Hidden from students")).toBeInTheDocument();
  });

  it("links to the single management surface in Operations", () => {
    renderPanel(makeCourse());
    const link = screen.getByRole("link", { name: "Manage in Operations" });
    expect(link).toHaveAttribute("href", "/operations?tab=active-courses");
  });

  it("renders no editor — visibility is not editable here", async () => {
    const onSave = renderPanel(makeCourse({ absence_form_visible: false }));
    expect(screen.queryByRole("switch")).not.toBeInTheDocument();

    // Even clicking the status text must not attempt a save.
    await userEvent.setup().click(screen.getByText("Hidden from students"));
    expect(onSave).not.toHaveBeenCalled();
  });
});
