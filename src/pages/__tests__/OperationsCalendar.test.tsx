import { afterAll, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import OperationsCalendar from "../OperationsCalendar";
import { ToastProvider } from "../../hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

function renderPage(initialEntry = "/calendar?view=month") {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <ToastProvider>
        <OperationsCalendar />
      </ToastProvider>
    </MemoryRouter>,
  );
}

function mockCalendarResponse(response: Parameters<typeof mockApiJson.mockResolvedValueOnce>[0]) {
  mockApiJson.mockReset();
  mockApiJson.mockResolvedValueOnce(response);
}

describe("OperationsCalendar", () => {
  const originalTimeZone = process.env.TZ;

  beforeAll(() => {
    process.env.TZ = "Asia/Bangkok";
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.setSystemTime(new Date("2026-06-02T12:00:00Z"));
  });

  afterAll(() => {
    vi.useRealTimers();
    process.env.TZ = originalTimeZone;
  });

  beforeEach(() => {
    mockApiJson.mockReset();
    mockApiJson.mockResolvedValueOnce({
      sessions: [],
      absence_days: [],
    });
  });

  it("renders week grid header", async () => {
    renderPage("/calendar?view=week");
    expect(await screen.findByText("Calendar")).toBeInTheDocument();
    expect(screen.getByText("Today")).toBeInTheDocument();
  });

  it("opens the day modal from month view and links to the absence detail page", async () => {
    mockCalendarResponse({
      sessions: [
        {
          id: "sess-1",
          course_id: "course-1",
          course_code: "0000000002",
          course_name: "SAT Math Scholar C2",
          subject_name: "SAT Math Scholar",
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

    const user = userEvent.setup();
    renderPage("/calendar?view=month");

    await screen.findByText("Calendar");
    await user.click(screen.getByRole("button", { name: /open details for tuesday, 2 june 2026/i }));

    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("Tuesday, 2 June 2026 · 1 session · 1 absence")).toBeInTheDocument();
    expect(within(dialog).getByText("W250389 · John Smith")).toBeInTheDocument();
    expect(within(dialog).getByText(/leave:/i)).toBeInTheDocument();
    expect(within(dialog).getByText(/sit-in:/i)).toBeInTheDocument();
    expect(within(dialog).getByRole("link", { name: /view details for w250389 · john smith/i })).toHaveAttribute("href", "/absences/abs-1");
  });

  it("keeps month session chips and detail modal on the same local calendar day", async () => {
    mockCalendarResponse({
      sessions: [
        {
          id: "sess-midnight",
          course_id: "course-1",
          course_code: "0000000002",
          course_name: "Midnight Math",
          subject_name: "Midnight Math",
          start_at: "2026-06-21T17:00:00Z",
          end_at: "2026-06-21T18:20:00Z",
          room_name: "Room 101",
          teacher_name: "Teacher A",
        },
      ],
      absence_days: [],
    });

    const user = userEvent.setup();
    renderPage("/calendar?view=month");

    await screen.findByText("Calendar");
    await user.click(screen.getByRole("button", { name: /open details for midnight math on monday, 22 june 2026/i }));

    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("Monday, 22 June 2026 · 1 session · 0 absences")).toBeInTheDocument();
    expect(within(dialog).getByText("Midnight Math")).toBeInTheDocument();
  });

  it("opens the day modal from a week session chip", async () => {
    mockCalendarResponse({
      sessions: [
        {
          id: "sess-1",
          course_id: "course-1",
          course_code: "0000000002",
          course_name: "SAT Math Beginner C2",
          subject_name: "SAT Math Beginner",
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
              status: "reviewed",
              subject_name: "Mathematics",
              subject_code: "MATH",
              date_from: "2026-06-02",
              date_to: "2026-06-02",
              sit_in_method: "zoom",
              sit_in_course_name: "Zoom",
              sit_in_subject_name: "Zoom",
            },
          ],
        },
      ],
    });

    const user = userEvent.setup();
    renderPage("/calendar?view=week");

    await screen.findByText("Calendar");
    await user.click(screen.getByRole("button", { name: /open details for sat math beginner on tuesday, 2 june 2026/i }));

    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("Tuesday, 2 June 2026 · 1 session · 1 absence")).toBeInTheDocument();
    expect(within(dialog).getByText("SAT Math Beginner")).toBeInTheDocument();
    expect(within(dialog).getByText("Zoom")).toBeInTheDocument();
  });

  it("closes the day modal with the close button", async () => {
    mockCalendarResponse({
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
              sit_in_method: "teacher_case",
              sit_in_course_name: null,
              sit_in_subject_name: null,
            },
          ],
        },
      ],
    });

    const user = userEvent.setup();
    renderPage("/calendar?view=month");

    await screen.findByText("Calendar");
    await user.click(screen.getByRole("button", { name: /open details for tuesday, 2 june 2026/i }));
    expect(await screen.findByRole("dialog")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /close dialog/i }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("opens overflow items and shows hidden absences", async () => {
    mockCalendarResponse({
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
            {
              id: "abs-2",
              wcode: "W250390",
              student_name: "Jane Roe",
              status: "reviewed",
              subject_name: "Mathematics",
              subject_code: "MATH",
              date_from: "2026-06-02",
              date_to: "2026-06-02",
              sit_in_method: "zoom",
              sit_in_course_name: "Zoom",
              sit_in_subject_name: "Zoom",
            },
            {
              id: "abs-3",
              wcode: "W250391",
              student_name: "Alex Chan",
              status: "actioned",
              subject_name: "Mathematics",
              subject_code: "MATH",
              date_from: "2026-06-02",
              date_to: "2026-06-02",
              sit_in_method: "teacher_case",
              sit_in_course_name: null,
              sit_in_subject_name: null,
            },
          ],
        },
      ],
    });

    const user = userEvent.setup();
    renderPage("/calendar?view=month");

    await screen.findByText("Calendar");
    await user.click(screen.getByRole("button", { name: /view all absence details for tuesday, 2 june 2026/i }));

    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("Tuesday, 2 June 2026 · 0 sessions · 3 absences")).toBeInTheDocument();
    expect(within(dialog).getByText("W250391 · Alex Chan")).toBeInTheDocument();
    expect(within(dialog).getByText(/actioned/i)).toBeInTheDocument();
  });

  it("loads the show filter from the URL and filters to sit-in sessions only", async () => {
    mockCalendarResponse({
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
        {
          id: "sess-2",
          course_id: "course-2",
          course_code: "0000000003",
          course_name: "Chemistry Regular",
          subject_name: "Chemistry",
          start_at: "2026-06-02T11:00:00Z",
          end_at: "2026-06-02T12:30:00Z",
          room_name: "Room 102",
          teacher_name: "Teacher B",
          sit_in_students: [],
        },
      ],
      absence_days: [
        {
          date: "2026-06-02",
          absences: [
            {
              id: "abs-1",
              wcode: "W250400",
              student_name: "Absence Student",
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

    renderPage("/calendar?view=week&show=sit-ins");

    await screen.findByText("Calendar");
    expect(screen.queryByRole("tablist", { name: "Calendar sections" })).not.toBeInTheDocument();
    expect(screen.getByText(/Summary:/)).toHaveTextContent("1 absences | 1 sit-in assignments");
    expect(screen.getByRole("combobox", { name: "Show" })).toHaveValue("sit-ins");

    expect(screen.getByRole("combobox", { name: "Subject" })).toBeInTheDocument();
    expect(screen.queryByRole("combobox", { name: "Status" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /open details for sat math/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /open details for chemistry regular/i })).not.toBeInTheDocument();
    expect(screen.queryByText("W250400 · Absence Student")).not.toBeInTheDocument();
  });

  it("hides sessions when the explicit show filter is absences only", async () => {
    mockCalendarResponse({
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
              wcode: "W250400",
              student_name: "Absence Student",
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

    const user = userEvent.setup();
    renderPage("/calendar?view=week");

    await screen.findByText("Calendar");
    await user.selectOptions(screen.getByRole("combobox", { name: "Show" }), "absences");

    expect(screen.queryByRole("combobox", { name: "Subject" })).not.toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Status" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /open details for sat math/i })).not.toBeInTheDocument();
    expect(screen.getByText("W250400 · Absence Student")).toBeInTheDocument();
  });

  it("shows an empty state for sit-in filters with no matching activity", async () => {
    mockCalendarResponse({
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

    const user = userEvent.setup();
    renderPage("/calendar?view=week");

    await screen.findByText("Calendar");
    await user.selectOptions(screen.getByRole("combobox", { name: "Show" }), "sit-ins");

    expect(screen.getByRole("combobox", { name: "Subject" })).toBeInTheDocument();
    expect(screen.queryByRole("combobox", { name: "Status" })).not.toBeInTheDocument();
    expect(screen.getByText("No sit-in assignments match these filters.")).toBeInTheDocument();
  });

  it("shows an empty state for absence filters with no matching activity", async () => {
    mockCalendarResponse({
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

    const user = userEvent.setup();
    renderPage("/calendar?view=week");

    await screen.findByText("Calendar");
    await user.selectOptions(screen.getByRole("combobox", { name: "Show" }), "absences");

    expect(screen.getByText("No absences match these filters.")).toBeInTheDocument();
  });
});
