import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import Availability from "../Availability";
import { ToastProvider } from "../../hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());
vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

describe("Availability empty state", () => {
  it("shows EmptyState when teachers exist but have no windows", async () => {
    mockApiJson.mockResolvedValueOnce([{ id: "t1", username: "Mr. Smith", role: "Teacher" }]);
    mockApiJson.mockResolvedValueOnce([{ id: "r1", name: "Room A", capacity: 30 }]);
    mockApiJson.mockResolvedValueOnce([]);
    render(
      <MemoryRouter>
        <ToastProvider>
          <Availability />
        </ToastProvider>
      </MemoryRouter>,
    );

    expect(await screen.findByText(/No availability windows configured/)).toBeInTheDocument();
  });
});
