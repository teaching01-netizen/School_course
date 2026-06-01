import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import CourseDetail from "../CourseDetail";
import { ToastProvider } from "../../hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

vi.mock("../../hooks/useAuth", () => ({
  useAuth: () => ({ user: { id: "admin-1", username: "Admin", role: "Admin" }, loading: false }),
}));

function renderCourseDetail() {
  render(
    <MemoryRouter initialEntries={["/courses/course-1"]}>
      <ToastProvider>
        <Routes>
          <Route path="/courses/:id" element={<CourseDetail />} />
        </Routes>
      </ToastProvider>
    </MemoryRouter>,
  );
}

describe("CourseDetail calendar grid", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockApiJson.mockImplementation((path: string) => {
      if (path === "/api/v1/courses/course-1") return Promise.resolve({ id: "course-1", code: "MATH-101", name: "Math" });
      if (path === "/api/v1/courses/course-1/crm-filter") return Promise.resolve({ enabled: false, locked: false, filter: null });
      if (path === "/api/v1/courses/course-1/students") return Promise.resolve([]);
      if (path === "/api/v1/rooms") return Promise.resolve([{ id: "room-1", name: "Room 101", capacity: 20 }]);
      if (path === "/api/v1/users?role=Teacher") return Promise.resolve([{ id: "teacher-1", username: "Teacher One", role: "Teacher" }]);
      if (path === "/api/v1/meta/time") return Promise.resolve({ institute_tz: "Asia/Bangkok", server_now: "2026-05-31T02:00:00Z" });
      throw new Error(`Unexpected API call: ${path}`);
    });
  });

  it("renders 24 time-slot rows in calendar view", async () => {
    mockApiJson.mockImplementation((path: string) => {
      const base: Record<string, unknown> = {
        "/api/v1/courses/course-1": { id: "course-1", code: "MATH-101", name: "Math" },
        "/api/v1/courses/course-1/crm-filter": { enabled: false, locked: false, filter: null },
        "/api/v1/courses/course-1/students": [],
        "/api/v1/rooms": [{ id: "room-1", name: "Room 101", capacity: 20 }],
        "/api/v1/users?role=Teacher": [{ id: "teacher-1", username: "Teacher One", role: "Teacher" }],
        "/api/v1/meta/time": { institute_tz: "Asia/Bangkok", server_now: "2026-05-31T02:00:00Z" },
      };
      if (path.startsWith("/api/v1/sessions?")) return Promise.resolve([]);
      if (base[path] !== undefined) return Promise.resolve(base[path]);
      throw new Error(`Unexpected API call: ${path}`);
    });

    renderCourseDetail();

    await screen.findByRole("button", { name: "Add…" });

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /calendar/i }));

    await waitFor(() => {
      expect(screen.getByText("00:00")).toBeInTheDocument();
      expect(screen.getByText("09:00")).toBeInTheDocument();
      expect(screen.getByText("16:00")).toBeInTheDocument();
      expect(screen.getByText("23:00")).toBeInTheDocument();
    });
  });

  it("places session in correct cell using institute timezone", async () => {
    // 2026-06-01T02:00:00Z = 2026-06-01T09:00:00 Bangkok (Monday)
    const session = {
      id: "sess-1",
      course_id: "course-1",
      room_id: "room-1",
      teacher_id: "teacher-1",
      start_at: "2026-06-01T02:00:00Z",
      end_at: "2026-06-01T04:00:00Z",
      version: 1,
    };

    mockApiJson.mockImplementation((path: string) => {
      const base: Record<string, unknown> = {
        "/api/v1/courses/course-1": { id: "course-1", code: "MATH-101", name: "Math" },
        "/api/v1/courses/course-1/crm-filter": { enabled: false, locked: false, filter: null },
        "/api/v1/courses/course-1/students": [],
        "/api/v1/rooms": [{ id: "room-1", name: "Room 101", capacity: 20 }],
        "/api/v1/users?role=Teacher": [{ id: "teacher-1", username: "Teacher One", role: "Teacher" }],
        "/api/v1/meta/time": { institute_tz: "Asia/Bangkok", server_now: "2026-05-31T02:00:00Z" },
      };
      if (path === "/api/v1/courses/course-1/sessions") return Promise.resolve([session]);
      if (base[path] !== undefined) return Promise.resolve(base[path]);
      throw new Error(`Unexpected API call: ${path}`);
    });

    renderCourseDetail();

    await screen.findByRole("button", { name: "Add…" });

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /calendar/i }));

    // Session card should render "Room 101" somewhere in the calendar
    await waitFor(() => {
      expect(screen.getAllByText("Room 101").length).toBeGreaterThanOrEqual(1);
    });

    // The 09:00 row should contain the session (Bangkok time), not 02:00
    const rows = document.querySelectorAll("tr");
    const row09 = Array.from(rows).find((r) => r.textContent?.includes("09:00") && !r.textContent?.includes("10:00"));
    const row02 = Array.from(rows).find((r) => r.textContent?.includes("02:00") && !r.textContent?.includes("12:00"));
    expect(row09).toBeDefined();
    expect(row09!.textContent).toContain("Room 101");
    expect(row02).toBeDefined();
    expect(row02!.textContent).not.toContain("Room 101");
  });

  it("toggles between table and calendar view", async () => {
    renderCourseDetail();
    await screen.findByRole("button", { name: "Add…" });

    const user = userEvent.setup();

    // Default is table view
    expect(screen.getByText("Date")).toBeInTheDocument();
    expect(screen.getByText("Begin")).toBeInTheDocument();

    // Switch to calendar
    await user.click(screen.getByRole("button", { name: /calendar/i }));
    expect(screen.getByText("Time")).toBeInTheDocument();
    expect(screen.getByText("MON")).toBeInTheDocument();

    // Switch back to table
    await user.click(screen.getByRole("button", { name: /table/i }));
    expect(screen.getByText("Date")).toBeInTheDocument();
  });
});
