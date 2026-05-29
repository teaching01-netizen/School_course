import { describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import SlotFinder from "../SlotFinder";
import { ToastProvider } from "../../hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());
const mockFindAvailableSlots = vi.hoisted(() => vi.fn());
vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson, findAvailableSlots: mockFindAvailableSlots };
});

describe("SlotFinder empty state", () => {
  it("shows EmptyState after search returns no slots", async () => {
    mockApiJson.mockResolvedValueOnce([{ id: "s1", wcode: "W001", full_name: "Test Student" }]);
    mockApiJson.mockResolvedValueOnce([{ id: "c1", code: "MATH", name: "Math" }]);
    mockFindAvailableSlots.mockResolvedValueOnce({ slots: [] });
    render(
      <MemoryRouter>
        <ToastProvider>
          <SlotFinder />
        </ToastProvider>
      </MemoryRouter>,
    );

    const user = userEvent.setup();
    // Wait for lookups to load
    await waitFor(() => {
      expect(screen.getByText("W001 — Test Student")).toBeInTheDocument();
    });
    const selects = screen.getAllByRole("combobox");
    // selects[0] = student, selects[1] = course
    await user.selectOptions(selects[0], "s1");
    await user.selectOptions(selects[1], "c1");
    await user.click(screen.getByRole("button", { name: /find slots/i }));

    expect(await screen.findByText(/No slots found in this range/)).toBeInTheDocument();
  });
});
