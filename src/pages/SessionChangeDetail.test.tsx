import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import SessionChangeDetail from "./SessionChangeDetail";

const mockApiJson = vi.hoisted(() => vi.fn());
const mockUseApiQuery = vi.hoisted(() => vi.fn());
const mockAddToast = vi.hoisted(() => vi.fn());

vi.mock("../api/client", async () => {
  const actual = await vi.importActual<typeof import("../api/client")>("../api/client");
  return { ...actual, apiJson: mockApiJson };
});
vi.mock("../hooks/useApiQuery", () => ({ useApiQuery: mockUseApiQuery }));
vi.mock("../hooks/useToast", () => ({ useToast: () => ({ addToast: mockAddToast }) }));

type NotificationStatus = {
  id: string;
  absence_id: string;
  message_type: string;
  channel: "sms" | "email";
  status: string;
  attempt_count: number;
  failure_reason: string | null;
  provider_message_id: string | null;
  created_at: string;
  sent_at: string | null;
};

function notification(overrides: Partial<NotificationStatus> = {}): NotificationStatus {
  return {
    id: "notif-1",
    absence_id: "abs-1",
    message_type: "sit_in_session_moved",
    channel: "sms",
    status: "queued",
    attempt_count: 1,
    failure_reason: null,
    provider_message_id: null,
    created_at: "2025-07-24T07:00:00.000Z",
    sent_at: null,
    ...overrides,
  };
}

function detailResponse(notifications: NotificationStatus[]) {
  return {
    change: {
      id: "change-1",
      session_id: "sess-1",
      session_version: 2,
      change_source: "session_update",
      changed_fields: { start_at: true },
      before_snapshot: { start_at: "2025-07-24T07:00:00.000Z" },
      after_snapshot: { start_at: "2025-07-24T08:00:00.000Z" },
      old_start_at: "2025-07-24T07:00:00.000Z",
      old_end_at: "2025-07-24T08:00:00.000Z",
      new_start_at: "2025-07-24T08:00:00.000Z",
      new_end_at: "2025-07-24T09:00:00.000Z",
      old_course: { code: "MATH101", name: "Mathematics" },
      new_course: { code: "MATH101", name: "Mathematics" },
      open_issue_count: 1,
      critical_issue_count: 1,
      created_at: "2025-07-20T00:00:00.000Z",
    },
    issues: [],
    notifications,
  };
}

function renderDetail(notifications: NotificationStatus[]) {
  const refetch = vi.fn().mockResolvedValue(undefined);
  mockUseApiQuery.mockReturnValue({
    data: detailResponse(notifications),
    loading: false,
    refreshing: false,
    error: null,
    refetch,
  });
  render(
    <MemoryRouter initialEntries={["/operations/session-changes/change-1"]}>
      <Routes>
        <Route path="/operations/session-changes/:id" element={<SessionChangeDetail />} />
      </Routes>
    </MemoryRouter>,
  );
  return { refetch };
}

beforeEach(() => {
  mockApiJson.mockReset();
  mockUseApiQuery.mockReset();
  mockAddToast.mockReset();
  mockApiJson.mockResolvedValue({ ok: true });
});

describe("SessionChangeDetail notification delivery", () => {
  // DELIVERY-004: a dead-letter notification is visible on the change detail.
  it("DELIVERY-004: dead-letter notification is visible with retry and cancel", () => {
    renderDetail([
      notification({ status: "dead_letter", attempt_count: 3, failure_reason: "recipient unreachable" }),
    ]);

    expect(screen.getByText("Notification delivery")).toBeInTheDocument();
    expect(screen.getByText("Dead Letter")).toBeInTheDocument();
    expect(screen.getByText(/recipient unreachable/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeInTheDocument();
  });

  // DELIVERY-005: retrying schedules a new attempt and confirms to staff.
  it("DELIVERY-005: retrying a dead-letter notification queues a retry", async () => {
    const user = userEvent.setup();
    const { refetch } = renderDetail([
      notification({ id: "notif-dead", status: "dead_letter", attempt_count: 3 }),
    ]);

    await user.click(screen.getByRole("button", { name: "Retry" }));

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/operations/notifications/notif-dead/retry",
        expect.objectContaining({ method: "POST" }),
      );
    });
    expect(mockAddToast).toHaveBeenCalledWith("success", "Notification queued for retry");
    expect(refetch).toHaveBeenCalled();
  });

  // DELIVERY-006: cancelling a queued notification posts the cancellation.
  it("DELIVERY-006: cancelling a queued notification posts the request", async () => {
    const user = userEvent.setup();
    const { refetch } = renderDetail([
      notification({ id: "notif-queued", status: "queued" }),
    ]);

    await user.click(screen.getByRole("button", { name: "Cancel" }));

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/operations/notifications/notif-queued/cancel",
        expect.objectContaining({ method: "POST" }),
      );
    });
    expect(mockAddToast).toHaveBeenCalledWith("success", "Notification cancelled");
    expect(refetch).toHaveBeenCalled();
  });

  // A queued notification offers cancel but no retry.
  it("queued notifications can be cancelled but not retried", () => {
    renderDetail([notification({ status: "queued" })]);

    expect(screen.getByRole("button", { name: "Cancel" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Retry" })).not.toBeInTheDocument();
  });

  // A failed notification shows its failure reason.
  it("failed notifications show the failure reason", () => {
    renderDetail([
      notification({ status: "failed", failure_reason: "provider timeout" }),
    ]);

    expect(screen.getByText("Failed")).toBeInTheDocument();
    expect(screen.getByText(/provider timeout/)).toBeInTheDocument();
  });

  // Delivered notifications are collapsed so only problems need attention.
  it("delivered notifications are collapsed into a summary", () => {
    renderDetail([
      notification({ id: "n1", status: "delivered", sent_at: "2025-07-24T07:01:00.000Z" }),
      notification({ id: "n2", status: "delivered", sent_at: "2025-07-24T07:02:00.000Z" }),
      notification({ id: "n3", status: "failed", failure_reason: "boom" }),
    ]);

    expect(screen.queryByText("2 successful notifications collapsed. No delivery needs attention.")).not.toBeInTheDocument();
    // The failed one stays visible; the delivered ones are not listed.
    expect(screen.getByText("Failed")).toBeInTheDocument();
    expect(screen.queryByText("Delivered")).not.toBeInTheDocument();
  });

  it("shows the collapsed summary when only delivered notifications exist", () => {
    renderDetail([
      notification({ status: "delivered", sent_at: "2025-07-24T07:01:00.000Z" }),
    ]);

    expect(screen.getByText("1 successful notification collapsed. No delivery needs attention.")).toBeInTheDocument();
  });

  it("shows an empty state when nothing was queued", () => {
    renderDetail([]);

    expect(screen.getByText("No notifications have been queued for this change.")).toBeInTheDocument();
  });

  // Failed retry requests surface an error toast, not silent success.
  it("shows an error toast when the notification action fails", async () => {
    mockApiJson.mockRejectedValue(new Error("conflict"));
    const user = userEvent.setup();
    renderDetail([notification({ id: "notif-dead", status: "dead_letter" })]);

    await user.click(screen.getByRole("button", { name: "Retry" }));

    await waitFor(() => {
      expect(mockAddToast).toHaveBeenCalledWith("error", expect.stringContaining("conflict"));
    });
  });
});
