import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { mockUseApiQuery } = vi.hoisted(() => ({ mockUseApiQuery: vi.fn() }));

vi.mock("../../hooks/useApiQuery", () => ({
  useApiQuery: mockUseApiQuery,
}));

import ScheduleConflicts from "../ScheduleConflicts";

describe("ScheduleConflicts", () => {
  beforeEach(() => {
    mockUseApiQuery.mockReset();
  });

  it("shows summary totals and expandable conflict rows when data loads", () => {
    // Given: the API returns one teacher overlap.
    mockUseApiQuery.mockReturnValue({
      data: {
        items: [
          {
            id: "teacher_overlap:session-1:session-2",
            conflict_type: "teacher_overlap",
            primary_session: {
              session_id: "session-1",
              course_id: "course-1",
              course_code: "MATH-1",
              course_name: "Math 1",
              subject_id: "subject-1",
              subject_name: "Mathematics",
              teacher_id: "teacher-1",
              teacher_name: "Ada Teacher",
              room_id: "room-1",
              room_name: "Room A",
              start_at: "2026-08-26T09:00:00Z",
              end_at: "2026-08-26T10:00:00Z",
            },
            conflicting_sessions: [
              {
                session_id: "session-2",
                course_id: "course-2",
                course_code: "PHY-1",
                course_name: "Physics 1",
                subject_id: "subject-2",
                subject_name: "Physics",
                teacher_id: "teacher-1",
                teacher_name: "Ada Teacher",
                room_id: "room-2",
                room_name: "Room B",
                start_at: "2026-08-26T09:30:00Z",
                end_at: "2026-08-26T10:30:00Z",
              },
            ],
            affected_students: [],
            shared_resource: { type: "teacher", id: "teacher-1", name: "Ada Teacher" },
            detected_at: "2026-08-26T09:30:00Z",
          },
        ],
        total_count: 1,
        offset: 0,
        limit: 50,
        summary: { total_conflicts: 1, room_overlaps: 0, teacher_overlaps: 1, student_overlaps: 0 },
      },
      loading: false,
      refreshing: false,
      error: null,
      refetch: vi.fn(),
    });

    // When: an admin opens the overview.
    render(
      <MemoryRouter initialEntries={["/schedule-conflicts"]}>
        <ScheduleConflicts />
      </MemoryRouter>,
    );

    // Then: the summary and both subjects are visible in the conflict row.
    expect(screen.getByRole("heading", { name: "Schedule conflicts" })).toBeInTheDocument();
    expect(screen.getByText("1 total conflict")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Mathematics" })).toHaveAttribute("href", "/courses/course-1");
    expect(screen.getByRole("link", { name: "Physics" })).toHaveAttribute("href", "/courses/course-2");
    expect(screen.getByRole("link", { name: "Ada Teacher" })).toHaveAttribute("href", "/courses/course-1");
  });
});
