import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import OperationsCalendar from "../OperationsCalendar";
import { ToastProvider } from "../../hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

function renderPage(initialEntry = "/") {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <ToastProvider>
        <OperationsCalendar />
      </ToastProvider>
    </MemoryRouter>,
  );
}

describe("OperationsCalendar", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockApiJson.mockResolvedValueOnce({
      sessions: [],
      absence_days: [],
    });
  });

  it("renders week grid header", async () => {
    renderPage();
    expect(await screen.findByText("Calendar")).toBeInTheDocument();
    expect(screen.getByText("Today")).toBeInTheDocument();
  });

  it("renders subject names and inline absence details in month view", async () => {
    mockApiJson.mockReset();
    mockApiJson.mockResolvedValueOnce({
      sessions: [
        {
          id: "sess-1",
          course_id: "course-1",
          course_code: "0000000002",
          course_name: "Algebra II",
          subject_name: "Mathematics",
          start_at: "2026-06-02T09:00:00Z",
          end_at: "2026-06-02T10:30:00Z",
          room_name: "Room 101",
          teacher_name: "Teacher A",
        },
      ],
      absence_days: [
        {
          date: "2026-06-02",
          absences: [
            {
              id: "abs-1",
              wcode: "W250389",
              student_name: "John Smith",
              status: "pending",
              subject_name: "Mathematics",
              subject_code: "MATH",
              date_from: "2026-06-02",
              date_to: "2026-06-02",
              sit_in_method: "physical",
              sit_in_course_name: "Physics",
              sit_in_subject_name: "Physics",
            },
          ],
        },
      ],
    });

    renderPage("/calendar?view=month");

    expect(await screen.findByText("Calendar")).toBeInTheDocument();
    expect(screen.queryByText("0000000002")).not.toBeInTheDocument();
    const absenceCard = screen.getByText("W250389 · John Smith").closest("div");
    expect(absenceCard).toHaveTextContent("Leave:");
    expect(absenceCard).toHaveTextContent("Mathematics");
    expect(absenceCard).toHaveTextContent("Sit-in:");
    expect(absenceCard).toHaveTextContent("Physics");
    expect(screen.getByRole("option", { name: "Mathematics" })).toBeInTheDocument();
  });
});
