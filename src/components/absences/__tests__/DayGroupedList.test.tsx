import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import DayGroupedList from "../DayGroupedList";
import type { ComponentProps } from "react";

type Session = ComponentProps<typeof DayGroupedList>["sessions"][number];

// 2026-06-01 = Monday, 2026-06-02 = Tuesday
const MOCK_SESSIONS: Session[] = [
  { id: "s1", date: "2026-06-01", start_at: "2026-06-01T09:00:00Z", end_at: "2026-06-01T10:30:00Z", subject_code: "MATH" },
  { id: "s2", date: "2026-06-01", start_at: "2026-06-01T14:00:00Z", end_at: "2026-06-01T15:30:00Z", subject_code: "PHYS" },
  { id: "s3", date: "2026-06-02", start_at: "2026-06-02T11:00:00Z", end_at: "2026-06-02T12:30:00Z", subject_code: "MATH" },
];

function defaultProps(overrides?: Partial<ComponentProps<typeof DayGroupedList>>) {
  return {
    sessions: MOCK_SESSIONS,
    absentSessionIds: new Set<string>(),
    coverSessionIds: new Set<string>(),
    onToggleAbsent: vi.fn(),
    onToggleCover: vi.fn(),
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("DayGroupedList", () => {
  it("renders sessions grouped by day of week", () => {
    render(<DayGroupedList {...defaultProps()} />);

    // Monday group header with session count
    expect(screen.getByText(/Monday/)).toBeInTheDocument();
    expect(screen.getByText(/2 sessions?/)).toBeInTheDocument();

    // Tuesday group header with session count
    expect(screen.getByText(/Tuesday/)).toBeInTheDocument();
    expect(screen.getByText(/1 session/)).toBeInTheDocument();
  });

  it("expands and collapses day groups", async () => {
    const user = userEvent.setup();
    render(<DayGroupedList {...defaultProps()} />);

    // Monday header should start expanded
    const mondayHeader = screen.getByRole("button", { name: /Monday/i });
    expect(mondayHeader).toHaveAttribute("aria-expanded", "true");

    // Click Monday header to collapse
    await user.click(mondayHeader);
    expect(mondayHeader).toHaveAttribute("aria-expanded", "false");

    // Tuesday header still expanded
    const tuesdayHeader = screen.getByRole("button", { name: /Tuesday/i });
    expect(tuesdayHeader).toHaveAttribute("aria-expanded", "true");

    // Click Monday header again to re-expand
    await user.click(mondayHeader);
    expect(mondayHeader).toHaveAttribute("aria-expanded", "true");
  });

  it("clicks session to toggle absent", async () => {
    const user = userEvent.setup();
    const onToggleAbsent = vi.fn();
    render(<DayGroupedList {...defaultProps({ onToggleAbsent })} />);

    // Click the first session chip
    const chip = screen.getByRole("button", { name: /09:00/ });
    await user.click(chip);

    expect(onToggleAbsent).toHaveBeenCalledWith("s1");
  });

  it("double-clicks session to toggle cover", async () => {
    const user = userEvent.setup();
    const onToggleCover = vi.fn();
    render(<DayGroupedList {...defaultProps({ onToggleCover })} />);

    // Double-click the first session chip
    const chip = screen.getByRole("button", { name: /09:00/ });
    await user.dblClick(chip);

    expect(onToggleCover).toHaveBeenCalledWith("s1");
  });

  it("shows empty state when no sessions", () => {
    render(<DayGroupedList {...defaultProps({ sessions: [] })} />);
    expect(screen.getByText(/No sessions/i)).toBeInTheDocument();
  });

  it("displays absent indicator on absent sessions", () => {
    render(<DayGroupedList {...defaultProps({ absentSessionIds: new Set(["s1"]) })} />);

    // s1 should have absent indicator
    const chip = screen.getByRole("button", { name: /09:00/ });
    expect(chip).toHaveAttribute("aria-pressed", "true");
  });

  it("displays cover indicator on cover sessions", () => {
    render(<DayGroupedList {...defaultProps({ coverSessionIds: new Set(["s3"]) })} />);

    // s3 should have cover indicator
    const chip = screen.getByRole("button", { name: /11:00/ });
    expect(chip).toHaveAttribute("aria-pressed", "true");
  });

  it("day headers are keyboard accessible", async () => {
    const user = userEvent.setup();
    render(<DayGroupedList {...defaultProps()} />);

    const mondayHeader = screen.getByRole("button", { name: /Monday/i });
    expect(mondayHeader).toHaveAttribute("aria-expanded", "true");

    mondayHeader.focus();
    await user.keyboard("{Enter}");

    // Should collapse (aria-expanded toggled)
    expect(mondayHeader).toHaveAttribute("aria-expanded", "false");

    // Re-expand with Space
    await user.keyboard(" ");
    expect(mondayHeader).toHaveAttribute("aria-expanded", "true");
  });

  it("Shift+Enter toggles cover (WR-01)", async () => {
    const user = userEvent.setup();
    const onToggleCover = vi.fn();
    const onToggleAbsent = vi.fn();
    render(
      <DayGroupedList
        {...defaultProps({ onToggleCover, onToggleAbsent })}
      />,
    );

    const chip = screen.getByRole("button", { name: /09:00/ });
    chip.focus();
    await user.keyboard("{Shift>}{Enter}{/Shift}");

    expect(onToggleCover).toHaveBeenCalledWith("s1");
    // absent should not be called when not already absent
    expect(onToggleAbsent).not.toHaveBeenCalled();
  });

  it("Shift+Enter on absent session clears absent and sets cover (CR-01 mutual exclusivity)", async () => {
    const user = userEvent.setup();
    const onToggleCover = vi.fn();
    const onToggleAbsent = vi.fn();
    render(
      <DayGroupedList
        {...defaultProps({
          onToggleCover,
          onToggleAbsent,
          absentSessionIds: new Set(["s1"]),
        })}
      />,
    );

    const chip = screen.getByRole("button", { name: /09:00/ });
    chip.focus();
    await user.keyboard("{Shift>}{Enter}{/Shift}");

    expect(onToggleCover).toHaveBeenCalledWith("s1");
    expect(onToggleAbsent).toHaveBeenCalledWith("s1");
  });

  it("double-click on absent session sets cover and clears absent (CR-01)", async () => {
    const user = userEvent.setup();
    const onToggleCover = vi.fn();
    const onToggleAbsent = vi.fn();
    render(
      <DayGroupedList
        {...defaultProps({
          onToggleCover,
          onToggleAbsent,
          absentSessionIds: new Set(["s1"]),
        })}
      />,
    );

    const chip = screen.getByRole("button", { name: /09:00/ });
    await user.dblClick(chip);

    expect(onToggleCover).toHaveBeenCalledWith("s1");
    // absent should be cleared
    expect(onToggleAbsent).toHaveBeenCalledWith("s1");
  });

  it("sessions are sorted by start_at within each day (WR-05)", () => {
    const unsorted: Session[] = [
      { id: "s-late", date: "2026-06-01", start_at: "2026-06-01T14:00:00Z", end_at: "2026-06-01T15:00:00Z", subject_code: "PHYS" },
      { id: "s-early", date: "2026-06-01", start_at: "2026-06-01T09:00:00Z", end_at: "2026-06-01T10:00:00Z", subject_code: "MATH" },
    ];
    render(<DayGroupedList {...defaultProps({ sessions: unsorted })} />);

    const chips = screen.getAllByRole("button", { name: /–/ });
    // First chip should be the 09:00 session, second the 14:00
    expect(chips[0]).toHaveTextContent("09:00");
    expect(chips[1]).toHaveTextContent("14:00");
  });

  it("new day groups from prop changes stay expanded (WR-03)", async () => {
    const user = userEvent.setup();
    const mondaySession: Session[] = [
      { id: "s1", date: "2026-06-01", start_at: "2026-06-01T09:00:00Z", end_at: "2026-06-01T10:00:00Z", subject_code: "MATH" },
    ];
    const { rerender } = render(<DayGroupedList {...defaultProps({ sessions: mondaySession })} />);

    // Monday expanded
    const mondayHeader = screen.getByRole("button", { name: /Monday/i });
    expect(mondayHeader).toHaveAttribute("aria-expanded", "true");

    // Add a Wednesday session via prop change
    const withWednesday: Session[] = [
      ...mondaySession,
      { id: "s4", date: "2026-06-03", start_at: "2026-06-03T10:00:00Z", end_at: "2026-06-03T11:00:00Z", subject_code: "ENG" },
    ];
    rerender(<DayGroupedList {...defaultProps({ sessions: withWednesday })} />);

    // Wednesday should also be expanded (not collapsed)
    const wednesdayHeader = screen.getByRole("button", { name: /Wednesday/i });
    expect(wednesdayHeader).toHaveAttribute("aria-expanded", "true");
  });

  it("formatTime handles time without minutes (WR-04)", () => {
    render(<DayGroupedList {...defaultProps({ sessions: [{ id: "s-bad", date: "2026-06-01", start_at: "2026-06-01T09", end_at: "2026-06-01T10", subject_code: "TEST" }] })} />);
    // Should render "09" not "09:undefined"
    const chip = screen.getByRole("button", { name: /09/ });
    expect(chip).toHaveTextContent("09");
    expect(chip.textContent).not.toContain("undefined");
  });

  it("session buttons have no redundant role attribute (WR-02)", () => {
    render(<DayGroupedList {...defaultProps()} />);
    const chip = screen.getByRole("button", { name: /09:00/ });
    // Native button has implicit role — should not have explicit role="button"
    expect(chip).not.toHaveAttribute("role");
  });
});
