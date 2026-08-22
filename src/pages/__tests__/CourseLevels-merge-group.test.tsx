import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import CourseLevels from "../CourseLevels";
import { renderWithProviders, setupCourseLevelsApi } from "./courseLevelsHarness";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

describe("CourseLevels add level", () => {
  beforeEach(() => mockApiJson.mockReset());

  it("adds a level to the selected unassigned course", async () => {
    setupCourseLevelsApi(mockApiJson);
    render(renderWithProviders(<CourseLevels />));
    const dialog = await screen.findByRole("dialog", { name: "Manage Course Levels" });
    const user = userEvent.setup();

    mockApiJson.mockResolvedValueOnce({ ok: true });
    await user.click(screen.getByRole("button", { name: "Add level" }));

    await waitFor(() => expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/admin/courses/c3/level",
      expect.objectContaining({ method: "PUT", body: JSON.stringify({ level: 3, cycle_id: "cy1" }) }),
    ));
    expect(dialog).toHaveTextContent("MATH-301");
  });
});
