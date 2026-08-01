import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, act, fireEvent } from "@testing-library/react";
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

function makeCourse(overrides: Record<string, unknown> = {}) {
  return {
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
    ...overrides,
  };
}

function makeTeacherUsers() {
  return [
    { id: "teacher-1", username: "Teacher One", role: "Teacher" },
    { id: "teacher-2", username: "Teacher Two", role: "Teacher" },
  ];
}

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

function baseMock() {
  mockApiJson.mockReset();
  mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
    if (path === "/api/v1/courses/course-1" && init?.method === "PATCH") {
      return Promise.resolve(makeCourse());
    }
    if (path === "/api/v1/courses/course-1") return Promise.resolve(makeCourse());
    if (path === "/api/v1/courses/course-1/crm-filter") return Promise.resolve({ enabled: false, locked: false, filter: null });
    if (path === "/api/v1/courses/course-1/students") return Promise.resolve([]);
    if (path === "/api/v1/courses/course-1/sessions") return Promise.resolve([]);
    if (path === "/api/v1/rooms") return Promise.resolve([{ id: "room-1", name: "Room 101", capacity: 20 }]);
    if (path === "/api/v1/users?role=Teacher") return Promise.resolve(makeTeacherUsers());
    if (path === "/api/v1/meta/time") return Promise.resolve({ institute_tz: "Asia/Bangkok", server_now: "2026-05-31T02:00:00Z" });
    throw new Error(`Unexpected API call: ${path}`);
  });
}

async function startEditing(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole("button", { name: "Edit" }));
}

function patchBody(): { expected_version: number; code: string; name: string; legacy_course_id: string | null; teachers: { teacher_id: string; is_primary: boolean }[] } {
  const call = mockApiJson.mock.calls.find(
    ([p, init]) => p === "/api/v1/courses/course-1" && (init as RequestInit | undefined)?.method === "PATCH",
  );
  if (!call) throw new Error("No PATCH call found");
  const body = (call[1] as RequestInit).body as string;
  return JSON.parse(body);
}

function patchCallCount(): number {
  return mockApiJson.mock.calls.filter(
    ([p, init]) => p === "/api/v1/courses/course-1" && (init as RequestInit | undefined)?.method === "PATCH",
  ).length;
}

describe("CourseDetail teacher editor", () => {
  beforeEach(() => {
    baseMock();
  });

  it("displays multiple teachers with a Primary badge", async () => {
    renderCourseDetail();

    expect(await screen.findByText("Teacher One")).toBeInTheDocument();
    expect(screen.getByText("Teacher Two")).toBeInTheDocument();
    const primaryBadges = screen.getAllByText("Primary");
    expect(primaryBadges).toHaveLength(1);
  });

  it("selects several teachers and saves them with expected_version", async () => {
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/api/v1/courses/course-1" && init?.method === "PATCH") {
        return Promise.resolve(makeCourse());
      }
      if (path === "/api/v1/courses/course-1") return Promise.resolve(makeCourse({ teachers: [{ id: "teacher-1", username: "Teacher One", is_primary: true }] }));
      if (path === "/api/v1/courses/course-1/crm-filter") return Promise.resolve({ enabled: false, locked: false, filter: null });
      if (path === "/api/v1/courses/course-1/students") return Promise.resolve([]);
      if (path === "/api/v1/courses/course-1/sessions") return Promise.resolve([]);
      if (path === "/api/v1/rooms") return Promise.resolve([{ id: "room-1", name: "Room 101", capacity: 20 }]);
      if (path === "/api/v1/users?role=Teacher") return Promise.resolve(makeTeacherUsers());
      if (path === "/api/v1/meta/time") return Promise.resolve({ institute_tz: "Asia/Bangkok", server_now: "2026-05-31T02:00:00Z" });
      throw new Error(`Unexpected API call: ${path}`);
    });

    const user = userEvent.setup();
    renderCourseDetail();
    await startEditing(user);

    await user.click(screen.getByRole("combobox"));
    await user.click(await screen.findByRole("option", { name: "Teacher Two" }));

    await user.click(screen.getByRole("button", { name: "Save" }));

    await screen.findByText("Course updated");
    const body = patchBody();
    expect(body.expected_version).toBe(3);
    expect(body.teachers).toEqual([
      { teacher_id: "teacher-1", is_primary: true },
      { teacher_id: "teacher-2", is_primary: false },
    ]);
    expect(screen.getByRole("button", { name: "Edit" })).toBeInTheDocument();
  });

  it("changes the primary teacher", async () => {
    const user = userEvent.setup();
    renderCourseDetail();
    await startEditing(user);

    await user.click(screen.getByRole("radio", { name: /Teacher Two/ }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await screen.findByText("Course updated");
    expect(patchBody().teachers).toEqual([
      { teacher_id: "teacher-1", is_primary: false },
      { teacher_id: "teacher-2", is_primary: true },
    ]);
  });

  it("removes the primary teacher while retaining other teachers", async () => {
    const user = userEvent.setup();
    renderCourseDetail();
    await startEditing(user);

    await user.click(screen.getByRole("button", { name: "Remove Teacher One" }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await screen.findByText("Course updated");
    expect(patchBody().teachers).toEqual([{ teacher_id: "teacher-2", is_primary: false }]);
  });

  it("chooses no primary teacher", async () => {
    const user = userEvent.setup();
    renderCourseDetail();
    await startEditing(user);

    await user.click(screen.getByRole("radio", { name: /No primary teacher/ }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await screen.findByText("Course updated");
    expect(patchBody().teachers).toEqual([
      { teacher_id: "teacher-1", is_primary: false },
      { teacher_id: "teacher-2", is_primary: false },
    ]);
  });

  it("reloads the latest course on stale_edit and does not retry", async () => {
    const latest = makeCourse({ version: 4, name: "Math Latest" });
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/api/v1/courses/course-1" && init?.method === "PATCH") {
        const err = new ApiRequestError("course was updated by another user", { code: "stale_edit", status: 409 });
        err.details = { current: latest };
        return Promise.reject(err);
      }
      if (path === "/api/v1/courses/course-1") return Promise.resolve(makeCourse());
      if (path === "/api/v1/courses/course-1/crm-filter") return Promise.resolve({ enabled: false, locked: false, filter: null });
      if (path === "/api/v1/courses/course-1/students") return Promise.resolve([]);
      if (path === "/api/v1/courses/course-1/sessions") return Promise.resolve([]);
      if (path === "/api/v1/rooms") return Promise.resolve([{ id: "room-1", name: "Room 101", capacity: 20 }]);
      if (path === "/api/v1/users?role=Teacher") return Promise.resolve(makeTeacherUsers());
      if (path === "/api/v1/meta/time") return Promise.resolve({ institute_tz: "Asia/Bangkok", server_now: "2026-05-31T02:00:00Z" });
      throw new Error(`Unexpected API call: ${path}`);
    });

    const user = userEvent.setup();
    renderCourseDetail();
    await startEditing(user);

    await user.click(screen.getByRole("button", { name: "Save" }));

    await screen.findByText("Another user changed this course. The latest version has been loaded.");
    expect(patchCallCount()).toBe(1);
    // Edit form stays open; cancelling reveals the reloaded course.
    expect(screen.getByRole("button", { name: "Save" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(await screen.findByText("Math Latest")).toBeInTheDocument();
  });

  it("re-seeds the edit form from the reloaded course after stale_edit", async () => {
    const latest = makeCourse({
      version: 4,
      name: "Math Latest",
      primary_teacher_id: "teacher-2",
      teachers: [
        { id: "teacher-1", username: "Teacher One", is_primary: false },
        { id: "teacher-2", username: "Teacher Two", is_primary: true },
      ],
    });
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/api/v1/courses/course-1" && init?.method === "PATCH") {
        const err = new ApiRequestError("course was updated by another user", { code: "stale_edit", status: 409 });
        err.details = { current: latest };
        return Promise.reject(err);
      }
      if (path === "/api/v1/courses/course-1") return Promise.resolve(makeCourse());
      if (path === "/api/v1/courses/course-1/crm-filter") return Promise.resolve({ enabled: false, locked: false, filter: null });
      if (path === "/api/v1/courses/course-1/students") return Promise.resolve([]);
      if (path === "/api/v1/courses/course-1/sessions") return Promise.resolve([]);
      if (path === "/api/v1/rooms") return Promise.resolve([{ id: "room-1", name: "Room 101", capacity: 20 }]);
      if (path === "/api/v1/users?role=Teacher") return Promise.resolve(makeTeacherUsers());
      if (path === "/api/v1/meta/time") return Promise.resolve({ institute_tz: "Asia/Bangkok", server_now: "2026-05-31T02:00:00Z" });
      throw new Error(`Unexpected API call: ${path}`);
    });

    const user = userEvent.setup();
    renderCourseDetail();
    await startEditing(user);

    // Stale form state before the conflict.
    await user.click(screen.getByRole("radio", { name: /Teacher Two/ }));

    await user.click(screen.getByRole("button", { name: "Save" }));

    await screen.findByText("Another user changed this course. The latest version has been loaded.");
    // Name input reflects the reloaded course.
    expect(screen.getByDisplayValue("Math Latest")).toBeInTheDocument();
    // Teacher set (incl. primary) matches the reloaded course: Teacher Two is now primary.
    expect(screen.getByRole("radio", { name: /Teacher Two/ })).toBeChecked();
    expect(screen.getByRole("radio", { name: /Teacher One/ })).not.toBeChecked();
  });

  it("shows a friendly message when a teacher is in use", async () => {
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/api/v1/courses/course-1" && init?.method === "PATCH") {
        const err = new ApiRequestError("teacher has future sessions", { code: "teacher_in_use", status: 409 });
        err.details = {
          teacher_id: "teacher-2",
          teacher_name: "Teacher Two",
          future_session_count: 8,
          earliest_session_start_at: "2026-08-05T09:00:00Z",
          session_ids: ["sess-1", "sess-2"],
          series_ids: [],
        };
        return Promise.reject(err);
      }
      if (path === "/api/v1/courses/course-1") return Promise.resolve(makeCourse());
      if (path === "/api/v1/courses/course-1/crm-filter") return Promise.resolve({ enabled: false, locked: false, filter: null });
      if (path === "/api/v1/courses/course-1/students") return Promise.resolve([]);
      if (path === "/api/v1/courses/course-1/sessions") return Promise.resolve([]);
      if (path === "/api/v1/rooms") return Promise.resolve([{ id: "room-1", name: "Room 101", capacity: 20 }]);
      if (path === "/api/v1/users?role=Teacher") return Promise.resolve(makeTeacherUsers());
      if (path === "/api/v1/meta/time") return Promise.resolve({ institute_tz: "Asia/Bangkok", server_now: "2026-05-31T02:00:00Z" });
      throw new Error(`Unexpected API call: ${path}`);
    });

    const user = userEvent.setup();
    renderCourseDetail();
    await startEditing(user);

    await user.click(screen.getByRole("button", { name: "Remove Teacher Two" }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await screen.findByText(/Teacher Two cannot be removed\. They are assigned to 8 future sessions\. Earliest affected session: 5 Aug 2026, 16:00\. Review or reassign those sessions before removing this teacher\./);
    // Edit form stays open so the user can review.
    expect(screen.getByRole("button", { name: "Save" })).toBeInTheDocument();
  });

  it("keeps the edit form open on other API validation errors", async () => {
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/api/v1/courses/course-1" && init?.method === "PATCH") {
        return Promise.reject(new ApiRequestError("invalid teacher", { code: "invalid_teacher", status: 400 }));
      }
      if (path === "/api/v1/courses/course-1") return Promise.resolve(makeCourse());
      if (path === "/api/v1/courses/course-1/crm-filter") return Promise.resolve({ enabled: false, locked: false, filter: null });
      if (path === "/api/v1/courses/course-1/students") return Promise.resolve([]);
      if (path === "/api/v1/courses/course-1/sessions") return Promise.resolve([]);
      if (path === "/api/v1/rooms") return Promise.resolve([{ id: "room-1", name: "Room 101", capacity: 20 }]);
      if (path === "/api/v1/users?role=Teacher") return Promise.resolve(makeTeacherUsers());
      if (path === "/api/v1/meta/time") return Promise.resolve({ institute_tz: "Asia/Bangkok", server_now: "2026-05-31T02:00:00Z" });
      throw new Error(`Unexpected API call: ${path}`);
    });

    const user = userEvent.setup();
    renderCourseDetail();
    await startEditing(user);

    const nameInput = screen.getByDisplayValue("Math");
    await user.clear(nameInput);
    await user.type(nameInput, "Algebra");

    await user.click(screen.getByRole("button", { name: "Save" }));

    await screen.findByText("invalid teacher");
    expect(screen.getByRole("button", { name: "Save" })).toBeInTheDocument();
    expect(screen.getByDisplayValue("Algebra")).toBeInTheDocument();
  });

  it("prevents duplicate submissions while saving", async () => {
    let resolvePatch: (value: unknown) => void = () => {};
    const patchPromise = new Promise((resolve) => { resolvePatch = resolve; });
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/api/v1/courses/course-1" && init?.method === "PATCH") return patchPromise;
      if (path === "/api/v1/courses/course-1") return Promise.resolve(makeCourse());
      if (path === "/api/v1/courses/course-1/crm-filter") return Promise.resolve({ enabled: false, locked: false, filter: null });
      if (path === "/api/v1/courses/course-1/students") return Promise.resolve([]);
      if (path === "/api/v1/courses/course-1/sessions") return Promise.resolve([]);
      if (path === "/api/v1/rooms") return Promise.resolve([{ id: "room-1", name: "Room 101", capacity: 20 }]);
      if (path === "/api/v1/users?role=Teacher") return Promise.resolve(makeTeacherUsers());
      if (path === "/api/v1/meta/time") return Promise.resolve({ institute_tz: "Asia/Bangkok", server_now: "2026-05-31T02:00:00Z" });
      throw new Error(`Unexpected API call: ${path}`);
    });

    const user = userEvent.setup();
    renderCourseDetail();
    await startEditing(user);

    const saveButton = screen.getByRole("button", { name: "Save" });
    await user.click(saveButton);
    await waitFor(() => expect(saveButton).toBeDisabled());

    // A second click while saving must not fire another PATCH.
    fireEvent.click(saveButton);
    expect(patchCallCount()).toBe(1);

    await act(async () => { resolvePatch(makeCourse()); });
    await screen.findByText("Course updated");
    expect(screen.getByRole("button", { name: "Edit" })).toBeInTheDocument();
  });
});
