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

function switchControl() {
  return screen.getByRole("switch", { name: "Show MATH-101 in the student absence form" });
}

describe("CoursePropertiesPanel absence form visibility", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("defaults to shown when the flag is absent (legacy payloads) or true", () => {
    renderPanel(makeCourse());
    expect(switchControl()).toHaveAttribute("aria-checked", "true");
    expect(screen.getByText("Shown to students")).toBeInTheDocument();

    renderPanel(makeCourse({ absence_form_visible: true }));
    expect(screen.getAllByText("Shown to students").length).toBeGreaterThan(0);
  });

  it("renders the switch off with a Hidden chip when the course is hidden", () => {
    renderPanel(makeCourse({ absence_form_visible: false }));
    expect(switchControl()).toHaveAttribute("aria-checked", "false");
    expect(screen.getByText("Hidden from students")).toBeInTheDocument();
  });

  it("saves false in one click from a visible course", async () => {
    const onSave = renderPanel(makeCourse());
    const user = userEvent.setup();

    await user.click(switchControl());
    expect(onSave).toHaveBeenCalledWith("absence_form_visible", { absence_form_visible: false });
  });

  it("saves true in one click from a hidden course", async () => {
    const onSave = renderPanel(makeCourse({ absence_form_visible: false }));
    const user = userEvent.setup();

    await user.click(switchControl());
    expect(onSave).toHaveBeenCalledWith("absence_form_visible", { absence_form_visible: true });
  });

  it("locks the switch while another field is saving", () => {
    render(
      <CoursePropertiesPanel
        course={makeCourse()}
        teacherOptions={[]}
        savingField="name"
        onSave={vi.fn()}
      />,
    );
    expect(switchControl()).toBeDisabled();
  });
});
