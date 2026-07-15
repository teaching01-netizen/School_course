import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import Schedule from "../Schedule";
import { ToastProvider } from "@/hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

vi.mock("@/hooks/useInstituteMeta", () => ({
  default: () => ({ serverNow: "2026-08-01T10:00:00Z", instituteTZ: "Asia/Bangkok" }),
}));

const courses = [{ id: "course-1", code: "MATH-101", name: "Math 101" }];
const rooms = [{ id: "room-1", name: "Room 1", capacity: 20 }];
const teachers = [{ id: "teacher-1", username: "teacher.one", role: "Teacher" }];

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

const futureSession = {
  id: "session-1",
  series_id: "series-1",
  course_id: "course-1",
  room_id: "room-1",
  teacher_id: "teacher-1",
  start_at: "2026-08-03T03:00:00Z",
  end_at: "2026-08-03T04:00:00Z",
  version: 1,
};

const series = {
  id: "series-1",
  course_id: "course-1",
  room_id: "room-1",
  teacher_id: "teacher-1",
  weekdays: [1],
  start_local_time: "10:00",
  duration_minutes: 60,
  start_date: "2026-08-03",
  end_date: "2026-10-03",
  count: null,
  version: 2,
};

function renderSchedule() {
  render(
    <MemoryRouter>
      <ToastProvider>
        <Schedule />
      </ToastProvider>
    </MemoryRouter>,
  );
}

function seriesPreflightBodies(): Array<Record<string, unknown>> {
  return mockApiJson.mock.calls
    .filter(([path, init]) => path === "/api/v1/scheduling/preflight_series" && init?.method === "POST")
    .map(([, init]) => JSON.parse(init.body as string));
}

describe("Schedule series edit preflight stabilization", () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.setSystemTime(new Date("2026-08-01T10:00:00Z"));
    mockApiJson.mockReset();
    mockApiJson.mockImplementation(async (path, init) => {
      if (typeof path === "string" && path.startsWith("/api/v1/sessions?start=")) return [futureSession];
      if (path === "/api/v1/series/series-1" && init?.method === "GET") return series;
      if (path === "/api/v1/scheduling/preflight_series" && init?.method === "POST") {
        return { status: "available", occurrences_planned: 9 };
      }
      return [];
    });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it.each([
    ["This & Future", "Edit Series (This & Future)"],
    ["Edit Series", "Edit Series (Future Only)"],
  ])("includes the current series ID in the %s preflight", async (action, modalTitle) => {
    renderSchedule();
    await screen.findByText("MATH-101 — Math 101");

    fireEvent.click(screen.getByRole("button", { name: action }));
    await screen.findByText(modalTitle);

    await waitFor(() => {
      expect(seriesPreflightBodies()).toContainEqual(expect.objectContaining({ series_id: "series-1" }));
    });
  });
});
