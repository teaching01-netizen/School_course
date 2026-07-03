import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import TeacherDashboard from "../TeacherDashboard";

const mockUseApiQuery = vi.hoisted(() => vi.fn());
const mockUseInstituteMeta = vi.hoisted(() => vi.fn());

vi.mock("../../hooks/useApiQuery", () => ({
  useApiQuery: mockUseApiQuery,
}));

vi.mock("../../hooks/useInstituteMeta", () => ({
  default: mockUseInstituteMeta,
}));

const response = {
  week_start: "2026-06-01",
  week_end: "2026-06-30",
  teacher: { id: "t-1", username: "teacher" },
  sessions: [
    {
      id: "sess-1",
      course_id: "course-1",
      course_code: "SATM101",
      course_name: "SAT Math Beginner",
      subject_name: "Mathematics",
      start_at: "2026-06-01T17:30:00Z",
      end_at: "2026-06-01T18:30:00Z",
      room_name: "Room 12",
      absent_count: 0,
      absent_students: [],
      sit_in_visitors: [],
    },
  ],
  summary: {
    total_sessions: 1,
    total_absences: 0,
    total_sit_ins: 0,
  },
  pending_absence_requests: [],
};

describe("TeacherDashboard", () => {
  beforeEach(() => {
    mockUseApiQuery.mockReset();
    mockUseInstituteMeta.mockReturnValue({
      serverNow: "2026-06-15T03:00:00Z",
      instituteTZ: "Asia/Bangkok",
      loaded: true,
    });
    mockUseApiQuery.mockImplementation((url: string | null) => ({
      data: url ? response : null,
      loading: false,
      refreshing: false,
      error: null,
      refetch: vi.fn(),
    }));
  });

  it("groups a Bangkok evening session on the next institute day", async () => {
    render(
      <MemoryRouter>
        <TeacherDashboard />
      </MemoryRouter>,
    );

    expect(await screen.findByText("June 2026")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /tuesday, 2 june 2026/i })).toBeInTheDocument();
    expect(mockUseApiQuery).toHaveBeenCalledWith("/api/v1/teacher/dashboard?month_start=2026-06-01", expect.any(Array));
  });

  it("clears the selected day when the month changes", async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <TeacherDashboard />
      </MemoryRouter>,
    );

    expect(await screen.findByText("June 2026")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /tuesday, 2 june 2026/i }));
    expect(await screen.findByRole("dialog")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /next month/i }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
    await waitFor(() => {
      expect(mockUseApiQuery.mock.calls.at(-1)?.[0]).toBe("/api/v1/teacher/dashboard?month_start=2026-07-01");
    });
  });
});
