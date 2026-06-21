import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { TeacherDashboardSession } from "../../types";
import CalendarDayCell from "./CalendarDayCell";

const session: TeacherDashboardSession = {
  id: "session-1",
  course_id: "course-1",
  course_code: "0000000344",
  course_name: "Math Advanced C2/26",
  subject_name: "Mathematics",
  start_at: "2026-06-21T10:00:00Z",
  end_at: "2026-06-21T11:00:00Z",
  room_name: null,
  absent_count: 0,
  absent_students: [],
  sit_in_visitors: [],
};

function renderCell(currentSession: TeacherDashboardSession) {
  render(
    <CalendarDayCell
      date={new Date("2026-06-21T00:00:00Z")}
      sessions={[currentSession]}
      isToday={false}
      isCurrentMonth
      isSelected={false}
      onClick={vi.fn()}
    />,
  );
}

describe("CalendarDayCell", () => {
  it("shows the subject name instead of the course code", () => {
    renderCell(session);

    expect(screen.getByText("Mathematics")).toBeInTheDocument();
    expect(screen.queryByText("0000000344")).not.toBeInTheDocument();
  });

  it("falls back to the course name when the subject name is missing", () => {
    renderCell({ ...session, subject_name: null });

    expect(screen.getByText("Math Advanced C2/26")).toBeInTheDocument();
    expect(screen.queryByText("0000000344")).not.toBeInTheDocument();
  });
});
