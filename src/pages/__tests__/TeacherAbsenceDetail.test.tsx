import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import TeacherAbsenceDetail from "../TeacherAbsenceDetail";
import { ToastProvider } from "../../hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

const mockUseAuth = vi.hoisted(() => vi.fn());
vi.mock("../../hooks/useAuth", () => ({
  useAuth: mockUseAuth,
}));

const detail = {
  id: "abs-1",
  wcode: "W250389",
  student_name: "John Smith",
  student_nickname: "John",
  course_code: "MATH-201",
  course_name: "Algebra II",
  subject_name: "Mathematics",
  date_from: "2026-06-20",
  date_to: "2026-06-21",
  reason_category: "medical",
  reason: "Appointment",
  status: "pending",
  version: 1,
  missed_sessions: [{ session_id: "s-1", course_code: "MATH-201", course_name: "Algebra II", subject_name: "Mathematics", room_name: "A1", start_at: "2026-06-20T02:00:00Z", end_at: "2026-06-20T03:00:00Z" }],
  sit_in_sessions: [{ session_id: "s-2", course_code: "PHYS-201", course_name: "Physics II", subject_name: "Physics", room_name: "B1", start_at: "2026-06-22T02:00:00Z", end_at: "2026-06-22T03:00:00Z" }],
};

describe("Teacher absence detail", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockUseAuth.mockReturnValue({ user: { username: "teacher", role: "Teacher" }, logout: vi.fn() });
  });

  it("loads the teacher-scoped endpoint and renders teaching-relevant data read-only", async () => {
    mockApiJson.mockResolvedValueOnce(detail);
    render(
      <MemoryRouter initialEntries={["/teacher-dashboard/absences/abs-1"]}>
        <ToastProvider>
          <Routes>
            <Route path="/teacher-dashboard/absences/:id" element={<TeacherAbsenceDetail />} />
          </Routes>
        </ToastProvider>
      </MemoryRouter>,
    );

    expect(await screen.findByText("John")).toBeInTheDocument();
    expect(mockApiJson).toHaveBeenCalledWith("/api/v1/teacher/absences/abs-1", { method: "GET" });
    expect(screen.getByText("Appointment")).toBeInTheDocument();
    expect(screen.getByText(/read-only/i)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /back to dashboard/i })).toHaveAttribute("href", "/teacher-dashboard");
    expect(screen.queryByRole("button", { name: /override sit-in/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /mark reviewed|mark actioned|cancel|save note/i })).not.toBeInTheDocument();
    expect(screen.queryByText(/admin note|timeline/i)).not.toBeInTheDocument();
    expect(screen.getAllByText("Mathematics").length).toBeGreaterThan(0);
    expect(screen.getByText("Physics")).toBeInTheDocument();
    expect(screen.queryByText(/MATH-201|PHYS-201/)).not.toBeInTheDocument();
  });

  it("shows the override action for administrators on the teacher detail route", async () => {
    mockUseAuth.mockReturnValue({ user: { username: "admin", role: "Admin" }, logout: vi.fn() });
    mockApiJson.mockResolvedValueOnce(detail);
    render(
      <MemoryRouter initialEntries={["/teacher-dashboard/absences/abs-1"]}>
        <ToastProvider>
          <Routes>
            <Route path="/teacher-dashboard/absences/:id" element={<TeacherAbsenceDetail />} />
          </Routes>
        </ToastProvider>
      </MemoryRouter>,
    );

    expect(await screen.findByText("John")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /override sit-in/i })).toBeInTheDocument();
  });

  it("links back to /absences/dashboard when user is an Admin", async () => {
    mockUseAuth.mockReturnValue({ user: { username: "admin", role: "Admin" }, logout: vi.fn() });
    mockApiJson.mockResolvedValueOnce(detail);
    render(
      <MemoryRouter initialEntries={["/teacher-dashboard/absences/abs-1"]}>
        <ToastProvider>
          <Routes>
            <Route path="/teacher-dashboard/absences/:id" element={<TeacherAbsenceDetail />} />
          </Routes>
        </ToastProvider>
      </MemoryRouter>,
    );

    expect(await screen.findByText("John")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /back to dashboard/i })).toHaveAttribute("href", "/absences/dashboard");
  });
});
