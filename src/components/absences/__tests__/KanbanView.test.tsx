import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import KanbanView from "../KanbanView";
import { ToastProvider } from "../../../hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

const PENDING_ABSENCE = {
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
  sit_in_method: "physical",
  status: "pending",
  version: 1,
  created_at: "2026-05-27T09:00:00Z",
  updated_at: "2026-05-27T09:00:00Z",
};

function renderKanban() {
  return render(
    <MemoryRouter initialEntries={["/absences?view=board"]}>
      <ToastProvider>
        <KanbanView filters={{ query: "", subject: "", dateFrom: "", dateTo: "" }} />
      </ToastProvider>
    </MemoryRouter>,
  );
}

describe("KanbanView hard delete", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows delete button for pending absences", async () => {
    mockApiJson.mockImplementation(async (url: string) => {
      if (url.includes("status=pending")) return { items: [PENDING_ABSENCE], total_count: 1, offset: 0, limit: 20 };
      return { items: [], total_count: 0, offset: 0, limit: 20 };
    });

    renderKanban();

    expect(await screen.findByText("John Smith")).toBeInTheDocument();
    const deleteBtn = screen.getByRole("button", { name: /delete/i });
    expect(deleteBtn).toBeInTheDocument();
  });

  it("opens delete confirmation modal when delete button is clicked", async () => {
    mockApiJson.mockImplementation(async (url: string) => {
      if (url.includes("status=pending")) return { items: [PENDING_ABSENCE], total_count: 1, offset: 0, limit: 20 };
      return { items: [], total_count: 0, offset: 0, limit: 20 };
    });

    renderKanban();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /delete/i }));

    const modal = screen.getByRole("dialog");
    expect(within(modal).getByText("Permanently delete absence")).toBeInTheDocument();
    expect(within(modal).getByText(/permanently remove the absence record/i)).toBeInTheDocument();
    expect(within(modal).getByText("John Smith")).toBeInTheDocument();
  });

  it("calls DELETE API and removes card on successful deletion", async () => {
    mockApiJson.mockImplementation(async (url: string, init?: RequestInit) => {
      if (init?.method === "DELETE") return { status: "deleted" };
      if (url.includes("status=pending")) return { items: [PENDING_ABSENCE], total_count: 1, offset: 0, limit: 20 };
      return { items: [], total_count: 0, offset: 0, limit: 20 };
    });

    renderKanban();
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
    mockApiJson.mockImplementation(async (url: string, init?: RequestInit) => {
      if (init?.method === "DELETE") throw new Error("Delete failed");
      if (url.includes("status=pending")) return { items: [PENDING_ABSENCE], total_count: 1, offset: 0, limit: 20 };
      return { items: [], total_count: 0, offset: 0, limit: 20 };
    });

    renderKanban();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /delete/i }));
    await user.click(screen.getByRole("button", { name: /delete permanently/i }));

    await waitFor(() => {
      expect(screen.getByText("Delete failed")).toBeInTheDocument();
    });
  });

  it("closes modal when Back button is clicked", async () => {
    mockApiJson.mockImplementation(async (url: string) => {
      if (url.includes("status=pending")) return { items: [PENDING_ABSENCE], total_count: 1, offset: 0, limit: 20 };
      return { items: [], total_count: 0, offset: 0, limit: 20 };
    });

    renderKanban();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /delete/i }));
    expect(screen.getByText("Permanently delete absence")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /back/i }));
    expect(screen.queryByText("Permanently delete absence")).not.toBeInTheDocument();
  });

  it("shows Delete Permanently link in cancel modal that transitions to delete modal", async () => {
    mockApiJson.mockImplementation(async (url: string) => {
      if (url.includes("status=pending")) return { items: [PENDING_ABSENCE], total_count: 1, offset: 0, limit: 20 };
      return { items: [], total_count: 0, offset: 0, limit: 20 };
    });

    renderKanban();
    const user = userEvent.setup();

    await screen.findByText("John Smith");
    const cancelBtn = screen.getByRole("button", { name: /^Cancel$/ });
    await user.click(cancelBtn);
    expect(screen.getByText("Cancel absence")).toBeInTheDocument();
    expect(screen.getByText(/delete permanently/i)).toBeInTheDocument();

    await user.click(screen.getByText(/delete permanently/i));
    expect(screen.queryByText("Cancel absence")).not.toBeInTheDocument();
    expect(screen.getByText("Permanently delete absence")).toBeInTheDocument();
    expect(screen.getByText(/permanently remove the absence record/i)).toBeInTheDocument();
  });
});
