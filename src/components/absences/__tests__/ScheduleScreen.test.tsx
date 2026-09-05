import { act, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import ScheduleScreen from "../public-form/ScheduleScreen";
import type { SubjectSessions } from "@/features/absences/types";

/** Local calendar date key N days from today. */
function dateKey(dayOffset: number): string {
  const d = new Date();
  d.setDate(d.getDate() + dayOffset);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

function iso(hour: number, dayOffset: number): string {
  const d = new Date();
  d.setDate(d.getDate() + dayOffset);
  d.setHours(hour, 0, 0, 0);
  return d.toISOString();
}

const MATH_GROUP: SubjectSessions = {
  subject_id: "subject-math",
  subject_code: "MATH",
  subject_name: "Mathematics",
  course_id: "course-math",
  course_code: "MATH201",
  course_name: "Mathematics",
  teacher_name: "Ms. Jane",
  sessions: [
    {
      id: "session-math-1",
      start_at: iso(17, 0),
      end_at: iso(19, 0),
      date: dateKey(0),
      already_absent: false,
    },
  ],
};

function renderSchedule() {
  const onToggleDay = vi.fn((_group: SubjectSessions, _sessionIds: string[]) => true);
  const onLimitTap = vi.fn();
  render(
    <ScheduleScreen
      groups={[MATH_GROUP]}
      selectedIds={new Set(["session-math-1"])}
      onToggleDay={onToggleDay}
      sitInSelections={{}}
      onLimitTap={onLimitTap}
    />,
  );
  return { onToggleDay };
}

function mathRow(): HTMLElement {
  const row = Array.from(document.querySelectorAll("label")).find(
    (label) => label.textContent?.includes("Mathematics") && label.querySelector('input[type="checkbox"]'),
  );
  if (!row) throw new Error("No Mathematics schedule row found");
  return row;
}

/** User-event's pointer clicks deadlock under this repo's vitest fake timers,
 *  so interactions use act-wrapped native clicks — the same React onChange
 *  contract the real pointer would hit. */
function removeMathRow() {
  act(() => {
    mathRow().click();
  });
}

function undoToast(): HTMLElement | null {
  return screen.queryByRole("status");
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

it("renders the removal toast inside the bottom-anchored sticky undo stack", () => {
  renderSchedule();

  removeMathRow();

  const toast = undoToast();
  expect(toast).toBeInTheDocument();
  expect(toast).toHaveClass("absence-undo-toast");
  expect(toast).toHaveTextContent("Mathematics removed");
  expect(screen.getByRole("button", { name: "Undo" })).toBeInTheDocument();

  // The toast lives in the sticky stack that CSS anchors to the bottom of the
  // scrolling main — and the stack is the last block of the screen, so a
  // removal deep in the agenda is never answered at the top of the page.
  const stack = toast?.closest(".absence-undo-stack");
  expect(stack).not.toBeNull();
  expect(stack?.parentElement?.lastElementChild).toBe(stack);
});

it("auto-dismisses the toast after six seconds without touching the selection", () => {
  const { onToggleDay } = renderSchedule();

  removeMathRow();
  expect(undoToast()).toBeInTheDocument();

  act(() => {
    vi.advanceTimersByTime(6000);
  });

  expect(undoToast()).not.toBeInTheDocument();
  // Dismissal is not a decision: the removal stays removed.
  expect(onToggleDay).toHaveBeenCalledTimes(1);
});