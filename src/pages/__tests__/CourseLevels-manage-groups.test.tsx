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

describe("CourseLevels group operations", () => {
  beforeEach(() => mockApiJson.mockReset());

  it("adds a course group and assigns a searched course from the same manager", async () => {
    setupCourseLevelsApi(mockApiJson);
    render(renderWithProviders(<CourseLevels />));
    const dialog = await screen.findByRole("dialog", { name: "Manage Course Levels" });
    const user = userEvent.setup();

    await user.type(within(dialog).getByPlaceholderText("New group name"), "SAT Physics");
    mockApiJson.mockResolvedValueOnce({ id: "g3", name: "SAT Physics", course_count: 0, sit_in_rule_id: null });
    await user.click(within(dialog).getByRole("button", { name: "Add" }));
    expect(await within(dialog).findByRole("button", { name: "SAT Physics" })).toBeInTheDocument();

    const addCourse = within(dialog).getAllByRole("combobox")[1];
    await user.click(addCourse);
    await user.type(addCourse, "MATH-101");
    mockApiJson.mockResolvedValueOnce({ ok: true });
    await user.click(await within(dialog).findByRole("option", { name: /MATH-101/ }));

    await waitFor(() => expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/admin/courses/c1/root-course-group",
      expect.objectContaining({ method: "PUT", body: JSON.stringify({ root_course_group_id: "g3" }) }),
    ));
  });
});
