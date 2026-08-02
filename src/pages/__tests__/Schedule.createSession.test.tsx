import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import Schedule from "../Schedule";
import { ToastProvider } from "@/hooks/useToast";
import { ApiRequestError } from "@/api/client";
import { queryClient } from "@/query/cache";

const mockApiJson = vi.hoisted(() => vi.fn());
const queryFixture = vi.hoisted(() => ({ url: "", sessions: [] as unknown[] }));
const mockRefetch = vi.hoisted(() => vi.fn(async () => {
  if (queryFixture.url) await mockApiJson(queryFixture.url, { method: "GET" });
  return { data: queryFixture.sessions };
}));
const lookupFixture = vi.hoisted(() => ({
  courses: [{ id: "course-1", code: "MATH-101", name: "Math 101" }],
  rooms: [
    { id: "room-1", name: "Room 1", capacity: 20 },
    { id: "room-2", name: "Room 2", capacity: 20 },
  ],
  teachers: [
    { id: "teacher-1", username: "teacher.one", role: "Teacher" },
    { id: "teacher-2", username: "teacher.two", role: "Teacher" },
  ],
}));

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

vi.mock("@/hooks/useInstituteMeta", () => ({
  default: () => ({ serverNow: "2026-08-02T10:00:00Z", instituteTZ: "Asia/Bangkok" }),
}));

vi.mock("@/features/scheduling/hooks/useLookups", () => ({
  default: () => ({
    ...lookupFixture,
    courseById: new Map(lookupFixture.courses.map((item) => [item.id, item])),
    roomById: new Map(lookupFixture.rooms.map((item) => [item.id, item])),
    teacherById: new Map(lookupFixture.teachers.map((item) => [item.id, item])),
    courseOptions: lookupFixture.courses.map((item) => ({ value: item.id, label: `${item.code} — ${item.name}` })),
    teacherOptions: lookupFixture.teachers.map((item) => ({ value: item.id, label: item.username })),
  }),
}));

vi.mock("@/query/useOperationalQuery", () => ({
  useOperationalQuery: (_queryKey: unknown, url: string | null) => {
    queryFixture.url = url ?? "";
    return { data: queryFixture.sessions, error: null, isPending: false, refetch: mockRefetch };
  },
}));

function renderSchedule() {
  return render(
    <MemoryRouter>
      <ToastProvider>
        <Schedule />
      </ToastProvider>
    </MemoryRouter>,
  );
}

async function openCreate(user: ReturnType<typeof userEvent.setup>) {
  renderSchedule();
  await user.click(screen.getByRole("button", { name: "Create Session" }));
  return screen.getByRole("dialog", { name: "Create Session" });
}

async function fillTime(user: ReturnType<typeof userEvent.setup>, dialog: HTMLElement) {
  await user.clear(within(dialog).getByLabelText(/start \(local time\)/i));
  await user.type(within(dialog).getByLabelText(/start \(local time\)/i), "2026-08-03T10:00");
  await user.clear(within(dialog).getByLabelText(/end \(local time\)/i));
  await user.type(within(dialog).getByLabelText(/end \(local time\)/i), "2026-08-03T11:00");
}

function conflictError(kind: string, message: string): ApiRequestError {
  const error = new ApiRequestError(message, { status: 409, code: kind });
  error.details = {
    kind,
    requested: {
      course_id: "course-1",
      room_id: "room-1",
      teacher_id: "teacher-1",
      start_at: "2026-08-03T03:00:00.000Z",
      end_at: "2026-08-03T04:00:00.000Z",
    },
    conflicts: [],
  };
  return error;
}

function sessionPosts() {
  return mockApiJson.mock.calls.filter(([path, init]) => path === "/api/v1/sessions" && init?.method === "POST");
}

describe("Schedule standalone create user stories", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    queryFixture.url = "";
    queryClient.clear();
  });

  it("creates a standalone session through the real controls after available preflight", async () => {
    const user = userEvent.setup();
    const requestKinds: string[] = [];
    let sessionListCalls = 0;
    mockApiJson.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path.startsWith("/api/v1/sessions?start=")) {
        sessionListCalls += 1;
        return [];
      }
      if (path === "/api/v1/scheduling/preflight" && init?.method === "POST") {
        requestKinds.push("preflight");
        return { status: "available" };
      }
      if (path === "/api/v1/sessions" && init?.method === "POST") {
        requestKinds.push("create");
        return { id: "session-1" };
      }
      return [];
    });

    const dialog = await openCreate(user);
    expect(within(dialog).getByRole("combobox", { name: "Course" })).toHaveValue("MATH-101 — Math 101");
    expect(within(dialog).getByRole("combobox", { name: "Teacher" })).toHaveValue("teacher.one");
    await user.selectOptions(within(dialog).getByRole("combobox", { name: "Room" }), "room-1");
    await fillTime(user, dialog);

    await waitFor(() => expect(within(dialog).getByText("Available")).toBeInTheDocument());
    await user.click(within(dialog).getByRole("button", { name: /^Create$/i }));

    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Create Session" })).not.toBeInTheDocument());
    expect(requestKinds).toEqual(["preflight", "create"]);
    expect(sessionPosts()).toHaveLength(1);
    expect(JSON.parse(String(sessionPosts()[0][1]?.body))).toEqual({
      course_id: "course-1",
      room_id: "room-1",
      teacher_id: "teacher-1",
      start_at: "2026-08-03T03:00:00.000Z",
      end_at: "2026-08-03T04:00:00.000Z",
    });
    expect(screen.getByText("Session created")).toBeInTheDocument();
    await waitFor(() => expect(sessionListCalls).toBe(1));
  });

  it("keeps a nil-room standalone session provisional and submits room_id null", async () => {
    const user = userEvent.setup();
    mockApiJson.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path.startsWith("/api/v1/sessions?start=")) return [];
      if (path === "/api/v1/scheduling/preflight" && init?.method === "POST") return { status: "provisional" };
      if (path === "/api/v1/sessions" && init?.method === "POST") return { id: "session-1" };
      return [];
    });

    const dialog = await openCreate(user);
    await fillTime(user, dialog);
    await waitFor(() => expect(within(dialog).getByText("Provisional")).toBeInTheDocument());
    expect(within(dialog).getByRole("button", { name: /^Create$/i })).toBeEnabled();

    await user.click(within(dialog).getByRole("button", { name: /^Create$/i }));
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Create Session" })).not.toBeInTheDocument());
    expect(JSON.parse(String(sessionPosts()[0][1]?.body))).toEqual(expect.objectContaining({ room_id: null }));
  });

  it("recovers from a blocked Room 1 preflight after selecting Room 2", async () => {
    const user = userEvent.setup();
    const preflightRooms: unknown[] = [];
    mockApiJson.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path.startsWith("/api/v1/sessions?start=")) return [];
      if (path === "/api/v1/scheduling/preflight" && init?.method === "POST") {
        const body = JSON.parse(String(init.body));
        preflightRooms.push(body.room_id);
        if (body.room_id === "room-1") throw conflictError("room_overlap", "Room 1 is already booked");
        return { status: "available" };
      }
      if (path === "/api/v1/sessions" && init?.method === "POST") return { id: "session-1" };
      return [];
    });

    const dialog = await openCreate(user);
    await user.selectOptions(within(dialog).getByRole("combobox", { name: "Room" }), "room-1");
    await fillTime(user, dialog);
    await waitFor(() => expect(within(dialog).getByText("Room already booked")).toBeInTheDocument());
    expect(within(dialog).getByText("Try a different room or time slot")).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: /Blocked/i })).toBeDisabled();

    await user.selectOptions(within(dialog).getByRole("combobox", { name: "Room" }), "room-2");
    await waitFor(() => expect(within(dialog).getByText("Available")).toBeInTheDocument());
    expect(within(dialog).queryByText("Try a different room or time slot")).not.toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: /^Create$/i }));
    await waitFor(() => expect(sessionPosts()).toHaveLength(1));
    expect(preflightRooms).toEqual(["room-1", "room-2"]);
    expect(JSON.parse(String(sessionPosts()[0][1]?.body))).toEqual(expect.objectContaining({ room_id: "room-2" }));
  });

  it("shows guidance when the selected course has no assigned teachers", async () => {
    const user = userEvent.setup();
    mockApiJson.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path.startsWith("/api/v1/sessions?start=")) return [];
      if (path === "/api/v1/scheduling/preflight" && init?.method === "POST") {
        throw conflictError("course_has_no_assigned_teachers", "No teachers are assigned to this course");
      }
      return [];
    });

    const dialog = await openCreate(user);
    await fillTime(user, dialog);
    await waitFor(() => expect(within(dialog).getByText("course has no assigned teachers")).toBeInTheDocument());
    expect(within(dialog).getByText("Configure teacher assignments for this course before scheduling")).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: /Blocked/i })).toBeDisabled();
    expect(sessionPosts()).toHaveLength(0);
  });

  it("shows guidance when the selected teacher is not assigned to the course", async () => {
    const user = userEvent.setup();
    mockApiJson.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path.startsWith("/api/v1/sessions?start=")) return [];
      if (path === "/api/v1/scheduling/preflight" && init?.method === "POST") {
        throw conflictError("teacher_not_assigned_to_course", "Teacher is not assigned to this course");
      }
      return [];
    });

    const dialog = await openCreate(user);
    await user.click(within(dialog).getByRole("combobox", { name: "Teacher" }));
    await user.click(within(dialog).getByRole("option", { name: "teacher.two" }));
    await fillTime(user, dialog);
    await waitFor(() => expect(within(dialog).getByText("teacher not assigned to course")).toBeInTheDocument());
    expect(within(dialog).getByText("Choose a teacher assigned to this course, or update the course's teacher assignments")).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: /Blocked/i })).toBeDisabled();
    expect(sessionPosts()).toHaveLength(0);
  });

  it("preserves the create form after a final-write room conflict", async () => {
    const user = userEvent.setup();
    let sessionListCalls = 0;
    mockApiJson.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path.startsWith("/api/v1/sessions?start=")) {
        sessionListCalls += 1;
        return [];
      }
      if (path === "/api/v1/scheduling/preflight" && init?.method === "POST") return { status: "available" };
      if (path === "/api/v1/sessions" && init?.method === "POST") throw new ApiRequestError("Room 1 was taken", { status: 409, code: "room_overlap" });
      return [];
    });

    const dialog = await openCreate(user);
    await user.selectOptions(within(dialog).getByRole("combobox", { name: "Room" }), "room-1");
    await fillTime(user, dialog);
    await waitFor(() => expect(within(dialog).getByText("Available")).toBeInTheDocument());
    await user.click(within(dialog).getByRole("button", { name: /^Create$/i }));

    await waitFor(() => expect(screen.getByText("room_overlap: Room 1 was taken")).toBeInTheDocument());
    expect(screen.getByRole("dialog", { name: "Create Session" })).toBeInTheDocument();
    expect(within(screen.getByRole("dialog", { name: "Create Session" })).getByRole("combobox", { name: "Room" })).toHaveValue("room-1");
    expect(screen.queryByText("Session created")).not.toBeInTheDocument();
    expect(sessionPosts()).toHaveLength(1);
    expect(sessionListCalls).toBe(0);
  });

  it("sends only one create request for rapid Save clicks", async () => {
    const user = userEvent.setup();
    let releaseCreate: (() => void) | undefined;
    const pendingCreate = new Promise<{ id: string }>((resolve) => {
      releaseCreate = () => resolve({ id: "session-1" });
    });
    mockApiJson.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path.startsWith("/api/v1/sessions?start=")) return [];
      if (path === "/api/v1/scheduling/preflight" && init?.method === "POST") return { status: "available" };
      if (path === "/api/v1/sessions" && init?.method === "POST") return pendingCreate;
      return [];
    });

    const dialog = await openCreate(user);
    await fillTime(user, dialog);
    await waitFor(() => expect(within(dialog).getByText("Available")).toBeInTheDocument());
    const createButton = within(dialog).getByRole("button", { name: /^Create$/i });
    await act(async () => {
      fireEvent.click(createButton);
      fireEvent.click(createButton);
    });

    expect(sessionPosts()).toHaveLength(1);
    releaseCreate?.();
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Create Session" })).not.toBeInTheDocument());
  });
});
