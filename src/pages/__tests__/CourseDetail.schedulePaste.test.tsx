import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import CourseDetail from "../CourseDetail";
import { ToastProvider } from "../../hooks/useToast";
import { ApiRequestError } from "@/api/client";

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
  if (path in BASE_HANDLERS) return Promise.resolve(BASE_HANDLERS[path as keyof typeof BASE_HANDLERS]);
  throw new Error(`Unexpected API call: ${path}`);
}

describe("Course detail schedule paste", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
  });

  it("previews pasted schedule rows and creates sessions after preflight passes", async () => {
    mockApiJson.mockImplementation(defaultMock);

    const user = userEvent.setup();
    renderCourseDetail();

    await user.click(await screen.findByRole("button", { name: "Add…" }));
    await user.click(screen.getByRole("button", { name: /paste schedule/i }));
    const pasteBox = screen.getByLabelText(/paste schedule rows/i);
    await user.click(pasteBox);
    await user.paste(
      [
        "Date\tBegin\tEnd\tDuration\tClassroom\tConfirm\tBy\t",
        "Sun 31 May 26\t13:00\t15:00\t02:00\tRoom 101\t\t\t",
        "Sat 06 Jun 26\t15:00\t16:30\t01:30\t\t\t\t",
      ].join("\n"),
    );

    const preview = await screen.findByRole("table", { name: /pasted schedule preview/i });
    expect(within(preview).getByText("2026-05-31")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /create 2 sessions/i }));

    await waitFor(() => {
      const posts = mockApiJson.mock.calls.filter(([path, init]) => path === "/api/v1/sessions" && init?.method === "POST");
      expect(posts).toHaveLength(2);
      expect(JSON.parse(posts[0][1].body as string)).toMatchObject({
        course_id: "course-1",
        room_id: "room-1",
        teacher_id: "teacher-1",
      });
      expect(JSON.parse(posts[1][1].body as string)).toMatchObject({
        course_id: "course-1",
        room_id: null,
        teacher_id: "teacher-1",
      });
    });
  });

  it("shows conflict modal when some preflights reject and allows adding non-conflicting only", async () => {
    const conflictErr = new ApiRequestError("Room already booked", { code: "conflict", status: 409 });
    conflictErr.details = {
      kind: "room_overlap",
      requested: {
        start_at: "2026-05-31T06:00:00.000Z",
        end_at: "2026-05-31T08:00:00.000Z",
        course_id: "course-1",
        room_id: "room-1",
        teacher_id: "teacher-1",
      },
      conflicts: [
        {
          session_id: "existing",
          course_id: "course-2",
          room_id: "room-1",
          teacher_id: "teacher-2",
          start_at: "2026-05-31T06:00:00.000Z",
          end_at: "2026-05-31T08:00:00.000Z",
        },
      ],
    };

    let idx = 0;
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/api/v1/scheduling/preflight") {
        idx++;
        if (idx === 1) return Promise.reject(conflictErr);
        return Promise.resolve({ status: "available" });
      }
      return defaultMock(path, init);
    });

    const user = userEvent.setup();
    renderCourseDetail();

    await user.click(await screen.findByRole("button", { name: "Add…" }));
    await user.click(screen.getByRole("button", { name: /paste schedule/i }));
    const pasteBox = screen.getByLabelText(/paste schedule rows/i);
    await user.click(pasteBox);
    await user.paste(
      [
        "Date\tBegin\tEnd\tDuration\tClassroom\tConfirm\tBy\t",
        "Sun 31 May 26\t13:00\t15:00\t02:00\tRoom 101\t\t\t",
        "Sat 06 Jun 26\t15:00\t16:30\t01:30\t\t\t\t",
      ].join("\n"),
    );

    await screen.findByRole("table", { name: /pasted schedule preview/i });
    await user.click(screen.getByRole("button", { name: /create 2 sessions/i }));

    await screen.findByText("Schedule Conflicts");

    expect(screen.getByText(/Room already booked/)).toBeInTheDocument();
    expect(screen.getByText("Available")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /add 1 available session/i }));

    await waitFor(() => {
      const posts = mockApiJson.mock.calls.filter(([path, init]) => path === "/api/v1/sessions" && init?.method === "POST");
      expect(posts).toHaveLength(1);
    });
  });
});
