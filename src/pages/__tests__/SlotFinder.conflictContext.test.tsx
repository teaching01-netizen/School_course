import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import SlotFinder from "../SlotFinder";
import { ToastProvider } from "../../hooks/useToast";
import { parseConflictContext } from "../../features/scheduling/components/ConflictContextCard";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

const STUDENTS = [{ id: "st1", wcode: "W10245", full_name: "Ariya S." }];
const SUBJECTS = [{ id: "sub-math", code: "MATH", name: "Calculus" }];
const COURSES = [
  { id: "course-1", version: 1, code: "MATH-101", name: "Calculus", primary_teacher_id: "teacher-1", subject_id: "sub-math", subject_code: "MATH", subject_name: "Calculus" },
];
const ST1_COURSES = [
  { id: "course-1", code: "MATH-101", name: "Calculus", subject_code: "MATH", subject_name: "Calculus" },
];

const ROOM_OVERLAP_QUERY = [
  "kind=room_overlap",
  "course_id=course-1",
  "teacher_id=teacher-1",
  "room_id=room-1",
  "room=Room+101",
  "teacher=Teacher+One",
  "start_at=2026-06-01T02%3A00%3A00Z",
  "end_at=2026-06-01T04%3A00%3A00Z",
].join("&");

const CONFLICT_ALERT = "Conflict you are finding alternatives for";

function renderSlotFinder(query = "") {
  render(
    <MemoryRouter initialEntries={[`/slot-finder${query ? `?${query}` : ""}`]}>
      <ToastProvider>
        <Routes>
          <Route path="/slot-finder" element={<SlotFinder />} />
        </Routes>
      </ToastProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  mockApiJson.mockReset();
  mockApiJson.mockImplementation(async (path: string) => {
    // The students list endpoint always returns the paginated envelope
    // ({ items, total_count, offset, limit }), never a bare array.
    if (path === "/api/v1/students" || path.startsWith("/api/v1/students?limit=")) {
      return { items: STUDENTS, total_count: STUDENTS.length, offset: 0, limit: 200 };
    }
    if (path === "/api/v1/students/st1/courses") return ST1_COURSES;
    if (path === "/api/v1/courses") return COURSES;
    if (path === "/api/v1/subjects") return SUBJECTS;
    if (path === "/api/v1/scheduling/find-slots") return { slots: [] };
    return [];
  });
});

describe("parseConflictContext", () => {
  it("returns null when the reason metadata is missing", () => {
    expect(parseConflictContext(new URLSearchParams("course_id=course-1"))).toBeNull();
    expect(parseConflictContext(new URLSearchParams(""))).toBeNull();
  });

  it("builds a displayable conflict from the carried query params", () => {
    const ctx = parseConflictContext(new URLSearchParams(ROOM_OVERLAP_QUERY));
    expect(ctx).not.toBeNull();
    expect(ctx!.details.kind).toBe("room_overlap");
    expect(ctx!.details.requested).toMatchObject({
      course_id: "course-1",
      teacher_id: "teacher-1",
      room_id: "room-1",
      start_at: "2026-06-01T02:00:00Z",
      end_at: "2026-06-01T04:00:00Z",
    });
    expect(ctx!.roomName).toBe("Room 101");
    expect(ctx!.teacherName).toBe("Teacher One");
  });
});

describe("SlotFinder conflict context", () => {
  it("explains the conflict reason the user arrived with", async () => {
    renderSlotFinder(ROOM_OVERLAP_QUERY);

    const alert = await screen.findByRole("alert", { name: CONFLICT_ALERT });
    expect(within(alert).getByText("Finding alternatives for")).toBeInTheDocument();
    expect(within(alert).getByText("Room 101 is already booked at this time")).toBeInTheDocument();
    expect(within(alert).getByText(/^Requested ·/)).toBeInTheDocument();
    expect(within(alert).getByText("Try a different room or time slot")).toBeInTheDocument();
  });

  it("prefills the subject from the carried course and a date window around the requested slot", async () => {
    renderSlotFinder(ROOM_OVERLAP_QUERY);

    // The carried course resolves to its subject, which is what the form asks for.
    await waitFor(() => expect(screen.getByRole("combobox", { name: "Subject" })).toHaveValue("MATH — Calculus"));
    expect(screen.queryByLabelText("Course")).not.toBeInTheDocument();
    // Date prefill derives from the requested start time, which shifts with the
    // machine timezone — assert the window shape instead of an exact date.
    const start = screen.getByLabelText("Start date") as HTMLInputElement;
    const end = screen.getByLabelText("End date") as HTMLInputElement;
    expect(start.value).not.toBe("");
    expect(end.value).not.toBe("");
    expect(new Date(`${end.value}T00:00:00`).getTime() - new Date(`${start.value}T00:00:00`).getTime())
      .toBe(6 * 24 * 60 * 60 * 1000);
    // Teacher/room clashes carry no student, so the student stays free.
    expect(screen.getByRole("combobox", { name: "Student" })).toHaveValue("");
  });

  it("prefills the affected student for a student clash", async () => {
    const query = [
      "kind=student_overlap",
      "course_id=course-1",
      "teacher_id=teacher-1",
      "start_at=2026-06-01T02%3A00%3A00Z",
      "end_at=2026-06-01T04%3A00%3A00Z",
      "student_count=1",
      "student_id=st1",
      "student=Ariya+S.",
    ].join("&");
    renderSlotFinder(query);

    const alert = await screen.findByRole("alert", { name: CONFLICT_ALERT });
    expect(within(alert).getByText(/1 student would clash with another class/)).toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole("combobox", { name: "Student" })).toHaveValue("W10245 — Ariya S."));
    await waitFor(() => expect(screen.getByRole("combobox", { name: "Subject" })).toHaveValue("MATH — Calculus"));
    // The student's MATH course resolves, so the search is ready to run.
    await screen.findByText(/Searching MATH-101 · Calculus/);
    expect(screen.getByRole("button", { name: /Find Slots/i })).toBeEnabled();
  });

  it("stays calm when opened from the sidebar without conflict params", async () => {
    renderSlotFinder();
    await waitFor(() => expect(screen.getByRole("combobox", { name: "Subject" })).toBeInTheDocument());
    expect(screen.queryByRole("alert", { name: CONFLICT_ALERT })).not.toBeInTheDocument();
  });

  it("dismisses the conflict context to start a fresh search", async () => {
    renderSlotFinder(ROOM_OVERLAP_QUERY);
    const user = userEvent.setup();

    const alert = await screen.findByRole("alert", { name: CONFLICT_ALERT });
    await user.click(within(alert).getByRole("button", { name: "Dismiss conflict context" }));

    await waitFor(() => expect(screen.queryByRole("alert", { name: CONFLICT_ALERT })).not.toBeInTheDocument());
  });
});