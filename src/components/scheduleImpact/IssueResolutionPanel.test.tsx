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
  it("UI-NOTIF-001: keep shows a keep-and-notify confirmation", async () => {
    const user = userEvent.setup();
    await renderPanel();

    await user.click(await screen.findByText("Keep the current arrangement"));

    expect(await screen.findByText("Keep and notify?")).toBeInTheDocument();
    const confirmSection = screen.getByText("Keep and notify?").closest("section")!;
    expect(within(confirmSection).getByRole("button", { name: "Keep arrangement and notify" })).toBeEnabled();
  });

  it("UI-NOTIF-005: successful resolution shows the saved receipt", async () => {
    const user = userEvent.setup();
    await renderPanel();

    await user.click(await screen.findByText("Keep the current arrangement"));
    await user.click(await screen.findByRole("button", { name: "Keep arrangement and notify" }));

    expect(await screen.findByText("Arrangement updated")).toBeInTheDocument();
  });

  it("UI-NOTIF-006: not-notified resolution does not promise a notification", async () => {
    const user = userEvent.setup();
    const onResolve = vi.fn().mockResolvedValue({ ...queuedResolution, notification_status: "not_configured" });
    await renderPanel({ onResolve });

    await user.click(await screen.findByText("Keep the current arrangement"));
    await user.click(await screen.findByRole("button", { name: "Keep arrangement and notify" }));

    expect(await screen.findByText("Arrangement updated")).toBeInTheDocument();
    expect(screen.queryByText(/notification queued/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/student was notified/i)).not.toBeInTheDocument();
  });

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

    await user.click(await screen.findByText("Keep the current arrangement"));
    const confirmButton = await screen.findByRole("button", { name: "Keep arrangement and notify" });
    await user.click(confirmButton);
    await user.click(confirmButton);

    expect(onResolve).toHaveBeenCalledTimes(1);
    resolveRequest(queuedResolution);
  });

  it("UI-NOTIF-008: resolution conflict refreshes without a success message", async () => {
    const user = userEvent.setup();
    const onResolve = vi.fn().mockResolvedValue(null);
    await renderPanel({ onResolve });

    await user.click(await screen.findByText("Keep the current arrangement"));
    await user.click(await screen.findByRole("button", { name: "Keep arrangement and notify" }));

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/this issue changed while you were reviewing it/i);
    expect(screen.queryByText("Arrangement updated")).not.toBeInTheDocument();
    await waitFor(() => {
      const candidateCalls = mockApiJson.mock.calls.filter(([url]) => String(url).includes("/candidates"));
      expect(candidateCalls.length).toBeGreaterThanOrEqual(2);
    });
  });

  it("UI-NOTIF-010: mark for review requires a reason and promises no notification", async () => {
    const user = userEvent.setup();
    const onResolve = vi.fn().mockResolvedValue({ ...queuedResolution, notification_status: "not_required" });
    await renderPanel({ onResolve });

    await user.click(await screen.findByText("Ask another administrator to review"));

    expect(await screen.findByText("Mark for review?")).toBeInTheDocument();
    const confirmSection = screen.getByText("Mark for review?").closest("section")!;
    expect(within(confirmSection).queryByText(/notif/i)).not.toBeInTheDocument();

    expect(within(confirmSection).getByRole("button", { name: "Send for manual review" })).toBeDisabled();
    await user.selectOptions(within(confirmSection).getByLabelText("Reason"), "Needs owner review");
    expect(within(confirmSection).getByRole("button", { name: "Send for manual review" })).toBeEnabled();

    await user.click(within(confirmSection).getByRole("button", { name: "Send for manual review" }));
    await waitFor(() => expect(onResolve).toHaveBeenCalledWith(expect.anything(), "mark_for_review", undefined, "Needs owner review"));
  });

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

  it("UI-NOTIF-011: switching away from a reason-requiring action clears the reason", async () => {
    const user = userEvent.setup();
    const onResolve = vi.fn().mockResolvedValue({ ...queuedResolution, action: "keep" });
    await renderPanel({ onResolve });

    await user.click(await screen.findByText("Ask another administrator to review"));
    const reviewSection = (await screen.findByText("Mark for review?")).closest("section")!;
    await user.selectOptions(within(reviewSection).getByLabelText("Reason"), "Needs owner review");

    await user.click(screen.getByText("Keep the current arrangement"));
    const keepSection = (await screen.findByText("Keep and notify?")).closest("section")!;
    expect(within(keepSection).queryByLabelText("Reason")).not.toBeInTheDocument();

    await user.click(within(keepSection).getByRole("button", { name: "Keep arrangement and notify" }));
    await waitFor(() => expect(onResolve).toHaveBeenCalledWith(expect.anything(), "keep", undefined, ""));
  });

  it("cancel shows a dedicated confirmation without a notification promise", async () => {
    const user = userEvent.setup();
    const onResolve = vi.fn().mockResolvedValue({ ...queuedResolution, action: "cancel" });
    await renderPanel({ onResolve });

    await user.click(await screen.findByText("Cancel the sit-in"));

    expect(await screen.findByText("Cancel this sit-in?")).toBeInTheDocument();
    const confirmSection = screen.getByText("Cancel this sit-in?").closest("section")!;
    expect(within(confirmSection).queryByText(/will receive|will be notified/i)).not.toBeInTheDocument();
    await user.click(within(confirmSection).getByRole("button", { name: "Cancel arrangement" }));

    await waitFor(() => expect(onResolve).toHaveBeenCalledWith(expect.anything(), "cancel", undefined, ""));
  });
});

function serveCandidates(items: ImpactCandidate[]) {
  mockApiJson.mockImplementation((url: string) => {
    if (url.includes("/candidates")) return Promise.resolve({ items });
    if (url.includes("/activity")) return Promise.resolve({ items: [] });
    return Promise.reject(new Error(`unexpected apiJson call: ${url}`));
  });
}

function waitForCandidateRefresh() {
  return waitFor(() => {
    const candidateCalls = mockApiJson.mock.calls.filter(([url]) => String(url).includes("/candidates"));
    expect(candidateCalls.length).toBeGreaterThanOrEqual(2);
  });
}

describe("IssueResolutionPanel candidate selection", () => {
  it("UI-REASSIGN-001: an ineligible candidate cannot be selected and shows the badge", async () => {
    const user = userEvent.setup();
    const ineligible: ImpactCandidate = { ...candidate, session_id: "cand-2", room_name: "Room 10", eligible: false };
    serveCandidates([candidate, ineligible]);
    await renderPanel();
    await user.click(await screen.findByText("Move to another session"));

    expect(await screen.findByRole("radio", { name: /Room 9/ })).toBeEnabled();
    const unsafeRadio = screen.getByRole("radio", { name: /Room 10/ });
    expect(unsafeRadio).toBeDisabled();
    const unsafeLabel = unsafeRadio.closest("label")!;
    expect(within(unsafeLabel).getByText("Cannot be selected")).toBeInTheDocument();
  });

  it("UI-REASSIGN-002: a full candidate (capacity 0) cannot be selected and shows Full", async () => {
    const user = userEvent.setup();
    const full: ImpactCandidate = { ...candidate, session_id: "cand-2", room_name: "Room 10", available_capacity: 0 };
    serveCandidates([candidate, full]);
    await renderPanel();
    await user.click(await screen.findByText("Move to another session"));

    const fullRadio = await screen.findByRole("radio", { name: /Room 10/ });
    expect(fullRadio).toBeDisabled();
    const fullLabel = fullRadio.closest("label")!;
    expect(within(fullLabel).getByText("Full")).toBeInTheDocument();
  });

  it("UI-REASSIGN-003: a candidate with blocking reasons cannot be selected", async () => {
    const user = userEvent.setup();
    const blocked: ImpactCandidate = { ...candidate, session_id: "cand-2", room_name: "Room 10", blocking_reasons: [{ code: "full", message: "Session is full" }] };
    serveCandidates([candidate, blocked]);
    await renderPanel();
    await user.click(await screen.findByText("Move to another session"));

    expect(await screen.findByRole("radio", { name: /Room 10/ })).toBeDisabled();
    expect(screen.getByRole("radio", { name: /Room 9/ })).toBeEnabled();
  });

  it("UI-REASSIGN-004: negative capacity shows Capacity not limited without green emphasis", async () => {
    const user = userEvent.setup();
    const unlimited: ImpactCandidate = { ...candidate, session_id: "cand-2", room_name: "Room 10", available_capacity: -1 };
    serveCandidates([candidate, unlimited]);
    await renderPanel();
    await user.click(await screen.findByText("Move to another session"));

    const capacitySpan = await screen.findByText("Capacity not limited");
    expect(capacitySpan).not.toHaveClass("text-emerald-700");
  });

  it("UI-REASSIGN-005: refresh keeps the selection while the session still exists", async () => {
    const user = userEvent.setup();
    await renderPanel();
    await user.click(await screen.findByText("Move to another session"));

    const radio = await screen.findByRole("radio", { name: /Room 9/ });
    await user.click(within(screen.getByRole("radiogroup", { name: "Replacement session options" })).getByText(/Thu 14 Aug/));
    expect(radio).toBeChecked();

    await user.click(screen.getByRole("button", { name: /Refresh/i }));
    await waitForCandidateRefresh();
    await waitFor(() => expect(screen.getByRole("radio", { name: /Room 9/ })).toBeChecked());
  });

  it("UI-REASSIGN-006: refresh clears the selection when the session disappears", async () => {
    const user = userEvent.setup();
    await renderPanel();
    await user.click(await screen.findByText("Move to another session"));

    const radio = await screen.findByRole("radio", { name: /Room 9/ });
    await user.click(within(screen.getByRole("radiogroup", { name: "Replacement session options" })).getByText(/Thu 14 Aug/));
    expect(radio).toBeChecked();

    // Swap the mock BEFORE clicking Refresh so the second /candidates call resolves to the new list.
    serveCandidates([{ ...candidate, session_id: "cand-2", room_name: "Room 11" }]);
    await user.click(screen.getByRole("button", { name: /Refresh/i }));
    await waitForCandidateRefresh();
    await waitFor(() => {
      expect(screen.queryByRole("radio", { name: /Room 9/ })).not.toBeInTheDocument();
      const radios = within(screen.getByRole("radiogroup", { name: "Replacement session options" })).getAllByRole("radio");
      expect(radios).toHaveLength(1);
      expect(radios[0]).not.toBeChecked();
    });
  });

  it("UI-REASSIGN-007: refresh picks up a version bump for a kept selection", async () => {
    const user = userEvent.setup();
    const onResolve = vi.fn().mockResolvedValue(queuedResolution);
    await renderPanel({ onResolve });
    await user.click(await screen.findByText("Move to another session"));

    const radio = await screen.findByRole("radio", { name: /Room 9/ });
    await user.click(within(screen.getByRole("radiogroup", { name: "Replacement session options" })).getByText(/Thu 14 Aug/));
    expect(radio).toBeChecked();

    // Swap the mock BEFORE clicking Refresh so the second /candidates call resolves to the bumped version.
    serveCandidates([{ ...candidate, session_version: 4 }]);
    await user.click(screen.getByRole("button", { name: /Refresh/i }));
    await waitForCandidateRefresh();
    await waitFor(() => expect(screen.getByRole("radio", { name: /Room 9/ })).toBeChecked());

    await user.click(screen.getByRole("button", { name: "Confirm reassignment" }));
    await waitFor(() => expect(onResolve).toHaveBeenCalledTimes(1));
    expect(onResolve).toHaveBeenCalledWith(expect.anything(), "reassign", expect.objectContaining({ session_id: "cand-1", session_version: 4 }), expect.anything());
  });

  it("UI-REASSIGN-008: an empty candidate list shows the no-replacement copy", async () => {
    const user = userEvent.setup();
    serveCandidates([]);
    await renderPanel();
    await user.click(await screen.findByText("Move to another session"));

    expect(await screen.findByText("No safe replacement is currently available. Cancel the sit-in or mark this issue for review.")).toBeInTheDocument();
  });

  it("UI-REASSIGN-009: a failed refresh surfaces the retry alert", async () => {
    const user = userEvent.setup();
    await renderPanel();
    await user.click(await screen.findByText("Move to another session"));

    mockApiJson.mockImplementation((url: string) => {
      if (url.includes("/candidates")) return Promise.reject(new Error("network down"));
      if (url.includes("/activity")) return Promise.resolve({ items: [] });
      return Promise.reject(new Error(`unexpected apiJson call: ${url}`));
    });

    await user.click(screen.getByRole("button", { name: /Refresh/i }));
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/Replacement options are still unavailable/);
  });
});

describe("IssueResolutionPanel technical details", () => {
  it("UI-TECH-001: technical details start collapsed and toggle on click", async () => {
    const user = userEvent.setup();
    await renderPanel();

    const summary = screen.getByText("Technical details");
    const details = summary.closest("details")!;
    expect(details).not.toHaveAttribute("open");

    await user.click(summary);
    expect(details).toHaveAttribute("open");
    expect(screen.getByText(/Issue ID:/)).toBeInTheDocument();
    expect(screen.getByText(/Issue version:/)).toBeInTheDocument();
    expect(screen.getByText("Asia/Bangkok")).toBeInTheDocument();

    await user.click(summary);
    expect(details).not.toHaveAttribute("open");
  });
});

describe("IssueResolutionPanel activity trail", () => {
  it("UI-ACT-001: activity items render action, reason, and timestamp", async () => {
    mockApiJson.mockImplementation((url: string) => {
      if (url.includes("/candidates")) return Promise.resolve({ items: [candidate] });
      if (url.includes("/activity")) return Promise.resolve({ items: [{ action: "mark_for_review", reason: "Needs owner review", created_at: "2025-07-21T02:00:00.000Z" }] });
      return Promise.reject(new Error(`unexpected apiJson call: ${url}`));
    });
    await renderPanel();

    expect(await screen.findByText("mark for review")).toBeInTheDocument();
    expect(screen.getByText(/· Needs owner review/)).toBeInTheDocument();
    expect(screen.getByText(/21 Jul/)).toBeInTheDocument();
  });

  it("UI-ACT-002: an empty activity list shows the empty-state copy", async () => {
    await renderPanel();

    expect(await screen.findByText("No previous activity is recorded for this arrangement.")).toBeInTheDocument();
  });
});

describe("IssueResolutionPanel back navigation", () => {
  it("UI-BACK-001: Back returns to the action selector", async () => {
    const user = userEvent.setup();
    await renderPanel();

    await user.click(await screen.findByText("Keep the current arrangement"));
    expect(await screen.findByText("Keep and notify?")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Back" }));
    expect(screen.queryByText("Keep and notify?")).not.toBeInTheDocument();
    expect(screen.getByRole("radio", { name: "Keep the current arrangement" })).toBeInTheDocument();
  });
});
