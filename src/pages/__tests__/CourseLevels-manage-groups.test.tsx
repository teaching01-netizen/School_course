import { describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import CourseLevels from "../CourseLevels";
import { ToastProvider } from "../../hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

const courses = [
  {
    id: "course-1",
    code: "MATH-101",
    name: "Algebra I",
    subject_id: "subject-math",
    subject_code: "MATH",
    subject_name: "Mathematics",
    cycle_id: "cycle-1",
    cycle_label: "Cycle 2025-01",
    level: 1,
    root_course_group_id: "group-math",
    root_course_group_name: "SAT Math",
  },
  {
    id: "course-2",
    code: "PHYS-201",
    name: "Physics II",
    subject_id: "subject-physics",
    subject_code: "PHYS",
    subject_name: "Physics",
    cycle_id: "cycle-1",
    cycle_label: "Cycle 2025-01",
    level: 2,
    root_course_group_id: null,
    root_course_group_name: null,
  },
];

const groups = [
  { id: "group-math", name: "SAT Math", course_count: 1, sit_in_rule_id: null },
  { id: "group-verbal", name: "SAT Verbal", course_count: 0, sit_in_rule_id: null },
];

function setupPage() {
  mockApiJson
    .mockResolvedValueOnce([])
    .mockResolvedValueOnce(courses)
    .mockResolvedValueOnce({ absence_policies: { root_course_groups: {} } })
    .mockResolvedValueOnce(groups)
    .mockResolvedValueOnce({ subjects: [] })
    .mockResolvedValueOnce([])
    .mockResolvedValueOnce(groups)
    .mockResolvedValueOnce(courses)
    .mockResolvedValueOnce({ ok: true });
}

describe("CourseLevels manage groups", () => {
  it("centers all groups and assigns a globally searched course by subject name", async () => {
    setupPage();
    render(<MemoryRouter><ToastProvider><CourseLevels /></ToastProvider></MemoryRouter>);
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: "All levels" }));
    await user.click(screen.getByRole("button", { name: "Manage groups" }));

    const dialog = await screen.findByRole("dialog", { name: "Manage Groups" });
    expect(dialog).toHaveClass("modal-base");
    expect(within(dialog).getByRole("button", { name: "Select group SAT Math" })).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Select group SAT Verbal" })).toBeInTheDocument();

    await user.click(within(dialog).getByRole("button", { name: "Select group SAT Math" }));
    const picker = within(dialog).getByRole("combobox", { name: "Add course to SAT Math" });
    await user.type(picker, "Physics");

    const courseOption = await within(dialog).findByRole("option", { name: /PHYS-201 — Physics/ });
    expect(courseOption).toBeInTheDocument();

    await user.click(courseOption);

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/admin/courses/course-2/root-course-group",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ root_course_group_id: "group-math" }),
        }),
      );
    });
    expect(within(dialog).getByText("PHYS-201")).toBeInTheDocument();
  });
});
