import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import Absences from "../Absences";
import AbsenceDetail from "../AbsenceDetail";
import TeacherProfile from "../TeacherProfile";
import Reports from "../Reports";
import { ToastProvider } from "../../hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
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

const DETAIL = {
  id: "abs-1",
  wcode: "W250389",
  student_name: "John Smith",
  student_nickname: null,
  student_email: null,
  student_phone: null,
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
  sit_in_course_id: "sit-1",
  sit_in_course_code: "MATH-301",
  sit_in_course_name: "Calculus III",
  status: "pending",
  admin_notes: "",
  version: 1,
  created_at: "2026-05-27T09:00:00Z",
  updated_at: "2026-05-27T09:00:00Z",
  sit_ins: [],
  timeline: [],
};

const TEACHERS = [{ id: "t1", username: "jsmith", role: "Teacher" as const }];
const COURSES: unknown[] = [];
const ROOMS: unknown[] = [];
const SESSIONS: unknown[] = [];

describe("Absence cross-links", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('Absences inbox renders ⚙️ Settings link to Operations form-settings', async () => {
    mockApiJson.mockResolvedValueOnce(PAGE);
    render(
      <MemoryRouter initialEntries={["/absences"]}>
        <ToastProvider><Absences /></ToastProvider>
      </MemoryRouter>,
    );

    await waitFor(() => expect(screen.getByText("John Smith")).toBeInTheDocument());

    const settingsLink = screen.getByTitle("Configure absence form settings");
    expect(settingsLink).toBeInTheDocument();
    expect(settingsLink).toHaveAttribute("href", "/admin/operations?tab=form-settings");
  });

  it('Absence detail renders "View on Calendar" and "Find Alternative Slots" links', async () => {
    mockApiJson.mockResolvedValueOnce(DETAIL);
    render(
      <MemoryRouter initialEntries={["/absences/abs-1"]}>
        <ToastProvider>
          <Routes>
            <Route path="/absences/:id" element={<AbsenceDetail />} />
          </Routes>
        </ToastProvider>
      </MemoryRouter>,
    );

    expect(await screen.findByText("John Smith")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /view on calendar/i })).toHaveAttribute("href", "/absences/calendar");
    expect(screen.getByRole("link", { name: /find alternative slots/i })).toHaveAttribute("href", "/slot-finder");
  });

  it('Teacher profile renders "Manage account" link to Users page', async () => {
    mockApiJson
      .mockResolvedValueOnce(TEACHERS)
      .mockResolvedValueOnce(COURSES)
      .mockResolvedValueOnce(ROOMS)
      .mockResolvedValueOnce(SESSIONS);
    render(
      <MemoryRouter initialEntries={["/teachers/t1"]}>
        <ToastProvider>
          <Routes>
            <Route path="/teachers/:id" element={<TeacherProfile />} />
          </Routes>
        </ToastProvider>
      </MemoryRouter>,
    );

    await waitFor(() => expect(screen.getByText(/Back/i)).toBeInTheDocument());
    expect(screen.getByRole("link", { name: /manage account/i })).toHaveAttribute("href", "/users");
  });

  it('Reports renders "Absences" tab with link to Absence Dashboard', async () => {
    mockApiJson.mockResolvedValue([]);
    render(
      <MemoryRouter initialEntries={["/reports"]}>
        <ToastProvider><Reports /></ToastProvider>
      </MemoryRouter>,
    );

    await waitFor(() => expect(screen.getByText("Report")).toBeInTheDocument());

    const absTab = screen.getByRole("button", { name: /absences/i });
    expect(absTab).toBeInTheDocument();

    fireEvent.click(absTab);

    await waitFor(() => {
      const dashboardLink = screen.getByRole("link", { name: /go to absence dashboard/i });
      expect(dashboardLink).toBeInTheDocument();
      expect(dashboardLink).toHaveAttribute("href", "/absences/dashboard");
    });
  });
});
