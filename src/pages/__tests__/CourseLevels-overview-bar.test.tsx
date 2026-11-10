import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import CourseLevels from "../CourseLevels";
import { COURSES, GROUPS_WITH_RULE, renderWithProviders, setupCourseLevelsApi } from "./courseLevelsHarness";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

describe("CourseLevels status summary", () => {
  beforeEach(() => { mockApiJson.mockReset(); });

  it("scopes gap counts by root group, subject, and cycle", async () => {
    const mixedCourses = [
      { ...COURSES[0], level: 1 },
      { ...COURSES[1], level: 3 },
      { ...COURSES[3], cycle_id: "cy1", cycle_label: "Cycle 2025-01", level: 2, root_course_group_id: "g1", root_course_group_name: "SAT Math" },
    ];
    setupCourseLevelsApi(mockApiJson, { courses: mixedCourses, groups: GROUPS_WITH_RULE });
    render(renderWithProviders(<CourseLevels />));

    const summary = await screen.findByRole("region", { name: "Course level status summary" });
    expect(within(summary).getByRole("group", { name: "Total courses: 3" })).toBeInTheDocument();
    expect(within(summary).getByRole("group", { name: "Assigned levels: 3" })).toBeInTheDocument();
    expect(within(summary).getByRole("group", { name: "Gaps: 1" })).toBeInTheDocument();
    expect(within(summary).getByRole("group", { name: "Not set: 0" })).toBeInTheDocument();
    expect(screen.getAllByText("Needs attention").length).toBeGreaterThan(0);
  });

  it("never marks the unassigned bucket as ready", async () => {
    setupCourseLevelsApi(mockApiJson, { groups: GROUPS_WITH_RULE });
    render(renderWithProviders(<CourseLevels />));
    const manager = await screen.findByRole("region", { name: "Course level manager" });
    const user = userEvent.setup();

    await user.click(within(manager).getByRole("button", { name: "Unassigned" }));

    expect(within(manager).getByRole("heading", { name: "Unassigned" })).toBeInTheDocument();
    expect(within(manager).getAllByText("Needs group").length).toBeGreaterThan(0);
    expect(within(manager).queryByText("Ready")).not.toBeInTheDocument();
    expect(within(manager).getByRole("button", { name: "Add level" })).toBeDisabled();
  });
});
