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
