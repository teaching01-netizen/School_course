import { expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import WorkQueueComparison from "./WorkQueueComparison";
import type { ScheduleImpactIssue } from "../../features/scheduleImpact/types";

function baseIssue(overrides: Partial<ScheduleImpactIssue> = {}): ScheduleImpactIssue {
  return {
    id: "issue-1",
    absence_id: "abs-1",
    issue_type: "regular_session_overlap",
    severity: "critical",
    status: "open",
    issue_version: 1,
    wcode: "STU001",
    student_name: "Alice Johnson",
    start_at: "2025-07-24T07:00:00.000Z",
    end_at: "2025-07-24T08:00:00.000Z",
    details: { reasons: ["regular_session_overlap"], old_start_at: "2025-07-24T07:00:00.000Z", new_start_at: "2025-07-24T08:00:00.000Z" },
    suggested_resolutions: [],
    resolution_action: null,
    assignment_context: {
      assigned_at: "2025-07-20T03:00:00.000Z",
      original_session: {
        quality: "exact",
        source: "snapshot",
        snapshot: {
          start_at: "2025-07-24T07:00:00.000Z",
          end_at: "2025-07-24T08:00:00.000Z",
          room_name: "Room 3",
          teacher_name: "Dr Smith",
        },
      },
      current_session: {
        status: "active",
        session_id: "sess-2",
        version: 2,
        start_at: "2025-07-24T08:00:00.000Z",
        end_at: "2025-07-24T09:00:00.000Z",
        course_code: "MATH101",
        course_name: "Mathematics",
        room_name: "Room 5",
        teacher_name: "Dr Jones",
      },
    },
    change_context: {
      change_id: "change-1",
      before: {
        start_at: "2025-07-24T07:00:00.000Z",
        end_at: "2025-07-24T08:00:00.000Z",
      },
      after: {
        start_at: "2025-07-24T08:00:00.000Z",
        end_at: "2025-07-24T09:00:00.000Z",
      },
    },
    impact_context: {
      issue_type: "regular_session_overlap",
      severity: "critical",
      reasons: [{ code: "regular_session_overlap", message: "Overlaps regular class" }],
    },
    ...overrides,
  };
}

it("renders all four comparison rows", () => {
  const issue = baseIssue();
  render(<WorkQueueComparison issue={issue} />);

  expect(screen.getByText("Originally")).toBeInTheDocument();
  expect(screen.getByText("Now")).toBeInTheDocument();
  expect(screen.getByText("Impact")).toBeInTheDocument();
  expect(screen.getByText("Status")).toBeInTheDocument();
});

it("displays original and new times", () => {
  const issue = baseIssue();
  render(<WorkQueueComparison issue={issue} />);

  // The original time and now time should be displayed
  const listItems = screen.getAllByRole("listitem");
  expect(listItems).toHaveLength(4);
});

it("shows impact message with overlap indicator", () => {
  const issue = baseIssue();
  render(<WorkQueueComparison issue={issue} />);

  expect(screen.getByText(/overlaps regular class/i)).toBeInTheDocument();
});

it("shows status as Needs resolution for open issues", () => {
  const issue = baseIssue();
  render(<WorkQueueComparison issue={issue} />);

  expect(screen.getByText("Needs resolution")).toBeInTheDocument();
});

it("shows status for needs_review issues", () => {
  const issue = baseIssue({ status: "needs_review" });
  render(<WorkQueueComparison issue={issue} />);

  expect(screen.getByText("Needs resolution")).toBeInTheDocument();
});

it("shows session deleted when current session is null", () => {
  const issue = baseIssue({
    assignment_context: {
      assigned_at: "2025-07-20T03:00:00.000Z",
      original_session: {
        quality: "exact",
        source: "snapshot",
        snapshot: {
          start_at: "2025-07-24T07:00:00.000Z",
          end_at: "2025-07-24T08:00:00.000Z",
        },
      },
      current_session: null,
    },
  });
  render(<WorkQueueComparison issue={issue} />);

  expect(screen.getByText("Session deleted")).toBeInTheDocument();
});

it("shows session deleted when current session status is deleted", () => {
  const issue = baseIssue({
    assignment_context: {
      assigned_at: "2025-07-20T03:00:00.000Z",
      original_session: {
        quality: "exact",
        source: "snapshot",
        snapshot: {
          start_at: "2025-07-24T07:00:00.000Z",
          end_at: "2025-07-24T08:00:00.000Z",
        },
      },
      current_session: {
        status: "deleted",
        session_id: "sess-2",
        version: 2,
        start_at: "2025-07-24T08:00:00.000Z",
        end_at: "2025-07-24T09:00:00.000Z",
        course_code: "MATH101",
        course_name: "Mathematics",
        room_name: "Room 5",
        teacher_name: "Dr Jones",
      },
    },
  });
  render(<WorkQueueComparison issue={issue} />);

  expect(screen.getByText("Session deleted")).toBeInTheDocument();
});

it("has accessible list semantics", () => {
  const issue = baseIssue();
  render(<WorkQueueComparison issue={issue} />);

  expect(screen.getByRole("list", { name: /schedule comparison for alice johnson/i })).toBeInTheDocument();
  expect(screen.getAllByRole("listitem")).toHaveLength(4);
});

it("distinguishes date-only moves: Originally and Now must differ when the date changed", () => {
  // Hole A repro: Jan 1 09:00 -> Jan 2 09:00 (same wall-clock time, different date).
  // The queue row must show the date, otherwise it looks like "no change".
  const issue = baseIssue({
    assignment_context: {
      assigned_at: "2025-07-20T03:00:00.000Z",
      original_session: {
        quality: "exact",
        source: "snapshot",
        snapshot: {
          start_at: "2025-07-24T02:00:00.000Z", // 09:00 Bangkok, Jul 24
          end_at: "2025-07-24T03:00:00.000Z",
        },
      },
      current_session: {
        status: "active",
        session_id: "sess-2",
        version: 2,
        start_at: "2025-07-25T02:00:00.000Z", // 09:00 Bangkok, Jul 25
        end_at: "2025-07-25T03:00:00.000Z",
        course_code: "MATH101",
        course_name: "Mathematics",
        room_name: "Room 5",
        teacher_name: "Dr Jones",
      },
    },
    change_context: {
      change_id: "change-1",
      before: {
        start_at: "2025-07-24T02:00:00.000Z",
        end_at: "2025-07-24T03:00:00.000Z",
      },
      after: {
        start_at: "2025-07-25T02:00:00.000Z",
        end_at: "2025-07-25T03:00:00.000Z",
      },
    },
  });
  render(<WorkQueueComparison issue={issue} />);

  const listItems = screen.getAllByRole("listitem");
  const originallyText = listItems[0].textContent ?? "";
  const nowText = listItems[1].textContent ?? "";
  // Strip the row labels; the remaining displayed values must differ.
  const originallyValue = originallyText.replace("Originally", "").trim();
  const nowValue = nowText.replace("Now", "").trim();
  expect(originallyValue).not.toEqual(nowValue);
});

it("renders identical values when timestamps are unchanged (no phantom delta)", () => {
  // Acceptance C1: a true no-change must look like no change. Guards against
  // future "always show delta" regressions in the queue row. Note: the
  // component prefers the assignment snapshot / current session over
  // change_context, so all three sources must agree for a true no-change.
  const sameStart = "2025-07-24T02:00:00.000Z";
  const sameEnd = "2025-07-24T03:00:00.000Z";
  const issue = baseIssue({
    assignment_context: {
      assigned_at: "2025-07-20T03:00:00.000Z",
      original_session: {
        quality: "exact",
        source: "snapshot",
        snapshot: { start_at: sameStart, end_at: sameEnd },
      },
      current_session: {
        status: "active",
        session_id: "sess-2",
        version: 2,
        start_at: sameStart,
        end_at: sameEnd,
        course_code: "MATH101",
        course_name: "Mathematics",
        room_name: "Room 5",
        teacher_name: "Dr Jones",
      },
    },
    change_context: {
      change_id: "change-1",
      before: { start_at: sameStart, end_at: sameEnd },
      after: { start_at: sameStart, end_at: sameEnd },
    },
  });
  render(<WorkQueueComparison issue={issue} />);

  const listItems = screen.getAllByRole("listitem");
  const originallyValue = (listItems[0].textContent ?? "").replace("Originally", "").trim();
  const nowValue = (listItems[1].textContent ?? "").replace("Now", "").trim();
  expect(originallyValue).toEqual(nowValue);
});

it("falls back to issue times when change_context before/after is null", () => {
  const issue = baseIssue({
    change_context: {
      change_id: "change-1",
      before: null,
      after: null,
    },
  });
  render(<WorkQueueComparison issue={issue} />);

  // Should still render without crashing
  expect(screen.getByText("Originally")).toBeInTheDocument();
  expect(screen.getByText("Now")).toBeInTheDocument();
});
