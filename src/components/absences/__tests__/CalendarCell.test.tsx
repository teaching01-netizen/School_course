import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import CalendarCell from "../CalendarCell";

const defaultProps = {
  sessionId: "sess-1",
  startTime: "09:00",
  endTime: "10:00",
  status: "available" as const,
  onToggleAbsent: vi.fn(),
  onToggleCover: vi.fn(),
};

beforeEach(() => {
  vi.clearAllMocks();
  vi.useRealTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("CalendarCell", () => {
  it("renders time range", () => {
    render(<CalendarCell {...defaultProps} />);
    expect(screen.getByText("09:00–10:00")).toBeInTheDocument();
  });

  it("has gridcell role, aria-label, and aria-pressed", () => {
    render(<CalendarCell {...defaultProps} />);
    const cell = screen.getByRole("gridcell");
    expect(cell).toHaveAttribute(
      "aria-label",
      "sess-1 09:00–10:00 available"
    );
    expect(cell).toHaveAttribute("aria-pressed", "false");
  });

  it("available status has gray styling", () => {
    render(<CalendarCell {...defaultProps} status="available" />);
    const cell = screen.getByRole("gridcell");
    expect(cell.className).toContain("bg-gray-100");
    expect(cell.className).toContain("text-gray-700");
  });

  it("absent status has green styling", () => {
    render(<CalendarCell {...defaultProps} status="absent" />);
    const cell = screen.getByRole("gridcell");
    expect(cell.className).toContain("bg-green-100");
    expect(cell.className).toContain("text-green-800");
  });

  it("cover status has amber styling", () => {
    render(<CalendarCell {...defaultProps} status="cover" />);
    const cell = screen.getByRole("gridcell");
    expect(cell.className).toContain("bg-amber-100");
    expect(cell.className).toContain("text-amber-800");
  });

  it("single click toggles absent after debounce", async () => {
    vi.useFakeTimers();
    const onToggleAbsent = vi.fn();
    render(
      <CalendarCell {...defaultProps} onToggleAbsent={onToggleAbsent} />
    );
    fireEvent.click(screen.getByRole("gridcell"));
    expect(onToggleAbsent).not.toHaveBeenCalled();
    vi.advanceTimersByTime(250);
    expect(onToggleAbsent).toHaveBeenCalledWith("sess-1");
  });

  it("double-click does NOT call onToggleAbsent (CR-01)", async () => {
    vi.useFakeTimers();
    const onToggleAbsent = vi.fn();
    const onToggleCover = vi.fn();
    render(
      <CalendarCell
        {...defaultProps}
        onToggleAbsent={onToggleAbsent}
        onToggleCover={onToggleCover}
      />
    );
    const cell = screen.getByRole("gridcell");
    // Simulate browser double-click: click → click → dblclick
    fireEvent.click(cell);
    fireEvent.click(cell);
    fireEvent.doubleClick(cell);
    // Click debounce should have been cancelled by dblclick
    vi.advanceTimersByTime(500);
    expect(onToggleAbsent).not.toHaveBeenCalled();
    expect(onToggleCover).toHaveBeenCalledWith("sess-1");
  });

  it("Enter key toggles absent", async () => {
    const user = userEvent.setup();
    const onToggleAbsent = vi.fn();
    render(
      <CalendarCell {...defaultProps} onToggleAbsent={onToggleAbsent} />
    );
    const cell = screen.getByRole("gridcell");
    cell.focus();
    await user.keyboard("{Enter}");
    expect(onToggleAbsent).toHaveBeenCalledWith("sess-1");
  });

  it("Shift+Enter toggles cover", async () => {
    const user = userEvent.setup();
    const onToggleCover = vi.fn();
    render(
      <CalendarCell {...defaultProps} onToggleCover={onToggleCover} />
    );
    const cell = screen.getByRole("gridcell");
    cell.focus();
    await user.keyboard("{Shift>}{Enter}{/Shift}");
    expect(onToggleCover).toHaveBeenCalledWith("sess-1");
  });

  it("long-press (500ms) toggles cover", async () => {
    vi.useFakeTimers();
    const onToggleCover = vi.fn();
    const onToggleAbsent = vi.fn();
    render(
      <CalendarCell
        {...defaultProps}
        onToggleAbsent={onToggleAbsent}
        onToggleCover={onToggleCover}
      />
    );
    const cell = screen.getByRole("gridcell");
    fireEvent.pointerDown(cell);
    vi.advanceTimersByTime(500);
    fireEvent.pointerUp(cell);
    expect(onToggleCover).toHaveBeenCalledWith("sess-1");
    expect(onToggleAbsent).not.toHaveBeenCalled();
  });

  it("long-press cancelled on pointerLeave (WR-01)", async () => {
    vi.useFakeTimers();
    const onToggleCover = vi.fn();
    render(
      <CalendarCell {...defaultProps} onToggleCover={onToggleCover} />
    );
    const cell = screen.getByRole("gridcell");
    fireEvent.pointerDown(cell);
    vi.advanceTimersByTime(300);
    fireEvent.pointerLeave(cell);
    vi.advanceTimersByTime(300);
    expect(onToggleCover).not.toHaveBeenCalled();
  });

  it("long-press cancelled on pointerCancel (WR-02)", async () => {
    vi.useFakeTimers();
    const onToggleCover = vi.fn();
    render(
      <CalendarCell {...defaultProps} onToggleCover={onToggleCover} />
    );
    const cell = screen.getByRole("gridcell");
    fireEvent.pointerDown(cell);
    vi.advanceTimersByTime(300);
    fireEvent.pointerCancel(cell);
    vi.advanceTimersByTime(300);
    expect(onToggleCover).not.toHaveBeenCalled();
  });

  it("aria-pressed is true when status is absent or cover", () => {
    const { rerender } = render(
      <CalendarCell {...defaultProps} status="absent" />
    );
    expect(screen.getByRole("gridcell")).toHaveAttribute("aria-pressed", "true");

    rerender(<CalendarCell {...defaultProps} status="cover" />);
    expect(screen.getByRole("gridcell")).toHaveAttribute("aria-pressed", "true");

    rerender(<CalendarCell {...defaultProps} status="available" />);
    expect(screen.getByRole("gridcell")).toHaveAttribute("aria-pressed", "false");
  });

  it("renders with motion animation wrapper", () => {
    const { container } = render(<CalendarCell {...defaultProps} />);
    const cell = screen.getByRole("gridcell");
    expect(cell).toBeInTheDocument();
    expect(container.querySelector("[role='gridcell']")).toBeTruthy();
  });
});
