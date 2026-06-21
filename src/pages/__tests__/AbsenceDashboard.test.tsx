import { beforeEach, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import AbsenceDashboard from "../AbsenceDashboard";
import { ToastProvider } from "../../hooks/useToast";
import { ApiRequestError } from "../../api/client";
import { queryClient } from "../../query/cache";

const mockApiJson = vi.hoisted(() => vi.fn());
vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

const TEACHERS = [{ id: "teacher-1", username: "Teacher One", role: "Teacher" }];

function dashboardResponse(courseCode: string, monthStart: string) {
  const day = `${monthStart.slice(0, 8)}15`;
  const sessionStart = `${day}T09:00:00Z`;
  const sessionEnd = `${day}T10:00:00Z`;
  return {
    week_start: "2026-06-15",
    week_end: "2026-06-21",
    teacher: { id: "teacher-1", username: "Teacher One" },
    sessions: [
      {
        id: "session-1",
        course_id: "course-1",
        course_code: courseCode,
        course_name: "Algebra",
        subject_name: courseCode === "MATH102" ? "Mathematics II" : "Mathematics I",
        start_at: sessionStart,
        end_at: sessionEnd,
        room_name: "Room A",
        absent_count: 2,
        sit_in_visitors: [],
      },
    ],
    summary: { total_sessions: 1, total_absences: 2, total_sit_ins: 0 },
    pending_absence_requests: [],
  };
}

function renderDashboard() {
  render(
    <MemoryRouter>
      <ToastProvider>
        <AbsenceDashboard />
      </ToastProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  mockApiJson.mockReset();
  queryClient.clear();
});

it("loads teachers and fetches the selected teacher dashboard", async () => {
  mockApiJson.mockImplementation(async (url: string) => {
    if (url === "/api/v1/users?role=Teacher") return TEACHERS;
    if (url.includes("/api/v1/teacher/dashboard")) {
      const monthStart = new URLSearchParams(url.split("?")[1]).get("month_start") ?? "2026-06-01";
      return dashboardResponse("MATH101", monthStart);
    }
    throw new Error(`Unmocked API call: ${url}`);
  });

  renderDashboard();
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: /view dashboard/i }));

  await screen.findByText("Mathematics I");
  expect(mockApiJson).toHaveBeenCalledWith(
    expect.stringMatching(/\/api\/v1\/teacher\/dashboard\?month_start=\d{4}-\d{2}-\d{2}&teacher_id=teacher-1/),
  );
});

it("retries a failed dashboard request without showing stale sessions", async () => {
  const user = userEvent.setup();
  mockApiJson.mockImplementation(async (url: string) => {
    if (url === "/api/v1/users?role=Teacher") return TEACHERS;
    if (url.includes("/api/v1/teacher/dashboard")) {
      const monthStart = new URLSearchParams(url.split("?")[1]).get("month_start") ?? "2026-06-01";
      const dashboardCalls = mockApiJson.mock.calls.filter(([calledUrl]) =>
        String(calledUrl).includes("/api/v1/teacher/dashboard"),
      ).length;
      if (dashboardCalls === 1) return dashboardResponse("MATH101", monthStart);
      if (dashboardCalls === 2 || dashboardCalls === 3) throw new ApiRequestError("Server down", { status: 500 });
      return dashboardResponse("MATH102", monthStart);
    }
    throw new Error(`Unmocked API call: ${url}`);
  });

  renderDashboard();

  await user.click(await screen.findByRole("button", { name: /view dashboard/i }));
  await screen.findByText("Mathematics I");

  await user.click(screen.getByRole("button", { name: /next month/i }));

  await screen.findByText(/A server error occurred/i, {}, { timeout: 2_500 });
  expect(screen.queryByText("Mathematics I")).not.toBeInTheDocument();

  await user.click(screen.getByRole("button", { name: /retry/i }));

  await screen.findByText("Mathematics II");
  await waitFor(() => {
    expect(screen.queryByText(/failed to load dashboard/i)).not.toBeInTheDocument();
  });
});
