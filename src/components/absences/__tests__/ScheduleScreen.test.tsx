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

it("states how many sessions a class day includes so a merged grouping is never a surprise", () => {
  const single = MATH_GROUP.sessions[0];
  const mergedGroup: SubjectSessions = {
    ...MATH_GROUP,
    merge_group_id: "merge-math",
    merge_group_name: "Mathematics",
    sessions: [
      single,
      {
        id: "session-math-2",
        start_at: single.start_at,
        end_at: iso(21, 0),
        date: single.date,
        already_absent: false,
      },
    ],
  };
  render(
    <ScheduleScreen
      groups={[mergedGroup]}
      selectedIds={new Set()}
      onToggleDay={vi.fn(() => true)}
      onLimitTap={vi.fn()}
    />,
  );

  expect(screen.getByText(/includes 2 sessions/i)).toBeInTheDocument();
  // Backend session counts are never described as "classes": a merged pair is
  // still one class day.
  expect(screen.queryByText(/2 classes/i)).not.toBeInTheDocument();
});

it("offers an empty day a path forward to the next available class day", () => {
  // The only class sits six days out. Its month grid always has earlier empty
  // cells (the six-week grid starts before the class date), so the focused day
  // can be empty while a later reportable day exists to jump to.
  const laterOnly: SubjectSessions = {
    ...MATH_GROUP,
    sessions: [
      {
        id: "session-math-later",
        start_at: iso(17, 6),
        end_at: iso(19, 6),
        date: dateKey(6),
        already_absent: false,
      },
    ],
  };
  render(
    <ScheduleScreen
      groups={[laterOnly]}
      selectedIds={new Set()}
      onToggleDay={vi.fn(() => true)}
      onLimitTap={vi.fn()}
    />,
  );

  const expand = screen.getByRole("button", { name: /show the whole month/i });
  act(() => {
    expand.click();
  });

  const emptyCell = Array.from(document.querySelectorAll<HTMLElement>("[data-date-key]"))
    .filter((cell) => (cell.getAttribute("aria-label") ?? "").includes("No classes"))
    .find((cell) => (cell.getAttribute("data-date-key") ?? "") < dateKey(6));
  expect(emptyCell).not.toBeNull();
  act(() => {
    emptyCell!.click();
  });

  expect(screen.getByText(/no classes this day/i)).toBeInTheDocument();
  const jump = screen.getByRole("button", { name: /next available class day/i });
  expect(jump).toBeInTheDocument();
  act(() => {
    jump.click();
  });
  expect(screen.getByRole("checkbox", { name: /mathematics/i })).toBeInTheDocument();
});