import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import CourseLevels from "../CourseLevels";
import { renderWithProviders, setupCourseLevelsApi } from "./courseLevelsHarness";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

describe("CourseLevels primary surface", () => {
  beforeEach(() => mockApiJson.mockReset());

  it("keeps the manager as the main surface and supports reopening after close", async () => {
    setupCourseLevelsApi(mockApiJson);
    render(renderWithProviders(<CourseLevels />));
    const user = userEvent.setup();

    expect(await screen.findByRole("dialog", { name: "Manage Course Levels" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Close dialog" }));
    expect(screen.queryByRole("dialog", { name: "Manage Course Levels" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Manage levels" }));
    expect(await screen.findByRole("dialog", { name: "Manage Course Levels" })).toBeInTheDocument();
  });
});
