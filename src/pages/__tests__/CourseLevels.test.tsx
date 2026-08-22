import { describe, expect, it, vi, beforeEach } from "vitest";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import CourseLevels from "../CourseLevels";
import { GROUPS, GROUPS_WITH_RULE, RULES, renderWithProviders, setupCourseLevelsApi } from "./courseLevelsHarness";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

describe("CourseLevels", () => {
  beforeEach(() => mockApiJson.mockReset());

  it("opens one course-level manager with group, level, and status operations", async () => {
    setupCourseLevelsApi(mockApiJson);
    render(renderWithProviders(<CourseLevels />));

    const dialog = await screen.findByRole("dialog", { name: "Manage Course Levels" });
    expect(within(dialog).getByRole("columnheader", { name: "Status" })).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Add level" })).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "SAT Math" })).toBeInTheDocument();
    expect(await within(dialog).findByText("Zoom")).toBeInTheDocument();
    expect(within(dialog).getByText("Eligible")).toBeInTheDocument();
    expect(within(dialog).getAllByText("Not set").length).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: "All levels" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Ladder" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Active courses" })).not.toBeInTheDocument();
  });

  it("saves a row-level edit through the existing course level endpoint", async () => {
    setupCourseLevelsApi(mockApiJson, undefined, GROUPS_WITH_RULE);
    render(renderWithProviders(<CourseLevels />));
    const dialog = await screen.findByRole("dialog", { name: "Manage Course Levels" });
    const row = within(dialog).getByText("MATH-101").closest("tr");
    expect(row).not.toBeNull();

    const input = within(row as HTMLElement).getByRole("spinbutton", { name: "Level for MATH-101" });
    fireEvent.change(input, { target: { value: "3" } });
    mockApiJson.mockResolvedValueOnce({ ok: true });
    fireEvent.click(within(row as HTMLElement).getByRole("button", { name: "Save" }));

    await waitFor(() => expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/admin/courses/c1/level",
      expect.objectContaining({ method: "PUT", body: JSON.stringify({ level: 3, cycle_id: "cy1" }) }),
    ));
  });

  it("surfaces a missing rule and assigns one from the selected course group", async () => {
    setupCourseLevelsApi(mockApiJson, undefined, GROUPS, RULES);
    render(renderWithProviders(<CourseLevels />));

    const dialog = await screen.findByRole("dialog", { name: "Manage Course Levels" });
    expect(within(dialog).getByText("Rule needed")).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Add level" })).toBeDisabled();

    const ruleSelect = within(dialog).getByRole("combobox", { name: "Sit-in rule for SAT Math" });
    expect(ruleSelect).toHaveValue("");
    fireEvent.change(ruleSelect, { target: { value: "rule-level-ladder" } });

    await waitFor(() => expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/admin/root-course-groups/g1",
      expect.objectContaining({ method: "PUT", body: JSON.stringify({ sit_in_rule_id: "rule-level-ladder" }) }),
    ));
  });
});
