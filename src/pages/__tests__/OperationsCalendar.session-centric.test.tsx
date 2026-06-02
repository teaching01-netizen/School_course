import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import OperationsCalendar from "../OperationsCalendar";
import { ToastProvider } from "../../hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

function renderPage(initialEntry = "/calendar?view=week") {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <ToastProvider>
        <OperationsCalendar />
      </ToastProvider>
    </MemoryRouter>,
  );
}

describe("Calendar Session-Centric Redesign", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
  });

  describe("Session card with sit-in visitors", () => {
    it("renders sit-in visitors on session card in week view", async () => {
      mockApiJson.mockResolvedValueOnce({
        sessions: [
          {
            id: "sess-1",
            course_id: "course-1",
            course_code: "0000000002",
            course_name: "SAT Math Scholar",
            subject_name: "SAT Math",
            start_at: "2026-06-02T09:00:00Z",
            end_at: "2026-06-02T10:30:00Z",
            room_name: "Room 101",
            teacher_name: "Teacher A",
            sit_in_students: [
              {
                wcode: "W250389",
                student_name: "John Smith",
                absence_id: "abs-1",
                from_course_code: "0000000001",
                from_course_name: "Physics",
              },
            ],
          },
        ],
        absence_days: [],
      });

      renderPage("/calendar?view=week");

      await screen.findByText("Calendar");
      
      // Session card should show the visitor
      expect(screen.getByText(/W250389/)).toBeInTheDocument();
      expect(screen.getByText(/Physics/)).toBeInTheDocument();
    });

    it("renders multiple sit-in visitors with overflow", async () => {
      mockApiJson.mockResolvedValueOnce({
        sessions: [
          {
            id: "sess-1",
            course_id: "course-1",
            course_code: "0000000002",
            course_name: "SAT Math Scholar",
            subject_name: "SAT Math",
            start_at: "2026-06-02T09:00:00Z",
            end_at: "2026-06-02T10:30:00Z",
            room_name: "Room 101",
            teacher_name: "Teacher A",
            sit_in_students: [
              {
                wcode: "W250389",
                student_name: "John Smith",
                absence_id: "abs-1",
                from_course_code: "0000000001",
                from_course_name: "Physics",
              },
              {
                wcode: "W250390",
                student_name: "Jane Roe",
                absence_id: "abs-2",
                from_course_code: "0000000001",
                from_course_name: "Physics",
              },
              {
                wcode: "W250391",
                student_name: "Alex Chan",
                absence_id: "abs-3",
                from_course_code: "0000000003",
                from_course_name: "Chemistry",
              },
            ],
          },
        ],
        absence_days: [],
      });

      renderPage("/calendar?view=week");

      await screen.findByText("Calendar");
      
      // Should show first 2 visitors
      expect(screen.getByText(/W250389/)).toBeInTheDocument();
      expect(screen.getByText(/W250390/)).toBeInTheDocument();
      // Should show overflow indicator
      expect(screen.getByText(/\+1 more/)).toBeInTheDocument();
    });

    it("does not render visitor line when no sit-ins", async () => {
      mockApiJson.mockResolvedValueOnce({
        sessions: [
          {
            id: "sess-1",
            course_id: "course-1",
            course_code: "0000000002",
            course_name: "SAT Math Scholar",
            subject_name: "SAT Math",
            start_at: "2026-06-02T09:00:00Z",
            end_at: "2026-06-02T10:30:00Z",
            room_name: "Room 101",
            teacher_name: "Teacher A",
            sit_in_students: [],
          },
        ],
        absence_days: [],
      });

      renderPage("/calendar?view=week");

      await screen.findByText("Calendar");
      
      // Should not show visitor line
      expect(screen.queryByText(/Visitors:/)).not.toBeInTheDocument();
    });
  });

  describe("Absence indicator pill", () => {
    it("renders absence count pill in week view", async () => {
      mockApiJson.mockResolvedValueOnce({
        sessions: [],
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

      renderPage("/calendar?view=week");

      await screen.findByText("Calendar");
      
      // Should show absence count pill
      expect(screen.getByText("1")).toBeInTheDocument();
    });

    it("does not render absence pill when no absences", async () => {
      mockApiJson.mockResolvedValueOnce({
        sessions: [],
        absence_days: [],
      });

      renderPage("/calendar?view=week");

      await screen.findByText("Calendar");
      
      // Should not show absence count pills with green/amber/red colors
      // The absence pucks show "0" but with gray-100 bg (neutral)
      // This test verifies no active absence indicators exist
      const absencePucks = screen.getAllByText("0").filter(el => 
        el.tagName === "SPAN" && el.getAttribute("aria-hidden") === "true"
      );
      // All pucks should have gray-100 bg (no absences)
      absencePucks.forEach(puck => {
        expect(puck).toHaveClass("bg-gray-100");
      });
    });
  });

  describe("Day detail modal with sessions first", () => {
    it("opens modal with sessions section first, absences second", async () => {
      mockApiJson.mockResolvedValueOnce({
        sessions: [
          {
            id: "sess-1",
            course_id: "course-1",
            course_code: "0000000002",
            course_name: "SAT Math Scholar",
            subject_name: "SAT Math",
            start_at: "2026-06-02T09:00:00Z",
            end_at: "2026-06-02T10:30:00Z",
            room_name: "Room 101",
            teacher_name: "Teacher A",
            sit_in_students: [
              {
                wcode: "W250389",
                student_name: "John Smith",
                absence_id: "abs-1",
                from_course_code: "0000000001",
                from_course_name: "Physics",
              },
            ],
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

      const user = (await import("@testing-library/user-event")).default.setup();
      renderPage("/calendar?view=month");

      await screen.findByText("Calendar");
      
      // In month view, June 2 should be visible - click its day header to open modal
      await user.click(screen.getByRole("button", { name: /open details for tuesday, 2 june 2026/i }));

      const dialog = await screen.findByRole("dialog");
      
      // Sessions section should come first
      const sessionsHeader = within(dialog).getByText(/Sessions/);
      const absencesHeader = within(dialog).getByText(/Absences/);
      
      // Verify sessions header appears before absences header in DOM order
      expect(sessionsHeader.compareDocumentPosition(absencesHeader) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
      
      // Should show sit-in visitor in session section
      const wcodeElements = within(dialog).getAllByText(/W250389/);
      expect(wcodeElements.length).toBeGreaterThanOrEqual(2);
      const physicsElements = within(dialog).getAllByText(/Physics/);
      expect(physicsElements.length).toBeGreaterThanOrEqual(2);
    });

    it("shows absence details with status badge and view details link", async () => {
      mockApiJson.mockResolvedValueOnce({
        sessions: [],
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

      const user = (await import("@testing-library/user-event")).default.setup();
      renderPage("/calendar?view=month");

      await screen.findByText("Calendar");
      
      // Click the day header for June 2 to open modal
      await user.click(screen.getByRole("button", { name: /open details for tuesday, 2 june 2026/i }));

      const dialog = await screen.findByRole("dialog");
      
      // Should show absence details
      expect(within(dialog).getByText("W250389 · John Smith")).toBeInTheDocument();
      expect(within(dialog).getByText(/Pending/)).toBeInTheDocument();
      expect(within(dialog).getByRole("link", { name: /view details/i })).toHaveAttribute("href", "/absences/abs-1");
    });
  });
});
