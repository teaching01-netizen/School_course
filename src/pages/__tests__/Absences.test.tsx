import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
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
      sit_in_method: "zoom",
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

    expect(await screen.findByText("John Smith")).toBeInTheDocument();
    expect(screen.getByText("Awaiting review")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /view john smith absence/i })).toHaveAttribute("href", "/absences/abs-1");
    expect(mockApiJson).toHaveBeenCalledWith(
      expect.stringContaining("status=pending"),
      expect.objectContaining({ method: "GET" }),
    );
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

  it("bulk cancels selected absences with a recorded reason", async () => {
    mockApiJson
      .mockResolvedValueOnce(PAGE)
      .mockResolvedValueOnce({ status: "cancelled", version: 2 })
      .mockResolvedValueOnce({ ...PAGE, items: [] });
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByLabelText("Select W250389"));
    await user.click(screen.getByRole("button", { name: /cancel selected/i }));
    await user.type(screen.getByLabelText(/cancellation reason/i), "Reported in error");
    const confirm = screen.getByRole("button", { name: /^cancel absence/i });
    expect(confirm).toBeEnabled();
    await user.click(confirm);

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/absences/abs-1/status",
        expect.objectContaining({ body: JSON.stringify({ status: "cancelled", expected_version: 1, reason: "Reported in error" }) }),
      );
    });
  });
});
