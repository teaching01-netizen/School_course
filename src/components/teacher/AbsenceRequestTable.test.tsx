import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import type { TeacherDashboardSession } from "../../types";
import AbsenceRequestTable from "./AbsenceRequestTable";

const session: TeacherDashboardSession = {
  id: "session-1",
  course_id: "course-1",
  course_code: "0000000344",
  course_name: "Math Advanced C2/26",
  subject_name: "Mathematics",
  start_at: "2026-06-21T10:00:00Z",
  end_at: "2026-06-21T11:00:00Z",
  room_name: null,
  absent_count: 1,
  absent_students: [{
    wcode: "W001",
    nickname: "Ann",
    student_name: "Ann Student",
    absence_id: "absence-1",
    created_at: "2026-06-20T10:00:00Z",
  }],
  sit_in_visitors: [],
};

function renderTable(currentSession: TeacherDashboardSession) {
  render(
    <MemoryRouter>
      <AbsenceRequestTable sessions={[currentSession]} />
    </MemoryRouter>,
  );
}

describe("AbsenceRequestTable", () => {
  it("shows subject names without course codes", () => {
    renderTable(session);

    expect(screen.getAllByText("Mathematics").length).toBeGreaterThan(0);
    expect(screen.queryByText("0000000344")).not.toBeInTheDocument();
  });

  it("falls back to the course name without exposing the course code", () => {
    renderTable({ ...session, subject_name: null });

    expect(screen.getAllByText("Math Advanced C2/26").length).toBeGreaterThan(0);
    expect(screen.queryByText("0000000344")).not.toBeInTheDocument();
  });

  it("renders a mobile card with the same core data and destination as the desktop row", () => {
    renderTable(session);
    const mobileList = screen.getByTestId("mobile-absence-list");
    expect(mobileList).toHaveTextContent("Ann");
    expect(mobileList).toHaveTextContent("W001");
    expect(mobileList).toHaveTextContent("Mathematics");
    expect(mobileList).toHaveTextContent("Submitted");
    expect(mobileList.querySelector('a[href="/teacher-dashboard/absences/absence-1"]')).toBeTruthy();
  });
});
