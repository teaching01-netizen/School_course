import { beforeEach, describe, expect, it, vi, afterEach } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import { fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import SessionChanges from "../SessionChanges";
import { renderScheduleImpact } from "../../features/scheduleImpact/test/renderWithProviders";
import { buildImpactIssue, buildQueueResponse, buildHistoryItem } from "../../features/scheduleImpact/test/builders";
import type { ImpactCandidate, ResolutionResponse } from "../../features/scheduleImpact/types";
import { ApiRequestError } from "../../api/client";
import { queryClient } from "../../query/cache";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, apiJson: mockApiJson };
});

const alice = buildImpactIssue({
  id: "issue-1",
  student_name: "Alice Johnson",
  wcode: "STU001",
  severity: "critical",
  issue_version: 4,
  assignment_context: {
    original_session: { quality: "exact", source: "assignment_snapshot", snapshot: { start_at: "2026-07-31T03:00:00Z", end_at: "2026-07-31T04:00:00Z", course_code: "MATH101" } },
    current_session: {
      status: "active", session_id: "session-1", version: 7,
      start_at: "2026-07-31T06:00:00Z", end_at: "2026-07-31T07:00:00Z",
      course_code: "MATH101", course_name: "Mathematics", room_name: "Room 5", teacher_name: "Dr Jones",
    },
  },
});

const bob = buildImpactIssue({
  id: "issue-2",
  student_name: "Bob Smith",
  wcode: "STU002",
  severity: "warning",
  issue_version: 2,
  assignment_context: {
    original_session: { quality: "exact", source: "assignment_snapshot", snapshot: { start_at: "2026-07-31T03:00:00Z", end_at: "2026-07-31T04:00:00Z", course_code: "ENG201" } },
    current_session: {
      status: "active", session_id: "session-2", version: 3,
      start_at: "2026-07-31T06:00:00Z", end_at: "2026-07-31T07:00:00Z",
      course_code: "ENG201", course_name: "English", room_name: "Room 2", teacher_name: "Dr Lee",
    },
  },
});

const safeCandidate: ImpactCandidate = {
  session_id: "cand-1",
  session_version: 3,
  start_at: "2026-08-14T03:00:00Z",
  end_at: "2026-08-14T04:30:00Z",
  course_code: "MATH101",
  course_name: "Mathematics",
  room_name: "Room 9",
  teacher: "Dr Jones",
  available_capacity: 5,
  eligible: true,
  student_conflicts: false,
  generated_at: "2026-07-31T00:00:00Z",
};

interface ApiMockOptions {
  queue?: ReturnType<typeof buildQueueResponse>;
  queueError?: boolean;
  processing?: { items: unknown[] };
  history?: { items: ReturnType<typeof buildHistoryItem>[] };
  candidates?: { items: ImpactCandidate[] };
  activity?: { items: unknown[] };
  resolve?: (body: Record<string, unknown>) => ResolutionResponse | Promise<ResolutionResponse>;
}

function setupApiMock(options: ApiMockOptions = {}) {
  const queue = options.queue ?? buildQueueResponse([alice, bob], { summary: { need_attention: 2, critical: 1, warnings: 1, notification_failures: 0, notifications_configured: true }, limit: 25, offset: 0 });
  mockApiJson.mockImplementation((url: string, init?: RequestInit) => {
    if (url.startsWith("/api/v1/operations/schedule-impact?") || url === "/api/v1/operations/schedule-impact") {
      if (options.queueError) return Promise.reject(new ApiRequestError("backend down", { status: 500 }));
      return Promise.resolve(queue);
    }
    if (url === "/api/v1/operations/schedule-impact/processing") {
      return Promise.resolve(options.processing ?? { items: [] });
    }
    if (url.startsWith("/api/v1/operations/session-changes")) {
      return Promise.resolve(options.history ?? { items: [] });
    }
    if (url.includes("/candidates")) return Promise.resolve(options.candidates ?? { items: [safeCandidate] });
    if (url.includes("/activity")) return Promise.resolve(options.activity ?? { items: [] });
    if (url.endsWith("/resolve")) {
      const body = JSON.parse(String(init?.body)) as Record<string, unknown>;
      const result = options.resolve ? options.resolve(body) : { id: "issue-1", status: "resolved", action: String(body.action), notification_status: "queued" };
      return Promise.resolve(result);
    }
    return Promise.reject(new Error(`unexpected apiJson call: ${url}`));
  });
}

beforeEach(() => {
  mockApiJson.mockReset();
  // useApiQuery binds to the module-level client, not the provider's; clear it
  // so query cache state cannot leak between tests.
  queryClient.clear();
});

describe("SessionChanges queue view", () => {
  it("shows a loading skeleton while the queue is fetching", () => {
    mockApiJson.mockImplementation(() => new Promise(() => {}));
    renderScheduleImpact(<SessionChanges />);
    expect(screen.getByRole("status", { name: "Loading" })).toBeInTheDocument();
  });

  it("shows an error banner with the failure message", async () => {
    setupApiMock({ queueError: true });
    renderScheduleImpact(<SessionChanges />);
    // The global query client retries 5xx once with backoff before surfacing the error.
    expect(await screen.findByText(/Could not load Schedule Impact: backend down/i, {}, { timeout: 5000 })).toBeInTheDocument();
  });

  it("renders summary metrics including notification failures", async () => {
    setupApiMock({
      queue: buildQueueResponse([alice, bob], {
        summary: { need_attention: 3, critical: 2, warnings: 1, notification_failures: 2, notifications_configured: true },
        limit: 25, offset: 0,
      }),
    });
    renderScheduleImpact(<SessionChanges />);
    expect(await screen.findByText("2 critical")).toBeInTheDocument();
    expect(screen.getByText("3 total")).toBeInTheDocument();
    expect(screen.getByText("1 warnings")).toBeInTheDocument();
    expect(screen.getByText("2 notification failures")).toBeInTheDocument();
  });

  it("shows the notification settings banner when templates are unconfigured", async () => {
    setupApiMock({
      queue: buildQueueResponse([alice], { summary: { need_attention: 1, critical: 1, warnings: 0, notification_failures: 0, notifications_configured: false }, limit: 25, offset: 0 }),
    });
    renderScheduleImpact(<SessionChanges />);
    expect(await screen.findByText(/SMS and email templates are not configured/i)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open notification settings" })).toHaveAttribute("href", "/admin/absence-settings");
  });

  it("does not show the unconfigured banner when templates are configured", async () => {
    setupApiMock();
    renderScheduleImpact(<SessionChanges />);
    await screen.findByText("Alice Johnson");
    expect(screen.queryByText(/SMS and email templates are not configured/i)).not.toBeInTheDocument();
  });

  it("groups critical, needs_review, and warning issues into sections", async () => {
    const review = buildImpactIssue({ id: "issue-3", student_name: "Cara Liu", severity: "warning", status: "needs_review" });
    setupApiMock({ queue: buildQueueResponse([bob, review, alice], { limit: 25, offset: 0 }) });
    renderScheduleImpact(<SessionChanges />);
    await screen.findByText("Alice Johnson");
    const articles = screen.getAllByRole("article");
    expect(articles[0]).toHaveTextContent("Alice Johnson");
    expect(articles[1]).toHaveTextContent("Cara Liu");
    expect(articles[2]).toHaveTextContent("Bob Smith");
  });

  it("renders queue pagination controls and updates the offset in the URL", async () => {
    const page1 = buildQueueResponse([alice, bob], { limit: 25, offset: 0, pagination: { limit: 25, offset: 0, total: 73, has_more: true, next_offset: 25 } });
    const page2 = buildQueueResponse([bob], { limit: 25, offset: 25, pagination: { limit: 25, offset: 25, total: 73, has_more: true, next_offset: 50 } });
    const user = userEvent.setup();
    setupApiMock({ queue: page1 });
    renderScheduleImpact(<SessionChanges />);

    const nav = await screen.findByRole("navigation", { name: "Queue pagination" });
    expect(within(nav).getByText("Showing 1–25 of 73")).toBeInTheDocument();
    expect(within(nav).getByRole("button", { name: "Previous" })).toBeDisabled();
    expect(within(nav).getByRole("button", { name: "Next" })).toBeEnabled();

    // Second page response after the offset changes in the URL.
    mockApiJson.mockImplementation((url: string) => {
      if (url.startsWith("/api/v1/operations/schedule-impact?") && url.includes("offset=25")) return Promise.resolve(page2);
      if (url.startsWith("/api/v1/operations/schedule-impact?")) return Promise.resolve(page1);
      if (url.includes("/candidates")) return Promise.resolve({ items: [] });
      if (url.includes("/activity")) return Promise.resolve({ items: [] });
      return Promise.reject(new Error(`unexpected: ${url}`));
    });

    await user.click(within(nav).getByRole("button", { name: "Next" }));
    await waitFor(() => expect(screen.getByText("Showing 26–50 of 73")).toBeInTheDocument());
    expect(await screen.findByRole("button", { name: "Previous" })).toBeEnabled();

    await user.click(screen.getByRole("button", { name: "Previous" }));
    await waitFor(() => expect(screen.getByText("Showing 1–25 of 73")).toBeInTheDocument());
  });

  it("keeps previous data visible while a new page is fetched", async () => {
    let resolvePage2: (value: unknown) => void = () => {};
    setupApiMock({
      queue: buildQueueResponse([alice, bob], { limit: 25, offset: 0, pagination: { limit: 25, offset: 0, total: 73, has_more: true, next_offset: 25 } }),
    });
    renderScheduleImpact(<SessionChanges />);
    await screen.findByText("Alice Johnson");

    mockApiJson.mockImplementation((url: string) => {
      if (url.startsWith("/api/v1/operations/schedule-impact?") && url.includes("offset=25")) {
        return new Promise((resolve) => { resolvePage2 = resolve; });
      }
      if (url.startsWith("/api/v1/operations/schedule-impact?")) return Promise.resolve(buildQueueResponse([alice, bob], { limit: 25, offset: 0, pagination: { limit: 25, offset: 0, total: 73, has_more: true, next_offset: 25 } }));
      return Promise.reject(new Error(`unexpected: ${url}`));
    });

    const user = userEvent.setup();
    const nav = await screen.findByRole("navigation", { name: "Queue pagination" });
    await user.click(within(nav).getByRole("button", { name: "Next" }));
    // In-flight request: the previous rows must remain on screen.
    expect(screen.getByText("Alice Johnson")).toBeInTheDocument();
    expect(screen.queryByText(/Could not load Schedule Impact/i)).not.toBeInTheDocument();
    resolvePage2(buildQueueResponse([alice], { limit: 25, offset: 25, pagination: { limit: 25, offset: 25, total: 73, has_more: true, next_offset: 50 } }));
    await waitFor(() => expect(screen.queryByText("Bob Smith")).not.toBeInTheDocument());
  });
});

describe("SessionChanges search and filters", () => {
  afterEach(() => { vi.useRealTimers(); });

  it("debounces the search query by 350ms before fetching", async () => {
    setupApiMock();
    renderScheduleImpact(<SessionChanges />);
    await screen.findByText("Alice Johnson");
    mockApiJson.mockClear();

    vi.useFakeTimers();
    fireEvent.change(screen.getByLabelText("Search student, course, or session"), { target: { value: "Ali" } });
    expect(mockApiJson.mock.calls.some(([url]) => String(url).includes("q=Ali"))).toBe(false);
    await vi.advanceTimersByTimeAsync(400);
    vi.useRealTimers();

    await waitFor(() => expect(mockApiJson).toHaveBeenCalledWith(expect.stringContaining("q=Ali")));
  });

  it("issues a single request for a settled debounce value, not per keystroke", async () => {
    setupApiMock();
    renderScheduleImpact(<SessionChanges />);
    await screen.findByText("Alice Johnson");
    mockApiJson.mockClear();

    vi.useFakeTimers();
    const search = screen.getByLabelText("Search student, course, or session");
    fireEvent.change(search, { target: { value: "A" } });
    fireEvent.change(search, { target: { value: "Al" } });
    fireEvent.change(search, { target: { value: "Ali" } });
    // No request is issued for intermediate keystrokes while typing continues.
    expect(mockApiJson.mock.calls.some(([url]) => String(url).includes("q="))).toBe(false);
    await vi.advanceTimersByTimeAsync(400);
    vi.useRealTimers();

    await waitFor(() => {
      const calls = mockApiJson.mock.calls.filter(([url]) => String(url).includes("q=Ali"));
      expect(calls).toHaveLength(1);
    });
  });

  it("resets the offset to zero when the search query changes", async () => {
    setupApiMock();
    renderScheduleImpact(<SessionChanges />, { routes: ["/operations/schedule-impact?offset=50"] });
    await screen.findByText("Alice Johnson");
    mockApiJson.mockClear();

    vi.useFakeTimers();
    fireEvent.change(screen.getByLabelText("Search student, course, or session"), { target: { value: "Ali" } });
    await vi.advanceTimersByTimeAsync(400);
    vi.useRealTimers();

    await waitFor(() => {
      const searchCalls = mockApiJson.mock.calls.filter(([url]) => String(url).includes("q=Ali"));
      expect(searchCalls.length).toBeGreaterThan(0);
      expect(String(searchCalls[0][0])).toContain("offset=0");
      expect(String(searchCalls[0][0])).not.toContain("offset=50");
    });
  });
});

describe("SessionChanges keyboard shortcuts", () => {
  it("presses / to focus search, j/k to move selection, Enter to open, Escape to close", async () => {
    const user = userEvent.setup();
    setupApiMock();
    renderScheduleImpact(<SessionChanges />);
    await screen.findByText("Alice Johnson");

    await user.keyboard("/");
    expect(screen.getByLabelText("Search student, course, or session")).toHaveFocus();

    // Move focus out of the search input so j/k are not swallowed by the input guard.
    await user.click(screen.getByRole("button", { name: /needs attention/i }));

    await user.keyboard("j");
    const articles = screen.getAllByRole("article");
    expect(articles[0].className).toContain("bg-blue-50/60");

    await user.keyboard("j");
    expect(articles[1].className).toContain("bg-blue-50/60");

    await user.keyboard("{Enter}");
    expect(await screen.findByText("Resolve issue")).toBeInTheDocument();

    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByText("Resolve issue")).not.toBeInTheDocument());
  });

  it("presses r to open the panel with reassign preselected", async () => {
    const user = userEvent.setup();
    setupApiMock();
    renderScheduleImpact(<SessionChanges />);
    await screen.findByText("Alice Johnson");

    await user.keyboard("j");
    await user.keyboard("r");
    expect(await screen.findByText("Choose a replacement")).toBeInTheDocument();
  });

  it("presses n to open the panel with keep preselected", async () => {
    const user = userEvent.setup();
    setupApiMock();
    renderScheduleImpact(<SessionChanges />);
    await screen.findByText("Alice Johnson");

    await user.keyboard("j");
    await user.keyboard("n");
    expect(await screen.findByText("Keep and notify?")).toBeInTheDocument();
  });

  it("does not trigger shortcuts while typing in the search input", async () => {
    const user = userEvent.setup();
    setupApiMock();
    renderScheduleImpact(<SessionChanges />);
    await screen.findByText("Alice Johnson");

    await user.type(screen.getByLabelText("Search student, course, or session"), "j");
    expect(screen.queryByText("Resolve issue")).not.toBeInTheDocument();
    expect(screen.getAllByRole("article")[0].className).not.toContain("bg-blue-50/60");
  });
});

describe("SessionChanges resolution flow", () => {
  it("sends the expected issue version and no candidate fields for a keep action", async () => {
    const user = userEvent.setup();
    setupApiMock();
    renderScheduleImpact(<SessionChanges />);
    await screen.findByText("Alice Johnson");

    await user.click(screen.getAllByRole("button", { name: "Review" })[0]);
    await user.click(await screen.findByText("Keep the current arrangement"));
    await user.click(await screen.findByRole("button", { name: "Keep arrangement and notify" }));

    await waitFor(() => {
      const resolveCall = mockApiJson.mock.calls.find(([url]) => String(url).endsWith("/resolve"));
      expect(resolveCall).toBeDefined();
      const body = JSON.parse(String((resolveCall![1] as RequestInit).body)) as Record<string, unknown>;
      expect(body).toMatchObject({ action: "keep", reason: "", expected_issue_version: 4 });
      expect(body.candidate_session_id).toBeUndefined();
      expect(body.expected_session_version).toBeUndefined();
    });
  });

  it("includes candidate identifiers in a reassign payload", async () => {
    const user = userEvent.setup();
    setupApiMock();
    renderScheduleImpact(<SessionChanges />);
    await screen.findByText("Alice Johnson");

    await user.click(screen.getAllByRole("button", { name: "Review" })[0]);
    await user.click(await screen.findByText("Move to another session"));
    const candidateGroup = await screen.findByRole("radiogroup", { name: "Replacement session options" });
    await user.click(within(candidateGroup).getAllByRole("radio")[0]);
    await user.click(await screen.findByRole("button", { name: "Confirm reassignment" }));

    await waitFor(() => {
      const resolveCall = mockApiJson.mock.calls.find(([url]) => String(url).endsWith("/resolve"));
      const body = JSON.parse(String((resolveCall![1] as RequestInit).body)) as Record<string, unknown>;
      expect(body).toMatchObject({
        action: "reassign",
        candidate_session_id: "cand-1",
        expected_session_version: 3,
        expected_issue_version: 4,
      });
    });
  });

  it("trims the reason before sending it", async () => {
    const user = userEvent.setup();
    setupApiMock();
    renderScheduleImpact(<SessionChanges />);
    await screen.findByText("Alice Johnson");

    await user.click(screen.getAllByRole("button", { name: "Review" })[0]);
    await user.click(await screen.findByText("Ask another administrator to review"));
    const confirmSection = screen.getByText("Mark for review?").closest("section")!;
    await user.selectOptions(within(confirmSection).getByLabelText("Reason"), "Needs owner review");
    await user.click(within(confirmSection).getByRole("button", { name: "Send for manual review" }));

    await waitFor(() => {
      const resolveCall = mockApiJson.mock.calls.find(([url]) => String(url).endsWith("/resolve"));
      const body = JSON.parse(String((resolveCall![1] as RequestInit).body)) as Record<string, unknown>;
      expect(body).toMatchObject({ action: "mark_for_review", reason: "Needs owner review", expected_issue_version: 4 });
    });
  });

  it("keeps the success result visible instead of unmounting the panel (regression)", async () => {
    const user = userEvent.setup();
    setupApiMock();
    renderScheduleImpact(<SessionChanges />);
    await screen.findByText("Alice Johnson");

    await user.click(screen.getAllByRole("button", { name: "Review" })[0]);
    await user.click(await screen.findByText("Keep the current arrangement"));
    await user.click(await screen.findByRole("button", { name: "Keep arrangement and notify" }));

    expect(await screen.findByText("Arrangement updated")).toBeInTheDocument();
    // The panel must not disappear: the parent clears selection only via onClose.
    expect(screen.getByText("Arrangement updated")).toBeVisible();
    expect(screen.getByRole("button", { name: "Close" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Close" }));
    await waitFor(() => expect(screen.queryByText("Arrangement updated")).not.toBeInTheDocument());
  });

  it("refreshes the queue after a successful resolution", async () => {
    const user = userEvent.setup();
    setupApiMock();
    renderScheduleImpact(<SessionChanges />);
    await screen.findByText("Alice Johnson");
    mockApiJson.mockClear();

    await user.click(screen.getAllByRole("button", { name: "Review" })[0]);
    await user.click(await screen.findByText("Keep the current arrangement"));
    await user.click(await screen.findByRole("button", { name: "Keep arrangement and notify" }));
    await screen.findByText("Arrangement updated");

    await waitFor(() => {
      const queueCalls = mockApiJson.mock.calls.filter(([url]) => String(url).startsWith("/api/v1/operations/schedule-impact?"));
      expect(queueCalls.length).toBeGreaterThan(0);
    });
  });

  it("handles a resolution conflict by keeping the panel open and refetching", async () => {
    const user = userEvent.setup();
    setupApiMock({
      resolve: () => { throw new ApiRequestError("This issue changed while you were reviewing it", { code: "resolution_conflict", status: 409 }); },
    });
    renderScheduleImpact(<SessionChanges />);
    await screen.findByText("Alice Johnson");

    await user.click(screen.getAllByRole("button", { name: "Review" })[0]);
    await user.click(await screen.findByText("Keep the current arrangement"));
    await user.click(await screen.findByRole("button", { name: "Keep arrangement and notify" }));

    // Both the toast and the panel banner surface the conflict; the panel stays open.
    await waitFor(() => {
      const alerts = screen.getAllByRole("alert");
      expect(alerts.some((el) => el.textContent?.toLowerCase().includes("this issue changed while you were reviewing it"))).toBe(true);
    });
    expect(screen.queryByText("Arrangement updated")).not.toBeInTheDocument();
  });

  it("shows a generic error toast for unexpected resolve failures", async () => {
    const user = userEvent.setup();
    setupApiMock({
      resolve: () => { throw new Error("network exploded"); },
    });
    renderScheduleImpact(<SessionChanges />);
    await screen.findByText("Alice Johnson");

    await user.click(screen.getAllByRole("button", { name: "Review" })[0]);
    await user.click(await screen.findByText("Keep the current arrangement"));
    await user.click(await screen.findByRole("button", { name: "Keep arrangement and notify" }));

    expect(await screen.findByText("network exploded")).toBeInTheDocument();
  });
});

describe("SessionChanges processing view", () => {
  it("renders status badges and a retry control for failed changes", async () => {
    const user = userEvent.setup();
    setupApiMock({
      processing: {
        items: [
          { id: "change-1", course_code: "MATH101", course_name: "Mathematics", created_at: "2026-07-31T03:00:00Z", status: "failed", last_error: "resolver exploded" },
          { id: "change-2", course_code: "ENG201", course_name: "English", created_at: "2026-07-31T03:00:00Z", status: "processing", last_error: null },
          { id: "change-3", course_code: "SCI301", course_name: "Science", created_at: "2026-07-31T03:00:00Z", status: "delayed_by_batch", last_error: null },
        ],
      },
    });
    renderScheduleImpact(<SessionChanges />, { routes: ["/operations/schedule-impact?view=processing"] });

    expect(await screen.findByText("failed")).toBeInTheDocument();
    expect(screen.getByText("processing")).toBeInTheDocument();
    expect(screen.getByText("Waiting for batch")).toBeInTheDocument();
    expect(screen.getByText("resolver exploded")).toBeInTheDocument();

    // Only the failed row offers a retry.
    expect(screen.getAllByRole("button", { name: "Retry analysis" })).toHaveLength(1);
    await user.click(screen.getByRole("button", { name: "Retry analysis" }));
    await waitFor(() => expect(mockApiJson).toHaveBeenCalledWith("/api/v1/operations/session-changes/change-1/reprocess", expect.objectContaining({ method: "POST" })));
  });

  it("sends a single reprocess request when retry is clicked twice", async () => {
    const user = userEvent.setup();
    let finishRetry: (value: unknown) => void = () => {};
    setupApiMock({
      processing: { items: [{ id: "change-1", course_code: "MATH101", course_name: "", created_at: "2026-07-31T03:00:00Z", status: "failed", last_error: "boom" }] },
    });
    mockApiJson.mockImplementation((url: string) => {
      if (url.endsWith("/reprocess")) return new Promise((resolve) => { finishRetry = resolve; });
      if (url === "/api/v1/operations/schedule-impact/processing") return Promise.resolve({ items: [{ id: "change-1", course_code: "MATH101", course_name: "", created_at: "2026-07-31T03:00:00Z", status: "failed", last_error: "boom" }] });
      return Promise.reject(new Error(`unexpected: ${url}`));
    });
    renderScheduleImpact(<SessionChanges />, { routes: ["/operations/schedule-impact?view=processing"] });
    await screen.findByText("failed");

    const retry = screen.getByRole("button", { name: "Retry analysis" });
    await user.click(retry);
    await user.click(retry);
    const reprocessCalls = mockApiJson.mock.calls.filter(([url]) => String(url).endsWith("/reprocess"));
    expect(reprocessCalls).toHaveLength(1);
    finishRetry({ id: "change-1", analysis_status: "completed" });
  });

  it("shows the empty state when nothing is processing", async () => {
    setupApiMock({ processing: { items: [] } });
    renderScheduleImpact(<SessionChanges />, { routes: ["/operations/schedule-impact?view=processing"] });
    expect(await screen.findByText("No impact analyses are processing")).toBeInTheDocument();
  });
});

describe("SessionChanges history view", () => {
  it("fetches history with the fixed limit of 100", async () => {
    setupApiMock({ history: { items: [] } });
    renderScheduleImpact(<SessionChanges />, { routes: ["/operations/schedule-impact?view=history"] });
    await screen.findByText("No completed schedule changes have been recorded.");
    expect(mockApiJson).toHaveBeenCalledWith("/api/v1/operations/session-changes?limit=100");
  });

  it("renders history rows with unresolved counts and detail links", async () => {
    setupApiMock({
      history: {
        items: [
          buildHistoryItem({ id: "change-1", new_course_code: "MATH101", open_issue_count: 2, critical_issue_count: 1 }),
          buildHistoryItem({ id: "change-2", new_course_code: "ENG201", open_issue_count: 0, critical_issue_count: 0 }),
        ],
      },
    });
    renderScheduleImpact(<SessionChanges />, { routes: ["/operations/schedule-impact?view=history"] });

    expect(await screen.findByText("3 unresolved")).toBeInTheDocument();
    expect(screen.getByText("Completed")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /MATH101/ })).toHaveAttribute("href", "/operations/session-changes/change-1");
  });

  it("shows an em dash for rows with no affected arrangements", async () => {
    setupApiMock({ history: { items: [buildHistoryItem({ id: "change-1", open_issue_count: 0, critical_issue_count: 0 })] } });
    renderScheduleImpact(<SessionChanges />, { routes: ["/operations/schedule-impact?view=history"] });
    expect(await screen.findByText("—")).toBeInTheDocument();
  });
});
