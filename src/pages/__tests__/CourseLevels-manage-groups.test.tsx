import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import CourseLevels from "../CourseLevels";
import { GROUPS_WITH_RULE, renderWithProviders, setupCourseLevelsApi } from "./courseLevelsHarness";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

describe("CourseLevels group operations", () => {
  beforeEach(() => { mockApiJson.mockReset(); });

  it("adds a course group and assigns a searched course from the same manager", async () => {
    setupCourseLevelsApi(mockApiJson);
    render(renderWithProviders(<CourseLevels />));
    const manager = await screen.findByRole("region", { name: "Course level manager" });
    const user = userEvent.setup();

    await user.type(within(manager).getByPlaceholderText("New group name"), "SAT Physics");
    mockApiJson.mockResolvedValueOnce({ id: "g3", name: "SAT Physics", course_count: 0, sit_in_rule_id: null });
    await user.click(within(manager).getByRole("button", { name: "Add course group" }));
    expect(await within(manager).findByRole("button", { name: "SAT Physics" })).toBeInTheDocument();

    const newGroupRule = within(manager).getByRole("combobox", { name: "Sit-in rule for SAT Physics" });
    mockApiJson.mockResolvedValueOnce({ ok: true });
    await user.selectOptions(newGroupRule, "rule-level-ladder");

    const addCourse = within(manager).getByRole("combobox", { name: "Add course to SAT Physics" });
    await user.click(addCourse);
    await user.type(addCourse, "MATH-101");
    mockApiJson.mockResolvedValueOnce({ ok: true });
    await user.click(await screen.findByRole("option", { name: /MATH-101/ }));

    await waitFor(() => expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/admin/courses/c1/root-course-group",
      expect.objectContaining({ method: "PUT", body: JSON.stringify({ root_course_group_id: "g3" }) }),
    ));
  });

  it("blocks deleting a populated group and confirms deletion of an empty group", async () => {
    setupCourseLevelsApi(mockApiJson);
    render(renderWithProviders(<CourseLevels />));
    const manager = await screen.findByRole("region", { name: "Course level manager" });
    const user = userEvent.setup();

    await user.click(within(manager).getByRole("button", { name: "Delete SAT Math" }));
    expect(within(manager).getByText(/Move 3 courses out first/)).toBeInTheDocument();
    expect(mockApiJson).not.toHaveBeenCalledWith(
      "/api/v1/admin/root-course-groups/g1",
      expect.objectContaining({ method: "DELETE" }),
    );

    await user.click(within(manager).getByRole("button", { name: "Delete SAT Verbal" }));
    const dialog = await screen.findByRole("dialog", { name: "Delete course group?" });
    expect(within(dialog).getByText(/associated absence priorities/)).toBeInTheDocument();
    mockApiJson.mockResolvedValueOnce({ ok: true });
    await user.click(within(dialog).getByRole("button", { name: "Delete group" }));

    await waitFor(() => expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/admin/root-course-groups/g2",
      expect.objectContaining({ method: "DELETE" }),
    ));
    await waitFor(() => expect(within(manager).queryByRole("button", { name: "SAT Verbal" })).not.toBeInTheDocument());
  });

  it("renames and deletes the selected group without the desktop sidebar", async () => {
    setupCourseLevelsApi(mockApiJson);
    render(renderWithProviders(<CourseLevels />));
    const manager = await screen.findByRole("region", { name: "Course level manager" });
    const settings = within(manager).getByRole("region", { name: "Course group settings" });
    const user = userEvent.setup();

    expect(within(settings).getByPlaceholderText("New group name")).toBeInTheDocument();
    await user.click(within(settings).getByRole("button", { name: "Rename selected group SAT Math" }));
    const renameInput = within(settings).getByRole("textbox", { name: "Rename selected group SAT Math" });
    await user.clear(renameInput);
    await user.type(renameInput, "SAT Math A");
    mockApiJson.mockResolvedValueOnce({ ok: true });
    await user.click(within(settings).getByRole("button", { name: "Save name for selected group SAT Math" }));

    await waitFor(() => expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/admin/root-course-groups/g1",
      expect.objectContaining({ method: "PUT", body: JSON.stringify({ name: "SAT Math A" }) }),
    ));
    expect(await within(settings).findByRole("button", { name: "Delete selected group SAT Math A" })).toBeInTheDocument();

    await user.click(within(manager).getByRole("button", { name: "SAT Verbal" }));
    await user.click(within(settings).getByRole("button", { name: "Delete selected group SAT Verbal" }));
    const dialog = await screen.findByRole("dialog", { name: "Delete course group?" });
    mockApiJson.mockResolvedValueOnce({ ok: true });
    await user.click(within(dialog).getByRole("button", { name: "Delete group" }));

    await waitFor(() => expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/admin/root-course-groups/g2",
      expect.objectContaining({ method: "DELETE" }),
    ));
  });

  it("blocks unassigning a levelled course with an explanation", async () => {
    setupCourseLevelsApi(mockApiJson, { groups: GROUPS_WITH_RULE });
    render(renderWithProviders(<CourseLevels />));
    const manager = await screen.findByRole("region", { name: "Course level manager" });
    const row = within(manager).getByText("MATH-101").closest("tr") as HTMLElement;
    const user = userEvent.setup();

    const move = within(row).getByRole("combobox", { name: "Move MATH-101 to group" });
    await user.click(move);
    await user.type(move, "Unassigned");
    await user.click(await screen.findByRole("option", { name: "— Unassigned —" }));

    expect(await within(row).findByText(/Levelled courses cannot be left unassigned/)).toBeInTheDocument();
    expect(mockApiJson).not.toHaveBeenCalledWith(
      "/api/v1/admin/courses/c1/root-course-group",
      expect.objectContaining({ method: "PUT", body: JSON.stringify({ root_course_group_id: null }) }),
    );
  });
});
