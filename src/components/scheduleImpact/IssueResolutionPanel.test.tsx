import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import IssueResolutionPanel from "./IssueResolutionPanel";
import type { ImpactCandidate, ResolutionResponse, ScheduleImpactIssue } from "../../features/scheduleImpact/types";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, apiJson: mockApiJson };
});

function baseIssue(overrides: Partial<ScheduleImpactIssue> = {}): ScheduleImpactIssue {
  return {
    id: "issue-1",
    absence_id: "abs-1",
    issue_type: "sit_in_session_changed",
    severity: "critical",
    status: "open",
    issue_version: 1,
    wcode: "STU001",
    student_name: "Alice Johnson",
    start_at: "2025-07-24T07:00:00.000Z",
    end_at: "2025-07-24T08:00:00.000Z",
    details: { reasons: ["sit_in_session_changed"] },
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
    change_context: { change_id: "change-1", before: null, after: null },
    impact_context: {
      issue_type: "sit_in_session_changed",
      severity: "critical",
      reasons: [{ code: "sit_in_session_changed", message: "Sit-in session changed" }],
    },
    ...overrides,
  };
}

const candidate: ImpactCandidate = {
  session_id: "cand-1",
  session_version: 3,
  start_at: "2025-08-14T03:00:00.000Z",
  end_at: "2025-08-14T04:30:00.000Z",
  course_code: "MATH101",
  course_name: "Mathematics",
  room_name: "Room 9",
  teacher: "Dr Jones",
  available_capacity: 5,
  eligible: true,
  student_conflicts: false,
  generated_at: "2025-07-20T00:00:00.000Z",
};

const queuedResolution: ResolutionResponse = {
  id: "issue-1",
  status: "resolved",
  action: "keep",
  notification_status: "queued",
};

beforeEach(() => {
  mockApiJson.mockReset();
  mockApiJson.mockImplementation((url: string) => {
    if (url.includes("/candidates")) return Promise.resolve({ items: [candidate] });
    if (url.includes("/activity")) return Promise.resolve({ items: [] });
    return Promise.reject(new Error(`unexpected apiJson call: ${url}`));
  });
});

async function renderPanel(props: Partial<Parameters<typeof IssueResolutionPanel>[0]> = {}) {
  const onResolve = props.onResolve ?? vi.fn().mockResolvedValue(queuedResolution);
  const onClose = props.onClose ?? vi.fn();
  render(
    <IssueResolutionPanel
      issue={props.issue ?? baseIssue()}
      initialAction={props.initialAction ?? null}
      onClose={onClose}
      onResolve={onResolve}
    />,
  );
  await waitFor(() => expect(mockApiJson).toHaveBeenCalledWith(expect.stringContaining("/candidates")));
  return { onResolve, onClose };
}

describe("IssueResolutionPanel notification UX", () => {
  // UI-NOTIF-001: selecting keep shows a confirmation that states the student
  // will receive a notification ("Keep and notify?").
  it("UI-NOTIF-001: keep shows a keep-and-notify confirmation", async () => {
    const user = userEvent.setup();
    await renderPanel();

    await user.click(await screen.findByRole("button", { name: "Keep current arrangement" }));

    expect(await screen.findByText("Keep and notify?")).toBeInTheDocument();
    const confirmSection = screen.getByText("Keep and notify?").closest("section")!;
    expect(within(confirmSection).getByRole("button", { name: "Confirm" })).toBeEnabled();
  });

  // UI-NOTIF-005 (current contract): a successful resolution shows the
  // generic success receipt.
  it("UI-NOTIF-005: successful resolution shows the saved receipt", async () => {
    const user = userEvent.setup();
    await renderPanel();

    await user.click(await screen.findByRole("button", { name: "Keep current arrangement" }));
    await user.click(await screen.findByRole("button", { name: "Confirm" }));

    expect(await screen.findByText("Resolution saved successfully")).toBeInTheDocument();
  });

  // UI-NOTIF-006 (current contract): when the student was not notified the
  // panel must not claim otherwise — still just the generic saved receipt.
  it("UI-NOTIF-006: not-notified resolution does not promise a notification", async () => {
    const user = userEvent.setup();
    const onResolve = vi.fn().mockResolvedValue({ ...queuedResolution, notification_status: "not_configured" });
    await renderPanel({ onResolve });

    await user.click(await screen.findByRole("button", { name: "Keep current arrangement" }));
    await user.click(await screen.findByRole("button", { name: "Confirm" }));

    expect(await screen.findByText("Resolution saved successfully")).toBeInTheDocument();
    expect(screen.queryByText(/notification queued/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/student was notified/i)).not.toBeInTheDocument();
  });

  // UI-NOTIF-007: double-clicking confirm submits exactly one request.
  it("UI-NOTIF-007: double-click on confirm submits a single request", async () => {
    const user = userEvent.setup();
    let resolveRequest: (value: ResolutionResponse) => void = () => {};
    const onResolve = vi.fn(
      () =>
        new Promise<ResolutionResponse>((resolve) => {
          resolveRequest = resolve;
        }),
    );
    await renderPanel({ onResolve });

    await user.click(await screen.findByRole("button", { name: "Keep current arrangement" }));
    const confirmButton = await screen.findByRole("button", { name: "Confirm" });
    await user.click(confirmButton);
    await user.click(confirmButton);

    expect(onResolve).toHaveBeenCalledTimes(1);
    resolveRequest(queuedResolution);
  });

  // UI-NOTIF-008: a 409 conflict (onResolve returns null) refreshes the queue
  // and never shows a notification-success message.
  it("UI-NOTIF-008: resolution conflict refreshes without a success message", async () => {
    const user = userEvent.setup();
    const onResolve = vi.fn().mockResolvedValue(null);
    await renderPanel({ onResolve });

    await user.click(await screen.findByRole("button", { name: "Keep current arrangement" }));
    await user.click(await screen.findByRole("button", { name: "Confirm" }));

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/this issue changed while you were reviewing it/i);
    expect(screen.queryByText("Resolution saved successfully")).not.toBeInTheDocument();
    // The candidate list is refreshed after a conflict.
    await waitFor(() => {
      const candidateCalls = mockApiJson.mock.calls.filter(([url]) => String(url).includes("/candidates"));
      expect(candidateCalls.length).toBeGreaterThanOrEqual(2);
    });
  });

  // UI-NOTIF-010: mark for review requires a reason and shows no notification
  // promise.
  it("UI-NOTIF-010: mark for review requires a reason and promises no notification", async () => {
    const user = userEvent.setup();
    const onResolve = vi.fn().mockResolvedValue({ ...queuedResolution, notification_status: "not_required" });
    await renderPanel({ onResolve });

    await user.click(await screen.findByRole("button", { name: "Mark for manual review" }));

    expect(await screen.findByText("Mark for review?")).toBeInTheDocument();
    const confirmSection = screen.getByText("Mark for review?").closest("section")!;
    // No notification promise in the confirmation copy.
    expect(within(confirmSection).queryByText(/notif/i)).not.toBeInTheDocument();

    // Confirm stays disabled until a reason is selected.
    expect(within(confirmSection).getByRole("button", { name: "Confirm" })).toBeDisabled();
    await user.selectOptions(within(confirmSection).getByLabelText("Reason"), "Needs owner review");
    expect(within(confirmSection).getByRole("button", { name: "Confirm" })).toBeEnabled();

    await user.click(within(confirmSection).getByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(onResolve).toHaveBeenCalledWith(expect.anything(), "mark_for_review", undefined, "Needs owner review"));
  });

  // UI-NOTIF-010: dismiss (entered via initialAction) requires a reason and
  // promises no notification.
  it("UI-NOTIF-010: dismiss requires a reason and promises no notification", async () => {
    const user = userEvent.setup();
    const onResolve = vi.fn().mockResolvedValue({ ...queuedResolution, notification_status: "not_required" });
    await renderPanel({ initialAction: "dismiss", onResolve });

    expect(await screen.findByText("Dismiss this issue?")).toBeInTheDocument();
    const confirmSection = screen.getByText("Dismiss this issue?").closest("section")!;
    expect(within(confirmSection).queryByText(/notif/i)).not.toBeInTheDocument();

    const confirmButton = within(confirmSection).getByRole("button", { name: "Confirm" });
    expect(confirmButton).toBeDisabled();
    await user.selectOptions(within(confirmSection).getByLabelText("Reason"), "Duplicate issue");
    await user.click(within(confirmSection).getByRole("button", { name: "Confirm" }));

    await waitFor(() => expect(onResolve).toHaveBeenCalledWith(expect.anything(), "dismiss", undefined, "Duplicate issue"));
  });

  // Cancel keeps its own confirmation copy.
  it("cancel shows a dedicated confirmation without a notification promise", async () => {
    const user = userEvent.setup();
    const onResolve = vi.fn().mockResolvedValue({ ...queuedResolution, action: "cancel" });
    await renderPanel({ onResolve });

    await user.click(await screen.findByRole("button", { name: "Cancel arrangement" }));

    expect(await screen.findByText("Cancel this sit-in?")).toBeInTheDocument();
    const confirmSection = screen.getByText("Cancel this sit-in?").closest("section")!;
    expect(within(confirmSection).queryByText(/will receive|will be notified/i)).not.toBeInTheDocument();
    await user.click(within(confirmSection).getByRole("button", { name: "Confirm" }));

    await waitFor(() => expect(onResolve).toHaveBeenCalledWith(expect.anything(), "cancel", undefined, ""));
  });
});
