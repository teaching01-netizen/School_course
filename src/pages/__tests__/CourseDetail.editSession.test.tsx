import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
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

const COURSE = {
  id: "course-1",
  version: 3,
  code: "MATH-101",
  name: "Math",
  primary_teacher_id: "teacher-1",
  legacy_course_id: null,
  teachers: [
    { id: "teacher-1", username: "Teacher One", is_primary: true },
    { id: "teacher-2", username: "Teacher Two", is_primary: false },
  ],
};

const SESSIONS = [
  { id: "s1", series_id: null, course_id: "course-1", room_id: "room-1", teacher_id: "teacher-1", start_at: "2026-06-01T02:00:00Z", end_at: "2026-06-01T04:00:00Z", version: 4 },
  { id: "s2", series_id: null, course_id: "course-1", room_id: null, teacher_id: "teacher-2", start_at: "2026-06-03T01:30:00Z", end_at: "2026-06-03T03:30:00Z", version: 2 },
];

const ROOMS = [
  { id: "room-1", name: "Room 101", capacity: 20 },
  { id: "room-2", name: "Room 102", capacity: 30 },
];

const TEACHERS = [
  { id: "teacher-1", username: "Teacher One", role: "Teacher" },
  { id: "teacher-2", username: "Teacher Two", role: "Teacher" },
];

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

function baseImpl(path: string, init?: RequestInit) {
  if (path === "/api/v1/courses/course-1" && init?.method === "PATCH") return Promise.resolve(COURSE);
  if (path === "/api/v1/courses/course-1") return Promise.resolve(COURSE);
  if (path === "/api/v1/courses/course-1/crm-filter") return Promise.resolve({ enabled: false, locked: false, filter: null });
  if (path === "/api/v1/courses/course-1/students") return Promise.resolve([]);
  if (path === "/api/v1/courses/course-1/sessions") return Promise.resolve(SESSIONS);
  if (path === "/api/v1/rooms") return Promise.resolve(ROOMS);
  if (path === "/api/v1/users?role=Teacher") return Promise.resolve(TEACHERS);
  if (path === "/api/v1/meta/time") return Promise.resolve({ institute_tz: "Asia/Bangkok", server_now: "2026-05-31T02:00:00Z" });
  if (path === "/api/v1/operations/schedule-issues/summary") return Promise.resolve({ sessions: {} });
  if (path === "/api/v1/sessions/s1/attendance") return Promise.resolve([]);
  if (path === "/api/v1/sessions/s2/attendance") return Promise.resolve([]);
  if (path === "/api/v1/scheduling/preflight") return Promise.resolve({ status: "available" });
  if (path === "/api/v1/sessions/s1/change-preview") return Promise.resolve({ requires_acknowledgement: false });
  if (path === "/api/v1/sessions/s2/change-preview") return Promise.resolve({ requires_acknowledgement: false });
  if (path === "/api/v1/sessions/s1" && init?.method === "PATCH") return Promise.resolve({ session: SESSIONS[0], change_id: null });
  if (path === "/api/v1/sessions/s2" && init?.method === "PATCH") return Promise.resolve({ session: SESSIONS[1], change_id: null });
  throw new Error(`Unexpected API call: ${path} ${init?.method ?? ""}`);
}

function baseMock() {
  mockApiJson.mockReset();
  mockApiJson.mockImplementation(baseImpl);
}

function lastCallFor(path: string, method?: string): RequestInit | null {
  const calls = mockApiJson.mock.calls.filter(
    ([p, init]) => p === path && (method === undefined || (init as RequestInit | undefined)?.method === method),
  );
  if (calls.length === 0) return null;
  return calls.at(-1)![1] as RequestInit;
}

describe("CourseDetail session editing", () => {
  beforeEach(() => {
    baseMock();
  });

  it("opens a prefilled editor from the date cell and keeps the table calm", async () => {
    const user = userEvent.setup();
    renderCourseDetail();

    await user.click(await screen.findByRole("button", { name: "Edit session Mon 1 Jun 26" }));

    expect(screen.getByLabelText("Date")).toHaveValue("2026-06-01");
    expect(screen.getByLabelText("Start")).toHaveValue("09:00");
    expect(screen.getByLabelText("End")).toHaveValue("11:00");
    expect(screen.getByLabelText("Classroom")).toHaveValue("room-1");
    expect(screen.getByLabelText("Teacher")).toHaveValue("Teacher One");
    // Live summary reflects the zone-converted local times.
    expect(screen.getByText(/Mon 1 Jun 2026, 09:00–11:00/)).toBeInTheDocument();
  });

  it("focuses the clicked field when opening from a specific cell", async () => {
    const user = userEvent.setup();
    renderCourseDetail();

    await user.click(await screen.findByRole("button", { name: "Room 101" }));

    expect(screen.getByLabelText("Classroom")).toHaveFocus();
  });

  it("filters the visible schedule by institute-local session time", async () => {
    const user = userEvent.setup();
    renderCourseDetail();

    expect(await screen.findByRole("button", { name: "Edit session Mon 1 Jun 26" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Edit session Wed 3 Jun 26" })).toBeInTheDocument();

    await user.type(screen.getByLabelText("From"), "09:00");
    await user.type(screen.getByLabelText("To"), "11:00");

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Edit session Mon 1 Jun 26" })).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Edit session Wed 3 Jun 26" })).not.toBeInTheDocument();
    });
    expect(screen.getByText("Showing 1 of 2 sessions")).toBeInTheDocument();
  });

  it("moves focus between fields without losing the open editor", async () => {
    const user = userEvent.setup();
    renderCourseDetail();

    await user.click(await screen.findByRole("button", { name: "Edit session Mon 1 Jun 26" }));
    await user.keyboard("{Escape}");

    // Reopen, then click another cell of the same row: the editor stays open
    // and only focus moves.
    await user.click(await screen.findByRole("button", { name: "Edit session Mon 1 Jun 26" }));
    await user.click(screen.getByRole("button", { name: "09:00" }));

    expect(screen.getByLabelText("Start")).toHaveFocus();
    expect(screen.getByLabelText("Date")).toHaveValue("2026-06-01");
  });

  it("runs the availability check and saves with the session version", async () => {
    const user = userEvent.setup();
    renderCourseDetail();

    await user.click(await screen.findByRole("button", { name: "Edit session Mon 1 Jun 26" }));

    await waitFor(() => {
      expect(lastCallFor("/api/v1/scheduling/preflight", "POST")).not.toBeNull();
    });
    const preflightBody = JSON.parse(lastCallFor("/api/v1/scheduling/preflight", "POST")!.body as string);
    expect(preflightBody).toMatchObject({
      session_id: "s1",
      course_id: "course-1",
      teacher_id: "teacher-1",
      room_id: "room-1",
      start_at: "2026-06-01T02:00:00.000Z",
      end_at: "2026-06-01T04:00:00.000Z",
    });

    const save = screen.getByRole("button", { name: "Save" });
    await waitFor(() => expect(save).toBeEnabled());
    await user.click(save);

    await screen.findByText("Updated session");
    expect(lastCallFor("/api/v1/sessions/s1/change-preview", "POST")).not.toBeNull();
    const patchBody = JSON.parse(lastCallFor("/api/v1/sessions/s1", "PATCH")!.body as string);
    expect(patchBody).toMatchObject({
      expected_version: 4,
      course_id: "course-1",
      room_id: "room-1",
      teacher_id: "teacher-1",
      start_at: "2026-06-01T02:00:00.000Z",
      end_at: "2026-06-01T04:00:00.000Z",
    });
    // The editor closes after a successful save.
    expect(screen.queryByLabelText("Date")).not.toBeInTheDocument();
  });

  it("saves from the Start input with Enter", async () => {
    const user = userEvent.setup();
    renderCourseDetail();

    await user.click(await screen.findByRole("button", { name: "Edit session Mon 1 Jun 26" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Save" })).toBeEnabled());

    await user.type(screen.getByLabelText("Start"), "{Enter}");

    await screen.findByText("Updated session");
    expect(lastCallFor("/api/v1/sessions/s1", "PATCH")).not.toBeNull();
  });

  it("explains a blocked slot and disables saving until fixed", async () => {
    baseImplBlocked();
    const user = userEvent.setup();
    renderCourseDetail();

    await user.click(await screen.findByRole("button", { name: "Edit session Mon 1 Jun 26" }));

    await screen.findByText(/Room 101 is already booked at this time/);
    expect(screen.getByText(/Try a different room or time slot/)).toBeInTheDocument();
    // The slot-finder link carries the conflict so the next page can explain it.
    const slotFinderLink = screen.getByRole("link", { name: /Find alternative slots/ });
    expect(slotFinderLink.getAttribute("href")).toContain("kind=room_overlap");
    expect(slotFinderLink.getAttribute("href")).toContain("room=Room+101");
    const save = screen.getByRole("button", { name: "Fix conflicts to save" });
    expect(save).toBeDisabled();
  });

  it("labels a provisional save when no classroom is set and still allows it", async () => {
    baseImplProvisional();
    const user = userEvent.setup();
    renderCourseDetail();

    // s2 has room_id null — clicking its date cell opens the editor.
    await user.click(await screen.findByRole("button", { name: "Edit session Wed 3 Jun 26" }));

    await screen.findByText("Provisional");
    await screen.findByText(/no classroom assigned; you can still save/);
    const save = screen.getByRole("button", { name: "Save as provisional" });
    await waitFor(() => expect(save).toBeEnabled());
    await user.click(save);

    await screen.findByText("Updated session");
    const patchBody = JSON.parse(lastCallFor("/api/v1/sessions/s2", "PATCH")!.body as string);
    expect(patchBody).toMatchObject({ expected_version: 2, room_id: null });
  });

  it("cancels an edit with Escape without saving", async () => {
    const user = userEvent.setup();
    renderCourseDetail();

    await user.click(await screen.findByRole("button", { name: "Edit session Mon 1 Jun 26" }));
    await user.keyboard("{Escape}");

    expect(screen.queryByLabelText("Date")).not.toBeInTheDocument();
    expect(mockApiJson.mock.calls.some(([p, init]) => p.startsWith("/api/v1/sessions/") && (init as RequestInit | undefined)?.method === "PATCH")).toBe(false);
  });
});

function baseImplBlocked() {
  mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
    if (path === "/api/v1/scheduling/preflight") {
      const err = new ApiRequestError("room overlap", { code: "conflict", status: 409 });
      err.details = {
        kind: "room_overlap",
        requested: {
          start_at: "2026-06-01T02:00:00Z",
          end_at: "2026-06-01T04:00:00Z",
          course_id: "course-1",
          room_id: "room-1",
          teacher_id: "teacher-1",
        },
        conflicts: [
          {
            session_id: "other-session",
            course_id: "course-9",
            room_id: "room-1",
            teacher_id: "teacher-9",
            start_at: "2026-06-01T02:00:00Z",
            end_at: "2026-06-01T04:00:00Z",
          },
        ],
        total_conflicts: 1,
      };
      return Promise.reject(err);
    }
    return baseImpl(path, init);
  });
}

function baseImplProvisional() {
  mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
    if (path === "/api/v1/scheduling/preflight") return Promise.resolve({ status: "provisional" });
    return baseImpl(path, init);
  });
}
