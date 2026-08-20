import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import SitInListView from "../SitInListView";
import type { CalendarAbsence, CalendarSessionBrief } from "../../../features/absences/types";

function absence(overrides: Partial<CalendarAbsence> = {}): CalendarAbsence {
  return {
    id: "abs-1",
    wcode: "W260114",
    student_name: "Jane Roe",
    status: "pending",
    subject_name: "Mathematics",
    subject_code: "MATH",
    date_from: "2026-06-02",
    date_to: "2026-06-02",
    sit_in_method: "physical",
    sit_in_course_name: "Physics",
    sit_in_subject_name: "Physics",
    ...overrides,
  };
}

function sessionWith(startAt: string): CalendarSessionBrief {
  return {
    id: "sess-1",
    course_id: "c-1",
    course_code: "C1",
    course_name: "SAT Math",
    subject_name: "SAT Math",
    start_at: startAt,
    end_at: "2026-06-02T23:59:00Z",
    room_name: "Room 101",
  };
}

const emptyHandlers = {
  sessions: [] as CalendarSessionBrief[],
  absenceDays: [],
  hasFilters: false,
  hasAnySitIns: false,
  onClearFilters: vi.fn(),
};

describe("SitInListView absences mode", () => {
  it("renders absence rows from the absences list", () => {
    render(
      <SitInListView
        {...emptyHandlers}
        absences={[absence()]}
        mode="absences"
        zone="Asia/Bangkok"
      />,
    );

    expect(screen.getByRole("columnheader", { name: /student/i })).toBeInTheDocument();
    expect(screen.getByText("Jane Roe")).toBeInTheDocument();
    expect(screen.getByText("W260114")).toBeInTheDocument();
    expect(screen.getByText("Mathematics")).toBeInTheDocument();
    expect(screen.getByText("Physics")).toBeInTheDocument();
    expect(screen.getByText("2 Jun 2026")).toBeInTheDocument();
    expect(screen.getByText("Physical")).toBeInTheDocument();
    expect(screen.getByText("Pending")).toBeInTheDocument();
  });

  it("renders the absence date range when the absence spans multiple days", () => {
    render(
      <SitInListView
        {...emptyHandlers}
        absences={[absence({ date_from: "2026-06-02", date_to: "2026-06-04" })]}
        mode="absences"
        zone="Asia/Bangkok"
      />,
    );

    expect(screen.getByText("2 Jun – 4 Jun 2026")).toBeInTheDocument();
  });

  it("shows the filtered empty state when filters are active", () => {
    render(
      <SitInListView
        {...emptyHandlers}
        absences={[]}
        mode="absences"
        zone="Asia/Bangkok"
        hasFilters
      />,
    );

    expect(screen.getByText("No absences match your filters.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /clear filters/i })).toBeInTheDocument();
  });

  it("shows the unfiltered empty state when no absences exist in the range", () => {
    render(
      <SitInListView
        {...emptyHandlers}
        absences={[]}
        mode="absences"
        zone="Asia/Bangkok"
      />,
    );

    expect(screen.getByText("No absences recorded in this date range.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /clear filters/i })).not.toBeInTheDocument();
  });
});

describe("SitInListView sit-ins mode", () => {
  it("renders the session calendar day in the institute zone", () => {
    render(
      <SitInListView
        {...emptyHandlers}
        sessions={[
          {
            ...sessionWith("2026-06-02T23:30:00Z"),
            sit_in_students: [
              {
                wcode: "W260207",
                nickname: "Nut",
                student_name: null,
                absence_id: "abs-2",
                from_course_code: "ENG201",
                from_course_name: "English",
              },
            ],
          },
        ]}
        absences={[]}
        mode="sit-ins"
        zone="Asia/Bangkok"
      />,
    );

    // 23:30 UTC == 06:30 Bangkok on June 3 — the row must show the Bangkok day.
    expect(screen.getByText("Wednesday, 3 June 2026")).toBeInTheDocument();
    expect(screen.getByText("06:30")).toBeInTheDocument();
  });
});