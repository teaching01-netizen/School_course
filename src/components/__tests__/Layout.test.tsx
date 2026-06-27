import { beforeEach, expect, it, vi } from "vitest";
import { act, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { QueryClientProvider } from "@tanstack/react-query";
import Layout from "../Layout";
import { queryClient } from "@/query/cache";
import { applyRealtimeEvent } from "@/realtime/queryBridge";

function TestWrapper({ children }: { children: React.ReactNode }) {
  return <QueryClientProvider client={queryClient}><MemoryRouter>{children}</MemoryRouter></QueryClientProvider>;
}

const mockApiJson = vi.hoisted(() => vi.fn());
vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

vi.mock("../../hooks/useAuth", () => ({
  useAuth: () => ({ user: { username: "admin", role: "Admin" }, logout: vi.fn() }),
}));

beforeEach(() => {
  mockApiJson.mockReset();
  queryClient.clear();
});

it("shows pending absence count to administrators", async () => {
  mockApiJson.mockResolvedValueOnce({ pending_count: 12, today_count: 5 });
  render(<TestWrapper><Layout><div>Body</div></Layout></TestWrapper>);

  expect(await screen.findByLabelText("12 pending absences")).toBeInTheDocument();
  expect(mockApiJson).toHaveBeenCalledWith("/api/v1/absences/stats", { method: "GET" });
});

it("refetches authoritative stats instead of installing an event payload", async () => {
  mockApiJson
    .mockResolvedValueOnce({ pending_count: 12, reviewed_count: 0, actioned_count: 0, cancelled_count: 0 })
    .mockResolvedValueOnce({ pending_count: 15, reviewed_count: 0, actioned_count: 0, cancelled_count: 0 });
  render(<TestWrapper><Layout><div>Body</div></Layout></TestWrapper>);
  expect(await screen.findByLabelText("12 pending absences")).toBeInTheDocument();

  await act(async () => {
    await applyRealtimeEvent(queryClient, {
      type: "absent.stats.updated",
      channel: "absent:stats",
      payload: { pending_count: 3, reviewed_count: 0, actioned_count: 0, cancelled_count: 0 },
    });
  });

  expect(await screen.findByLabelText("15 pending absences")).toBeInTheDocument();
  expect(screen.queryByLabelText("3 pending absences")).not.toBeInTheDocument();
});
