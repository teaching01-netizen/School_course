import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import CourseDetail from "../CourseDetail";
import { ToastProvider } from "../../hooks/useToast";

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

/** Base API mock: institute date is 2026-05-31 (Sunday) in Asia/Bangkok. */
function baseMock() {
  return {
    "/api/v1/courses/course-1": { id: "course-1", code: "MATH-101", name: "Math" },
    "/api/v1/courses/course-1/crm-filter": { enabled: false, locked: false, filter: null },
    "/api/v1/courses/course-1/students": [],
    "/api/v1/rooms": [{ id: "room-1", name: "Room 101", capacity: 20 }],
    "/api/v1/users?role=Teacher": [{ id: "teacher-1", username: "Teacher One", role: "Teacher" }],
    "/api/v1/meta/time": { institute_tz: "Asia/Bangkok", server_now: "2026-05-31T02:00:00Z" },
  } as Record<string, unknown>;
}

describe("CourseDetail calendar grid", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockApiJson.mockImplementation((path: string) => {
      const base = baseMock();
      if (path.startsWith("/api/v1/sessions?")) return Promise.resolve([]);
      if (base[path] !== undefined) return Promise.resolve(base[path]);
      throw new Error(`Unexpected API call: ${path}`);
    });
  });

  it("renders Day/Week/Month controls and a Mon–Sun week grid", async () => {
    renderCourseDetail();

    await screen.findByRole("button", { name: "Add…" });

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /calendar/i }));

    // Calendar defaults to Week; Day and Month are available, and navigation is present.
    expect(screen.getByRole("button", { name: "Week" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "Day" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Month" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Previous week" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Next week" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Go to current date" })).toBeInTheDocument();

    // Week view shows one column per weekday — no per-hour time rows anymore.
    for (const day of ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"]) {
      expect(screen.getByText(day)).toBeInTheDocument();
    }
    expect(screen.queryByText("00:00")).not.toBeInTheDocument();
    expect(screen.queryByText("23:00")).not.toBeInTheDocument();
  });

  it("merges all of a day's sessions into that day's column using institute timezone", async () => {
    // 2026-05-27T02:00:00Z = 2026-05-27T09:00:00 Bangkok (Wednesday), within
    // the mocked institute week (Mon 25 May – Sun 31 May 2026).
    const session = {
      id: "sess-1",
      course_id: "course-1",
      room_id: "room-1",
      teacher_id: "teacher-1",
      start_at: "2026-05-27T02:00:00Z",
      end_at: "2026-05-27T04:00:00Z",
      version: 1,
    };

    mockApiJson.mockImplementation((path: string) => {
      const base = baseMock();
      if (path === "/api/v1/courses/course-1/sessions") return Promise.resolve([session]);
      if (path.startsWith("/api/v1/sessions?")) return Promise.resolve([]);
      if (base[path] !== undefined) return Promise.resolve(base[path]);
      throw new Error(`Unexpected API call: ${path}`);
    });

    renderCourseDetail();

    await screen.findByRole("button", { name: "Add…" });

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /calendar/i }));

    // Session card renders in Bangkok time (09:00), not UTC (02:00).
    await waitFor(() => {
      expect(screen.getByText("09:00–11:00")).toBeInTheDocument();
      expect(screen.getByText("Room 101")).toBeInTheDocument();
    });
    expect(screen.queryByText("02:00–04:00")).not.toBeInTheDocument();

    // The whole day lives in a single Wednesday cell — Monday must be empty.
    const headerCells = Array.from(document.querySelectorAll("thead tr")[0]!.children);
    const colIndex = (weekday: string) => headerCells.findIndex((c) => c.textContent?.includes(weekday));
    const bodyCells = Array.from(document.querySelectorAll("tbody tr")[0]!.children);
    expect(bodyCells[colIndex("Wed")]!.textContent).toContain("Room 101");
    expect(bodyCells[colIndex("Mon")]!.textContent).not.toContain("Room 101");
  });

  it("toggles between table and calendar view", async () => {
    renderCourseDetail();
    await screen.findByRole("button", { name: "Add…" });

    const user = userEvent.setup();

    // Default is table view
    expect(screen.getByText("Date")).toBeInTheDocument();
    expect(screen.getByText("Begin")).toBeInTheDocument();

    // Switch to calendar
    await user.click(screen.getByRole("button", { name: /calendar/i }));
    expect(screen.getByRole("button", { name: "Week" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "Previous week" })).toBeInTheDocument();

    // Switch back to table
    await user.click(screen.getByRole("button", { name: /table/i }));
    expect(screen.getByText("Date")).toBeInTheDocument();
  });

  it("shows a month grid with merged day cells and drills into day view", async () => {
    // Five sessions on Wednesday 27 May 2026 (08:00–13:00 Bangkok).
    const sessions = Array.from({ length: 5 }, (_, i) => ({
      id: `sess-${i + 1}`,
      course_id: "course-1",
      room_id: "room-1",
      teacher_id: "teacher-1",
      start_at: `2026-05-27T0${i + 1}:00:00Z`,
      end_at: `2026-05-27T0${i + 2}:00:00Z`,
      version: 1,
    }));

    mockApiJson.mockImplementation((path: string) => {
      const base = baseMock();
      if (path === "/api/v1/courses/course-1/sessions") return Promise.resolve(sessions);
      if (path.startsWith("/api/v1/sessions?")) return Promise.resolve([]);
      if (base[path] !== undefined) return Promise.resolve(base[path]);
      throw new Error(`Unexpected API call: ${path}`);
    });

    renderCourseDetail();

    await screen.findByRole("button", { name: "Add…" });

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /calendar/i }));
    await user.click(screen.getByRole("button", { name: "Month" }));

    await waitFor(() => expect(screen.getByText("May 2026")).toBeInTheDocument());

    // The 27th is one cell holding the first sessions plus a "+2 more" entry.
    const dayButton = screen.getByRole("button", { name: "Show Wednesday, 27 May 2026" });
    expect(dayButton).toBeInTheDocument();
    const dayCell = dayButton.parentElement!;
    expect(within(dayCell).getAllByRole("button", { name: /edit session/i })).toHaveLength(3);
    expect(within(dayCell).getByRole("button", { name: /show all sessions/i })).toBeInTheDocument();

    // Drilling into the day shows every session of the day in a single cell.
    await user.click(within(dayCell).getByRole("button", { name: /show all sessions/i }));
    expect(screen.getByRole("button", { name: "Day" })).toHaveAttribute("aria-pressed", "true");

    const bodyCells = Array.from(document.querySelectorAll("tbody tr")[0]!.children);
    expect(bodyCells).toHaveLength(1);
    expect(bodyCells[0]!.textContent).toContain("08:00–09:00");
    expect(bodyCells[0]!.textContent).toContain("12:00–13:00");
    expect(screen.getAllByText(/^\d{2}:\d{2}–\d{2}:\d{2}$/)).toHaveLength(5);
  });

  it("hard deletes an individual session from the course schedule after confirmation", async () => {
    const user = userEvent.setup();
    const session = {
      id: "sess-1",
      series_id: null,
      course_id: "course-1",
      room_id: "room-1",
      teacher_id: "teacher-1",
      start_at: "2026-06-01T02:00:00Z",
      end_at: "2026-06-01T04:00:00Z",
      version: 4,
    };
    let deleted = false;

    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      const base = baseMock();
      if (path === "/api/v1/courses/course-1/sessions") return Promise.resolve(deleted ? [] : [session]);
      if (path === "/api/v1/sessions/sess-1" && init?.method === "DELETE") {
        deleted = true;
        return Promise.resolve({ ok: true });
      }
      if (path.startsWith("/api/v1/sessions?")) return Promise.resolve([]);
      if (base[path] !== undefined) return Promise.resolve(base[path]);
      throw new Error(`Unexpected API call: ${path}`);
    });

    renderCourseDetail();

    await user.click(await screen.findByRole("button", { name: "Session actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete" }));
    expect(screen.getByRole("dialog", { name: /delete session/i })).toBeInTheDocument();
    expect(screen.getByText(/permanently delete this session/i)).toBeInTheDocument();

    await user.click(within(screen.getByRole("dialog", { name: /delete session/i })).getByRole("button", { name: /^delete session$/i }));

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/sessions/sess-1",
        expect.objectContaining({
          method: "DELETE",
          body: JSON.stringify({ expected_version: 4 }),
        }),
      );
    });
    await waitFor(() => expect(screen.getByText("No sessions in range")).toBeInTheDocument());
  });
});