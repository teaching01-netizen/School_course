import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
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

describe("Course detail legacy sync", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/api/v1/courses/course-1") return Promise.resolve({ id: "course-1", code: "MATH-101", name: "Math", legacy_course_id: "7090", legacy_last_synced_at: "2026-05-31T02:00:00Z" });
      if (path === "/api/v1/courses/course-1/crm-filter") return Promise.resolve({ enabled: false, locked: false, filter: null });
      if (path === "/api/v1/courses/course-1/students") return Promise.resolve([]);
      if (path.startsWith("/api/v1/sessions?")) return Promise.resolve([]);
      if (path === "/api/v1/rooms") return Promise.resolve([{ id: "room-1", name: "Room 101", capacity: 20 }]);
      if (path === "/api/v1/users?role=Teacher") return Promise.resolve([{ id: "teacher-1", username: "Teacher One", role: "Teacher" }]);
      if (path === "/api/v1/meta/time") return Promise.resolve({ institute_tz: "Asia/Bangkok", server_now: "2026-05-31T02:00:00Z" });
      if (path === "/api/v1/sessions" && init?.method === "POST") return Promise.resolve({ id: "created" });
      throw new Error(`Unexpected API call: ${path}`);
    });
  });

  it("shows legacy section when course has legacy_course_id", async () => {
    renderCourseDetail();
    const oldSystemLabel = await screen.findByText("Old System");
    expect(oldSystemLabel).toBeInTheDocument();
    expect(screen.getByText(/7090/)).toBeInTheDocument();
    const link = screen.getByRole("link", { name: /open in old system/i });
    expect(link).toBeInTheDocument();
    expect(link).toHaveAttribute("href", "https://warwick.azurewebsites.net/Admin/Courses/Detail?id=7090");
    expect(link).toHaveAttribute("target", "_blank");
  });
});
