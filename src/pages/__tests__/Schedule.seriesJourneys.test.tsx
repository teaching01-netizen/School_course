import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import Schedule from "../Schedule";
import { ToastProvider } from "../../hooks/useToast";
import { ApiRequestError } from "@/api/client";

const mockApiJson = vi.hoisted(() => vi.fn());
const mockSessionsQuery = vi.hoisted(() => ({
  data: [
    { id: "session-past", series_id: "series-1", course_id: "course-1", room_id: "room-1", teacher_id: "teacher-1", start_at: "2026-07-27T01:00:00Z", end_at: "2026-07-27T02:00:00Z", version: 1 },
    { id: "session-future", series_id: "series-1", course_id: "course-1", room_id: "room-1", teacher_id: "teacher-1", start_at: "2026-08-03T03:00:00Z", end_at: "2026-08-03T04:00:00Z", version: 1 },
  ],
  isPending: false,
  error: null,
  refetch: vi.fn(async () => ({ data: [] })),
}));
const mockLookups = vi.hoisted(() => {
  const courses = [{ id: "course-1", code: "MATH-101", name: "Math 101" }];
  const rooms = [{ id: "room-1", name: "Room 1", capacity: 20 }, { id: "room-2", name: "Room 2", capacity: 20 }];
  const teachers = [{ id: "teacher-1", username: "teacher.one", role: "Teacher" }];
  return { courses, rooms, teachers, courseById: new Map(courses.map((item) => [item.id, item])), roomById: new Map(rooms.map((item) => [item.id, item])), teacherById: new Map(teachers.map((item) => [item.id, item])), courseOptions: courses.map((item) => ({ value: item.id, label: `${item.code} — ${item.name}` })), teacherOptions: teachers.map((item) => ({ value: item.id, label: item.username })) };
});

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

vi.mock("@/query/useOperationalQuery", () => ({
  useOperationalQuery: () => mockSessionsQuery,
}));

vi.mock("@/hooks/useInstituteMeta", () => ({
  default: () => ({ serverNow: "2026-08-02T10:00:00Z", instituteTZ: "Asia/Bangkok" }),
}));

vi.mock("@/features/scheduling/hooks/useLookups", () => ({
  default: () => mockLookups,
}));

const seriesDefinition = { id: "series-1", course_id: "course-1", room_id: "room-1", teacher_id: "teacher-1", weekdays: [1], start_local_time: "10:00", duration_minutes: 60, start_date: "2026-07-27", end_date: "2026-10-03", count: null, version: 2 };

const laterOccurrenceConflict = new ApiRequestError("Teacher overlap", { status: 409, code: "schedule_conflict" });
laterOccurrenceConflict.details = {
  kind: "teacher_overlap",
  requested: { course_id: "course-1", room_id: "room-1", teacher_id: "teacher-1", start_at: "2026-08-17T03:00:00Z", end_at: "2026-08-17T04:00:00Z" },
  conflicts: [{ session_id: "blocking-session", course_id: "course-1", room_id: "room-1", teacher_id: "teacher-1", start_at: "2026-08-17T03:00:00Z", end_at: "2026-08-17T04:00:00Z" }],
};

type ApiInstallerOptions = {
  preflightResponse?: { status: "available"; occurrences_planned?: number };
  preflightSequence?: Array<{ status: "available"; occurrences_planned?: number } | Error>;
  preflightError?: Error;
  mutationResponse?: Record<string, unknown>;
};

function installApi(options: ApiInstallerOptions = {}) {
  let preflightAttempt = 0;
  mockApiJson.mockImplementation(async (path: string, init?: RequestInit) => {
    if (path === "/api/v1/operations/schedule-issues/summary") return { sessions: {} };
    if (path === "/api/v1/scheduling/preflight_series" && init?.method === "POST") {
      const sequenceValue = options.preflightSequence?.[preflightAttempt++];
      if (sequenceValue instanceof Error) throw sequenceValue;
      if (sequenceValue) return sequenceValue;
      if (options.preflightError) throw options.preflightError;
      return options.preflightResponse ?? { status: "available", occurrences_planned: 4 };
    }
    if (path === "/api/v1/series/series-1" && init?.method === "GET") return seriesDefinition;
    if (path === "/api/v1/series" && init?.method === "POST") return options.mutationResponse ?? {};
    if (path.startsWith("/api/v1/series/series-1") && (init?.method === "PATCH" || init?.method === "POST")) {
      return options.mutationResponse ?? {};
    }
    return [];
  });
}

function renderSchedule() {
  render(
    <MemoryRouter>
      <ToastProvider>
        <Schedule />
      </ToastProvider>
    </MemoryRouter>,
  );
}

async function openCreateSeries(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: "Create Series" }));
  return screen.getByRole("dialog", { name: "Create Series" });
}

function requestBodies(path: string, method: string) {
  return mockApiJson.mock.calls
    .filter(([calledPath, init]) => calledPath === path && init?.method === method)
    .map(([, init]) => JSON.parse(String(init?.body)));
}

function lastDialog(title: string) {
  const dialogs = screen.getAllByRole("dialog", { name: title });
  const dialog = dialogs[dialogs.length - 1];
  if (!dialog) throw new Error(`Expected ${title} dialog`);
  return dialog;
}

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  vi.setSystemTime(new Date("2026-08-02T10:00:00Z"));
  mockApiJson.mockReset();
  mockSessionsQuery.refetch.mockClear();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("Schedule recurring-series journeys", () => {
  it("US-SCH-008 creates a weekday/count series with planned occurrences and the full POST body", async () => {
    installApi({
      preflightResponse: { status: "available", occurrences_planned: 3 },
      mutationResponse: { series_id: "series-new" },
    });
    const user = userEvent.setup();
    renderSchedule();
    const dialog = await openCreateSeries(user);

    await user.selectOptions(within(dialog).getByLabelText("Room"), "room-1");
    await user.click(within(dialog).getByLabelText("Mon"));
    await user.click(within(dialog).getByLabelText("Wed"));
    await user.click(within(dialog).getByLabelText("End by count (advanced)"));
    await user.clear(within(dialog).getByLabelText("Count (total occurrences)"));
    await user.type(within(dialog).getByLabelText("Count (total occurrences)"), "3");

    await waitFor(() => expect(within(dialog).getByText("Occurrences planned: 3")).toBeInTheDocument());
    expect(within(dialog).getByRole("button", { name: "Create" })).toBeEnabled();
    await user.click(within(dialog).getByRole("button", { name: "Create" }));

    await waitFor(() => expect(requestBodies("/api/v1/series", "POST")).toHaveLength(1));
    expect(requestBodies("/api/v1/series", "POST")).toContainEqual({
      course_id: "course-1",
      room_id: "room-1",
      teacher_id: "teacher-1",
      weekdays: [1, 3],
      start_local_time: "16:00",
      duration_minutes: 120,
      start_date: "2026-08-02",
      end_date: null,
      count: 3,
    });
    await waitFor(() => expect(mockSessionsQuery.refetch).toHaveBeenCalledTimes(1));
  });

  it("US-SCH-009 keeps Save disabled and avoids POST when a later occurrence is blocked", async () => {
    installApi({ preflightSequence: [{ status: "available", occurrences_planned: 5 }, laterOccurrenceConflict] });
    const user = userEvent.setup();
    renderSchedule();
    const dialog = await openCreateSeries(user);

    await user.click(within(dialog).getByLabelText("Mon"));
    await user.click(within(dialog).getByLabelText("End by count (advanced)"));
    await waitFor(() => expect(within(dialog).getByText("First blocked occurrence")).toBeInTheDocument());
    expect(within(dialog).getByRole("button", { name: /Blocked/i })).toBeDisabled();
    expect(requestBodies("/api/v1/series", "POST")).toHaveLength(0);
  });

  it("US-SCH-010 sends series_id and pivot for This & Future while keeping visible history", async () => {
    installApi();
    const user = userEvent.setup();
    renderSchedule();
    fireEvent.change(screen.getByLabelText("Start date"), { target: { value: "2026-07-27" } });
    fireEvent.change(screen.getByLabelText("End date"), { target: { value: "2026-08-03" } });
    await screen.findByText("08:00–09:00");
    await screen.findByText("10:00–11:00");
    await user.click(screen.getByRole("button", { name: "This & Future" }));

    const dialog = await screen.findByRole("dialog", { name: "Edit Series (This & Future)" });
    await waitFor(() => expect(requestBodies("/api/v1/scheduling/preflight_series", "POST")).toContainEqual(
      expect.objectContaining({ series_id: "series-1", start_date: "2026-08-03" }),
    ));
    await waitFor(() => expect(within(dialog).getByRole("button", { name: "Save" })).toBeEnabled());
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    await waitFor(() => expect(requestBodies("/api/v1/series/series-1", "PATCH")).toHaveLength(1));
    expect(requestBodies("/api/v1/series/series-1", "PATCH")).toContainEqual({
      pivot_date: "2026-08-03",
      course_id: "course-1",
      room_id: "room-1",
      teacher_id: "teacher-1",
      weekdays: [1],
      start_local_time: "10:00",
      duration_minutes: 60,
      end_date: "2026-10-03",
      count: null,
      expected_version: 2,
    });
    await waitFor(() => expect(mockSessionsQuery.refetch).toHaveBeenCalledTimes(1));
    expect(screen.getByText("08:00–09:00")).toBeInTheDocument();
  });

  it("US-SCH-011 sends the complete edited definition to the entire-series endpoint", async () => {
    installApi();
    const user = userEvent.setup();
    renderSchedule();
    await screen.findByText("10:00–11:00");
    await user.click(screen.getByRole("button", { name: "Edit Series" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit Series (Future Only)" });

    await waitFor(() => expect(within(dialog).getByRole("button", { name: "Save" })).toBeEnabled());
    await user.selectOptions(within(dialog).getByLabelText("Room"), "room-2");
    await user.clear(within(dialog).getByLabelText(/Start time \(Bangkok\)/i));
    await user.type(within(dialog).getByLabelText(/Start time \(Bangkok\)/i), "11:30");
    await user.clear(within(dialog).getByLabelText("Duration (minutes)"));
    await user.type(within(dialog).getByLabelText("Duration (minutes)"), "90");
    await user.click(within(dialog).getByLabelText("Tue"));
    fireEvent.change(within(dialog).getByLabelText("End date"), { target: { value: "2026-10-10" } });
    await waitFor(() => expect(within(dialog).getByRole("button", { name: "Save" })).toBeEnabled());
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    await waitFor(() => expect(requestBodies("/api/v1/series/series-1/entire", "PATCH")).toHaveLength(1));
    expect(requestBodies("/api/v1/series/series-1/entire", "PATCH")).toContainEqual({
      expected_version: 2,
      course_id: "course-1",
      room_id: "room-2",
      teacher_id: "teacher-1",
      weekdays: [1, 2],
      start_local_time: "11:30",
      duration_minutes: 90,
      end_date: "2026-10-10",
      count: null,
    });
  });

  it.each([
    { scope: "this_and_future", pivotDate: "2026-08-03" },
    { scope: "entire_series_future_only", pivotDate: "" },
  ])("US-SCH-012 sends the $scope cancellation scope and reloads", async ({ scope, pivotDate }) => {
    installApi();
    const user = userEvent.setup();
    renderSchedule();
    await screen.findByText("10:00–11:00");
    await user.click(screen.getByRole("button", { name: "Cancel Series" }));
    const scopeDialog = await screen.findByRole("dialog", { name: "Cancel Series" });
    await waitFor(() => expect(within(scopeDialog).getByRole("combobox")).toHaveValue("this_and_future"));
    const pivotInput = within(scopeDialog).getByDisplayValue("2026-08-03");
    if (scope === "entire_series_future_only") {
      await user.selectOptions(within(scopeDialog).getByRole("combobox"), scope);
      expect(pivotInput).toBeDisabled();
    } else {
      expect(pivotInput).toBeEnabled();
    }
    await user.click(within(scopeDialog).getByRole("button", { name: "Cancel series" }));
    const confirmDialog = lastDialog("Cancel Series");
    await user.click(within(confirmDialog).getByRole("button", { name: "Cancel Series" }));

    await waitFor(() => expect(requestBodies("/api/v1/series/series-1/cancel", "POST")).toHaveLength(1));
    expect(requestBodies("/api/v1/series/series-1/cancel", "POST")).toContainEqual({
      scope,
      pivot_date: pivotDate,
      expected_version: 2,
    });
    await waitFor(() => expect(mockSessionsQuery.refetch).toHaveBeenCalledTimes(1));
  });
});
