import { beforeEach, expect, it, vi } from "vitest";
import { act, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { QueryClientProvider } from "@tanstack/react-query";
import Layout from "../Layout";
import { queryClient } from "@/query/cache";
import { applyRealtimeEvent } from "@/realtime/queryBridge";

function TestWrapper({ children }: { children: React.ReactNode }) {
  return <QueryClientProvider client={queryClient}><MemoryRouter>{children}</MemoryRouter></QueryClientProvider>;
}

const mockApiJson = vi.hoisted(() => vi.fn());
const mockUseAuth = vi.hoisted(() => vi.fn());
vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

vi.mock("../../hooks/useAuth", () => ({
  useAuth: mockUseAuth,
}));

beforeEach(() => {
  mockApiJson.mockReset();
  mockUseAuth.mockReturnValue({ user: { username: "admin", role: "Admin" }, logout: vi.fn() });
  queryClient.clear();
});

it("shows pending absence count to administrators", async () => {
  mockApiJson.mockResolvedValueOnce({ pending_count: 12, today_count: 5 });
  render(<TestWrapper><Layout><div>Body</div></Layout></TestWrapper>);

  expect(await screen.findByLabelText("12 pending absences")).toBeInTheDocument();
  expect(mockApiJson).toHaveBeenCalledWith("/api/v1/absences/stats", { method: "GET" });
});

it("refetches authoritative stats instead of installing an event payload", async () => {
  const respondWith = (pending: number) => {
    mockApiJson.mockImplementation((url: string) => {
      if (url.includes("/absences/stats")) return Promise.resolve({ pending_count: pending, reviewed_count: 0, actioned_count: 0, cancelled_count: 0 });
      return Promise.reject(new Error(`unexpected apiJson call: ${url}`));
    });
  };
  respondWith(12);
  render(<TestWrapper><Layout><div>Body</div></Layout></TestWrapper>);
  expect(await screen.findByLabelText("12 pending absences")).toBeInTheDocument();

  respondWith(15);

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

it("keeps the teacher shell scoped to the teacher dashboard", () => {
  mockUseAuth.mockReturnValue({ user: { username: "teacher", role: "Teacher" }, logout: vi.fn() });
  render(<TestWrapper><Layout><div>Body</div></Layout></TestWrapper>);

  expect(screen.getByRole("link", { name: "Dashboard" })).toHaveAttribute("href", "/teacher-dashboard");
  expect(screen.queryByText("Courses")).not.toBeInTheDocument();
  expect(screen.queryByText("Students")).not.toBeInTheDocument();
  expect(screen.queryByText("Users")).not.toBeInTheDocument();
});

it("renders the Schedule Impact link for admins", () => {
  mockApiJson.mockImplementation((path: string) => {
    if (path.startsWith("/api/v1/absences/stats")) return Promise.resolve({ pending_count: 0, today_count: 0 });
    if (path.startsWith("/api/v1/operations/schedule-impact")) return Promise.resolve({ summary: { critical: 0, need_attention: 0 } });
    return Promise.resolve({});
  });
  render(<TestWrapper><Layout><div>Body</div></Layout></TestWrapper>);

  expect(screen.getByRole("link", { name: "Schedule Impact" })).toHaveAttribute("href", "/operations/schedule-impact");
});

it("shows a critical schedule impact badge", async () => {
  mockApiJson.mockImplementation((path: string) => {
    if (path.startsWith("/api/v1/absences/stats")) return Promise.resolve({ pending_count: 0, today_count: 0 });
    if (path.startsWith("/api/v1/operations/schedule-impact")) return Promise.resolve({ summary: { critical: 3, need_attention: 0 } });
    return Promise.resolve({});
  });
  render(<TestWrapper><Layout><div>Body</div></Layout></TestWrapper>);

  expect(await screen.findByLabelText("3 critical schedule impacts")).toBeInTheDocument();
});

it("shows an unresolved schedule impact badge when there are no critical impacts", async () => {
  mockApiJson.mockImplementation((path: string) => {
    if (path.startsWith("/api/v1/absences/stats")) return Promise.resolve({ pending_count: 0, today_count: 0 });
    if (path.startsWith("/api/v1/operations/schedule-impact")) return Promise.resolve({ summary: { critical: 0, need_attention: 5 } });
    return Promise.resolve({});
  });
  render(<TestWrapper><Layout><div>Body</div></Layout></TestWrapper>);

  expect(await screen.findByLabelText("5 unresolved schedule impacts")).toBeInTheDocument();
});

it("shows no schedule impact badge when both counts are zero", async () => {
  mockApiJson.mockImplementation((path: string) => {
    if (path.startsWith("/api/v1/absences/stats")) return Promise.resolve({ pending_count: 0, today_count: 0 });
    if (path.startsWith("/api/v1/operations/schedule-impact")) return Promise.resolve({ summary: { critical: 0, need_attention: 0 } });
    return Promise.resolve({});
  });
  render(<TestWrapper><Layout><div>Body</div></Layout></TestWrapper>);

  await waitFor(() => expect(mockApiJson).toHaveBeenCalledWith(expect.stringContaining("/api/v1/operations/schedule-impact")));
  expect(screen.queryByLabelText(/schedule impacts/i)).toBeNull();
});

it("does not fetch schedule impact or show its link for teachers", () => {
  mockUseAuth.mockReturnValue({ user: { username: "teacher", role: "Teacher" }, logout: vi.fn() });
  render(<TestWrapper><Layout><div>Body</div></Layout></TestWrapper>);

  expect(mockApiJson.mock.calls.some(([path]) => typeof path === "string" && path.startsWith("/api/v1/operations/schedule-impact"))).toBe(false);
  expect(screen.queryByRole("link", { name: "Schedule Impact" })).toBeNull();
});

it("shows Legacy Sync to admins directly in the sidebar", () => {
  mockApiJson.mockImplementation((path: string) => {
    if (path.startsWith("/api/v1/absences/stats")) return Promise.resolve({ pending_count: 0, today_count: 0 });
    if (path.startsWith("/api/v1/operations/schedule-impact")) return Promise.resolve({ summary: { critical: 0, need_attention: 0 } });
    return Promise.resolve({});
  });
  render(<TestWrapper><Layout><div>Body</div></Layout></TestWrapper>);

  const legacySyncLink = screen.getByRole("link", { name: "Legacy Sync" });
  expect(legacySyncLink).toHaveAttribute("href", "/admin/legacy-sync");
});

it("does not show Legacy Sync to teachers", () => {
  mockUseAuth.mockReturnValue({ user: { username: "teacher", role: "Teacher" }, logout: vi.fn() });
  render(<TestWrapper><Layout><div>Body</div></Layout></TestWrapper>);

  expect(screen.queryByRole("link", { name: "Legacy Sync" })).toBeNull();
  expect(screen.queryByText("Legacy Sync")).toBeNull();
});
