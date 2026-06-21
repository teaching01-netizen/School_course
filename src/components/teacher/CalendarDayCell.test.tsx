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

function renderCell(currentSession?: TeacherDashboardSession, overrides: Partial<React.ComponentProps<typeof CalendarDayCell>> = {}) {
  render(
    <CalendarDayCell
      date={new Date("2026-06-21T00:00:00Z")}
      sessions={currentSession ? [currentSession] : []}
      isToday={false}
      isCurrentMonth
      isSelected={false}
      onClick={vi.fn()}
      {...overrides}
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

  it("announces an empty day without relying on status colour", () => {
    renderCell();
    expect(screen.getByRole("button", { name: /No sessions/i })).toBeInTheDocument();
  });

  it("announces absence and sit-in counts for mixed mobile indicators", () => {
    renderCell({
      ...session,
      absent_students: [{ wcode: "W1", nickname: "A", student_name: null, absence_id: "a1", created_at: null }],
      sit_in_visitors: [{ wcode: "W2", nickname: "B", student_name: null, from_course_code: "C", from_subject_name: "English", absence_id: "a2", session_start_at: session.start_at, session_end_at: session.end_at, absent_subject_name: "English", absence_date: "2026-06-20" }],
    });
    expect(screen.getByRole("button", { name: /1 session, 1 absence, 1 sit-in/i })).toBeInTheDocument();
  });

  it("preserves selected and today states alongside compact content", () => {
    renderCell(session, { isSelected: true, isToday: true });
    const cell = screen.getByRole("button", { name: /1 session/i });
    expect(cell.className).toContain("ring-[var(--color-wi-primary)]");
    expect(screen.getByText("21").className).toContain("bg-gray-900");
  });

  it("renders a larger, darker-amber mobile dot for sit-in-only sessions", () => {
    renderCell({
      ...session,
      sit_in_visitors: [{
        wcode: "W1",
        nickname: null,
        student_name: null,
        from_course_code: "C",
        from_subject_name: null,
        absence_id: "a1",
        session_start_at: session.start_at,
        session_end_at: session.end_at,
        absent_subject_name: null,
        absence_date: "2026-06-20",
      }],
    });
    const cell = screen.getByRole("button");
    expect(cell.innerHTML).toContain("bg-amber-600");
    expect(cell.innerHTML).toContain("h-2 w-2 rounded-full");
  });
});
