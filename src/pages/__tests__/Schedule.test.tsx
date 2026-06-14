import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import Schedule from "../Schedule";
import { ToastProvider } from "../../hooks/useToast";
import { ApiRequestError } from "@/api/client";

const mockApiJson = vi.hoisted(() => vi.fn());
vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

vi.mock("@/hooks/useInstituteMeta", () => ({
  default: () => ({ serverNow: "2026-05-29T10:00:00Z", instituteTZ: "Asia/Bangkok" }),
}));

const courses = [
  { id: "course-1", code: "MATH-101", name: "Math 101" },
  { id: "course-2", code: "SCI-201", name: "Science 201" },
];
const rooms = [
  { id: "room-1", name: "Room 1", capacity: 20 },
  { id: "room-2", name: "Room 2", capacity: 20 },
];
const teachers = [
  { id: "teacher-1", username: "teacher.one", role: "Teacher" },
  { id: "teacher-2", username: "teacher.two", role: "Teacher" },
];

vi.mock("@/hooks/useLookups", () => ({
  default: () => ({
    courses,
    rooms,
    teachers,
    courseById: new Map(courses.map((item) => [item.id, item])),
    roomById: new Map(rooms.map((item) => [item.id, item])),
    teacherById: new Map(teachers.map((item) => [item.id, item])),
    courseOptions: courses.map((item) => ({ value: item.id, label: `${item.code} — ${item.name}` })),
    teacherOptions: teachers.map((item) => ({ value: item.id, label: item.username })),
  }),
}));

beforeEach(() => {
  mockApiJson.mockReset();
});

const pastSession = {
  id: "session-1",
  series_id: null,
  course_id: "course-1",
  room_id: "room-1",
  teacher_id: "teacher-1",
  start_at: "2026-05-20T03:00:00Z",
  end_at: "2026-05-20T04:00:00Z",
  version: 7,
};

function showPastRange() {
  const dateInputs = document.querySelectorAll<HTMLInputElement>('input[type="date"]');
  fireEvent.change(dateInputs[0], { target: { value: "2026-05-20" } });
  fireEvent.change(dateInputs[1], { target: { value: "2026-05-20" } });
}

describe("Schedule empty state", () => {
  it("shows enhanced empty state message when no sessions exist", async () => {
    mockApiJson.mockResolvedValueOnce([]);
    render(
      <MemoryRouter>
        <ToastProvider>
          <Schedule />
        </ToastProvider>
      </MemoryRouter>,
    );

    expect(await screen.findByText(/No sessions for this date range/)).toBeInTheDocument();
    expect(screen.getByText(/Use the toolbar above to create a session or series/)).toBeInTheDocument();
  });
});

describe("Schedule inline editing", () => {
  it("edits a past session inline and sends the converted occurrence payload", async () => {
    const user = userEvent.setup();
    mockApiJson.mockImplementation(async (path, init) => {
      if (typeof path === "string" && path.startsWith("/api/v1/sessions?start=")) return [pastSession];
      if (path === "/api/v1/scheduling/preflight" && init?.method === "POST") return { status: "available" };
      if (path === "/api/v1/sessions/session-1" && init?.method === "PATCH") return { session: { ...pastSession, version: 8 } };
      return [];
    });

    render(
      <MemoryRouter>
        <ToastProvider>
          <Schedule />
        </ToastProvider>
      </MemoryRouter>,
    );

    showPastRange();
    await screen.findByText(/MATH-101/);
    await user.click(screen.getByRole("button", { name: /inline edit session MATH-101/i }));

    const form = screen.getByRole("form", { name: /inline edit session MATH-101/i });
    await user.click(within(form).getByRole("combobox", { name: /room/i }));
    await user.selectOptions(within(form).getByRole("combobox", { name: /room/i }), "room-2");
    await user.clear(within(form).getByLabelText(/start \(local time\)/i));
    await user.type(within(form).getByLabelText(/start \(local time\)/i), "2026-05-20T11:00");
    await user.clear(within(form).getByLabelText(/end \(local time\)/i));
    await user.type(within(form).getByLabelText(/end \(local time\)/i), "2026-05-20T12:00");

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/scheduling/preflight",
        expect.objectContaining({ method: "POST" }),
      );
    });

    await user.click(within(form).getByRole("button", { name: /^save$/i }));

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/sessions/session-1",
        expect.objectContaining({ method: "PATCH" }),
      );
    });
    const patchCall = mockApiJson.mock.calls.find(([path, init]) => path === "/api/v1/sessions/session-1" && init?.method === "PATCH");
    expect(JSON.parse(patchCall?.[1]?.body as string)).toEqual({
      expected_version: 7,
      course_id: "course-1",
      room_id: "room-2",
      teacher_id: "teacher-1",
      start_at: "2026-05-20T04:00:00.000Z",
      end_at: "2026-05-20T05:00:00.000Z",
    });
  });

  it("keeps inline save disabled when preflight blocks the past session edit", async () => {
    const user = userEvent.setup();
    mockApiJson.mockImplementation(async (path, init) => {
      if (typeof path === "string" && path.startsWith("/api/v1/sessions?start=")) return [pastSession];
      if (path === "/api/v1/scheduling/preflight" && init?.method === "POST") {
        const error = new ApiRequestError("Schedule conflict", { status: 409, code: "schedule_conflict" });
        error.details = {
          kind: "teacher_overlap",
          requested: {
            course_id: "course-1",
            room_id: "room-1",
            teacher_id: "teacher-1",
            start_at: "2026-05-20T03:00:00Z",
            end_at: "2026-05-20T04:00:00Z",
          },
          conflicts: [],
        };
        throw error;
      }
      return [];
    });

    render(
      <MemoryRouter>
        <ToastProvider>
          <Schedule />
        </ToastProvider>
      </MemoryRouter>,
    );

    showPastRange();
    await screen.findByText(/MATH-101/);
    await user.click(screen.getByRole("button", { name: /inline edit session MATH-101/i }));
    const form = screen.getByRole("form", { name: /inline edit session MATH-101/i });

    await waitFor(() => expect(within(form).getByRole("button", { name: /blocked/i })).toBeDisabled());
    await user.click(within(form).getByRole("button", { name: /blocked/i }));

    expect(mockApiJson.mock.calls.some(([path, init]) => path === "/api/v1/sessions/session-1" && init?.method === "PATCH")).toBe(false);
  });

  it("keeps inline edit open and reloads after a stale edit response", async () => {
    const user = userEvent.setup();
    mockApiJson.mockImplementation(async (path, init) => {
      if (typeof path === "string" && path.startsWith("/api/v1/sessions?start=")) return [pastSession];
      if (path === "/api/v1/scheduling/preflight" && init?.method === "POST") return { status: "available" };
      if (path === "/api/v1/sessions/session-1" && init?.method === "PATCH") {
        throw new ApiRequestError("Stale edit", { status: 409, code: "stale_edit" });
      }
      if (path === "/api/v1/sessions?ids=session-1") return [{ ...pastSession, version: 8, room_id: "room-2" }];
      return [];
    });

    render(
      <MemoryRouter>
        <ToastProvider>
          <Schedule />
        </ToastProvider>
      </MemoryRouter>,
    );

    showPastRange();
    await screen.findByText(/MATH-101/);
    await user.click(screen.getByRole("button", { name: /inline edit session MATH-101/i }));
    const form = screen.getByRole("form", { name: /inline edit session MATH-101/i });

    await waitFor(() => expect(within(form).getByRole("button", { name: /^save$/i })).not.toBeDisabled());
    await user.click(within(form).getByRole("button", { name: /^save$/i }));

    await waitFor(() => expect(within(form).getByRole("combobox", { name: /room/i })).toHaveValue("room-2"));
    expect(screen.getByText(/Stale edit: reloaded latest session/i)).toBeInTheDocument();
  });
});
