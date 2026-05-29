import { expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import Layout from "../Layout";

const mockApiJson = vi.hoisted(() => vi.fn());
vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

vi.mock("../../hooks/useAuth", () => ({
  useAuth: () => ({ user: { username: "admin", role: "Admin" }, logout: vi.fn() }),
}));

it("shows pending absence count to administrators", async () => {
  mockApiJson.mockResolvedValueOnce({ pending_count: 12, today_count: 5 });
  render(<MemoryRouter><Layout><div>Body</div></Layout></MemoryRouter>);

  expect(await screen.findByLabelText("12 pending absences")).toBeInTheDocument();
  expect(mockApiJson).toHaveBeenCalledWith("/api/v1/absences/stats", { method: "GET" });
});
