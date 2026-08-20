import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import AssignRooms, { computeBusyRooms } from "../AssignRooms";
import { renderWithProviders } from "./helpers";
import { queryClient } from "@/query/cache";
import type { Session } from "@/features/scheduling/types";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

vi.mock("../../hooks/useInstituteMeta", () => ({
  default: () => ({ serverNow: null, instituteTZ: "Asia/Bangkok", loaded: true }),
}));

const FIXTURE_SESSIONS: Session[] = [
  {
    id: "s1",
    course_id: "c1",
    room_id: null,
    teacher_id: "t1",
    start_at: "2026-08-19T09:00:00+07:00",
    end_at: "2026-08-19T10:30:00+07:00",
    version: 3,
  },
  {
    id: "s2",
    course_id: "c1",
    room_id: "r2",
    teacher_id: "t1",
    start_at: "2026-08-19T13:00:00+07:00",
    end_at: "2026-08-19T14:00:00+07:00",
    version: 5,
  },
];

const MOCK_COURSES = [{ id: "c1", code: "ENG101", name: "English Foundation", subject_name: "English" }];

function mockLookups() {
  mockApiJson.mockImplementation((url: string) => {
    if (url.startsWith("/api/v1/sessions?")) return Promise.resolve(FIXTURE_SESSIONS);
    if (url === "/api/v1/courses") return Promise.resolve(MOCK_COURSES);
    if (url === "/api/v1/rooms") {
      return Promise.resolve([
        { id: "r1", name: "Room 101", capacity: 20 },
        { id: "r2", name: "Room 102", capacity: null },
      ]);
    }
    if (url.startsWith("/api/v1/users")) {
      return Promise.resolve([{ id: "t1", username: "t1", full_name: "Teacher One", role: "Teacher" }]);
    }
    throw new Error(`Unmocked API call: ${url}`);
  });
}

function renderPage() {
  return renderWithProviders(
    <MemoryRouter>
      <AssignRooms />
    </MemoryRouter>,
  );
}

describe("AssignRooms page", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    queryClient.clear();
  });

  it("shows the session count and unassigned summary", async () => {
    mockLookups();
    renderPage();

    await waitFor(() => {
      expect(screen.getByText(/2 sessions/i)).toBeInTheDocument();
    });
    expect(screen.getByText(/1 unassigned/i)).toBeInTheDocument();
  });

  it("renders each session with time, course, teacher and room control", async () => {
    mockLookups();
    renderPage();

    await waitFor(() => {
      expect(screen.getAllByText("English")).toHaveLength(2);
    });
    expect(screen.getByText("09:00–10:30")).toBeInTheDocument();
    expect(screen.getByText("13:00–14:00")).toBeInTheDocument();
    expect(screen.getAllByText("Teacher One")).toHaveLength(2);
    expect(screen.getByRole("button", { name: /assign room/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /room 102/i })).toBeInTheDocument();
    expect(screen.queryByText("ENG101")).not.toBeInTheDocument();
    expect(screen.queryByText("English Foundation")).not.toBeInTheDocument();
  });

  it("marks unassigned sessions with an Unassigned chip", async () => {
    mockLookups();
    renderPage();

    await waitFor(() => {
      expect(screen.getAllByText("Unassigned")).toHaveLength(1);
    });
  });

  it("shows an empty state when the day has no sessions", async () => {
    mockApiJson.mockImplementation((url: string) => {
      if (url.startsWith("/api/v1/sessions?")) return Promise.resolve([]);
      if (url === "/api/v1/courses" || url === "/api/v1/rooms" || url.startsWith("/api/v1/users")) {
        return Promise.resolve([]);
      }
      throw new Error(`Unmocked API call: ${url}`);
    });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText(/no sessions/i)).toBeInTheDocument();
    });
  });

  it("assigns a room through the dropdown and persists via bulk-update", async () => {
    mockLookups();
    mockApiJson.mockImplementation((url: string) => {
      if (url === "/api/v1/sessions/bulk-update") {
        return Promise.resolve({
          batch_id: "b1",
          results: [
            {
              id: "s1",
              status: "updated",
              session: { ...FIXTURE_SESSIONS[0], room_id: "r1", version: 4 },
            },
          ],
        });
      }
      if (url.startsWith("/api/v1/sessions?")) return Promise.resolve(FIXTURE_SESSIONS);
      if (url === "/api/v1/courses") {
        return Promise.resolve(MOCK_COURSES);
      }
      if (url === "/api/v1/rooms") {
        return Promise.resolve([
          { id: "r1", name: "Room 101", capacity: 20 },
          { id: "r2", name: "Room 102", capacity: null },
        ]);
      }
      if (url.startsWith("/api/v1/users")) {
        return Promise.resolve([{ id: "t1", username: "t1", full_name: "Teacher One", role: "Teacher" }]);
      }
      throw new Error(`Unmocked API call: ${url}`);
    });

    const user = userEvent.setup();
    renderPage();

    await waitFor(() => {
      expect(screen.getByRole("button", { name: /assign room/i })).toBeInTheDocument();
    });
    await user.click(screen.getByRole("button", { name: /assign room/i }));
    await user.click(screen.getByRole("checkbox", { name: /room 101/i }));

    const bulkCall = mockApiJson.mock.calls.find(
      (call) => call[0] === "/api/v1/sessions/bulk-update",
    );
    expect(bulkCall).toBeDefined();
    expect(JSON.parse(String(bulkCall![1]!.body)).updates[0]).toEqual({
      id: "s1",
      expected_version: 3,
      room_id: "r1",
    });

    await waitFor(() => {
      expect(screen.getByRole("button", { name: /room 101/i })).toBeInTheDocument();
    });
    await waitFor(() => {
      expect(screen.getByText(/0 unassigned/i)).toBeInTheDocument();
    });
  });

  it("reverts and refetches when the server reports a stale edit", async () => {
    let sessionsCalls = 0;
    mockApiJson.mockImplementation((url: string) => {
      if (url.startsWith("/api/v1/sessions?")) {
        sessionsCalls += 1;
        return Promise.resolve(FIXTURE_SESSIONS);
      }
      if (url === "/api/v1/sessions/bulk-update") {
        return Promise.resolve({
          batch_id: "b1",
          results: [{ id: "s1", status: "stale_edit", error: "Session modified elsewhere" }],
        });
      }
      if (url === "/api/v1/courses") {
        return Promise.resolve(MOCK_COURSES);
      }
      if (url === "/api/v1/rooms") {
        return Promise.resolve([
          { id: "r1", name: "Room 101", capacity: 20 },
          { id: "r2", name: "Room 102", capacity: null },
        ]);
      }
      if (url.startsWith("/api/v1/users")) {
        return Promise.resolve([{ id: "t1", username: "t1", full_name: "Teacher One", role: "Teacher" }]);
      }
      throw new Error(`Unmocked API call: ${url}`);
    });

    const user = userEvent.setup();
    renderPage();

    await waitFor(() => {
      expect(screen.getByRole("button", { name: /assign room/i })).toBeInTheDocument();
    });
    const before = sessionsCalls;
    await user.click(screen.getByRole("button", { name: /assign room/i }));
    await user.click(screen.getByRole("checkbox", { name: /room 101/i }));

    await waitFor(() => {
      expect(sessionsCalls).toBeGreaterThan(before);
    });
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /assign room/i })).toBeInTheDocument();
    });
    expect(screen.queryByRole("button", { name: /room 101/i })).not.toBeInTheDocument();
  });

  it("clears an assigned room via the close button and persists room_id null via bulk-update", async () => {
    mockApiJson.mockImplementation((url: string) => {
      if (url === "/api/v1/sessions/bulk-update") {
        return Promise.resolve({
          batch_id: "b1",
          results: [
            { id: "s2", status: "updated", session: { ...FIXTURE_SESSIONS[1], room_id: null, version: 6 } },
          ],
        });
      }
      if (url.startsWith("/api/v1/sessions?")) return Promise.resolve(FIXTURE_SESSIONS);
      if (url === "/api/v1/courses") return Promise.resolve(MOCK_COURSES);
      if (url === "/api/v1/rooms") {
        return Promise.resolve([
          { id: "r1", name: "Room 101", capacity: 20 },
          { id: "r2", name: "Room 102", capacity: null },
        ]);
      }
      if (url.startsWith("/api/v1/users")) {
        return Promise.resolve([{ id: "t1", username: "t1", full_name: "Teacher One", role: "Teacher" }]);
      }
      throw new Error(`Unmocked API call: ${url}`);
    });

    const user = userEvent.setup();
    renderPage();

    await waitFor(() => {
      expect(screen.getByRole("button", { name: /clear room/i })).toBeInTheDocument();
    });
    await user.click(screen.getByRole("button", { name: /clear room/i }));

    const bulkCall = mockApiJson.mock.calls.find((call) => call[0] === "/api/v1/sessions/bulk-update");
    expect(bulkCall).toBeDefined();
    expect(JSON.parse(String(bulkCall![1]!.body)).updates[0]).toEqual({
      id: "s2",
      expected_version: 5,
      room_id: null,
    });

    await waitFor(() => {
      expect(screen.getByText(/2 unassigned/i)).toBeInTheDocument();
    });
    await waitFor(() => {
      expect(screen.queryByRole("button", { name: /clear room/i })).not.toBeInTheDocument();
    });
    expect(screen.getAllByRole("button", { name: /assign room/i })).toHaveLength(2);
  });

  function archivedCourseMock(s3sessions: Session[], archivedFetch: () => Promise<unknown>) {
    mockApiJson.mockImplementation((url: string) => {
      if (url === "/api/v1/courses/archived-c1") return archivedFetch();
      if (url.startsWith("/api/v1/sessions?")) return Promise.resolve(s3sessions);
      if (url === "/api/v1/courses") return Promise.resolve(MOCK_COURSES);
      if (url === "/api/v1/rooms") {
        return Promise.resolve([
          { id: "r1", name: "Room 101", capacity: 20 },
          { id: "r2", name: "Room 102", capacity: null },
        ]);
      }
      if (url.startsWith("/api/v1/users")) {
        return Promise.resolve([{ id: "t1", username: "t1", full_name: "Teacher One", role: "Teacher" }]);
      }
      throw new Error(`Unmocked API call: ${url}`);
    });
  }

  const ARCHIVED_SESSION: Session = {
    id: "s3",
    course_id: "archived-c1",
    room_id: null,
    teacher_id: "t1",
    start_at: "2026-08-19T15:00:00+07:00",
    end_at: "2026-08-19T16:00:00+07:00",
    version: 1,
  };

  it("resolves archived courses via /api/v1/courses/{id} and shows the subject name, not a raw UUID", async () => {
    archivedCourseMock([...FIXTURE_SESSIONS, ARCHIVED_SESSION], () =>
      Promise.resolve({
        id: "archived-c1",
        code: "MAT201",
        name: "Mathematics 2",
        subject_name: "Math",
      }),
    );
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Math")).toBeInTheDocument();
    });
    expect(screen.queryByText("archived-c1")).not.toBeInTheDocument();
    expect(screen.queryByText("Mathematics 2")).not.toBeInTheDocument();
    expect(screen.getByText(/3 sessions/i)).toBeInTheDocument();
  });

  it("shows a graceful unknown label instead of a raw UUID when a course cannot be resolved", async () => {
    archivedCourseMock([...FIXTURE_SESSIONS, ARCHIVED_SESSION], () =>
      Promise.reject(new Error("404 course not found")),
    );
    renderPage();

    await waitFor(() => {
      expect(screen.getAllByText("Unknown course")).toHaveLength(1);
    });
    expect(screen.queryByText("archived-c1")).not.toBeInTheDocument();
  });
});

describe("computeBusyRooms", () => {
  const zone = "Asia/Bangkok";
  const at = (id: string, start: string, end: string, roomId: string | null): Session => ({
    id,
    course_id: "c1",
    room_id: roomId,
    teacher_id: "t1",
    start_at: start,
    end_at: end,
    version: 1,
  });

  it("returns an empty map when no other session overlaps", () => {
    const a = at("a", "2026-08-19T02:00:00Z", "2026-08-19T03:00:00Z", "ra");
    const b = at("b", "2026-08-19T04:00:00Z", "2026-08-19T05:00:00Z", "rb");
    expect(computeBusyRooms(a, [a, b], zone).size).toBe(0);
  });

  it("marks a room busy with the conflicting session's local times", () => {
    const a = at("a", "2026-08-19T02:00:00Z", "2026-08-19T03:00:00Z", null);
    const b = at("b", "2026-08-19T02:30:00Z", "2026-08-19T04:00:00Z", "rb");
    const busy = computeBusyRooms(a, [a, b], zone);
    expect(busy.get("rb")).toBe("Busy 09:30–11:00");
  });

  it("treats adjacent sessions as non-overlapping", () => {
    const a = at("a", "2026-08-19T02:00:00Z", "2026-08-19T03:00:00Z", null);
    const b = at("b", "2026-08-19T03:00:00Z", "2026-08-19T04:00:00Z", "rb");
    expect(computeBusyRooms(a, [a, b], zone).has("rb")).toBe(false);
  });

  it("excludes the session itself", () => {
    const a = at("a", "2026-08-19T02:00:00Z", "2026-08-19T03:00:00Z", "ra");
    const busy = computeBusyRooms(a, [a], zone);
    expect(busy.has("ra")).toBe(false);
  });

  it("ignores overlapping sessions without a room", () => {
    const a = at("a", "2026-08-19T02:00:00Z", "2026-08-19T03:00:00Z", null);
    const b = at("b", "2026-08-19T02:30:00Z", "2026-08-19T04:00:00Z", null);
    expect(computeBusyRooms(a, [a, b], zone).size).toBe(0);
  });

  it("collects multiple busy rooms from several overlapping sessions", () => {
    const a = at("a", "2026-08-19T02:00:00Z", "2026-08-19T03:00:00Z", null);
    const b = at("b", "2026-08-19T02:30:00Z", "2026-08-19T04:00:00Z", "rb");
    const c = at("c", "2026-08-19T01:30:00Z", "2026-08-19T02:30:00Z", "rc");
    const busy = computeBusyRooms(a, [a, b, c], zone);
    expect(busy.size).toBe(2);
    expect(busy.get("rb")).toBe("Busy 09:30–11:00");
    expect(busy.get("rc")).toBe("Busy 08:30–09:30");
  });
});