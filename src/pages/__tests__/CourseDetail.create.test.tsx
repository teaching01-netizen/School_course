import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within, fireEvent } from "@testing-library/react";
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

const BASE_HANDLERS: Record<string, unknown> = {
  "/api/v1/courses/course-1": { id: "course-1", code: "MATH-101", name: "Math" },
  "/api/v1/courses/course-1/crm-filter": { enabled: false, locked: false, filter: null },
  "/api/v1/courses/course-1/students": [],
  "/api/v1/rooms": [{ id: "room-1", name: "Room 101", capacity: 20 }],
  "/api/v1/users?role=Teacher": [{ id: "teacher-1", username: "Teacher One", role: "Teacher" }],
  "/api/v1/meta/time": { institute_tz: "Asia/Bangkok", server_now: "2026-05-31T02:00:00Z" },
  "/api/v1/scheduling/preflight": { status: "available" },
  "/api/v1/scheduling/preflight_series": { status: "available" },
};

function defaultMock(path: string, init?: RequestInit) {
  if (path.startsWith("/api/v1/courses/") && path.endsWith("/sessions")) return Promise.resolve([]);
  if (path === "/api/v1/sessions" && init?.method === "POST") return Promise.resolve({ id: "created" });
  if (path === "/api/v1/series" && init?.method === "POST") return Promise.resolve({ id: "series-created" });
  if (path in BASE_HANDLERS) return Promise.resolve(BASE_HANDLERS[path as keyof typeof BASE_HANDLERS]);
  throw new Error(`Unexpected API call: ${path}`);
}

describe("CourseDetail create flows", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockApiJson.mockImplementation(defaultMock);
  });

  it("creates a one-off session with the fixed course displayed read-only", async () => {
    const user = userEvent.setup();
    renderCourseDetail();

    await user.click(await screen.findByRole("button", { name: "Add…" }));
    await user.click(screen.getByRole("button", { name: /one-off session/i }));

    const modal = screen.getByRole("dialog", { name: /add to schedule/i });

    expect(within(modal).getByLabelText(/course/i)).toHaveValue("MATH-101 — Math");

    await user.selectOptions(within(modal).getByRole("combobox", { name: /room/i }), "room-1");
    const teacherInput = within(modal).getByRole("combobox", { name: /teacher/i });
    await user.click(teacherInput);
    await user.type(teacherInput, "Teacher One");
    fireEvent.blur(teacherInput);

    await user.clear(within(modal).getByLabelText(/start \(local time\)/i));
    await user.type(within(modal).getByLabelText(/start \(local time\)/i), "2026-05-31T10:00");
    await user.clear(within(modal).getByLabelText(/end \(local time\)/i));
    await user.type(within(modal).getByLabelText(/end \(local time\)/i), "2026-05-31T12:00");

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/scheduling/preflight",
        expect.objectContaining({ method: "POST" }),
      );
    });

    await waitFor(() => {
      expect(within(modal).getByRole("button", { name: /^create session$/i })).not.toBeDisabled();
    });
    await user.click(within(modal).getByRole("button", { name: /^create session$/i }));

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/sessions",
        expect.objectContaining({ method: "POST" }),
      );
    });

    const postCall = mockApiJson.mock.calls.find(([path, init]) => path === "/api/v1/sessions" && init?.method === "POST");
    expect(JSON.parse(postCall?.[1]?.body as string)).toEqual({
      course_id: "course-1",
      room_id: "room-1",
      teacher_id: "teacher-1",
      start_at: "2026-05-31T03:00:00.000Z",
      end_at: "2026-05-31T05:00:00.000Z",
    });
  });

  it("creates a recurring series through the shared recurrence fields", async () => {
    const user = userEvent.setup();
    renderCourseDetail();

    await user.click(await screen.findByRole("button", { name: "Add…" }));

    const modal = screen.getByRole("dialog", { name: /add to schedule/i });

    await user.selectOptions(within(modal).getByRole("combobox", { name: /room/i }), "room-1");
    const teacherInput = within(modal).getByRole("combobox", { name: /teacher/i });
    await user.click(teacherInput);
    await user.type(teacherInput, "Teacher One");
    fireEvent.blur(teacherInput);

    await user.click(within(modal).getByLabelText("Mon"));
    await user.click(within(modal).getByLabelText("Wed"));
    await user.clear(within(modal).getByLabelText(/start time/i));
    await user.type(within(modal).getByLabelText(/start time/i), "16:00");
    await user.clear(within(modal).getByLabelText(/duration/i));
    await user.type(within(modal).getByLabelText(/duration/i), "90");
    await user.clear(within(modal).getByLabelText(/start date/i));
    await user.type(within(modal).getByLabelText(/start date/i), "2026-05-31");
    await user.clear(within(modal).getByLabelText(/end date/i));
    await user.type(within(modal).getByLabelText(/end date/i), "2026-06-30");

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/scheduling/preflight_series",
        expect.objectContaining({ method: "POST" }),
      );
    });

    await waitFor(() => {
      expect(within(modal).getByRole("button", { name: /^create series$/i })).not.toBeDisabled();
    });
    await user.click(within(modal).getByRole("button", { name: /^create series$/i }));

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/series",
        expect.objectContaining({ method: "POST" }),
      );
    });

    const postCall = mockApiJson.mock.calls.find(([path, init]) => path === "/api/v1/series" && init?.method === "POST");
    expect(JSON.parse(postCall?.[1]?.body as string)).toEqual({
      course_id: "course-1",
      room_id: "room-1",
      teacher_id: "teacher-1",
      weekdays: [1, 3],
      start_local_time: "16:00",
      duration_minutes: 90,
      start_date: "2026-05-31",
      end_date: "2026-06-30",
      count: null,
    });
  });
});
