import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import CourseLevels from "../CourseLevels";
import { renderWithProviders, setupCourseLevelsApi } from "./courseLevelsHarness";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

describe("CourseLevels group assignment", () => {
  beforeEach(() => mockApiJson.mockReset());

  it("moves a course to another group from its row actions", async () => {
    setupCourseLevelsApi(mockApiJson);
    render(renderWithProviders(<CourseLevels />));
    const dialog = await screen.findByRole("dialog", { name: "Manage Course Levels" });
    const row = within(dialog).getByText("MATH-101").closest("tr") as HTMLElement;
    const user = userEvent.setup();

    await user.click(within(row).getByRole("combobox"));
    await user.type(within(row).getByRole("combobox"), "SAT Verbal");
    mockApiJson.mockResolvedValueOnce({ ok: true });
    await user.click(await within(dialog).findByRole("option", { name: "SAT Verbal" }));

    await waitFor(() => expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/admin/courses/c1/root-course-group",
      expect.objectContaining({ method: "PUT", body: JSON.stringify({ root_course_group_id: "g2" }) }),
    ));
  });
});
