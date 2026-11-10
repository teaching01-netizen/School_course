import { describe, expect, it, vi, beforeEach } from "vitest";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import CourseLevels from "../CourseLevels";
import { COURSES, GROUPS, GROUPS_WITH_RULE, RULES, renderWithProviders, setupCourseLevelsApi } from "./courseLevelsHarness";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

describe("CourseLevels", () => {
  beforeEach(() => { mockApiJson.mockReset(); });

  it("renders one inline course-level manager with group, level, and status operations", async () => {
    setupCourseLevelsApi(mockApiJson);
    render(renderWithProviders(<CourseLevels />));

    const manager = await screen.findByRole("region", { name: "Course level manager" });
    expect(within(manager).getByRole("columnheader", { name: "Status" })).toBeInTheDocument();
    expect(within(manager).getByRole("button", { name: "Add level" })).toBeInTheDocument();
    expect(within(manager).getByRole("button", { name: "SAT Math" })).toBeInTheDocument();
    expect(await within(manager).findByText("Zoom")).toBeInTheDocument();
    expect(within(manager).getByText("Eligible")).toBeInTheDocument();
    expect(within(manager).getAllByText("Not set").length).toBeGreaterThan(0);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Manage levels" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "All levels" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Ladder" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Active courses" })).not.toBeInTheDocument();
  });

  it("saves a row-level edit through the existing course level endpoint", async () => {
    setupCourseLevelsApi(mockApiJson, { groups: GROUPS_WITH_RULE });
    render(renderWithProviders(<CourseLevels />));
    const manager = await screen.findByRole("region", { name: "Course level manager" });
    const row = within(manager).getByText("MATH-101").closest("tr");
    expect(row).not.toBeNull();

    const input = within(row as HTMLElement).getByRole("spinbutton", { name: "Level for MATH-101" });
    expect(input).toBeEnabled();
    fireEvent.change(input, { target: { value: "3" } });
    expect(input).toHaveValue(3);
    const saveButton = within(row as HTMLElement).getByRole("button", { name: "Save level for MATH-101" });
    expect(saveButton).toBeEnabled();
    mockApiJson.mockResolvedValueOnce({ ok: true });
    fireEvent.click(saveButton);

    await waitFor(() => expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/admin/courses/c1/level",
      expect.objectContaining({ method: "PUT", body: JSON.stringify({ level: 3, cycle_id: "cy1" }) }),
    ));
    await waitFor(() => expect(input).toHaveValue(3));
  });

  it("retains a row draft and exposes an inline error when the save fails", async () => {
    setupCourseLevelsApi(mockApiJson, { groups: GROUPS_WITH_RULE });
    render(renderWithProviders(<CourseLevels />));
    const manager = await screen.findByRole("region", { name: "Course level manager" });
    const row = within(manager).getByText("MATH-101").closest("tr") as HTMLElement;
    const input = within(row).getByRole("spinbutton", { name: "Level for MATH-101" });
    const saveButton = within(row).getByRole("button", { name: "Save level for MATH-101" });

    fireEvent.change(input, { target: { value: "4" } });
    mockApiJson.mockRejectedValueOnce(new Error("Level save failed"));
    fireEvent.click(saveButton);

    expect(await within(row).findByText("Level save failed")).toBeInTheDocument();
    expect(input).toHaveValue(4);
    expect(saveButton).toBeEnabled();

    mockApiJson.mockResolvedValueOnce({ ok: true });
    fireEvent.click(saveButton);
    await waitFor(() => expect(saveButton).toBeDisabled());
    expect(input).toHaveValue(4);
  });

  it("surfaces a missing rule and assigns one from the selected course group", async () => {
    setupCourseLevelsApi(mockApiJson, { groups: GROUPS, rules: RULES });
    render(renderWithProviders(<CourseLevels />));

    const manager = await screen.findByRole("region", { name: "Course level manager" });
    expect(within(manager).getAllByText("Rule needed").length).toBeGreaterThan(0);
    expect(within(manager).getByRole("button", { name: "Add level" })).toBeDisabled();

    const ruleSelect = within(manager).getByRole("combobox", { name: "Sit-in rule for SAT Math" });
    expect(ruleSelect).toHaveValue("");
    fireEvent.change(ruleSelect, { target: { value: "rule-level-ladder" } });

    await waitFor(() => expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/admin/root-course-groups/g1",
      expect.objectContaining({ method: "PUT", body: JSON.stringify({ sit_in_rule_id: "rule-level-ladder" }) }),
    ));
    await waitFor(() => expect(ruleSelect).toHaveValue("rule-level-ladder"));
    expect(within(manager).getByText("Configured: Level Ladder")).toBeInTheDocument();
  });

  it("rejects a duplicate level used by a course in another root group", async () => {
    const crossGroupCourses = [
      ...COURSES,
      { ...COURSES[3], id: "c5", code: "MATH-202", name: "Algebra II (split group)", subject_id: "s1", subject_code: "MATH", subject_name: "Mathematics", cycle_id: "cy1", cycle_label: "Cycle 2025-01", level: 5, root_course_group_id: "g2", root_course_group_name: "SAT Verbal" },
    ];
    setupCourseLevelsApi(mockApiJson, { courses: crossGroupCourses, groups: GROUPS_WITH_RULE });
    render(renderWithProviders(<CourseLevels />));
    const manager = await screen.findByRole("region", { name: "Course level manager" });
    const user = userEvent.setup();

    const courseSelect = within(manager).getByRole("combobox", { name: "Course for new level" });
    await user.click(courseSelect);
    await user.type(courseSelect, "MATH-301");
    await user.click(await screen.findByRole("option", { name: /MATH-301/ }));
    const levelInput = within(manager).getByRole("spinbutton", { name: "Level" });
    fireEvent.change(levelInput, { target: { value: "5" } });
    await user.click(within(manager).getByRole("button", { name: "Add level" }));

    expect((await within(manager).findAllByText(/Level 5 is already used by MATH-202/)).length).toBeGreaterThan(0);
    expect(mockApiJson).not.toHaveBeenCalledWith(
      "/api/v1/admin/courses/c3/level",
      expect.objectContaining({ method: "PUT" }),
    );
  });

  it("never marks a group Ready when a levelled course has no cycle", async () => {
    const courses = [
      { ...COURSES[0], cycle_id: "", cycle_label: "", level: 1 },
      { ...COURSES[1], level: 2 },
      { ...COURSES[2], level: 3 },
    ];
    setupCourseLevelsApi(mockApiJson, { courses, groups: GROUPS_WITH_RULE });
    render(renderWithProviders(<CourseLevels />));
    const manager = await screen.findByRole("region", { name: "Course level manager" });

    expect(within(manager).getAllByText("Needs attention").length).toBeGreaterThan(0);
    expect(within(manager).queryByText("Ready")).not.toBeInTheDocument();
    expect(within(manager).getByText("A level is set without a cycle")).toBeInTheDocument();
    const setupLinks = within(manager).getAllByRole("link", { name: /Open course setup for MATH-101/ });
    expect(setupLinks.length).toBeGreaterThan(0);
    for (const link of setupLinks) expect(link).toHaveAttribute("href", "/courses/c1");
  });

  it("keeps the page heading visible while loading and retries a failed required load", async () => {
    setupCourseLevelsApi(mockApiJson, { failures: { courses: "Network unavailable" } });
    const user = userEvent.setup();
    render(renderWithProviders(<CourseLevels />));

    expect(screen.getByRole("heading", { name: "Course Levels" })).toBeInTheDocument();
    expect(await screen.findByText("Course levels could not be loaded.")).toBeInTheDocument();
    expect(screen.getByText("Network unavailable")).toBeInTheDocument();

    setupCourseLevelsApi(mockApiJson);
    await user.click(screen.getByRole("button", { name: "Retry loading course levels" }));
    expect(await screen.findByRole("region", { name: "Course level manager" })).toBeInTheDocument();
    expect(screen.queryByText("Course levels could not be loaded.")).not.toBeInTheDocument();
  });

  it("preserves an existing rule assignment when the rule list fails and exposes a retry", async () => {
    setupCourseLevelsApi(mockApiJson, { groups: GROUPS_WITH_RULE, failures: { rules: "Rule service unavailable" } });
    const user = userEvent.setup();
    render(renderWithProviders(<CourseLevels />));

    const manager = await screen.findByRole("region", { name: "Course level manager" });
    expect(within(manager).getByText("Sit-in rules could not be loaded.")).toBeInTheDocument();
    expect(within(manager).getByText(/Existing rule assignments are preserved/)).toBeInTheDocument();
    const ruleSelect = within(manager).getByRole("combobox", { name: "Sit-in rule for SAT Math" });
    expect(ruleSelect).toBeDisabled();
    expect(ruleSelect).toHaveValue("rule-level-ladder");

    setupCourseLevelsApi(mockApiJson, { groups: GROUPS_WITH_RULE, rules: RULES });
    await user.click(within(manager).getByRole("button", { name: "Retry sit-in rules" }));
    await waitFor(() => expect(ruleSelect).toBeEnabled());
    expect(within(manager).queryByText("Sit-in rules could not be loaded.")).not.toBeInTheDocument();
    expect(ruleSelect).toHaveValue("rule-level-ladder");
  });
});
