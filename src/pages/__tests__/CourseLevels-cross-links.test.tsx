import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import CourseLevels from "../CourseLevels";
import { renderWithProviders, setupCourseLevelsApi } from "./courseLevelsHarness";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

describe("CourseLevels manager orientation", () => {
  beforeEach(() => mockApiJson.mockReset());

  it("orients the page around one manager instead of cross-linked views", async () => {
    setupCourseLevelsApi(mockApiJson);
    render(renderWithProviders(<CourseLevels />));

    expect(await screen.findByRole("heading", { name: "Course Levels" })).toBeInTheDocument();
    expect(screen.getByText("Manage course groups, level assignments, and readiness status from one workspace.")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Manage Rules" })).not.toBeInTheDocument();
  });
});
