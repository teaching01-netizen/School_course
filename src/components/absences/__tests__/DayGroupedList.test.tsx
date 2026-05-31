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
});
