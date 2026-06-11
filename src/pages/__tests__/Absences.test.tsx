import { beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import Absences from "../Absences";
import { ToastProvider } from "../../hooks/useToast";

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
    vi.clearAllMocks();
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
  });

  it("renders missed session dates in the inbox table", async () => {
    mockApiJson.mockResolvedValueOnce(PAGE_WITH_MISSED_SESSIONS);
    renderPage();

    const absenceLink = await screen.findByRole("link", { name: /view john smith absence/i });
    const row = absenceLink.closest("tr");
    if (!row) {
      throw new Error("Expected absence table row");
    }
    expect(row).toHaveTextContent("1 Jun");
    expect(row).toHaveTextContent("8 Jun");
    expect(row).toHaveTextContent("SAT Math Scholar C2");
    expect(row).not.toHaveTextContent("000000004");
    expect(row).not.toHaveTextContent("31 May - 30 Jun");
  });

  it("marks an absence reviewed using its current version and reloads results", async () => {
    mockApiJson
      .mockResolvedValueOnce(PAGE)
      .mockResolvedValueOnce({ status: "reviewed", version: 2 })
      .mockResolvedValueOnce({ ...PAGE, items: [{ ...PAGE.items[0], status: "reviewed", version: 2 }] });
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

    expect(await screen.findByText("All caught up! No absences match these filters.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /view all/i })).toHaveAttribute("href", "/absences");
    expect(screen.getByRole("link", { name: /view dashboard/i })).toHaveAttribute("href", "/absences/dashboard");
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
    mockApiJson
      .mockResolvedValueOnce(PAGE)
      .mockResolvedValueOnce({ status: "cancelled", version: 2 })
      .mockResolvedValueOnce({ ...PAGE, items: [] });
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
      return twoItemPage;
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
});
