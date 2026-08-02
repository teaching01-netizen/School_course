import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
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
  teachers: [{ id: "teacher-1", username: "teacher.one", role: "Teacher" }],
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
  await user.type(within(dialog).getByLabelText(/start \(local time\)/i), "2026-08-03T10:00");
  await user.type(within(dialog).getByLabelText(/end \(local time\)/i), "2026-08-03T11:00");
}

function sessionPosts() {
  return mockApiJson.mock.calls.filter(([path, init]) => path === "/api/v1/sessions" && init?.method === "POST");
}

function conflictError(): ApiRequestError {
  const error = new ApiRequestError("Room 1 was taken", { status: 409, code: "room_overlap" });
  error.details = {
    kind: "room_overlap",
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

type PreflightResult = { status: "available" | "provisional" };

describe("Schedule create failure recovery", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    queryFixture.url = "";
    queryClient.clear();
  });

  it("does not submit during a preflight outage and retries the current values", async () => {
    const user = userEvent.setup();
    let checks = 0;
    const bodies: string[] = [];
    mockApiJson.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path.startsWith("/api/v1/sessions?start=")) return [];
      if (path === "/api/v1/scheduling/preflight" && init?.method === "POST") {
        checks += 1;
        bodies.push(String(init.body));
        if (checks === 1) throw new ApiRequestError("Availability service unavailable", { status: 503, code: "preflight_unavailable" });
        return { status: "available" };
      }
      if (path === "/api/v1/sessions" && init?.method === "POST") return { id: "session-1" };
      return [];
    });

    const dialog = await openCreate(user);
    await user.selectOptions(within(dialog).getByRole("combobox", { name: "Room" }), "room-1");
    await fillTime(user, dialog);
    await waitFor(() => expect(screen.getByTestId("preflight-error")).toHaveTextContent("Could not check the schedule"));
    expect(within(dialog).getByRole("button", { name: /Unavailable/i })).toBeDisabled();
    expect(sessionPosts()).toHaveLength(0);

    await user.click(screen.getByRole("button", { name: "Try checking availability again" }));
    await waitFor(() => expect(within(dialog).getByText("Available")).toBeInTheDocument());
    expect(bodies).toHaveLength(2);
    expect(JSON.parse(bodies[0])).toEqual(JSON.parse(bodies[1]));
    expect(sessionPosts()).toHaveLength(0);

    await user.click(within(dialog).getByRole("button", { name: /^Create$/i }));
    await waitFor(() => expect(sessionPosts()).toHaveLength(1));
  });

  it("keeps the newer room preflight result when an older response resolves late", async () => {
    const user = userEvent.setup();
    const requests: Array<{ body: string; resolve: (result: PreflightResult) => void }> = [];
    mockApiJson.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path.startsWith("/api/v1/sessions?start=")) return [];
      if (path === "/api/v1/scheduling/preflight" && init?.method === "POST") {
        return new Promise<PreflightResult>((resolve) => {
          requests.push({ body: String(init.body), resolve });
        });
      }
      return [];
    });

    const dialog = await openCreate(user);
    await user.selectOptions(within(dialog).getByRole("combobox", { name: "Room" }), "room-1");
    await fillTime(user, dialog);
    await waitFor(() => expect(requests).toHaveLength(1));
    await user.selectOptions(within(dialog).getByRole("combobox", { name: "Room" }), "room-2");
    await waitFor(() => expect(requests).toHaveLength(2));
    const first = requests[0];
    const second = requests[1];
    if (!first || !second) throw new Error("Expected two preflight requests");

    second.resolve({ status: "available" });
    await waitFor(() => expect(within(dialog).getByText("Available")).toBeInTheDocument());
    first.resolve({ status: "provisional" });
    await waitFor(() => expect(within(dialog).getByText("Available")).toBeInTheDocument());
    expect(within(dialog).queryByText("Blocked")).not.toBeInTheDocument();
    expect(JSON.parse(first.body).room_id).toBe("room-1");
    expect(JSON.parse(second.body).room_id).toBe("room-2");
  });

  it("recovers from a final-write conflict by keeping the modal and resubmitting corrected values", async () => {
    const user = userEvent.setup();
    let writes = 0;
    mockApiJson.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path.startsWith("/api/v1/sessions?start=")) return [];
      if (path === "/api/v1/scheduling/preflight" && init?.method === "POST") return { status: "available" };
      if (path === "/api/v1/sessions" && init?.method === "POST") {
        writes += 1;
        if (writes === 1) throw conflictError();
        return { id: "session-1" };
      }
      return [];
    });

    const dialog = await openCreate(user);
    await user.selectOptions(within(dialog).getByRole("combobox", { name: "Room" }), "room-1");
    await fillTime(user, dialog);
    await waitFor(() => expect(within(dialog).getByText("Available")).toBeInTheDocument());
    await user.click(within(dialog).getByRole("button", { name: /^Create$/i }));
    await waitFor(() => expect(screen.getByText("room_overlap: Room 1 was taken")).toBeInTheDocument());
    expect(screen.getByRole("dialog", { name: "Create Session" })).toBeInTheDocument();
    expect(sessionPosts()).toHaveLength(1);

    await user.selectOptions(within(dialog).getByRole("combobox", { name: "Room" }), "room-2");
    await waitFor(() => expect(within(dialog).getByText("Available")).toBeInTheDocument());
    await user.click(within(dialog).getByRole("button", { name: /^Create$/i }));
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Create Session" })).not.toBeInTheDocument());
    expect(sessionPosts()).toHaveLength(2);
    expect(JSON.parse(String(sessionPosts()[0][1]?.body)).room_id).toBe("room-1");
    expect(JSON.parse(String(sessionPosts()[1][1]?.body)).room_id).toBe("room-2");
    expect(screen.getByText("Session created")).toBeInTheDocument();
  });

  it("aborts and ignores a pending preflight when the create modal closes", async () => {
    const user = userEvent.setup();
    let resolvePreflight: ((result: PreflightResult) => void) | undefined;
    let requestSignal: AbortSignal | undefined;
    mockApiJson.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path.startsWith("/api/v1/sessions?start=")) return [];
      if (path === "/api/v1/scheduling/preflight" && init?.method === "POST") {
        requestSignal = init.signal ?? undefined;
        return new Promise<PreflightResult>((resolve) => { resolvePreflight = resolve; });
      }
      return [];
    });

    const dialog = await openCreate(user);
    await user.selectOptions(within(dialog).getByRole("combobox", { name: "Room" }), "room-1");
    await fillTime(user, dialog);
    await waitFor(() => expect(requestSignal).toBeDefined());
    await user.click(within(dialog).getByRole("button", { name: "Close dialog" }));
    expect(screen.queryByRole("dialog", { name: "Create Session" })).not.toBeInTheDocument();
    expect(requestSignal?.aborted).toBe(true);
    resolvePreflight?.({ status: "available" });
    expect(sessionPosts()).toHaveLength(0);
    expect(screen.queryByText("Available")).not.toBeInTheDocument();
  });
});
