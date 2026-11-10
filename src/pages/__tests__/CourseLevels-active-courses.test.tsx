import { describe, expect, it, vi, beforeEach } from "vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import CourseLevels from "../CourseLevels";
import { EMPTY_SUGGESTIONS, GROUPS_WITH_RULE, renderWithProviders, setupCourseLevelsApi } from "./courseLevelsHarness";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

describe("CourseLevels primary surface", () => {
  beforeEach(() => { mockApiJson.mockReset(); });

  it("keeps the manager as the main inline surface without a modal", async () => {
    setupCourseLevelsApi(mockApiJson, { groups: GROUPS_WITH_RULE, suggestions: EMPTY_SUGGESTIONS });
    render(renderWithProviders(<CourseLevels />));

    const manager = await screen.findByRole("region", { name: "Course level manager" });
    expect(within(manager).getByRole("heading", { name: "Course level manager" })).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Manage levels" })).not.toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Possible same course" })).toBeInTheDocument();
  });

  it("keeps the manager mounted while switching course groups", async () => {
    setupCourseLevelsApi(mockApiJson, { groups: GROUPS_WITH_RULE, suggestions: EMPTY_SUGGESTIONS });
    render(renderWithProviders(<CourseLevels />));
    const manager = await screen.findByRole("region", { name: "Course level manager" });
    const user = userEvent.setup();

    await user.click(within(manager).getByRole("button", { name: "SAT Verbal" }));
    expect(within(manager).getByRole("heading", { name: "SAT Verbal" })).toBeInTheDocument();
    expect(within(manager).getByText("No courses in this group yet.")).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("preserves a dirty draft when switching groups and shows the unsaved status", async () => {
    setupCourseLevelsApi(mockApiJson, { groups: GROUPS_WITH_RULE, suggestions: EMPTY_SUGGESTIONS });
    render(renderWithProviders(<CourseLevels />));
    const manager = await screen.findByRole("region", { name: "Course level manager" });
    const user = userEvent.setup();

    const input = within(manager).getByRole("spinbutton", { name: "Level for MATH-101" });
    fireEvent.change(input, { target: { value: "5" } });
    expect(within(manager).getByText("1 unsaved change")).toBeInTheDocument();

    await user.click(within(manager).getByRole("button", { name: "SAT Verbal" }));
    await user.click(within(manager).getByRole("button", { name: "SAT Math" }));

    expect(within(manager).getByRole("spinbutton", { name: "Level for MATH-101" })).toHaveValue(5);
    expect(within(manager).getByText("1 unsaved change")).toBeInTheDocument();
  });
});
