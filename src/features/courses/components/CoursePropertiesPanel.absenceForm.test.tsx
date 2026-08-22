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

describe("CoursePropertiesPanel active status (read-only)", () => {
  it("shows Active when the course is its subject's active class, Off otherwise", () => {
    renderPanel(makeCourse({ is_active_course: true }));
    expect(screen.getByText("Active")).toBeInTheDocument();
  });

  it("shows Off by default", () => {
    renderPanel(makeCourse());
    expect(screen.getByText("Off")).toBeInTheDocument();
  });

  it("links to the single management surface in Operations", () => {
    renderPanel(makeCourse());
    const link = screen.getByRole("link", { name: "Manage in Operations" });
    expect(link).toHaveAttribute("href", "/operations?tab=active-courses");
  });

  it("renders no editor — active status is not editable here", async () => {
    const onSave = renderPanel(makeCourse());
    expect(screen.queryByRole("switch")).not.toBeInTheDocument();

    // Even clicking the status chip must not attempt a save.
    await userEvent.setup().click(screen.getByText("Off"));
    expect(onSave).not.toHaveBeenCalled();
  });
});
