import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import Absences from "../Absences";
import { ToastProvider } from "../../hooks/useToast";
import { queryClient } from "../../query/cache";
import { ApiRequestError } from "@/api/client";

const mockApiJson = vi.hoisted(() => vi.fn());
const mockApiBlobDownload = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson, downloadApiFile: mockApiBlobDownload };
});

const PAGE = {
  items: [
    {
      id: "abs-1",
      wcode: "W250389",
      student_name: "John Smith",
      student_nickname: null,
      course_id: "course-1",
      course_code: "MATH-201",
      course_name: "Algebra II",
      subject_id: "subj-1",
      subject_code: "MATH",
      subject_name: "Mathematics",
      date_from: "2026-06-02",
      date_to: "2026-06-06",
      reason_category: "medical",
      reason: "Appointment",
      sit_in_method: "physical",
      sit_in_subject_name: "SAT Math Scholar C2",
      sit_in_course_code: "000000004",
      sit_in_course_name: "SAT Math Scholar C2",
      status: "pending",
      version: 1,
      created_at: "2026-05-27T09:00:00Z",
      updated_at: "2026-05-27T09:00:00Z",
    },
  ],
  total_count: 1,
  offset: 0,
  limit: 25,
};

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function freshPage() {
  return structuredClone(PAGE);
}

const PAGE_WITH_MISSED_SESSIONS = {
  ...PAGE,
  items: [
    {
      ...PAGE.items[0],
      missed_sessions: [
        {
          id: "miss-1",
          session_id: "sess-1",
          course_id: "course-1",
          course_code: "MATH-201",
          course_name: "Algebra II",
          start_at: "2026-06-01T09:00:00+07:00",
          end_at: "2026-06-01T12:00:00+07:00",
        },
        {
          id: "miss-2",
          session_id: "sess-2",
          course_id: "course-1",
          course_code: "MATH-201",
          course_name: "Algebra II",
          start_at: "2026-06-08T09:00:00+07:00",
          end_at: "2026-06-08T12:00:00+07:00",
        },
      ],
      sit_ins: [
        {
          id: "sit-1",
          session_id: "sit-session-1",
          course_id: "sit-course-1",
          course_code: "MATH-301",
          course_name: "SAT Math Scholar C2",
          subject_name: "Mathematics",
          start_at: "2026-06-03T10:00:00+07:00",
          end_at: "2026-06-03T11:30:00+07:00",
        },
      ],
    },
  ],
};

const PAGE_WITH_CROSS_SUBJECT_SIT_IN = {
  ...PAGE,
  items: [
    {
      ...PAGE.items[0],
      sit_in_method: "physical",
      sit_in_subject_name: "Natural Sciences",
      sit_ins: [
        {
          id: "sit-1",
          session_id: "sit-session-1",
          course_id: "sit-course-1",
          course_code: "PHYS-101",
          course_name: "Physics I",
          subject_name: "Natural Sciences",
          start_at: "2026-06-03T10:00:00+07:00",
          end_at: "2026-06-03T11:30:00+07:00",
        },
      ],
    },
  ],
};

const PAGE_WITH_SAME_SUBJECT_SIT_IN = {
  ...PAGE,
  items: [
    {
      ...PAGE.items[0],
      sit_ins: [
        {
          id: "sit-1",
          session_id: "sit-session-1",
          course_id: "sit-course-1",
          course_code: "MATH-301",
          course_name: "SAT Math Scholar C2",
          subject_name: "Mathematics",
          start_at: "2026-06-03T10:00:00+07:00",
          end_at: "2026-06-03T11:30:00+07:00",
        },
      ],
    },
  ],
};

function renderPage(path = "/absences?status=pending") {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <ToastProvider>
        <Absences />
      </ToastProvider>
    </MemoryRouter>,
  );
}

describe("Absence inbox", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    queryClient.clear();
  });

  it("loads shareable status filters and renders a triage row", async () => {
    mockApiJson.mockResolvedValueOnce(PAGE);
    renderPage();

    const student = await screen.findByText("John Smith");
    const row = student.closest("tr");
    if (!row) {
      throw new Error("Expected absence table row");
    }
    expect(within(row).getByText("Pending")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /view john smith absence/i })).toHaveAttribute("href", "/absences/abs-1");
    expect(mockApiJson).toHaveBeenCalledWith(
      expect.stringContaining("status=pending"),
      expect.objectContaining({ method: "GET" }),
    );
    expect(mockApiJson).toHaveBeenCalledWith(
      expect.stringContaining("bucket=active"),
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("renders leave session times under subject and picked sit-in times under sit-in", async () => {
    mockApiJson.mockResolvedValueOnce(PAGE_WITH_MISSED_SESSIONS);
    renderPage();

    const absenceLink = await screen.findByRole("link", { name: /view john smith absence/i });
    const row = absenceLink.closest("tr");
    if (!row) {
      throw new Error("Expected absence table row");
    }
    expect(row).toHaveTextContent("Mathematics");
    expect(row).toHaveTextContent("1 Jun");
    expect(row).toHaveTextContent("8 Jun");
    expect(row).toHaveTextContent("09:00");
    expect(row).toHaveTextContent("12:00");
    expect(row).toHaveTextContent("Mathematics");
    expect(row).not.toHaveTextContent("SAT Math Scholar C2");
    expect(row).toHaveTextContent("3 Jun");
    expect(row).toHaveTextContent("10:00");
    expect(row).toHaveTextContent("11:30");
    expect(row).not.toHaveTextContent("Requested 27 May");
    expect(row).not.toHaveTextContent("000000004");
    expect(row).not.toHaveTextContent("31 May - 30 Jun");
  });

  it("shows sit-in subject when no physical session is selected yet", async () => {
    mockApiJson.mockResolvedValueOnce(PAGE);
    renderPage();

    const absenceLink = await screen.findByRole("link", { name: /view john smith absence/i });
    const row = absenceLink.closest("tr");
    if (!row) {
      throw new Error("Expected absence table row");
    }
    expect(row).toHaveTextContent("SAT Math Scholar C2");
    expect(row).toHaveTextContent("No session selected");
    expect(row).not.toHaveTextContent("Not assigned");
  });

  it("marks an absence reviewed using its current version and reloads results", async () => {
    const initialPage = freshPage();
    const updatedPage = freshPage();
    updatedPage.items[0].status = "reviewed";
    updatedPage.items[0].version = 2;
    mockApiJson
      .mockResolvedValueOnce(initialPage)
      .mockResolvedValueOnce({ status: "reviewed", version: 2 })
      .mockResolvedValueOnce(updatedPage);
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /mark reviewed/i }));

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/absences/abs-1/status",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ status: "reviewed", expected_version: 1 }),
        }),
      );
    });
  });

  it("keeps a row pending until the review request resolves", async () => {
    const review = deferred<{ status: string; version: number }>();
    const initialPage = freshPage();
    const updatedPage = freshPage();
    updatedPage.items[0].status = "reviewed";
    updatedPage.items[0].version = 2;
    mockApiJson
      .mockResolvedValueOnce(initialPage)
      .mockImplementationOnce(() => review.promise)
      .mockResolvedValueOnce(updatedPage);
    renderPage();
    const user = userEvent.setup();

    const row = (await screen.findByText("John Smith")).closest("tr");
    if (!row) {
      throw new Error("Expected absence table row");
    }
    const statusCell = row.querySelector('[data-label="Status"]');
    if (!statusCell) {
      throw new Error("Expected status cell");
    }

    await user.click(screen.getByRole("button", { name: /mark reviewed/i }));

    expect(statusCell).toHaveTextContent("Pending");
    expect(statusCell).not.toHaveTextContent("Reviewed");

    await act(async () => {
      review.resolve({ status: "reviewed", version: 2 });
    });

    await waitFor(() => {
      expect(statusCell).toHaveTextContent("Reviewed");
    });
  });

  it("keeps selected rows pending until the bulk review request resolves", async () => {
    const review = deferred<{ succeeded: string[]; failed: Array<{ id: string; error: string }>; total_processed: number }>();
    const initialPage = freshPage();
    const updatedPage = freshPage();
    updatedPage.items[0].status = "reviewed";
    updatedPage.items[0].version = 2;
    mockApiJson
      .mockResolvedValueOnce(initialPage)
      .mockImplementationOnce(() => review.promise)
      .mockResolvedValueOnce(updatedPage);
    renderPage();
    const user = userEvent.setup();

    const row = (await screen.findByText("John Smith")).closest("tr");
    if (!row) {
      throw new Error("Expected absence table row");
    }
    const statusCell = row.querySelector('[data-label="Status"]');
    if (!statusCell) {
      throw new Error("Expected status cell");
    }

    await user.click(await screen.findByLabelText("Select W250389"));
    const batchBar = screen.getByText("1 selected").parentElement!;
    await user.click(within(batchBar).getByRole("button", { name: /mark reviewed/i }));

    expect(statusCell).toHaveTextContent("Pending");
    expect(statusCell).not.toHaveTextContent("Reviewed");

    await act(async () => {
      review.resolve({ succeeded: ["abs-1"], failed: [], total_processed: 1 });
    });

    await waitFor(() => {
      expect(statusCell).toHaveTextContent("Reviewed");
    });
  });

  it("exports the active filtered report", async () => {
    mockApiJson.mockResolvedValueOnce(PAGE);
    mockApiBlobDownload.mockResolvedValueOnce(undefined);
    renderPage("/absences?status=reviewed&query=W25");
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /export csv/i }));

    expect(mockApiBlobDownload).toHaveBeenCalledWith(expect.stringMatching(/status=reviewed.*query=W25|query=W25.*status=reviewed/));
  });

  it("exports only selected absence records from the bulk bar", async () => {
    mockApiJson.mockResolvedValueOnce(PAGE);
    mockApiBlobDownload.mockResolvedValueOnce(undefined);
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByLabelText("Select W250389"));
    await user.click(screen.getByRole("button", { name: /export selected/i }));

    expect(mockApiBlobDownload).toHaveBeenCalledWith(expect.stringContaining("ids=abs-1"));
  });

  it("shows actionable empty state with CTA links when no records match filters", async () => {
    mockApiJson.mockResolvedValueOnce({ items: [], total_count: 0, offset: 0, limit: 25 });
    renderPage("/absences?status=cancelled");

    expect(await screen.findByText("No archived absences match these filters.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /view active table/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /view dashboard/i })).toHaveAttribute("href", "/absences/dashboard");
  });

  it("switches from active table to archived table", async () => {
    mockApiJson.mockResolvedValue({ ...PAGE, items: [], total_count: 0 });
    renderPage("/absences");
    const user = userEvent.setup();

    await screen.findByText("All caught up! No active absences match these filters.");
    await user.click(screen.getByRole("button", { name: /archived table/i }));

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        expect.stringContaining("bucket=archived"),
        expect.objectContaining({ method: "GET" }),
      );
    });
    expect(screen.getByRole("option", { name: "Actioned" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Cancelled" })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "Pending" })).not.toBeInTheDocument();
  });

  it("renders missed session dates in the board view", async () => {
    mockApiJson.mockImplementation(async (url: string) => {
      if (url.includes("status=pending")) return PAGE_WITH_MISSED_SESSIONS;
      return { items: [], total_count: 0, offset: 0, limit: 25 };
    });

    renderPage("/absences?view=board");

    expect(await screen.findByText("Absence Board")).toBeInTheDocument();
    const name = await screen.findByText("John Smith");
    const card = name.closest('[tabindex="0"]');
    if (!card) {
      throw new Error("Expected board card");
    }
    expect(card).toHaveTextContent("1 Jun");
    expect(card).toHaveTextContent("8 Jun");
    expect(card).toHaveTextContent("SAT Math Scholar C2");
    expect(card).not.toHaveTextContent("000000004");
    expect(card).not.toHaveTextContent("31 May - 30 Jun");
  });

  it("shows Actioned button for reviewed absences but not for pending", async () => {
    const reviewedPage = {
      ...PAGE,
      items: [{ ...PAGE.items[0], status: "reviewed" }],
      total_count: 1,
    };
    const pendingPage = {
      ...PAGE,
      items: [{ ...PAGE.items[0], id: "abs-2", wcode: "W999999", status: "pending" }],
      total_count: 1,
    };

    // First render: reviewed item — Actioned button should appear
    mockApiJson.mockResolvedValueOnce(reviewedPage);
    renderPage("/absences?status=reviewed");
    expect(await screen.findByText("John Smith")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /actioned/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /mark reviewed/i })).not.toBeInTheDocument();

    // Unmount and re-render: pending item — Actioned button should NOT appear
    cleanup();
    mockApiJson.mockResolvedValueOnce(pendingPage);
    renderPage("/absences?status=pending");
    expect(await screen.findByText("John Smith")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /actioned/i })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /mark reviewed/i })).toBeInTheDocument();
  });

  it("bulk cancels selected absences with a recorded reason", async () => {
    const initialPage = freshPage();
    const cancelledPage = freshPage();
    cancelledPage.items = [];
    mockApiJson
      .mockResolvedValueOnce(initialPage)
      .mockResolvedValueOnce({ status: "cancelled", version: 2 })
      .mockResolvedValueOnce(cancelledPage);
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByLabelText("Select W250389"));
    await user.click(screen.getByRole("button", { name: /cancel selected/i }));
    await user.selectOptions(screen.getByLabelText(/cancellation reason/i), "other");
    await user.type(screen.getByLabelText(/additional details/i), "Reported in error");
    const confirm = screen.getByRole("button", { name: /^cancel absence/i });
    expect(confirm).toBeEnabled();
    await user.click(confirm);

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/absences/batch-status",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({
            ids: ["abs-1"],
            status: "cancelled",
            reason: JSON.stringify({ category: "other", detail: "Reported in error" }),
            expected_versions: { "abs-1": 1 },
          }),
        }),
      );
    });
  });

  it("shows retry failed button after partial batch failure and retries only failed items", async () => {
    vi.resetAllMocks();
    const twoItemPage = {
      items: [
        { ...PAGE.items[0], id: "abs-1", wcode: "W250389", status: "pending" },
        { ...PAGE.items[0], id: "abs-2", wcode: "W999999", student_name: "Jane Doe", status: "pending", version: 1 },
      ],
      total_count: 2,
      offset: 0,
      limit: 25,
    };
    mockApiJson.mockImplementation(async (url: string) => {
      if (url.includes("/batch-status")) {
        return {
          succeeded: ["abs-1"],
          failed: [{ id: "abs-2", error: "version mismatch" }],
          total_processed: 2,
        };
      }
      return structuredClone(twoItemPage);
    });

    renderPage("/absences");
    const user = userEvent.setup();

    await waitFor(() => {
      expect(screen.getByText("Jane Doe")).toBeInTheDocument();
    });

    await user.click(screen.getByLabelText("Select W250389"));
    await user.click(screen.getByLabelText("Select W999999"));

    const batchBar = screen.getByText("2 selected").parentElement!;
    await user.click(within(batchBar).getByRole("button", { name: /mark reviewed/i }));

    await waitFor(() => {
      expect(screen.getByText("1 failed")).toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: /retry failed/i })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /retry failed/i }));

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        expect.stringContaining("/api/v1/absences/batch-status"),
        expect.objectContaining({
          method: "POST",
          body: expect.stringContaining("abs-2"),
        }),
      );
    });
  });

  it("shows delete button for non-cancelled absences", async () => {
    mockApiJson.mockResolvedValueOnce(PAGE);
    renderPage();

    await screen.findByText("John Smith");
    const deleteBtn = screen.getByRole("button", { name: /delete/i });
    expect(deleteBtn).toBeInTheDocument();
  });

  it("shows delete button for cancelled absences", async () => {
    const cancelledPage = {
      ...PAGE,
      items: [{ ...PAGE.items[0], status: "cancelled" }],
    };
    mockApiJson.mockResolvedValueOnce(cancelledPage);
    renderPage("/absences?status=cancelled");

    await screen.findByText("John Smith");
    expect(screen.getByRole("button", { name: /delete/i })).toBeInTheDocument();
  });

  it("opens confirmation modal when delete button is clicked", async () => {
    mockApiJson.mockResolvedValueOnce(PAGE);
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /delete/i }));

    const modal = screen.getByRole("dialog");
    expect(within(modal).getByText("Permanently delete absence")).toBeInTheDocument();
    expect(within(modal).getByText(/permanently remove the absence record/i)).toBeInTheDocument();
    expect(within(modal).getByText("John Smith")).toBeInTheDocument();
    expect(within(modal).getByText(/this action cannot be undone/i)).toBeInTheDocument();
  });

  it("calls DELETE API and reloads on successful deletion", async () => {
    mockApiJson
      .mockResolvedValueOnce(PAGE)
      .mockResolvedValueOnce({ status: "deleted" })
      .mockResolvedValueOnce({ ...PAGE, items: [] });
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /delete/i }));
    await user.click(screen.getByRole("button", { name: /delete permanently/i }));

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/absences/abs-1",
        expect.objectContaining({ method: "DELETE" }),
      );
    });
    await waitFor(() => {
      expect(screen.queryByText("John Smith")).not.toBeInTheDocument();
    });
  });

  it("shows stale edit error toast on delete API conflict", async () => {
    mockApiJson
      .mockResolvedValueOnce(PAGE)
      .mockRejectedValueOnce(new ApiRequestError("Version mismatch", { status: 409, code: "stale_edit" }));
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /delete/i }));
    await user.click(screen.getByRole("button", { name: /delete permanently/i }));

    await waitFor(() => {
      expect(screen.getByText("Absence was changed by another user. Reload and try again.")).toBeInTheDocument();
    });
  });

  it("shows stale edit error toast on cancel API conflict", async () => {
    mockApiJson
      .mockResolvedValueOnce(PAGE)
      .mockRejectedValueOnce(new ApiRequestError("Version mismatch", { status: 409, code: "stale_edit" }))
      .mockResolvedValueOnce(PAGE);
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /cancel/i }));
    await user.selectOptions(screen.getByLabelText(/cancellation reason/i), "other");
    await user.click(screen.getByRole("button", { name: /Cancel Absence/i }));

    await waitFor(() => {
      expect(screen.getByText("One or more absences were changed by another user. Reload and try again.")).toBeInTheDocument();
    });
  });

  it("shows error toast when delete API fails", async () => {
    mockApiJson
      .mockResolvedValueOnce(PAGE)
      .mockRejectedValueOnce(new Error("Delete failed"));
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /delete/i }));
    await user.click(screen.getByRole("button", { name: /delete permanently/i }));

    await waitFor(() => {
      expect(screen.getByText("Delete failed")).toBeInTheDocument();
    });
  });

  it("closes modal when Back button is clicked", async () => {
    mockApiJson.mockResolvedValueOnce(PAGE);
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /delete/i }));
    expect(screen.getByText("Permanently delete absence")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /back/i }));
    expect(screen.queryByText("Permanently delete absence")).not.toBeInTheDocument();
  });

  it("shows Delete Permanently link in cancel modal that transitions to delete modal", async () => {
    mockApiJson.mockResolvedValueOnce(PAGE);
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /cancel/i }));
    const cancelModal = screen.getByText("Cancel absence").closest("[role='dialog']") as HTMLElement;
    expect(cancelModal).toBeInTheDocument();
    expect(within(cancelModal).getByText(/delete permanently/i)).toBeInTheDocument();

    await user.click(within(cancelModal).getByText(/delete permanently/i));
    expect(screen.queryByText("Cancel absence")).not.toBeInTheDocument();
    expect(screen.getByText("Permanently delete absence")).toBeInTheDocument();
    expect(screen.getByText(/permanently remove the absence record/i)).toBeInTheDocument();
  });

  it("shows sit-in subject name for cross-subject sit-in sessions", async () => {
    mockApiJson.mockResolvedValueOnce(PAGE_WITH_CROSS_SUBJECT_SIT_IN);
    renderPage();

    const absenceLink = await screen.findByRole("link", { name: /view john smith absence/i });
    const row = absenceLink.closest("tr");
    if (!row) throw new Error("Expected absence table row");
    expect(row).toHaveTextContent("Natural Sciences");
    expect(row).toHaveTextContent("10:00");
    expect(row).toHaveTextContent("11:30");
    expect(row).not.toHaveTextContent("Physics I");
  });

  it("shows sit-in subject name for same-subject sit-in sessions", async () => {
    mockApiJson.mockResolvedValueOnce(PAGE_WITH_SAME_SUBJECT_SIT_IN);
    renderPage();

    const absenceLink = await screen.findByRole("link", { name: /view john smith absence/i });
    const row = absenceLink.closest("tr");
    if (!row) throw new Error("Expected absence table row");
    expect(row).toHaveTextContent("Mathematics");
    expect(row).toHaveTextContent("10:00");
    expect(row).toHaveTextContent("11:30");
    expect(row).not.toHaveTextContent("SAT Math Scholar C2");
  });

  it("shows Special Approve button for non-cancelled, non-special_approved absences", async () => {
    mockApiJson.mockResolvedValueOnce(PAGE);
    renderPage();

    await screen.findByText("John Smith");
    expect(screen.getByRole("button", { name: /special approve/i })).toBeInTheDocument();
  });

  it("hides Special Approve button for cancelled absences", async () => {
    const cancelledPage = {
      ...PAGE,
      items: [{ ...PAGE.items[0], status: "cancelled" }],
    };
    mockApiJson.mockResolvedValueOnce(cancelledPage);
    renderPage("/absences?status=cancelled");

    await screen.findByText("John Smith");
    expect(screen.queryByRole("button", { name: /special approve/i })).not.toBeInTheDocument();
  });

  it("hides Special Approve button for already special_approved absences", async () => {
    const specialApprovedPage = {
      ...PAGE,
      items: [{ ...PAGE.items[0], status: "special_approved" }],
    };
    mockApiJson.mockResolvedValueOnce(specialApprovedPage);
    renderPage("/absences?status=pending");

    await screen.findByText("John Smith");
    expect(screen.queryByRole("button", { name: /special approve/i })).not.toBeInTheDocument();
  });

  it("opens special approve confirmation modal with student details", async () => {
    mockApiJson.mockResolvedValueOnce(PAGE);
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /special approve/i }));

    const modal = screen.getByRole("dialog");
    expect(within(modal).getByText("Special Approve Absence")).toBeInTheDocument();
    expect(within(modal).getByText(/John Smith/)).toBeInTheDocument();
    expect(within(modal).getByText(/W250389/)).toBeInTheDocument();
    expect(within(modal).getByText(/count toward the student/i)).toBeInTheDocument();
  });

  it("closes special approve modal on Back button click", async () => {
    mockApiJson.mockResolvedValueOnce(PAGE);
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /special approve/i }));
    expect(screen.getByRole("dialog")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /back/i }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("calls status API with special_approved on confirm", async () => {
    const updatedPage = freshPage();
    updatedPage.items[0].status = "special_approved";
    updatedPage.items[0].version = 2;
    mockApiJson
      .mockResolvedValueOnce(PAGE)
      .mockResolvedValueOnce({ status: "special_approved", version: 2 })
      .mockResolvedValueOnce(updatedPage);
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /special approve/i }));
    await user.click(screen.getByRole("button", { name: /confirm special approve/i }));

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/absences/abs-1/status",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ status: "special_approved", expected_version: 1 }),
        }),
      );
    });
  });

  it("shows success toast after special approve", async () => {
    const updatedPage = freshPage();
    updatedPage.items[0].status = "special_approved";
    mockApiJson
      .mockResolvedValueOnce(PAGE)
      .mockResolvedValueOnce({ status: "special_approved", version: 2 })
      .mockResolvedValueOnce(updatedPage);
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /special approve/i }));
    await user.click(screen.getByRole("button", { name: /confirm special approve/i }));

    await waitFor(() => {
      expect(screen.getByText("Absence marked as special approved")).toBeInTheDocument();
    });
  });

  it("shows stale edit error toast on special approve conflict", async () => {
    mockApiJson
      .mockResolvedValueOnce(PAGE)
      .mockRejectedValueOnce(new ApiRequestError("Version mismatch", { status: 409, code: "stale_edit" }))
      .mockResolvedValueOnce(PAGE);
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /special approve/i }));
    await user.click(screen.getByRole("button", { name: /confirm special approve/i }));

    await waitFor(() => {
      expect(screen.getByText(/changed by another user/i)).toBeInTheDocument();
    });
  });

  it("shows error toast when special approve API fails", async () => {
    mockApiJson
      .mockResolvedValueOnce(PAGE)
      .mockRejectedValueOnce(new Error("Special approve failed"));
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /special approve/i }));
    await user.click(screen.getByRole("button", { name: /confirm special approve/i }));

    await waitFor(() => {
      expect(screen.getByText("Special approve failed")).toBeInTheDocument();
    });
  });

  it("renders purple badge for special_approved status", async () => {
    const specialApprovedPage = {
      ...PAGE,
      items: [{ ...PAGE.items[0], status: "special_approved" }],
    };
    mockApiJson.mockResolvedValueOnce(specialApprovedPage);
    renderPage("/absences?status=pending");

    const badge = await screen.findByText("Special Approved");
    expect(badge).toHaveClass("bg-purple-50");
    expect(badge).toHaveClass("text-purple-700");
  });

  it("makes correct API call sequence for special approve with SMS preview", async () => {
    const updatedPage = freshPage();
    updatedPage.items[0].status = "special_approved";
    mockApiJson
      .mockResolvedValueOnce(PAGE)
      .mockResolvedValueOnce({ status: "special_approved", version: 2 })
      .mockResolvedValueOnce({ preview: { phones: ["+66812345678"], message: "Special SMS preview" } });
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /special approve/i }));
    await user.click(screen.getByRole("button", { name: /confirm special approve/i }));

    // 1. Status update called first
    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/absences/abs-1/status",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ status: "special_approved", expected_version: 1 }),
        }),
      );
    });

    // 2. SMS dry-run preview called second
    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/absences/batch-send-success-sms",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ ids: ["abs-1"], dry_run: true }),
        }),
      );
    });

    // 3. SmsConfirmModal appears with preview message
    await waitFor(() => {
      expect(screen.getByText("Special SMS preview")).toBeInTheDocument();
    });
  });

  it("Send button sends SMS and closes modal", async () => {
    const updatedPage = freshPage();
    updatedPage.items[0].status = "special_approved";
    mockApiJson
      .mockResolvedValueOnce(PAGE)
      .mockResolvedValueOnce({ status: "special_approved", version: 2 })
      .mockResolvedValueOnce({ preview: { phones: ["+66812345678"], message: "Preview" } })
      .mockResolvedValueOnce({ sent: true, recipient_count: 1 });
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /special approve/i }));
    await user.click(screen.getByRole("button", { name: /confirm special approve/i }));

    await waitFor(() => {
      expect(screen.getByText("Preview")).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: /send sms/i }));

    await waitFor(() => {
      // Verify actual send (no dry_run)
      const sendCalls = mockApiJson.mock.calls.filter(
        (c: unknown[]) => c[0] === "/api/v1/absences/batch-send-success-sms",
      );
      const lastCall = sendCalls[sendCalls.length - 1];
      const body = JSON.parse((lastCall[1] as RequestInit).body as string);
      expect(body.dry_run).toBeUndefined();
      expect(body.ids).toEqual(["abs-1"]);
    });

    await waitFor(() => {
      expect(screen.getByText(/sms notification sent/i)).toBeInTheDocument();
    });
  });

  it("Skip button closes modal without sending SMS", async () => {
    const updatedPage = freshPage();
    updatedPage.items[0].status = "special_approved";
    mockApiJson
      .mockResolvedValueOnce(PAGE)
      .mockResolvedValueOnce({ status: "special_approved", version: 2 })
      .mockResolvedValueOnce({ preview: { phones: ["+66812345678"], message: "Preview" } });
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /special approve/i }));
    await user.click(screen.getByRole("button", { name: /confirm special approve/i }));

    await waitFor(() => {
      expect(screen.getByText("Preview")).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: /skip/i }));

    await waitFor(() => {
      expect(screen.getByText(/sms skipped/i)).toBeInTheDocument();
    });

    // Verify no non-dry-run SMS call was made
    const sendCalls = mockApiJson.mock.calls.filter(
      (c: unknown[]) => c[0] === "/api/v1/absences/batch-send-success-sms",
    );
    for (const call of sendCalls) {
      const body = JSON.parse((call[1] as RequestInit).body as string);
      expect(body.dry_run).toBe(true);
    }
  });

  it("no SMS sent during status update itself", async () => {
    const updatedPage = freshPage();
    updatedPage.items[0].status = "special_approved";
    mockApiJson
      .mockResolvedValueOnce(PAGE)
      .mockResolvedValueOnce({ status: "special_approved", version: 2 })
      .mockResolvedValueOnce({ preview: null });
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /special approve/i }));
    await user.click(screen.getByRole("button", { name: /confirm special approve/i }));

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/absences/abs-1/status",
        expect.anything(),
      );
    });

    // Verify batch-send-success-sms was called with dry_run: true
    await waitFor(() => {
      const smsCalls = mockApiJson.mock.calls.filter(
        (c: unknown[]) => c[0] === "/api/v1/absences/batch-send-success-sms",
      );
      expect(smsCalls.length).toBeGreaterThanOrEqual(1);
      const body = JSON.parse((smsCalls[0][1] as RequestInit).body as string);
      expect(body.dry_run).toBe(true);
    });

    // Verify NO non-dry-run call was made
    const nonDryRunCalls = mockApiJson.mock.calls.filter(
      (c: unknown[]) => {
        if (c[0] !== "/api/v1/absences/batch-send-success-sms") return false;
        const body = JSON.parse((c[1] as RequestInit).body as string);
        return body.dry_run !== true;
      },
    );
    expect(nonDryRunCalls).toHaveLength(0);
  });

  it("handles SMS preview failure gracefully", async () => {
    const updatedPage = freshPage();
    updatedPage.items[0].status = "special_approved";
    mockApiJson
      .mockResolvedValueOnce(PAGE)
      .mockResolvedValueOnce({ status: "special_approved", version: 2 })
      .mockRejectedValueOnce(new Error("SMS preview failed"));
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /special approve/i }));
    await user.click(screen.getByRole("button", { name: /confirm special approve/i }));

    // Status update should still succeed
    await waitFor(() => {
      expect(screen.getByText("Absence marked as special approved")).toBeInTheDocument();
    });

    // SMS modal should NOT appear (preview failed)
    expect(screen.queryByText("Special SMS preview")).not.toBeInTheDocument();
  });
});
