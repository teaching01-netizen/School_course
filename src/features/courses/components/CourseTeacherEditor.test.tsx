import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { CourseTeacherEditor } from "./CourseTeacherEditor";
import type { EditableTeacher } from "../types";

const options = [
  { value: "teacher-1", label: "Teacher One", keywords: "teacher-1" },
  { value: "teacher-2", label: "Teacher Two", keywords: "teacher-2" },
  { value: "teacher-3", label: "Teacher Three", keywords: "teacher-3" },
];

function renderEditor(teachers: EditableTeacher[], onChange = vi.fn()) {
  render(<CourseTeacherEditor teachers={teachers} onChange={onChange} options={options} />);
  return onChange;
}

describe("CourseTeacherEditor", () => {
  it("renders a radio per assigned teacher, a Primary badge, and a No primary option", () => {
    renderEditor([
      { teacher_id: "teacher-1", is_primary: true },
      { teacher_id: "teacher-2", is_primary: false },
    ]);

    expect(screen.getByRole("radio", { name: /Teacher One/ })).toBeInTheDocument();
    expect(screen.getByRole("radio", { name: /Teacher Two/ })).toBeInTheDocument();
    expect(screen.getByText("Primary")).toBeInTheDocument();
    expect(screen.getByRole("radio", { name: /No primary teacher/ })).toBeInTheDocument();
    expect(screen.getAllByRole("radio")).toHaveLength(3);
  });

  it("renders no radios when no teachers are assigned", () => {
    renderEditor([]);
    expect(screen.queryByRole("radio")).not.toBeInTheDocument();
    expect(screen.queryByText("Primary teacher")).not.toBeInTheDocument();
  });

  it("sets primary teacher via radio", async () => {
    const onChange = renderEditor([
      { teacher_id: "teacher-1", is_primary: true },
      { teacher_id: "teacher-2", is_primary: false },
    ]);
    const user = userEvent.setup();

    await user.click(screen.getByRole("radio", { name: /Teacher Two/ }));

    expect(onChange).toHaveBeenCalledWith([
      { teacher_id: "teacher-1", is_primary: false },
      { teacher_id: "teacher-2", is_primary: true },
    ]);
  });

  it("switches primary between teachers", async () => {
    const onChange = renderEditor([
      { teacher_id: "teacher-1", is_primary: false },
      { teacher_id: "teacher-2", is_primary: true },
    ]);
    const user = userEvent.setup();

    await user.click(screen.getByRole("radio", { name: /Teacher One/ }));

    expect(onChange).toHaveBeenCalledWith([
      { teacher_id: "teacher-1", is_primary: true },
      { teacher_id: "teacher-2", is_primary: false },
    ]);
  });

  it("removing the primary teacher resets primary to none while retaining other teachers", async () => {
    const onChange = renderEditor([
      { teacher_id: "teacher-1", is_primary: true },
      { teacher_id: "teacher-2", is_primary: false },
    ]);
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: "Remove Teacher One" }));

    expect(onChange).toHaveBeenCalledWith([{ teacher_id: "teacher-2", is_primary: false }]);
  });

  it("choosing No primary teacher clears the primary flag", async () => {
    const onChange = renderEditor([
      { teacher_id: "teacher-1", is_primary: true },
      { teacher_id: "teacher-2", is_primary: false },
    ]);
    const user = userEvent.setup();

    await user.click(screen.getByRole("radio", { name: /No primary teacher/ }));

    expect(onChange).toHaveBeenCalledWith([
      { teacher_id: "teacher-1", is_primary: false },
      { teacher_id: "teacher-2", is_primary: false },
    ]);
  });

  it("defaults a newly added teacher to non-primary", async () => {
    const onChange = renderEditor([{ teacher_id: "teacher-1", is_primary: true }]);
    const user = userEvent.setup();

    await user.click(screen.getByRole("combobox"));
    await user.click(await screen.findByRole("option", { name: "Teacher Three" }));

    expect(onChange).toHaveBeenCalledWith([
      { teacher_id: "teacher-1", is_primary: true },
      { teacher_id: "teacher-3", is_primary: false },
    ]);
  });
});
