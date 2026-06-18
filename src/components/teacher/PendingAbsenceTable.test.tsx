import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import PendingAbsenceTable from "./PendingAbsenceTable";
import type { PendingAbsenceRequest } from "../../types";

function wrap(ui: React.ReactElement) {
  return render(<MemoryRouter>{ui}</MemoryRouter>);
}

const sampleRequests: PendingAbsenceRequest[] = [
  {
    id: "ab-1",
    wcode: "STU001",
    student_name: "Alice Smith",
    nickname: "Ali",
    course_code: "MATH101",
    course_name: "Calculus I",
    subject_name: "Mathematics",
    date_from: "2026-06-18",
    date_to: "2026-06-18",
    reason: "Sick",
    reason_category: "medical",
    created_at: new Date(Date.now() - 3_600_000).toISOString(),
  },
  {
    id: "ab-2",
    wcode: "STU002",
    student_name: "Bob Jones",
    nickname: null,
    course_code: "PHY201",
    course_name: "Electromagnetism",
    subject_name: "Physics",
    date_from: "2026-06-15",
    date_to: "2026-06-20",
    reason: null,
    reason_category: null,
    created_at: new Date(Date.now() - 86_400_000).toISOString(),
  },
];

describe("PendingAbsenceTable", () => {
  it("renders empty state when no requests", () => {
    wrap(<PendingAbsenceTable requests={[]} />);
    expect(screen.getByText("No pending requests.")).toBeInTheDocument();
  });

  it("renders student names and wcodes", () => {
    wrap(<PendingAbsenceTable requests={sampleRequests} />);
    expect(screen.getByText("Ali")).toBeInTheDocument();
    expect(screen.getByText("STU001")).toBeInTheDocument();
    expect(screen.getByText("Bob Jones")).toBeInTheDocument();
    expect(screen.getByText("STU002")).toBeInTheDocument();
  });

  it("renders course codes and subject names", () => {
    wrap(<PendingAbsenceTable requests={sampleRequests} />);
    expect(screen.getByText("MATH101")).toBeInTheDocument();
    expect(screen.getByText("Mathematics")).toBeInTheDocument();
    expect(screen.getByText("PHY201")).toBeInTheDocument();
    expect(screen.getByText("Physics")).toBeInTheDocument();
  });

  it("renders date range for single-day and multi-day", () => {
    wrap(<PendingAbsenceTable requests={sampleRequests} />);
    expect(screen.getByText("2026-06-18")).toBeInTheDocument();
    expect(screen.getByText("2026-06-15 – 2026-06-20")).toBeInTheDocument();
  });

  it("renders relative time for submitted column", () => {
    wrap(<PendingAbsenceTable requests={sampleRequests} />);
    expect(screen.getByText("1h ago")).toBeInTheDocument();
    expect(screen.getByText("Yesterday")).toBeInTheDocument();
  });

  it("each row links to /absences/:id", () => {
    wrap(<PendingAbsenceTable requests={sampleRequests} />);
    const links = screen.getAllByRole("link");
    const absenceLinks = links.filter((l) => l.getAttribute("href")?.startsWith("/absences/"));
    expect(absenceLinks).toHaveLength(4);
    expect(absenceLinks[0]).toHaveAttribute("href", "/absences/ab-1");
    expect(absenceLinks[2]).toHaveAttribute("href", "/absences/ab-2");
  });

  it("renders initials in avatar circle from display name", () => {
    wrap(<PendingAbsenceTable requests={sampleRequests} />);
    expect(screen.getByText("A")).toBeInTheDocument();
    expect(screen.getByText("BJ")).toBeInTheDocument();
  });
});
