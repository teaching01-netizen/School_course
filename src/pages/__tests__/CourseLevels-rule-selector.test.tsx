import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import CourseLevels from "../CourseLevels";
import { COURSES, GROUPS, GROUPS_WITH_RULE, RULES, renderWithProviders, setupCourseLevelsApi } from "./courseLevelsHarness";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

describe("CourseLevels rule and membership guards", () => {
  beforeEach(() => { mockApiJson.mockReset(); });

  it("blocks moving a levelled course into a group without a sit-in rule", async () => {
    setupCourseLevelsApi(mockApiJson, { groups: GROUPS });
    render(renderWithProviders(<CourseLevels />));
    const manager = await screen.findByRole("region", { name: "Course level manager" });
    const row = within(manager).getByText("MATH-101").closest("tr") as HTMLElement;
    const user = userEvent.setup();

    const move = within(row).getByRole("combobox", { name: "Move MATH-101 to group" });
    await user.click(move);
    await user.type(move, "SAT Verbal");
    await user.click(await screen.findByRole("option", { name: "SAT Verbal" }));

    expect(await within(row).findByText(/Assign a sit-in rule before moving a levelled course/)).toBeInTheDocument();
    expect(mockApiJson).not.toHaveBeenCalledWith(
      "/api/v1/admin/courses/c1/root-course-group",
      expect.objectContaining({ method: "PUT", body: JSON.stringify({ root_course_group_id: "g2" }) }),
    );
  });

  it("disables a row whose course has no cycle and links to course setup", async () => {
    const courses = [{ ...COURSES[0], cycle_id: "", cycle_label: "" }, ...COURSES.slice(1)];
    setupCourseLevelsApi(mockApiJson, { courses, groups: GROUPS_WITH_RULE });
    render(renderWithProviders(<CourseLevels />));
    const manager = await screen.findByRole("region", { name: "Course level manager" });
    const row = within(manager).getByText("MATH-101").closest("tr") as HTMLElement;

    expect(within(row).getByRole("spinbutton", { name: "Level for MATH-101" })).toBeDisabled();
    expect(within(row).getByText(/Add a cycle to this course/)).toBeInTheDocument();
    const setupLinks = within(manager).getAllByRole("link", { name: "Open course setup for MATH-101" });
    expect(setupLinks.length).toBeGreaterThan(0);
    for (const link of setupLinks) expect(link).toHaveAttribute("href", "/courses/c1");
  });

  it("retains the rule selection when the rule mutation fails", async () => {
    setupCourseLevelsApi(mockApiJson, { groups: GROUPS, rules: RULES });
    render(renderWithProviders(<CourseLevels />));
    const manager = await screen.findByRole("region", { name: "Course level manager" });
    const user = userEvent.setup();

    const ruleSelect = within(manager).getByRole("combobox", { name: "Sit-in rule for SAT Math" });
    mockApiJson.mockRejectedValueOnce(new Error("Rule update failed"));
    await user.selectOptions(ruleSelect, "rule-level-ladder");

    expect(await within(manager).findByText("Rule update failed")).toBeInTheDocument();
    expect(ruleSelect).toHaveValue("");
    expect(within(manager).getByRole("button", { name: "Add level" })).toBeDisabled();

    mockApiJson.mockResolvedValueOnce({ ok: true });
    await user.selectOptions(ruleSelect, "rule-level-ladder");
    await waitFor(() => expect(ruleSelect).toHaveValue("rule-level-ladder"));
    expect(within(manager).queryByText("Rule update failed")).not.toBeInTheDocument();
  });
});
