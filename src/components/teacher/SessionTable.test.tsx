import { describe, it, expect } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import SessionTable from "./SessionTable";
import type { TeacherDashboardSession } from "../../types";

function wrap(ui: React.ReactElement) {
  return render(<MemoryRouter>{ui}</MemoryRouter>);
}

const sessions: TeacherDashboardSession[] = [
  {
    id: "s1",
    course_id: "c1",
    course_code: "MATH101",
    course_name: "Calculus I",
    subject_name: "Mathematics",
    start_at: "2026-06-18T10:00:00Z",
    end_at: "2026-06-18T11:30:00Z",
    room_name: "Room 301",
    absent_count: 0,
    absent_students: [],
    sit_in_visitors: [],
  },
  {
    id: "s2",
    course_id: "c2",
    course_code: "PHY201",
    course_name: "Electromagnetism",
    subject_name: "Physics",
    start_at: "2026-06-18T09:00:00Z",
    end_at: "2026-06-18T10:00:00Z",
    room_name: null,
    absent_count: 0,
    absent_students: [],
    sit_in_visitors: [],
  },
  {
    id: "s3",
    course_id: "c1",
    course_code: "MATH101",
    course_name: "Calculus I",
    subject_name: "Mathematics",
    start_at: "2026-06-18T14:00:00Z",
    end_at: "2026-06-18T15:00:00Z",
    room_name: "Room 201",
    absent_count: 2,
    absent_students: [
      { wcode: "STU001", nickname: null, student_name: "Alice", absence_id: "ab-1", created_at: null },
      { wcode: "STU002", nickname: "Bob", student_name: null, absence_id: "ab-2", created_at: null },
    ],
    sit_in_visitors: [],
  },
  {
    id: "s4",
    course_id: "c3",
    course_code: "CHEM101",
    course_name: "Organic Chemistry",
    subject_name: null,
    start_at: "2026-06-17T11:00:00Z",
    end_at: "2026-06-17T12:30:00Z",
    room_name: "Lab A",
    absent_count: 0,
    absent_students: [],
    sit_in_visitors: [
      { wcode: "STU003", nickname: null, student_name: "Charlie", from_course_code: "BIO101", from_subject_name: null, absence_id: "ab-3", session_start_at: "2026-06-17T11:00:00Z", session_end_at: "2026-06-17T12:30:00Z", absent_subject_name: null },
    ],
  },
];

describe("SessionTable", () => {
  it("renders empty state when no sessions", () => {
    render(<SessionTable sessions={[]} />);
    expect(screen.getByText("No sessions in this period.")).toBeInTheDocument();
  });

  it("renders course codes and subject names", () => {
    wrap(<SessionTable sessions={sessions} />);
    expect(screen.getByText("PHY201")).toBeInTheDocument();
    expect(screen.getByText("Physics")).toBeInTheDocument();
    expect(screen.getByText("CHEM101")).toBeInTheDocument();
    expect(screen.getAllByText("MATH101")).toHaveLength(2);
  });

  it("renders room names or placeholder", () => {
    wrap(<SessionTable sessions={sessions} />);
    expect(screen.getByText("Room 301")).toBeInTheDocument();
    expect(screen.getByText("Lab A")).toBeInTheDocument();
    const roomCells = screen.getAllByText("—");
    expect(roomCells.length).toBeGreaterThanOrEqual(1);
  });

  it("shows absence badge with correct count", () => {
    wrap(<SessionTable sessions={sessions} />);
    expect(screen.getByText("2")).toBeInTheDocument();
  });

  it("shows sit-in badge with correct count", () => {
    wrap(<SessionTable sessions={sessions} />);
    expect(screen.getByText("1")).toBeInTheDocument();
  });

  it("shows status pills for each row", () => {
    wrap(<SessionTable sessions={sessions} />);
    expect(screen.getAllByText("Absences")).toHaveLength(2);
    expect(screen.getAllByText("Sit-ins")).toHaveLength(2);
    expect(screen.getAllByText("OK")).toHaveLength(2);
  });

  it("renders sessions sorted chronologically by start_at", () => {
    wrap(<SessionTable sessions={sessions} />);
    const cells = screen.getAllByRole("cell");
    // cells[0] = date header (colSpan=7); then data rows: s4, s2, s1, s3
    expect(cells[3]?.textContent).toContain("CHEM101");
    expect(cells[11]?.textContent).toContain("PHY201");
    expect(cells[18]?.textContent).toContain("MATH101");
    expect(cells[25]?.textContent).toContain("MATH101");
  });

  it("clicking a row with absences reveals expanded detail", () => {
    wrap(<SessionTable sessions={sessions} />);
    const sectionHeaders = screen.queryAllByText("Absences");
    expect(sectionHeaders).toHaveLength(2); // header + 1 row's status pill
    fireEvent.click(screen.getByText("2"));
    const expandedHeaders = screen.getAllByText("Absences");
    expect(expandedHeaders).toHaveLength(3); // header + status pill + section header
  });

  it("expanded detail shows absent student names and wcodes", () => {
    wrap(<SessionTable sessions={sessions} />);
    fireEvent.click(screen.getByText("2"));
    expect(screen.getByText("Alice")).toBeInTheDocument();
    expect(screen.getByText("STU001")).toBeInTheDocument();
    expect(screen.getByText("Bob")).toBeInTheDocument();
    expect(screen.getByText("STU002")).toBeInTheDocument();
  });

  it("expanded detail shows absent student course context", () => {
    wrap(<SessionTable sessions={sessions} />);
    fireEvent.click(screen.getByText("2"));
    const lines = screen.getAllByText(/Absent from MATH101/);
    expect(lines).toHaveLength(2);
    expect(lines[0]).toBeInTheDocument();
  });

  it("expanded detail contains View links for each absence", () => {
    wrap(<SessionTable sessions={sessions} />);
    fireEvent.click(screen.getByText("2"));
    const viewLinks = screen.getAllByText("View →");
    expect(viewLinks).toHaveLength(2);
    expect(viewLinks[0]).toHaveAttribute("href", "/absences/ab-1");
    expect(viewLinks[1]).toHaveAttribute("href", "/absences/ab-2");
  });

  it("expanded sit-in detail shows visitor info", () => {
    wrap(<SessionTable sessions={sessions} />);
    fireEvent.click(screen.getByText("CHEM101"));
    expect(screen.getByText("Charlie")).toBeInTheDocument();
    expect(screen.getByText("STU003")).toBeInTheDocument();
    expect(screen.getByText(/Sit-in from BIO101/)).toBeInTheDocument();
    const viewLinks = screen.getAllByText("View →");
    expect(viewLinks[0]).toHaveAttribute("href", "/absences/ab-3");
  });

  it("clicking the same row again collapses the detail", () => {
    wrap(<SessionTable sessions={sessions} />);
    const badge = screen.getAllByText("2")[0];
    fireEvent.click(badge);
    expect(screen.getAllByText("View →")).toHaveLength(2);
    fireEvent.click(badge); // same element, still in the DOM
    expect(screen.queryByText("View →")).not.toBeInTheDocument();
  });

  it("only one row can be expanded at a time", () => {
    wrap(<SessionTable sessions={sessions} />);
    fireEvent.click(screen.getByText("2"));
    expect(screen.getAllByText("View →")).toHaveLength(2);
    fireEvent.click(screen.getByText("CHEM101"));
    // s3 (absences) collapsed, s4 (sit-ins) expanded — only sit-in View link
    const afterLinks = screen.getAllByText("View →");
    expect(afterLinks).toHaveLength(1);
    expect(afterLinks[0]).toHaveAttribute("href", "/absences/ab-3");
  });
});
