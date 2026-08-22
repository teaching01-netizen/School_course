import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, within } from "@testing-library/react";
import CourseLevels from "../CourseLevels";
import { COURSES, GROUPS, renderWithProviders, setupCourseLevelsApi } from "./courseLevelsHarness";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

describe("CourseLevels status summary", () => {
  beforeEach(() => mockApiJson.mockReset());

  it("shows counts for assigned levels, gaps, and unset courses", async () => {
    const gapCourses = [
      { ...COURSES[0], level: 1 },
      { ...COURSES[1], level: 3 },
      { ...COURSES[2], level: null },
      COURSES[3],
    ];
    setupCourseLevelsApi(mockApiJson, gapCourses, GROUPS);
    render(renderWithProviders(<CourseLevels />));

    const summary = await screen.findByRole("region", { name: "Course level status summary" });
    expect(summary).toHaveTextContent("4");
    expect(within(summary).getByText("Assigned levels")).toBeInTheDocument();
    expect(within(summary).getByText("Gaps")).toBeInTheDocument();
    expect(within(summary).getByText("Not set")).toBeInTheDocument();
    expect(screen.getByText("Needs attention")).toBeInTheDocument();
  });
});
