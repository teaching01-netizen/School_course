import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import CalendarMonth from "./CalendarMonth";

function renderMonth() {
  return render(
    <CalendarMonth
      viewMonthKey="2026-06-01"
      sessions={[]}
      todayKey="2026-06-21"
      zone="Asia/Bangkok"
      selectedDayKey={null}
      onSelectDay={vi.fn()}
      onPrevMonth={vi.fn()}
      onNextMonth={vi.fn()}
      onToday={vi.fn()}
    />,
  );
}

describe("CalendarMonth", () => {
  it("renders the month label", () => {
    renderMonth();
    expect(screen.getByText("June 2026")).toBeInTheDocument();
  });

  it("shows a color legend explaining red, amber, and green states", () => {
    renderMonth();
    expect(screen.getByText(/Has absences/i)).toBeInTheDocument();
    expect(screen.getByText(/Has sit-ins/i)).toBeInTheDocument();
    expect(screen.getByText(/All OK/i)).toBeInTheDocument();
  });

  it("includes a red dot in the legend", () => {
    renderMonth();
    // The legend has colored dot indicators
    const container = screen.getByText(/Has absences/i).parentElement;
    expect(container?.innerHTML).toContain("bg-red-500");
  });

  it("includes an amber dot in the legend", () => {
    renderMonth();
    const container = screen.getByText(/Has sit-ins/i).parentElement;
    expect(container?.innerHTML).toContain("bg-amber-500");
  });

  it("includes a green dot in the legend", () => {
    renderMonth();
    const container = screen.getByText(/All OK/i).parentElement;
    expect(container?.innerHTML).toContain("bg-green-500");
  });
});
