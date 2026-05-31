import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, within, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import CalendarGrid from "../CalendarGrid";
import type { SubjectSessions } from "@/types";

// Mock CalendarCell to simplify grid tests — includes all new props
vi.mock("../CalendarCell", () => ({
  default: ({
    sessionId,
    startTime,
    endTime,
    status,
    alreadyAbsent,
    onToggleAbsent,
    onToggleCover,
  }: {
    sessionId: string;
    startTime: string;
    endTime: string;
    status: string;
    alreadyAbsent?: boolean;
    onToggleAbsent: (id: string) => void;
    onToggleCover: (id: string) => void;
  }) => (
    <button
      type="button"
      data-testid={`cell-${sessionId}`}
      aria-label={`${sessionId} ${startTime}–${endTime} ${status}${alreadyAbsent ? " already-absent" : ""}`}
      aria-pressed={status !== "available"}
      data-already-absent={alreadyAbsent ? "true" : undefined}
      onClick={() => onToggleAbsent(sessionId)}
      onDoubleClick={() => onToggleCover(sessionId)}
    >
      {startTime}–{endTime}
    </button>
  ),
}));

// 2026-06-01 = Monday, 06-02 = Tuesday, 06-03 = Wednesday, 06-04 = Thursday, 06-05 = Friday
const MOCK_SUBJECT_SESSIONS: SubjectSessions[] = [
  {
    subject_id: "sub-1",
    subject_code: "MATH",
    subject_name: "Mathematics",
    course_id: "course-1",
    course_code: "MATH-101",
    sessions: [
      {
        id: "s1",
        start_at: "2026-06-01T09:00:00Z",
        end_at: "2026-06-01T10:30:00Z",
        date: "2026-06-01",
        already_absent: false,
      },
      {
        id: "s2",
        start_at: "2026-06-02T14:00:00Z",
        end_at: "2026-06-02T15:30:00Z",
        date: "2026-06-02",
        already_absent: false,
      },
    ],
  },
  {
    subject_id: "sub-2",
    subject_code: "PHYS",
    subject_name: "Physics",
    course_id: "course-2",
    course_code: "PHYS-201",
    sessions: [
      {
        id: "s3",
        start_at: "2026-06-03T11:00:00Z",
        end_at: "2026-06-03T12:30:00Z",
        date: "2026-06-03",
        already_absent: false,
      },
    ],
  },
];

function defaultProps(
  overrides?: Partial<React.ComponentProps<typeof CalendarGrid>>,
) {
  return {
    subjectSessions: MOCK_SUBJECT_SESSIONS,
    onSelectionChange: vi.fn(),
    onToggleCover: vi.fn(),
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("CalendarGrid", () => {
  it("renders a grid with role=grid", () => {
    render(<CalendarGrid {...defaultProps()} />);
    expect(screen.getByRole("grid")).toBeInTheDocument();
  });

  it("shows day-of-week headers for days with sessions", () => {
    render(<CalendarGrid {...defaultProps()} />);
    const grid = screen.getByRole("grid");
    // Sessions are on Mon (Jun 1), Tue (Jun 2), Wed (Jun 3)
    for (const day of ["Mon", "Tue", "Wed"]) {
      expect(within(grid).getByText(day)).toBeInTheDocument();
    }
  });

  it("shows time labels on the left side", () => {
    render(<CalendarGrid {...defaultProps()} />);
    // Time labels should be present for the grid rows
    expect(screen.getByText("09:00")).toBeInTheDocument();
    expect(screen.getByText("11:00")).toBeInTheDocument();
    expect(screen.getByText("14:00")).toBeInTheDocument();
  });

  it("renders a CalendarCell for each session", () => {
    render(<CalendarGrid {...defaultProps()} />);
    // 3 sessions across 3 subjects
    expect(screen.getByTestId("cell-s1")).toBeInTheDocument();
    expect(screen.getByTestId("cell-s2")).toBeInTheDocument();
    expect(screen.getByTestId("cell-s3")).toBeInTheDocument();
  });

  it("shows empty state when no sessions exist", () => {
    render(<CalendarGrid {...defaultProps({ subjectSessions: [] })} />);
    expect(screen.getByText(/no sessions/i)).toBeInTheDocument();
  });

  it("auto-selects all sessions as absent on load", () => {
    const onSelectionChange = vi.fn();
    render(
      <CalendarGrid
        {...defaultProps({ onSelectionChange })}
      />,
    );

    // All three cells should be rendered with absent status
    const s1Cell = screen.getByTestId("cell-s1");
    const s2Cell = screen.getByTestId("cell-s2");
    const s3Cell = screen.getByTestId("cell-s3");

    expect(s1Cell).toHaveAttribute("aria-pressed", "true");
    expect(s2Cell).toHaveAttribute("aria-pressed", "true");
    expect(s3Cell).toHaveAttribute("aria-pressed", "true");

    // Parent should be notified with all session IDs
    expect(onSelectionChange).toHaveBeenCalledWith(
      expect.objectContaining({
        size: 3,
        has: expect.any(Function),
      }),
    );
    const lastCall = onSelectionChange.mock.calls[0][0] as Set<string>;
    expect(lastCall.has("s1")).toBe(true);
    expect(lastCall.has("s2")).toBe(true);
    expect(lastCall.has("s3")).toBe(true);
  });

  // CR-01: Arrow key directions should match visual row-major layout
  it("supports arrow-key navigation between cells (row-major)", () => {
    render(<CalendarGrid {...defaultProps()} />);

    const s1Cell = screen.getByTestId("cell-s1");
    s1Cell.focus();

    const grid = screen.getByRole("grid");

    // s1 is Mon 09:00, s2 is Tue 14:00, s3 is Wed 11:00
    // dayColumns = [Mon, Tue, Wed], timeRows = [09:00, 11:00, 14:00]
    // Row-major order:
    //   09:00/Mon=s1, 09:00/Tue=empty, 09:00/Wed=empty
    //   11:00/Mon=empty, 11:00/Tue=empty, 11:00/Wed=s3
    //   14:00/Mon=empty, 14:00/Tue=s2, 14:00/Wed=empty
    // ArrowRight from s1 → empty Tue 09:00 (next cell in row)
    fireEvent.keyDown(grid, { key: "ArrowRight" });
    expect(screen.getByTestId("cell-empty-2026-06-02-09:00")).toHaveFocus();
  });

  it("ArrowDown moves to next time row (same column)", () => {
    // Use a grid with same-day sessions at different times
    const sameDaySessions: SubjectSessions[] = [
      {
        subject_id: "sub-1",
        subject_code: "MATH",
        subject_name: "Mathematics",
        course_id: "course-1",
        course_code: "MATH-101",
        sessions: [
          {
            id: "s1",
            start_at: "2026-06-01T09:00:00Z",
            end_at: "2026-06-01T10:00:00Z",
            date: "2026-06-01",
            already_absent: false,
          },
          {
            id: "s2",
            start_at: "2026-06-01T11:00:00Z",
            end_at: "2026-06-01T12:00:00Z",
            date: "2026-06-01",
            already_absent: false,
          },
        ],
      },
    ];

    render(<CalendarGrid {...defaultProps({ subjectSessions: sameDaySessions })} />);

    const s1Cell = screen.getByTestId("cell-s1");
    s1Cell.focus();

    const grid = screen.getByRole("grid");
    // ArrowDown should move from 09:00 to 11:00 on the same day (Mon)
    fireEvent.keyDown(grid, { key: "ArrowDown" });
    expect(screen.getByTestId("cell-s2")).toHaveFocus();
  });

  // CR-02: Multiple sessions at same time on same day
  it("renders multiple sessions at the same time on the same day", () => {
    const duplicateTimeSessions: SubjectSessions[] = [
      {
        subject_id: "sub-1",
        subject_code: "MATH",
        subject_name: "Mathematics",
        course_id: "course-1",
        course_code: "MATH-101",
        sessions: [
          {
            id: "s1",
            start_at: "2026-06-01T09:00:00Z",
            end_at: "2026-06-01T10:00:00Z",
            date: "2026-06-01",
            already_absent: false,
          },
        ],
      },
      {
        subject_id: "sub-2",
        subject_code: "PHYS",
        subject_name: "Physics",
        course_id: "course-2",
        course_code: "PHYS-201",
        sessions: [
          {
            id: "s-dup",
            start_at: "2026-06-01T09:00:00Z",
            end_at: "2026-06-01T10:00:00Z",
            date: "2026-06-01",
            already_absent: false,
          },
        ],
      },
    ];

    render(<CalendarGrid {...defaultProps({ subjectSessions: duplicateTimeSessions })} />);

    // Both sessions at same time on Monday should be rendered
    expect(screen.getByTestId("cell-s1")).toBeInTheDocument();
    expect(screen.getByTestId("cell-s-dup")).toBeInTheDocument();
  });

  it("both duplicate sessions are independently selectable", async () => {
    const duplicateTimeSessions: SubjectSessions[] = [
      {
        subject_id: "sub-1",
        subject_code: "MATH",
        subject_name: "Mathematics",
        course_id: "course-1",
        course_code: "MATH-101",
        sessions: [
          {
            id: "s1",
            start_at: "2026-06-01T09:00:00Z",
            end_at: "2026-06-01T10:00:00Z",
            date: "2026-06-01",
            already_absent: false,
          },
        ],
      },
      {
        subject_id: "sub-2",
        subject_code: "PHYS",
        subject_name: "Physics",
        course_id: "course-2",
        course_code: "PHYS-201",
        sessions: [
          {
            id: "s-dup",
            start_at: "2026-06-01T09:00:00Z",
            end_at: "2026-06-01T10:00:00Z",
            date: "2026-06-01",
            already_absent: false,
          },
        ],
      },
    ];

    const onSelectionChange = vi.fn();
    render(
      <CalendarGrid
        {...defaultProps({
          subjectSessions: duplicateTimeSessions,
          onSelectionChange,
        })}
      />,
    );

    // Both should start as absent (auto-selected)
    expect(screen.getByTestId("cell-s1")).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("cell-s-dup")).toHaveAttribute("aria-pressed", "true");

    // Click s-dup to deselect it
    const user = userEvent.setup();
    await user.click(screen.getByTestId("cell-s-dup"));

    // s-dup should now be available, s1 still absent
    expect(screen.getByTestId("cell-s-dup")).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByTestId("cell-s1")).toHaveAttribute("aria-pressed", "true");
  });

  // WR-01: onToggleCover is passed through
  it("passes onToggleCover through to CalendarCell", async () => {
    const onToggleCover = vi.fn();
    render(<CalendarGrid {...defaultProps({ onToggleCover })} />);

    const user = userEvent.setup();
    // Double-click triggers onToggleCover in CalendarCell
    await user.dblClick(screen.getByTestId("cell-s1"));

    expect(onToggleCover).toHaveBeenCalledWith("s1");
  });

  // WR-02: already_absent is passed through and styled
  it("passes already_absent to CalendarCell for pre-existing absences", () => {
    const sessionsWithAbsent: SubjectSessions[] = [
      {
        subject_id: "sub-1",
        subject_code: "MATH",
        subject_name: "Mathematics",
        course_id: "course-1",
        course_code: "MATH-101",
        sessions: [
          {
            id: "s-absent",
            start_at: "2026-06-01T09:00:00Z",
            end_at: "2026-06-01T10:00:00Z",
            date: "2026-06-01",
            already_absent: true,
          },
          {
            id: "s-new",
            start_at: "2026-06-02T09:00:00Z",
            end_at: "2026-06-02T10:00:00Z",
            date: "2026-06-02",
            already_absent: false,
          },
        ],
      },
    ];

    render(<CalendarGrid {...defaultProps({ subjectSessions: sessionsWithAbsent })} />);

    const absentCell = screen.getByTestId("cell-s-absent");
    const newCell = screen.getByTestId("cell-s-new");

    // Pre-existing absence should be marked
    expect(absentCell).toHaveAttribute("data-already-absent", "true");
    expect(absentCell).toHaveAttribute("aria-label", expect.stringContaining("already-absent"));

    // New absence should not be marked
    expect(newCell).not.toHaveAttribute("data-already-absent");
  });

  // WR-03: Empty cells are included in navigation
  it("renders empty cells with tabIndex for keyboard navigation", () => {
    render(<CalendarGrid {...defaultProps()} />);

    // Mon has sessions at 09:00 but NOT at 11:00 or 14:00
    // The empty cell at 11:00/Mon should exist with tabIndex
    const emptyCell = screen.getByTestId("cell-empty-2026-06-01-11:00");
    expect(emptyCell).toBeInTheDocument();
    expect(emptyCell).toHaveAttribute("tabindex", "-1");
    // Should NOT have aria-hidden
    expect(emptyCell).not.toHaveAttribute("aria-hidden");
  });

  it("navigates across empty cells with ArrowRight", () => {
    // Grid with sessions at different times on different days
    // so there are gaps in the row-major order
    const sparseSessions: SubjectSessions[] = [
      {
        subject_id: "sub-1",
        subject_code: "MATH",
        subject_name: "Mathematics",
        course_id: "course-1",
        course_code: "MATH-101",
        sessions: [
          {
            id: "s1",
            start_at: "2026-06-01T09:00:00Z",
            end_at: "2026-06-01T10:00:00Z",
            date: "2026-06-01",
            already_absent: false,
          },
          // No session at 09:00 on Tue or Wed, so ArrowRight goes through empty cells
          {
            id: "s2",
            start_at: "2026-06-03T09:00:00Z",
            end_at: "2026-06-03T10:00:00Z",
            date: "2026-06-03",
            already_absent: false,
          },
        ],
      },
    ];

    render(<CalendarGrid {...defaultProps({ subjectSessions: sparseSessions })} />);

    const s1Cell = screen.getByTestId("cell-s1");
    s1Cell.focus();

    // ArrowRight should go through the empty Tue cell to land on s2 (Wed)
    // Row 09:00 has: Mon(s1), empty-Tue, Wed(s2)
    // sortedCellIds: [s1, empty-2026-06-02-09:00, s2]
    // ArrowRight from s1 → empty-Tue → s2
    // Fire event on the focused cell (matches real browser behavior)
    fireEvent.keyDown(s1Cell, { key: "ArrowRight" });
    // Should focus the empty Tue cell
    expect(screen.getByTestId("cell-empty-2026-06-02-09:00")).toHaveFocus();

    const emptyCell = screen.getByTestId("cell-empty-2026-06-02-09:00");
    fireEvent.keyDown(emptyCell, { key: "ArrowRight" });
    expect(screen.getByTestId("cell-s2")).toHaveFocus();
  });
});
