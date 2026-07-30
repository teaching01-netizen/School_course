import { expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import ResolutionComparison from "./ResolutionComparison";
import type { ScheduleImpactIssue, ImpactCandidate } from "../../features/scheduleImpact/types";

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
          course_code: "MATH101",
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
        room_name: "Room 3",
        teacher_name: "Dr Smith",
      },
      after: {
        start_at: "2025-07-24T08:00:00.000Z",
        end_at: "2025-07-24T09:00:00.000Z",
        room_name: "Room 5",
        teacher_name: "Dr Jones",
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

const candidate: ImpactCandidate = {
  session_id: "sess-candidate",
  session_version: 1,
  start_at: "2025-07-24T08:00:00.000Z",
  end_at: "2025-07-24T09:00:00.000Z",
  course_code: "MATH101",
  course_name: "Mathematics",
  room_name: "Room 5",
  teacher: "Dr Jones",
  available_capacity: 10,
  eligible: true,
  student_conflicts: false,
  generated_at: "2025-07-24T06:00:00.000Z",
};

it("renders all four sections of the resolution comparison", () => {
  const issue = baseIssue();
  const onAction = vi.fn();
  render(<ResolutionComparison issue={issue} selectedCandidate={candidate} onAction={onAction} busy={false} resolutionError={null} />);

  expect(screen.getByRole("heading", { name: /originally assigned/i })).toBeInTheDocument();
  expect(screen.getByRole("heading", { name: /session now/i })).toBeInTheDocument();
  expect(screen.getByRole("heading", { name: /changes/i })).toBeInTheDocument();
  expect(screen.getByRole("heading", { name: /impact and actions/i })).toBeInTheDocument();
});

it("displays the original assignment details", () => {
  const issue = baseIssue();
  render(<ResolutionComparison issue={issue} selectedCandidate={candidate} onAction={vi.fn()} busy={false} resolutionError={null} />);

  expect(screen.getAllByText(/Room 3/).length).toBeGreaterThanOrEqual(1);
  expect(screen.getAllByText(/Dr Smith/).length).toBeGreaterThanOrEqual(1);
  expect(screen.getByText(/Captured when assigned on/)).toBeInTheDocument();
});

it("displays the current session details", () => {
  const issue = baseIssue();
  render(<ResolutionComparison issue={issue} selectedCandidate={candidate} onAction={vi.fn()} busy={false} resolutionError={null} />);

  expect(screen.getAllByText(/Room 5/).length).toBeGreaterThanOrEqual(1);
  expect(screen.getAllByText(/Dr Jones/).length).toBeGreaterThanOrEqual(1);
});

it("shows only changed fields in the changes section", () => {
  const issue = baseIssue();
  render(<ResolutionComparison issue={issue} selectedCandidate={candidate} onAction={vi.fn()} busy={false} resolutionError={null} />);

  expect(screen.getByText("Time")).toBeInTheDocument();
  expect(screen.getByText("Room")).toBeInTheDocument();
  expect(screen.getByText("Teacher")).toBeInTheDocument();
});

it("shows impact message and overlap warning", () => {
  const issue = baseIssue();
  render(<ResolutionComparison issue={issue} selectedCandidate={candidate} onAction={vi.fn()} busy={false} resolutionError={null} />);

  expect(screen.getAllByText(/overlaps/i).length).toBeGreaterThanOrEqual(1);
  expect(screen.getAllByText(/regular class/i).length).toBeGreaterThanOrEqual(1);
});

it("renders action buttons with correct labels", () => {
  const issue = baseIssue();
  render(<ResolutionComparison issue={issue} selectedCandidate={candidate} onAction={vi.fn()} busy={false} resolutionError={null} />);

  expect(screen.getByRole("button", { name: /reassign sit-in/i })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: /keep current arrangement/i })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: /mark for manual review/i })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: /cancel arrangement/i })).toBeInTheDocument();
});

it("calls onAction when an action button is clicked", async () => {
  const user = userEvent.setup();
  const onAction = vi.fn();
  const issue = baseIssue();
  render(<ResolutionComparison issue={issue} selectedCandidate={candidate} onAction={onAction} busy={false} resolutionError={null} />);

  await user.click(screen.getByRole("button", { name: /reassign sit-in/i }));
  expect(onAction).toHaveBeenCalledWith("reassign");

  await user.click(screen.getByRole("button", { name: /keep current arrangement/i }));
  expect(onAction).toHaveBeenCalledWith("keep");

  await user.click(screen.getByRole("button", { name: /cancel arrangement/i }));
  expect(onAction).toHaveBeenCalledWith("cancel");

  await user.click(screen.getByRole("button", { name: /mark for manual review/i }));
  expect(onAction).toHaveBeenCalledWith("mark_for_review");
});

it("disables reassign button when no candidate is selected", () => {
  const issue = baseIssue();
  render(<ResolutionComparison issue={issue} selectedCandidate={undefined} onAction={vi.fn()} busy={false} resolutionError={null} />);

  expect(screen.getByRole("button", { name: /reassign sit-in/i })).toBeDisabled();
});

it("disables all action buttons when busy", () => {
  const issue = baseIssue();
  render(<ResolutionComparison issue={issue} selectedCandidate={candidate} onAction={vi.fn()} busy={true} resolutionError={null} />);

  expect(screen.getByRole("button", { name: /reassign sit-in/i })).toBeDisabled();
  expect(screen.getByRole("button", { name: /keep current arrangement/i })).toBeDisabled();
  expect(screen.getByRole("button", { name: /mark for manual review/i })).toBeDisabled();
  expect(screen.getByRole("button", { name: /cancel arrangement/i })).toBeDisabled();
});

it("shows resolution error when provided", () => {
  const issue = baseIssue();
  render(<ResolutionComparison issue={issue} selectedCandidate={candidate} onAction={vi.fn()} busy={false} resolutionError="Something went wrong" />);

  expect(screen.getByRole("alert")).toHaveTextContent("Something went wrong");
});

it("shows reconstructed legacy badge for reconstructed snapshots", () => {
  const issue = baseIssue({
    assignment_context: {
      assigned_at: "2025-07-20T03:00:00.000Z",
      original_session: {
        quality: "reconstructed",
        source: "inferred",
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
  });
  render(<ResolutionComparison issue={issue} selectedCandidate={candidate} onAction={vi.fn()} busy={false} resolutionError={null} />);

  expect(screen.getByText(/reconstructed from schedule history/i)).toBeInTheDocument();
});

it("shows unavailable message when snapshot is unavailable", () => {
  const issue = baseIssue({
    assignment_context: {
      assigned_at: null,
      original_session: {
        quality: "unavailable",
        source: "none",
        snapshot: null,
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
  render(<ResolutionComparison issue={issue} selectedCandidate={candidate} onAction={vi.fn()} busy={false} resolutionError={null} />);

  expect(screen.getByText(/original assignment details unavailable/i)).toBeInTheDocument();
  expect(screen.getByText(/created before historical snapshots were recorded/i)).toBeInTheDocument();
});

it("shows deleted session state when current session is null", () => {
  const issue = baseIssue({
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
      current_session: null,
    },
  });
  render(<ResolutionComparison issue={issue} selectedCandidate={candidate} onAction={vi.fn()} busy={false} resolutionError={null} />);

  expect(screen.getByText(/original session has been deleted/i)).toBeInTheDocument();
});

it("has accessible section headings and live region", () => {
  const issue = baseIssue();
  render(<ResolutionComparison issue={issue} selectedCandidate={candidate} onAction={vi.fn()} busy={false} resolutionError={null} />);

  // Check that the region has an accessible label
  const region = screen.getByRole("region", { name: /resolution comparison/i });
  expect(region).toBeInTheDocument();

  // Check that action buttons are in a group
  const actionGroup = screen.getByRole("group", { name: /resolution actions/i });
  expect(actionGroup).toBeInTheDocument();
});
