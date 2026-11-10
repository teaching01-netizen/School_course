import { describe, expect, it, vi, beforeEach } from "vitest";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import CourseLevels from "../CourseLevels";
import { GROUPS_WITH_RULE, renderWithProviders, setupCourseLevelsApi } from "./courseLevelsHarness";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

describe("CourseLevels add level", () => {
  beforeEach(() => { mockApiJson.mockReset(); });

  it("requires an explicit course selection before adding a level", async () => {
    setupCourseLevelsApi(mockApiJson, { groups: GROUPS_WITH_RULE });
    render(renderWithProviders(<CourseLevels />));
    const manager = await screen.findByRole("region", { name: "Course level manager" });
    const user = userEvent.setup();

    expect(within(manager).getByRole("button", { name: "Add level" })).toBeDisabled();
    const courseSelect = within(manager).getByRole("combobox", { name: "Course for new level" });
    await user.click(courseSelect);
    await user.type(courseSelect, "MATH-301");
    await user.click(await screen.findByRole("option", { name: /MATH-301/ }));

    expect(within(manager).getByRole("spinbutton", { name: "Level" })).toHaveValue(3);
    mockApiJson.mockResolvedValueOnce({ ok: true });
    await user.click(within(manager).getByRole("button", { name: "Add level" }));

    await waitFor(() => expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/admin/courses/c3/level",
      expect.objectContaining({ method: "PUT", body: JSON.stringify({ level: 3, cycle_id: "cy1" }) }),
    ));
    await waitFor(() => expect(within(manager).getByRole("combobox", { name: "Course for new level" })).toHaveValue(""));
    expect(within(manager).getByRole("spinbutton", { name: "Level" })).toHaveValue(null);
  });

  it("retains the add-level form after a failed save and clears it only after success", async () => {
    setupCourseLevelsApi(mockApiJson, { groups: GROUPS_WITH_RULE });
    render(renderWithProviders(<CourseLevels />));
    const manager = await screen.findByRole("region", { name: "Course level manager" });
    const user = userEvent.setup();

    const courseSelect = within(manager).getByRole("combobox", { name: "Course for new level" });
    await user.click(courseSelect);
    await user.type(courseSelect, "MATH-301");
    await user.click(await screen.findByRole("option", { name: /MATH-301/ }));

    mockApiJson.mockRejectedValueOnce(new Error("Save failed"));
    await user.click(within(manager).getByRole("button", { name: "Add level" }));

    expect((await within(manager).findAllByText("Save failed")).length).toBeGreaterThan(0);
    expect(courseSelect).toHaveValue("MATH-301 — Algebra III");
    expect(within(manager).getByRole("spinbutton", { name: "Level" })).toHaveValue(3);

    mockApiJson.mockResolvedValueOnce({ ok: true });
    await user.click(within(manager).getByRole("button", { name: "Add level" }));
    await waitFor(() => expect(within(manager).getByRole("combobox", { name: "Course for new level" })).toHaveValue(""));
  });

  it("preserves dirty drafts in other rows when a course save updates the course list", async () => {
    setupCourseLevelsApi(mockApiJson, { groups: GROUPS_WITH_RULE });
    render(renderWithProviders(<CourseLevels />));
    const manager = await screen.findByRole("region", { name: "Course level manager" });
    const user = userEvent.setup();

    const firstLevel = within(manager).getByRole("spinbutton", { name: "Level for MATH-101" });
    const secondLevel = within(manager).getByRole("spinbutton", { name: "Level for MATH-201" });
    fireEvent.change(firstLevel, { target: { value: "5" } });
    fireEvent.change(secondLevel, { target: { value: "6" } });

    const courseSelect = within(manager).getByRole("combobox", { name: "Course for new level" });
    await user.click(courseSelect);
    await user.type(courseSelect, "MATH-301");
    await user.click(await screen.findByRole("option", { name: /MATH-301/ }));
    mockApiJson.mockResolvedValueOnce({ ok: true });
    await user.click(within(manager).getByRole("button", { name: "Add level" }));

    await waitFor(() => expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/admin/courses/c3/level",
      expect.objectContaining({ method: "PUT" }),
    ));
    expect(firstLevel).toHaveValue(5);
    expect(secondLevel).toHaveValue(6);
    expect(within(manager).getByText("2 unsaved changes")).toBeInTheDocument();
  });
});
