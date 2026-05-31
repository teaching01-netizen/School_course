import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, within, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import CalendarGrid from "../CalendarGrid";
import type { SubjectSessions } from "@/types";

// Mock CalendarCell to simplify grid tests
vi.mock("../CalendarCell", () => ({
  default: ({
    sessionId,
    startTime,
    endTime,
    status,
    onToggleAbsent,
  }: {
    sessionId: string;
    startTime: string;
    endTime: string;
    status: string;
    onToggleAbsent: (id: string) => void;
  }) => (
    <button
      type="button"
      data-testid={`cell-${sessionId}`}
      aria-label={`${sessionId} ${startTime}–${endTime} ${status}`}
      aria-pressed={status !== "available"}
      onClick={() => onToggleAbsent(sessionId)}
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

  it("supports arrow-key navigation between cells", () => {
    render(<CalendarGrid {...defaultProps()} />);

    const s1Cell = screen.getByTestId("cell-s1");
    s1Cell.focus();

    // Fire ArrowRight on the grid — handler uses activeElement to find current cell
    const grid = screen.getByRole("grid");
    fireEvent.keyDown(grid, { key: "ArrowRight" });
    expect(screen.getByTestId("cell-s2")).toHaveFocus();
  });
});
