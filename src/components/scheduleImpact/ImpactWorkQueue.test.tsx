import { expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import ImpactWorkQueue from "./ImpactWorkQueue";
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
    details: { reasons: ["regular_session_overlap"] },
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
      before: { start_at: "2025-07-24T07:00:00.000Z", end_at: "2025-07-24T08:00:00.000Z" },
      after: { start_at: "2025-07-24T08:00:00.000Z", end_at: "2025-07-24T09:00:00.000Z" },
    },
    impact_context: {
      issue_type: "regular_session_overlap",
      severity: "critical",
      reasons: [{ code: "regular_session_overlap", message: "Overlaps regular class" }],
    },
    ...overrides,
  };
}

it("shows empty state when no items", () => {
  render(<ImpactWorkQueue items={[]} density="comfortable" selectedID={null} onOpen={vi.fn()} />);
  expect(screen.getByText(/no student arrangements need attention/i)).toBeInTheDocument();
});

it("renders issue in comfortable density with comparison", () => {
  const issue = baseIssue();
  const onOpen = vi.fn();
  render(<ImpactWorkQueue items={[issue]} density="comfortable" selectedID={null} onOpen={onOpen} />);

  expect(screen.getByText("Alice Johnson")).toBeInTheDocument();
  expect(screen.getByText("Originally")).toBeInTheDocument();
  expect(screen.getByText("Now")).toBeInTheDocument();
  expect(screen.getByText("Impact")).toBeInTheDocument();
  expect(screen.getByText("Status")).toBeInTheDocument();
});

it("renders issue in compact density with comparison in table", () => {
  const issue = baseIssue();
  const onOpen = vi.fn();
  render(<ImpactWorkQueue items={[issue]} density="compact" selectedID={null} onOpen={onOpen} />);

  expect(screen.getByText("Alice Johnson")).toBeInTheDocument();
  // Compact mode should show the table headers
  expect(screen.getByText("Originally / Now")).toBeInTheDocument();
});

it("calls onOpen when Review button is clicked", async () => {
  const user = userEvent.setup();
  const issue = baseIssue();
  const onOpen = vi.fn();
  render(<ImpactWorkQueue items={[issue]} density="comfortable" selectedID={null} onOpen={onOpen} />);

  await user.click(screen.getByRole("button", { name: /review/i }));
  expect(onOpen).toHaveBeenCalledWith(issue);
});

it("shows reconstructed badge for reconstructed snapshots", () => {
  const issue = baseIssue({
    assignment_context: {
      assigned_at: "2025-07-20T03:00:00.000Z",
      original_session: {
        quality: "reconstructed",
        source: "inferred",
        snapshot: {
          start_at: "2025-07-24T07:00:00.000Z",
          end_at: "2025-07-24T08:00:00.000Z",
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
  });
  render(<ImpactWorkQueue items={[issue]} density="comfortable" selectedID={null} onOpen={vi.fn()} />);

  expect(screen.getByText("Reconstructed")).toBeInTheDocument();
});

it("shows deleted session message when current session is deleted", () => {
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
  render(<ImpactWorkQueue items={[issue]} density="comfortable" selectedID={null} onOpen={vi.fn()} />);

  expect(screen.getByText(/assigned session has been deleted/i)).toBeInTheDocument();
});

it("highlights selected issue", () => {
  const issue = baseIssue();
  render(<ImpactWorkQueue items={[issue]} density="comfortable" selectedID="issue-1" onOpen={vi.fn()} />);

  const article = screen.getByRole("article");
  expect(article.className).toContain("bg-blue-50/60");
});
