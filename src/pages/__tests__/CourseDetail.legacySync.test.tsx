import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
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

  it("shows the legacy link popover when the course has legacy_course_id", async () => {
    const user = userEvent.setup();
    renderCourseDetail();

    // Resting state is just the icon; the management UI lives in the popover.
    const linkButton = await screen.findByRole("button", { name: "Legacy system link" });
    expect(linkButton).toBeInTheDocument();

    await user.click(linkButton);

    expect(screen.getByText("Old System")).toBeInTheDocument();
    expect(screen.getByText(/7090/)).toBeInTheDocument();
    expect(screen.getByText(/managed by the legacy sync service/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Remove link" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Queue refresh" })).toBeInTheDocument();
  });
});
