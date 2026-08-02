import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import Schedule from "../Schedule";
import { ToastProvider } from "../../hooks/useToast";
import { ApiRequestError } from "@/api/client";
import { queryClient } from "@/query/cache";

const mockApiJson = vi.hoisted(() => vi.fn());
const queryFixture = vi.hoisted(() => ({ url: "", sessions: [] as unknown[] }));
const mockRefetch = vi.hoisted(() => vi.fn(async () => {
  if (queryFixture.url) await mockApiJson(queryFixture.url, { method: "GET" });
  return { data: queryFixture.sessions };
}));

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

vi.mock("@/query/useOperationalQuery", () => ({
  useOperationalQuery: (_queryKey: unknown, url: string | null) => {
    queryFixture.url = url ?? "";
    return { data: queryFixture.sessions, error: null, isPending: false, refetch: mockRefetch };
  },
}));

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

vi.mock("@/features/scheduling/hooks/useLookups", () => ({
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

const initialSession = {
  id: "session-1",
  series_id: null,
  course_id: "course-1",
  room_id: "room-1",
  teacher_id: "teacher-1",
  start_at: "2026-05-20T03:00:00Z",
  end_at: "2026-05-20T04:00:00Z",
  version: 7,
};

const updatedSession = {
  ...initialSession,
  room_id: "room-2",
  version: 8,
};

type PatchResponse =
  | { change_id?: string }
  | Error
  | ((attempt: number, init?: RequestInit) => { change_id?: string });

function showPastRange() {
  const dateInputs = document.querySelectorAll<HTMLInputElement>('input[type="date"]');
  fireEvent.change(dateInputs[0], { target: { value: "2026-05-20" } });
  fireEvent.change(dateInputs[1], { target: { value: "2026-05-20" } });
}

function renderSchedule() {
  render(
    <MemoryRouter>
      <ToastProvider>
        <Schedule />
      </ToastProvider>
    </MemoryRouter>,
  );
  showPastRange();
}

function patchCalls() {
  return mockApiJson.mock.calls.filter(
    ([path, init]) => path === "/api/v1/sessions/session-1" && init?.method === "PATCH",
  );
}

function installScheduleDispatch(options: {
  preview?: Record<string, unknown>;
  patch?: PatchResponse;
  latest?: typeof initialSession;
}) {
  let current = options.latest ?? initialSession;
  let patchCount = 0;
  queryFixture.sessions = [current];
  mockApiJson.mockImplementation(async (path: string, init?: RequestInit) => {
    if (path.startsWith("/api/v1/sessions?start=")) {
      queryFixture.sessions = [current];
      return queryFixture.sessions;
    }
    if (path === "/api/v1/operations/schedule-issues/summary" && init?.method === "POST") {
      return { sessions: {} };
    }
    if (path === "/api/v1/sessions/session-1/attendance" && init?.method === "GET") return [];
    if (path === "/api/v1/scheduling/preflight" && init?.method === "POST") return { status: "available" };
    if (path === "/api/v1/sessions/session-1/change-preview" && init?.method === "POST") {
      return options.preview ?? { requires_acknowledgement: false };
    }
    if (path === "/api/v1/sessions/session-1" && init?.method === "PATCH") {
      patchCount += 1;
      if (typeof options.patch === "function") {
        return options.patch(patchCount, init);
      }
      if (options.patch instanceof Error) throw options.patch;
      return options.patch ?? { change_id: "change-1" };
    }
    if (path === "/api/v1/sessions?ids=session-1" && init?.method === "GET") {
      current = updatedSession;
      queryFixture.sessions = [current];
      return queryFixture.sessions;
    }
    throw new Error(`Unexpected API call: ${init?.method ?? "GET"} ${path}`);
  });
}

beforeEach(() => {
  mockApiJson.mockReset();
  mockRefetch.mockClear();
  queryFixture.url = "";
  queryFixture.sessions = [];
  queryClient.clear();
});

describe("Schedule edit journeys", () => {
  it("cancels the edit without sending a PATCH", async () => {
    const user = userEvent.setup();
    installScheduleDispatch({});
    renderSchedule();

    await screen.findByText(/MATH-101/);
    await user.click(screen.getByRole("button", { name: /^Edit$/i }));
    const editDialog = await screen.findByRole("dialog", { name: /edit session/i });

    await user.click(within(editDialog).getByRole("button", { name: /^Cancel$/i }));

    expect(screen.queryByRole("dialog", { name: /edit session/i })).not.toBeInTheDocument();
    expect(patchCalls()).toHaveLength(0);
  });

  it("requires acknowledgement, exposes an accessible impact dialog, and sends acknowledge_impact on Enter", async () => {
    const user = userEvent.setup();
    installScheduleDispatch({
      preview: {
        requires_acknowledgement: true,
        impact_summary: { predicted_student_overlaps: 2, short_notice: true },
      },
    });
    renderSchedule();

    await screen.findByText(/MATH-101/);
    await user.click(screen.getByRole("button", { name: /^Edit$/i }));
    const editDialog = await screen.findByRole("dialog", { name: /edit session/i });
    await waitFor(() => expect(within(editDialog).getByRole("button", { name: /^Save$/i })).not.toBeDisabled());

    await user.click(within(editDialog).getByRole("button", { name: /^Save$/i }));
    const impactDialog = await screen.findByRole("dialog", { name: /review student impact/i });
    const impactHeading = within(impactDialog).getByRole("heading", { name: /review student impact/i });
    const confirm = within(impactDialog).getByRole("button", { name: /save change and review 2/i });

    expect(impactDialog).toHaveAttribute("aria-modal", "true");
    expect(impactDialog).toHaveAttribute("aria-labelledby", impactHeading.id);
    expect(confirm).toHaveAttribute("type", "button");
    expect(patchCalls()).toHaveLength(0);

    confirm.focus();
    await user.keyboard("{Enter}");

    await waitFor(() => expect(patchCalls()).toHaveLength(1));
    const body = JSON.parse(patchCalls()[0]?.[1]?.body as string);
    expect(body.expected_version).toBe(7);
    expect(body.acknowledge_impact).toBe(true);
  });

  it("lets Admin A recover from Admin B's stale update and resubmits with the refreshed version", async () => {
    const user = userEvent.setup();
    installScheduleDispatch({
      patch: (attempt: number) => {
        if (attempt === 1) throw new ApiRequestError("Stale edit", { status: 409, code: "stale_edit" });
        return { change_id: "change-2" };
      },
    });
    renderSchedule();

    await screen.findByText(/MATH-101/);
    await user.click(screen.getByRole("button", { name: /^Edit$/i }));
    let editDialog = await screen.findByRole("dialog", { name: /edit session/i });
    await waitFor(() => expect(within(editDialog).getByRole("button", { name: /^Save$/i })).not.toBeDisabled());

    await user.click(within(editDialog).getByRole("button", { name: /^Save$/i }));
    await waitFor(() => expect(screen.getByText(/Stale edit: reloaded latest session/i)).toBeInTheDocument());
    editDialog = screen.getByRole("dialog", { name: /edit session/i });
    await waitFor(() => expect(within(editDialog).getByRole("combobox", { name: /room/i })).toHaveValue("room-2"));
    await waitFor(() => expect(within(editDialog).getByRole("button", { name: /^Save$/i })).not.toBeDisabled());

    await user.click(within(editDialog).getByRole("button", { name: /^Save$/i }));
    await waitFor(() => expect(patchCalls()).toHaveLength(2));

    const firstBody = JSON.parse(patchCalls()[0]?.[1]?.body as string);
    const secondBody = JSON.parse(patchCalls()[1]?.[1]?.body as string);
    expect(firstBody.expected_version).toBe(7);
    expect(secondBody.expected_version).toBe(8);
  });
});
