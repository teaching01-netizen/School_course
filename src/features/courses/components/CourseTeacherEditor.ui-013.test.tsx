import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { CourseTeacherEditor } from "./CourseTeacherEditor";
import type { EditableTeacher } from "../types";

const options = [
  { value: "teacher-1", label: "Teacher One", keywords: "teacher-1" },
  { value: "teacher-2", label: "Teacher Two", keywords: "teacher-2" },
  { value: "teacher-3", label: "Teacher Three", keywords: "teacher-3" },
];

function renderEditor(teachers: EditableTeacher[]) {
  const result = render(
    <CourseTeacherEditor teachers={teachers} onChange={vi.fn()} options={options} />,
  );
  return result;
}

describe("UI-013: Refresh after successful save matches server response", () => {
  it("displays updated teacher list from server response", () => {
    const initial: EditableTeacher[] = [
      { teacher_id: "teacher-1", is_primary: true },
      { teacher_id: "teacher-2", is_primary: false },
    ];

    const { rerender } = renderEditor(initial);

    // Verify initial state: Teacher One is primary, Teacher Two is not
    expect(screen.getByRole("radio", { name: /Teacher One/ })).toBeChecked();
    expect(screen.getByRole("radio", { name: /Teacher Two/ })).not.toBeChecked();
    expect(screen.getByText("Primary")).toBeInTheDocument();

    // After save, server returns updated teacher list:
    // B becomes primary, A is removed entirely
    const updated: EditableTeacher[] = [
      { teacher_id: "teacher-2", is_primary: true },
    ];

    rerender(
      <CourseTeacherEditor
        teachers={updated}
        onChange={vi.fn()}
        options={options}
      />,
    );

    // Verify UI reflects server response
    expect(screen.queryByRole("radio", { name: /Teacher One/ })).not.toBeInTheDocument();
    expect(screen.getByRole("radio", { name: /Teacher Two/ })).toBeChecked();
    expect(screen.getByText("Primary")).toBeInTheDocument();
    expect(screen.getAllByRole("radio")).toHaveLength(2); // B + no-primary
  });

  it("reflects new primary teacher assignment after server update", () => {
    const initial: EditableTeacher[] = [
      { teacher_id: "teacher-1", is_primary: true },
      { teacher_id: "teacher-2", is_primary: false },
    ];

    const { rerender } = renderEditor(initial);

    // Verify initial
    expect(screen.getByRole("radio", { name: /Teacher One/ })).toBeChecked();
    expect(screen.getByRole("radio", { name: /Teacher Two/ })).not.toBeChecked();

    // Server response flips primary
    const updated: EditableTeacher[] = [
      { teacher_id: "teacher-1", is_primary: false },
      { teacher_id: "teacher-2", is_primary: true },
    ];

    rerender(
      <CourseTeacherEditor
        teachers={updated}
        onChange={vi.fn()}
        options={options}
      />,
    );

    // Verify UI reflects the flip
    expect(screen.getByRole("radio", { name: /Teacher One/ })).not.toBeChecked();
    expect(screen.getByRole("radio", { name: /Teacher Two/ })).toBeChecked();
    expect(screen.getByText("Primary")).toBeInTheDocument();
  });

  it("clears all teachers when server returns empty list", () => {
    const initial: EditableTeacher[] = [
      { teacher_id: "teacher-1", is_primary: true },
    ];

    const { rerender } = renderEditor(initial);

    expect(screen.getByRole("radio", { name: /Teacher One/ })).toBeInTheDocument();

    rerender(
      <CourseTeacherEditor
        teachers={[]}
        onChange={vi.fn()}
        options={options}
      />,
    );

    expect(screen.queryByRole("radio")).not.toBeInTheDocument();
    expect(screen.queryByText("Primary teacher")).not.toBeInTheDocument();
  });
});
