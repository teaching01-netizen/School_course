import { fireEvent, render, screen } from "@testing-library/react";
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
    expect(container?.innerHTML).toContain("bg-[var(--color-wi-red)]");
  });

  it("includes an amber dot in the legend", () => {
    renderMonth();
    const container = screen.getByText(/Has sit-ins/i).parentElement;
    expect(container?.innerHTML).toContain("bg-[var(--color-wi-amber)]");
  });

  it("includes a green dot in the legend", () => {
    renderMonth();
    const container = screen.getByText(/All OK/i).parentElement;
    expect(container?.innerHTML).toContain("bg-[var(--color-wi-green)]");
  });
});

describe("CalendarMonth keyboard shortcuts", () => {
  function renderHandlers() {
    const handlers = { onPrevMonth: vi.fn(), onNextMonth: vi.fn(), onToday: vi.fn() };
    render(
      <CalendarMonth
        viewMonthKey="2026-06-01"
        sessions={[]}
        todayKey="2026-06-21"
        zone="Asia/Bangkok"
        selectedDayKey={null}
        onSelectDay={vi.fn()}
        onPrevMonth={handlers.onPrevMonth}
        onNextMonth={handlers.onNextMonth}
        onToday={handlers.onToday}
      />,
    );
    return handlers;
  }

  it("navigates months with arrow keys and today with t when focus is not in an editable element", () => {
    const handlers = renderHandlers();
    fireEvent.keyDown(document.body, { key: "ArrowLeft" });
    fireEvent.keyDown(document.body, { key: "ArrowRight" });
    fireEvent.keyDown(document.body, { key: "t" });
    expect(handlers.onPrevMonth).toHaveBeenCalledTimes(1);
    expect(handlers.onNextMonth).toHaveBeenCalledTimes(1);
    expect(handlers.onToday).toHaveBeenCalledTimes(1);
  });

  it("ignores arrow shortcuts while an input has focus", () => {
    const handlers = renderHandlers();
    const input = document.createElement("input");
    document.body.appendChild(input);
    input.focus();
    fireEvent.keyDown(input, { key: "ArrowLeft" });
    expect(handlers.onPrevMonth).not.toHaveBeenCalled();
    input.remove();
  });

  it("ignores the t shortcut while text is selected in a contenteditable element", () => {
    const handlers = renderHandlers();
    const editor = document.createElement("div");
    editor.setAttribute("contenteditable", "true");
    document.body.appendChild(editor);
    editor.focus();
    fireEvent.keyDown(editor, { key: "t" });
    expect(handlers.onToday).not.toHaveBeenCalled();
    editor.remove();
  });

  it("ignores shortcuts while a dialog is the keydown target", () => {
    const handlers = renderHandlers();
    const dialog = document.createElement("div");
    dialog.setAttribute("role", "dialog");
    document.body.appendChild(dialog);
    dialog.focus();
    fireEvent.keyDown(dialog, { key: "ArrowRight" });
    expect(handlers.onNextMonth).not.toHaveBeenCalled();
    dialog.remove();
  });
});
