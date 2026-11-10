import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import CourseLevels from "../CourseLevels";
import { EMPTY_SUGGESTIONS, renderWithProviders, setupCourseLevelsApi } from "./courseLevelsHarness";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

describe("CourseLevels manager orientation", () => {
  beforeEach(() => { mockApiJson.mockReset(); });

  it("orients the page around the inline manager with suggestions secondary", async () => {
    setupCourseLevelsApi(mockApiJson, { suggestions: EMPTY_SUGGESTIONS });
    render(renderWithProviders(<CourseLevels />));

    expect(await screen.findByRole("heading", { name: "Course Levels" })).toBeInTheDocument();
    expect(screen.getByText("Manage course groups, level assignments, and readiness status from one workspace.")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Manage Rules" })).not.toBeInTheDocument();

    const manager = await screen.findByRole("region", { name: "Course level manager" });
    const suggestions = screen.getByRole("region", { name: "Possible same course" });
    expect(manager.compareDocumentPosition(suggestions) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("links to Sit-in Rules for rule configuration", async () => {
    setupCourseLevelsApi(mockApiJson, { suggestions: EMPTY_SUGGESTIONS });
    render(renderWithProviders(<CourseLevels />));
    const manager = await screen.findByRole("region", { name: "Course level manager" });

    const links = screen.getAllByRole("link", { name: /Sit-in Rules/ });
    expect(links.length).toBeGreaterThan(0);
    for (const link of links) expect(link).toHaveAttribute("href", "/admin/operations?tab=sit-in-rules");
    expect(manager).toBeInTheDocument();
  });
});
