import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within, fireEvent } from "@testing-library/react";
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
  if (path === "/api/v1/series" && init?.method === "POST") return Promise.resolve({ id: "series-created" });
  if (path in BASE_HANDLERS) return Promise.resolve(BASE_HANDLERS[path as keyof typeof BASE_HANDLERS]);
  throw new Error(`Unexpected API call: ${path} ${init?.method ?? ""}`);
}

function lastCallFor(path: string, method?: string): RequestInit | null {
  const calls = mockApiJson.mock.calls.filter(
    ([p, init]) => p === path && (method === undefined || (init as RequestInit | undefined)?.method === method),
  );
  if (calls.length === 0) return null;
  return calls.at(-1)![1] as RequestInit;
}

async function openCreatePopover(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole("button", { name: "Add…" }));
  return screen.getByRole("dialog", { name: "New session" });
}

describe("CourseDetail create flows", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockApiJson.mockImplementation(defaultMock);
  });

  it("creates a one-off session from the Add… popover with the course fixed and the first teacher prefilled", async () => {
    const user = userEvent.setup();
    renderCourseDetail();

    const popover = await openCreatePopover(user);

    // The course is the page context — shown, not editable.
    expect(within(popover).getByText(/MATH-101 — Math/)).toBeInTheDocument();
    expect(within(popover).getByLabelText("Course")).toHaveValue("MATH-101 — Math");
    expect(within(popover).getByLabelText("Teacher")).toHaveValue("Teacher One");

    await user.selectOptions(within(popover).getByLabelText("Classroom"), "room-1");
    fireEvent.change(within(popover).getByLabelText("Date"), { target: { value: "2026-05-31" } });
    fireEvent.change(within(popover).getByLabelText("Start"), { target: { value: "10:00" } });
    fireEvent.change(within(popover).getByLabelText("End"), { target: { value: "12:00" } });

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/scheduling/preflight",
        expect.objectContaining({ method: "POST" }),
      );
    });
    const preflightBody = JSON.parse(lastCallFor("/api/v1/scheduling/preflight", "POST")!.body as string);
    expect(preflightBody).toMatchObject({
      session_id: null,
      course_id: "course-1",
      room_id: "room-1",
      teacher_id: "teacher-1",
      start_at: "2026-05-31T03:00:00.000Z",
      end_at: "2026-05-31T05:00:00.000Z",
    });

    const create = within(popover).getByRole("button", { name: /^create session$/i });
    await waitFor(() => expect(create).toBeEnabled());
    await user.click(create);

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith("/api/v1/sessions", expect.objectContaining({ method: "POST" }));
    });
    const postCall = lastCallFor("/api/v1/sessions", "POST")!;
    expect(JSON.parse(postCall.body as string)).toEqual({
      course_id: "course-1",
      room_id: "room-1",
      teacher_id: "teacher-1",
      start_at: "2026-05-31T03:00:00.000Z",
      end_at: "2026-05-31T05:00:00.000Z",
    });

    // Popover closes and the page confirms.
    await screen.findByText("Session created");
    expect(screen.queryByRole("dialog", { name: "New session" })).not.toBeInTheDocument();
  });

  it("shows the availability result inside the popover before creating", async () => {
    const user = userEvent.setup();
    renderCourseDetail();

    const popover = await openCreatePopover(user);

    // Not enough detail yet — the strip says what is missing, quiet and calm.
    expect(within(popover).getByText(/Set Date, time to check availability/)).toBeInTheDocument();

    await user.selectOptions(within(popover).getByLabelText("Classroom"), "room-1");
    fireEvent.change(within(popover).getByLabelText("Date"), { target: { value: "2026-05-31" } });
    fireEvent.change(within(popover).getByLabelText("Start"), { target: { value: "10:00" } });
    fireEvent.change(within(popover).getByLabelText("End"), { target: { value: "12:00" } });

    await within(popover).findByText("Available");
    expect(within(popover).getByText(/no conflicts with this arrangement/)).toBeInTheDocument();
    await waitFor(() => expect(within(popover).getByRole("button", { name: /^create session$/i })).toBeEnabled());
  });

  it("explains a blocked slot and disables the create button until fixed", async () => {
    const blockErr = new ApiRequestError("Room already booked", { code: "conflict", status: 409 });
    blockErr.details = {
      kind: "room_overlap",
      requested: {
        start_at: "2026-05-31T03:00:00.000Z",
        end_at: "2026-05-31T05:00:00.000Z",
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
          start_at: "2026-05-31T03:00:00.000Z",
          end_at: "2026-05-31T05:00:00.000Z",
        },
      ],
      total_conflicts: 1,
    };
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/api/v1/scheduling/preflight") return Promise.reject(blockErr);
      return defaultMock(path, init);
    });

    const user = userEvent.setup();
    renderCourseDetail();

    const popover = await openCreatePopover(user);

    await user.selectOptions(within(popover).getByLabelText("Classroom"), "room-1");
    fireEvent.change(within(popover).getByLabelText("Date"), { target: { value: "2026-05-31" } });
    fireEvent.change(within(popover).getByLabelText("Start"), { target: { value: "10:00" } });
    fireEvent.change(within(popover).getByLabelText("End"), { target: { value: "12:00" } });

    await screen.findByText(/Room 101 is already booked at this time/);
    expect(screen.getByText(/Try a different room or time slot/)).toBeInTheDocument();
    const create = screen.getByRole("button", { name: "Fix conflicts to save" });
    expect(create).toBeDisabled();
  });

  it("creates a provisional session when no classroom is set and labels the button honestly", async () => {
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/api/v1/scheduling/preflight") return Promise.resolve({ status: "provisional" });
      return defaultMock(path, init);
    });

    const user = userEvent.setup();
    renderCourseDetail();

    const popover = await openCreatePopover(user);

    fireEvent.change(within(popover).getByLabelText("Date"), { target: { value: "2026-05-31" } });
    fireEvent.change(within(popover).getByLabelText("Start"), { target: { value: "10:00" } });
    fireEvent.change(within(popover).getByLabelText("End"), { target: { value: "12:00" } });

    await within(popover).findByText("Provisional");
    expect(within(popover).getByText(/no classroom assigned; you can still create/)).toBeInTheDocument();

    const create = within(popover).getByRole("button", { name: "Create as provisional" });
    await waitFor(() => expect(create).toBeEnabled());
    await user.click(create);

    await screen.findByText("Session created");
    const postBody = JSON.parse(lastCallFor("/api/v1/sessions", "POST")!.body as string);
    expect(postBody).toMatchObject({ course_id: "course-1", room_id: null, teacher_id: "teacher-1" });
  });

  it("creates with Enter from the time field", async () => {
    const user = userEvent.setup();
    renderCourseDetail();

    const popover = await openCreatePopover(user);

    await user.selectOptions(within(popover).getByLabelText("Classroom"), "room-1");
    fireEvent.change(within(popover).getByLabelText("Date"), { target: { value: "2026-05-31" } });
    fireEvent.change(within(popover).getByLabelText("Start"), { target: { value: "10:00" } });
    fireEvent.change(within(popover).getByLabelText("End"), { target: { value: "12:00" } });

    await waitFor(() => expect(within(popover).getByRole("button", { name: /^create session$/i })).toBeEnabled());
    await user.type(within(popover).getByLabelText("End"), "{Enter}");

    await screen.findByText("Session created");
    expect(lastCallFor("/api/v1/sessions", "POST")).not.toBeNull();
  });

  it("dismisses the create popover with Escape without creating anything", async () => {
    const user = userEvent.setup();
    renderCourseDetail();

    const popover = await openCreatePopover(user);

    await user.type(within(popover).getByLabelText("Date"), "2026-05-31");
    await user.keyboard("{Escape}");

    expect(screen.queryByRole("dialog", { name: "New session" })).not.toBeInTheDocument();
    expect(mockApiJson.mock.calls.some(([p, init]) => p === "/api/v1/sessions" && (init as RequestInit | undefined)?.method === "POST")).toBe(false);
  });

  it("opens the recurring series flow from the popover quick link", async () => {
    const user = userEvent.setup();
    renderCourseDetail();

    const popover = await openCreatePopover(user);
    await user.click(within(popover).getByRole("button", { name: /recurring series/i }));

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
      expect(mockApiJson).toHaveBeenCalledWith("/api/v1/series", expect.objectContaining({ method: "POST" }));
    });

    const postCall = lastCallFor("/api/v1/series", "POST")!;
    expect(JSON.parse(postCall.body as string)).toEqual({
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
