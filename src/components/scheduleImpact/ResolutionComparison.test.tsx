import { expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
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

it("renders three sections: what changed, why, and actions", () => {
  const issue = baseIssue();
  render(<ResolutionComparison issue={issue} selectedCandidate={candidate} onAction={vi.fn()} busy={false} resolutionError={null} />);

  expect(screen.getByText("What changed")).toBeInTheDocument();
  expect(screen.getByText("Why this needs attention")).toBeInTheDocument();
  expect(screen.getByText("What should happen?")).toBeInTheDocument();
});

it("displays the original assignment details", () => {
  const issue = baseIssue();
  render(<ResolutionComparison issue={issue} selectedCandidate={candidate} onAction={vi.fn()} busy={false} resolutionError={null} />);

  expect(screen.getAllByText(/Room 3/).length).toBeGreaterThanOrEqual(1);
  expect(screen.getAllByText(/Dr Smith/).length).toBeGreaterThanOrEqual(1);
});

it("displays the current session details", () => {
  const issue = baseIssue();
  render(<ResolutionComparison issue={issue} selectedCandidate={candidate} onAction={vi.fn()} busy={false} resolutionError={null} />);

  expect(screen.getAllByText(/Room 5/).length).toBeGreaterThanOrEqual(1);
  expect(screen.getAllByText(/Dr Jones/).length).toBeGreaterThanOrEqual(1);
});

it("renders names from canonical snapshot entities", () => {
  const issue = baseIssue({
    assignment_context: {
      ...baseIssue().assignment_context,
      original_session: {
        ...baseIssue().assignment_context.original_session,
        snapshot: {
          start_at: "2025-07-24T07:00:00.000Z",
          end_at: "2025-07-24T08:00:00.000Z",
          course: { id: "course-1", code: "MATH101", name: "Mathematics" },
          room: { id: "room-3", name: "Room 3" },
          teacher: { id: "teacher-3", name: "Dr Smith" },
        },
      },
    },
  });
  render(<ResolutionComparison issue={issue} selectedCandidate={candidate} onAction={vi.fn()} busy={false} resolutionError={null} />);

  expect(screen.getByText("Room 3")).toBeInTheDocument();
  expect(screen.getByText("Dr Smith")).toBeInTheDocument();
});

it("shows impact explanation for overlap", () => {
  const issue = baseIssue();
  render(<ResolutionComparison issue={issue} selectedCandidate={candidate} onAction={vi.fn()} busy={false} resolutionError={null} />);

  expect(screen.getByText(/overlaps/i)).toBeInTheDocument();
  expect(screen.getByText(/regular class/i)).toBeInTheDocument();
});

it("renders action options as radio buttons", () => {
  const issue = baseIssue();
  render(<ResolutionComparison issue={issue} selectedCandidate={candidate} onAction={vi.fn()} busy={false} resolutionError={null} />);

  expect(screen.getByText("Move to another session")).toBeInTheDocument();
  expect(screen.getByText("Keep the current arrangement")).toBeInTheDocument();
  expect(screen.getByText("Cancel the sit-in")).toBeInTheDocument();
  expect(screen.getByText("Ask another administrator to review")).toBeInTheDocument();
  expect(screen.getAllByRole("radio").length).toBeGreaterThanOrEqual(4);
});

it("calls onAction when an action radio is clicked", async () => {
  const user = userEvent.setup();
  const onAction = vi.fn();
  const issue = baseIssue();
  render(<ResolutionComparison issue={issue} selectedCandidate={candidate} onAction={onAction} busy={false} resolutionError={null} />);

  await user.click(screen.getByText("Move to another session"));
  expect(onAction).toHaveBeenCalledWith("reassign");

  await user.click(screen.getByText("Keep the current arrangement"));
  expect(onAction).toHaveBeenCalledWith("keep");

  await user.click(screen.getByText("Cancel the sit-in"));
  expect(onAction).toHaveBeenCalledWith("cancel");

  await user.click(screen.getByText("Ask another administrator to review"));
  expect(onAction).toHaveBeenCalledWith("mark_for_review");
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

  expect(screen.getByText(/assigned session has been deleted/i)).toBeInTheDocument();
});

it("has accessible action group", () => {
  const issue = baseIssue();
  render(<ResolutionComparison issue={issue} selectedCandidate={candidate} onAction={vi.fn()} busy={false} resolutionError={null} />);

  const actionGroup = screen.getByRole("radiogroup", { name: /resolution actions/i });
  expect(actionGroup).toBeInTheDocument();
});

it("uses backend policy verbatim, overriding the frontend defaults", () => {
  const issue = baseIssue({
    action_policy: [{ action: "keep", allowed: true, reason_required: false, disabled_reason: null, notification_expected: true }],
  });
  render(<ResolutionComparison issue={issue} selectedCandidate={candidate} onAction={vi.fn()} busy={false} resolutionError={null} />);

  expect(screen.getByText("Keep the current arrangement")).toBeInTheDocument();
  expect(screen.queryByText("Move to another session")).not.toBeInTheDocument();
  expect(screen.queryByText("Cancel the sit-in")).not.toBeInTheDocument();
  expect(screen.queryByText("Ask another administrator to review")).not.toBeInTheDocument();
  expect(screen.getAllByRole("radio")).toHaveLength(1);
});

it("disables an allowed action with a disabled_reason and shows the reason", () => {
  const issue = baseIssue({
    action_policy: [{ action: "reassign", allowed: true, reason_required: false, disabled_reason: "No replacement sessions exist", notification_expected: true }],
  });
  render(<ResolutionComparison issue={issue} selectedCandidate={candidate} onAction={vi.fn()} busy={false} resolutionError={null} />);

  expect(screen.getByText("Move to another session")).toBeInTheDocument();
  expect(screen.getByText("No replacement sessions exist")).toBeInTheDocument();
  expect(screen.getByRole("radio")).toBeDisabled();
});

it("renders a disallowed action as a non-interactive row that never calls onAction", async () => {
  const user = userEvent.setup();
  const onAction = vi.fn();
  const issue = baseIssue({
    action_policy: [
      { action: "cancel", allowed: false, reason_required: false, disabled_reason: "Sit-in already cancelled", notification_expected: true },
      { action: "keep", allowed: true, reason_required: false, disabled_reason: null, notification_expected: true },
    ],
  });
  render(<ResolutionComparison issue={issue} selectedCandidate={candidate} onAction={onAction} busy={false} resolutionError={null} />);

  expect(screen.getByText("Cancel the sit-in")).toBeInTheDocument();
  expect(screen.getByText("Sit-in already cancelled")).toBeInTheDocument();
  const radios = screen.getAllByRole("radio");
  expect(radios).toHaveLength(2);
  expect(radios.filter((r) => (r as HTMLInputElement).disabled)).toHaveLength(1);

  await user.click(screen.getByText("Sit-in already cancelled"));
  expect(onAction).not.toHaveBeenCalled();

  await user.click(screen.getByText("Keep the current arrangement"));
  expect(onAction).toHaveBeenCalledWith("keep");
});

it("shows short_notice_change impact copy", () => {
  const issue = baseIssue({
    issue_type: "short_notice_change",
    impact_context: {
      issue_type: "short_notice_change",
      severity: "warning",
      reasons: [{ code: "short_notice_change", message: "Changed on short notice" }],
    },
  });
  render(<ResolutionComparison issue={issue} selectedCandidate={candidate} onAction={vi.fn()} busy={false} resolutionError={null} />);

  expect(screen.getByText("The student needs a clear update before the session begins.")).toBeInTheDocument();
});

it("shows past_time_change impact copy", () => {
  const issue = baseIssue({
    issue_type: "past_time_change",
    impact_context: {
      issue_type: "past_time_change",
      severity: "warning",
      reasons: [{ code: "past_time_change", message: "Changed to past time" }],
    },
  });
  render(<ResolutionComparison issue={issue} selectedCandidate={candidate} onAction={vi.fn()} busy={false} resolutionError={null} />);

  expect(screen.getByText("The original arrangement can no longer be used.")).toBeInTheDocument();
});

it("shows the deleted-session impact copy for a deleted current session", () => {
  render(
    <ResolutionComparison
      issue={baseIssue({
        assignment_context: {
          ...baseIssue().assignment_context,
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
      })}
      selectedCandidate={candidate}
      onAction={vi.fn()}
      busy={false}
      resolutionError={null}
    />
  );
  expect(screen.getByText("The assigned session has been deleted. The student needs a new arrangement.")).toBeInTheDocument();
});

it("disables all allowed action radios while busy", () => {
  const issue = baseIssue();
  render(<ResolutionComparison issue={issue} selectedCandidate={candidate} onAction={vi.fn()} busy={true} resolutionError={null} />);
  for (const radio of screen.getAllByRole("radio")) {
    expect(radio).toBeDisabled();
  }

  cleanup();

  render(<ResolutionComparison issue={issue} selectedCandidate={candidate} onAction={vi.fn()} busy={false} resolutionError={null} />);
  for (const radio of screen.getAllByRole("radio")) {
    expect(radio).toBeEnabled();
  }
});
