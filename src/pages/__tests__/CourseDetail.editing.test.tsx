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
    { id: "teacher-3", username: "Teacher Three", role: "Teacher" },
  ];
}

function makeSubjects() {
  return [
    { id: "subject-1", code: "S-MATH", name: "Mathematics" },
    { id: "subject-2", code: "S-PHY", name: "Physics" },
  ];
}

function renderCourseDetail() {
  return render(
    <MemoryRouter initialEntries={["/courses/course-1"]}>
      <ToastProvider>
        <Routes>
          <Route path="/courses/:id" element={<CourseDetail />} />
        </Routes>
      </ToastProvider>
    </MemoryRouter>,
  );
}

/** The default route table. Per-test mocks can delegate non-PATCH paths here
 *  so lookups like the teacher users list keep resolving. */
function baseImpl(path: string, init?: RequestInit) {
  if (path === "/api/v1/courses/course-1" && init?.method === "PATCH") {
    return Promise.resolve(makeCourse());
  }
  if (path === "/api/v1/courses/course-1") return Promise.resolve(makeCourse());
  if (path === "/api/v1/courses/course-1/crm-filter") return Promise.resolve({ enabled: false, locked: false, filter: null });
  if (path === "/api/v1/courses/course-1/students") return Promise.resolve([]);
  if (path === "/api/v1/courses/course-1/sessions") return Promise.resolve([]);
  if (path === "/api/v1/courses/course-1/legacy-conflicts") return Promise.resolve({ course_id: "course-1", legacy_course_id: null, open_conflicts: [] });
  if (path === "/api/v1/rooms") return Promise.resolve([{ id: "room-1", name: "Room 101", capacity: 20 }]);
  if (path === "/api/v1/users?role=Teacher") return Promise.resolve(makeTeacherUsers());
  if (path === "/api/v1/subjects") return Promise.resolve(makeSubjects());
  if (path === "/api/v1/meta/time") return Promise.resolve({ institute_tz: "Asia/Bangkok", server_now: "2026-05-31T02:00:00Z" });
  throw new Error(`Unexpected API call: ${path}`);
}

function baseMock() {
  mockApiJson.mockReset();
  mockApiJson.mockImplementation(baseImpl);
}

/** Opens the subject picker through the page title. */
async function startTitleEdit(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole("button", { name: "Edit subject" }));
}

/** Opens the teachers property popover by clicking the chip value. */
async function startTeachersEdit(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole("button", { name: /Teacher One/ }));
}

/** Opens the course properties popover from the header icon. All property
 *  rows live inside it, so panel interactions go through here first. */
async function openProperties(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole("button", { name: "Edit course properties" }));
}

function patchBodies(): Array<Record<string, unknown>> {
  return mockApiJson.mock.calls
    .filter(
      ([p, init]) => p === "/api/v1/courses/course-1" && (init as RequestInit | undefined)?.method === "PATCH",
    )
    .map((call) => JSON.parse((call[1] as RequestInit).body as string));
}

function patchBody(): Record<string, unknown> {
  const bodies = patchBodies();
  if (bodies.length === 0) throw new Error("No PATCH call found");
  return bodies[bodies.length - 1];
}

function patchCallCount(): number {
  return patchBodies().length;
}

describe("CourseDetail property editing", () => {
  beforeEach(() => {
    baseMock();
  });

  it("shows the Teacher, Hour, Student and Type summary at the top of the page", async () => {
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/api/v1/courses/course-1" && init?.method === "PATCH") {
        return Promise.resolve(makeCourse({ hour: 120, student_count: 30, course_type: "Group" }));
      }
      return baseImpl(path, init);
    });
    renderCourseDetail();

    expect(await screen.findByText("Teacher")).toBeInTheDocument();
    expect(screen.getByText("Teacher One, Teacher Two")).toBeInTheDocument();
    expect(screen.getByText("Hour")).toBeInTheDocument();
    expect(screen.getByText("Student")).toBeInTheDocument();
    expect(screen.getByText("Type")).toBeInTheDocument();
    // Hour / Remaining / Student / Type are unset in the fixture → em dash placeholders.
    expect(screen.getAllByText("—")).toHaveLength(4);
  });

  it("computes the Remaining stat from the scheduled session durations", async () => {
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/api/v1/courses/course-1" && init?.method === "PATCH") {
        return Promise.resolve(makeCourse({ hour: 10 }));
      }
      if (path === "/api/v1/courses/course-1") return Promise.resolve(makeCourse({ hour: 10 }));
      if (path === "/api/v1/courses/course-1/sessions") {
        // Two 60-minute sessions = 120 minutes used against the 10h the user set.
        return Promise.resolve([
          { id: "s1", course_id: "course-1", room_id: null, teacher_id: "t1", start_at: "2026-05-01T09:00:00Z", end_at: "2026-05-01T10:00:00Z", version: 1 },
          { id: "s2", course_id: "course-1", room_id: null, teacher_id: "t1", start_at: "2026-05-04T09:00:00Z", end_at: "2026-05-04T10:00:00Z", version: 1 },
        ]);
      }
      if (path === "/api/v1/operations/schedule-issues/summary") {
        return Promise.resolve({ sessions: {} });
      }
      return baseImpl(path, init);
    });
    renderCourseDetail();

    const pill = await screen.findByTestId("remaining-pill");
    expect(pill).toHaveTextContent("08:00");
    expect(pill).toHaveAttribute("data-usage", "remaining");
  });

  it("displays multiple teachers with a Primary badge", async () => {
    const user = userEvent.setup();
    renderCourseDetail();
    await openProperties(user);

    expect(await screen.findByText("Teacher One")).toBeInTheDocument();
    expect(screen.getByText("Teacher Two")).toBeInTheDocument();
    const primaryBadges = screen.getAllByText("Primary");
    expect(primaryBadges).toHaveLength(1);
  });

  it("shows the subject name in the title and falls back to the course name", async () => {
    const { unmount } = renderCourseDetail();

    // No subject set → the title falls back to the course name.
    expect(await screen.findByRole("button", { name: "Edit subject" })).toHaveTextContent("Math");
    unmount();

    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/api/v1/courses/course-1") return Promise.resolve(makeCourse({ subject_name: "Mathematics" }));
      return baseImpl(path, init);
    });
    renderCourseDetail();
    expect(await screen.findByRole("button", { name: "Edit subject" })).toHaveTextContent("Mathematics");
  });

  it("edits the subject from the title", async () => {
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/api/v1/courses/course-1" && init?.method === "PATCH") {
        return Promise.resolve(makeCourse({ subject_id: "subject-2", subject_name: "Physics", version: 4 }));
      }
      return baseImpl(path, init);
    });

    const user = userEvent.setup();
    renderCourseDetail();
    await startTitleEdit(user);

    await user.type(await screen.findByRole("combobox", { name: "Search subject" }), "phys");
    await user.click(await screen.findByRole("option", { name: /Physics/ }));

    await screen.findByText("Course updated");
    const body = patchBody();
    expect(body.subject_id).toBe("subject-2");
    expect(body.expected_version).toBe(3);
    // The heading now shows the chosen subject name.
    expect(screen.getByRole("button", { name: "Edit subject" })).toHaveTextContent("Physics");
  });

  it("cancels a subject pick with Escape without saving", async () => {
    const user = userEvent.setup();
    renderCourseDetail();
    await startTitleEdit(user);

    await user.type(screen.getByRole("combobox", { name: "Search subject" }), "phys");
    await user.keyboard("{Escape}");

    expect(patchCallCount()).toBe(0);
    expect(screen.queryByRole("combobox", { name: "Search subject" })).not.toBeInTheDocument();
  });

  it("selects several teachers and saves them with expected_version", async () => {
    const user = userEvent.setup();
    renderCourseDetail();
    await openProperties(user);
    await startTeachersEdit(user);

    // Teacher Three is not on the course yet, so it is the only selectable
    // option (MultiTeacherSelect filters out already-selected teachers).
    await user.click(screen.getByRole("combobox"));
    await user.click(await screen.findByRole("option", { name: "Teacher Three" }));

    await user.click(screen.getByRole("button", { name: "Save" }));

    await screen.findByText("Course updated");
    const body = patchBody();
    expect(body.expected_version).toBe(3);
    expect(body.code).toBe("MATH-101");
    expect(body.teachers).toEqual([
      { teacher_id: "teacher-1", is_primary: true },
      { teacher_id: "teacher-2", is_primary: false },
      { teacher_id: "teacher-3", is_primary: false },
    ]);
    // Popover closes after a successful save.
    expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
  });

  it("changes the primary teacher", async () => {
    const user = userEvent.setup();
    renderCourseDetail();
    await openProperties(user);
    await startTeachersEdit(user);

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
    await openProperties(user);
    await startTeachersEdit(user);

    await user.click(screen.getByRole("button", { name: "Remove Teacher One" }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await screen.findByText("Course updated");
    expect(patchBody().teachers).toEqual([{ teacher_id: "teacher-2", is_primary: false }]);
  });

  it("chooses no primary teacher", async () => {
    const user = userEvent.setup();
    renderCourseDetail();
    await openProperties(user);
    await startTeachersEdit(user);

    await user.click(screen.getByRole("radio", { name: /No primary teacher/ }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await screen.findByText("Course updated");
    expect(patchBody().teachers).toEqual([
      { teacher_id: "teacher-1", is_primary: false },
      { teacher_id: "teacher-2", is_primary: false },
    ]);
  });

  it("edits the course name through the Name property row", async () => {
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/api/v1/courses/course-1" && init?.method === "PATCH") {
        return Promise.resolve(makeCourse({ name: "Algebra", version: 4 }));
      }
      return baseImpl(path, init);
    });

    const user = userEvent.setup();
    renderCourseDetail();
    await openProperties(user);

    await user.click(await screen.findByRole("button", { name: "Math" }));
    const input = await screen.findByRole("textbox", { name: "Name" });
    await user.clear(input);
    await user.type(input, "Algebra");
    await user.keyboard("{Enter}");

    await screen.findByText("Course updated");
    const body = patchBody();
    expect(body.name).toBe("Algebra");
    expect(body.code).toBe("MATH-101");
    expect(body.expected_version).toBe(3);
    // The row now shows the saved name.
    expect(screen.getByRole("button", { name: "Algebra" })).toBeInTheDocument();
  });

  it("cancels a name edit with Escape without saving", async () => {
    const user = userEvent.setup();
    renderCourseDetail();
    await openProperties(user);

    await user.click(await screen.findByRole("button", { name: "Math" }));
    const input = await screen.findByRole("textbox", { name: "Name" });
    await user.clear(input);
    await user.type(input, "Algebra");
    await user.keyboard("{Escape}");

    expect(patchCallCount()).toBe(0);
    expect(screen.queryByRole("textbox", { name: "Name" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Math" })).toBeInTheDocument();
  });

  it("reloads the latest course on stale_edit and does not retry", async () => {
    const latest = makeCourse({ version: 4, subject_id: "subject-2", subject_name: "Physics", name: "Math" });
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/api/v1/courses/course-1" && init?.method === "PATCH") {
        const err = new ApiRequestError("course was updated by another user", { code: "stale_edit", status: 409 });
        err.details = { current: latest };
        return Promise.reject(err);
      }
      return baseImpl(path, init);
    });

    const user = userEvent.setup();
    renderCourseDetail();
    await startTitleEdit(user);

    // Pick a subject — the save conflicts with a concurrent edit elsewhere.
    await user.click(await screen.findByRole("option", { name: /S-MATH/ }));

    await screen.findByText("Another user changed this course. The latest version has been loaded.");
    expect(patchCallCount()).toBe(1);
    // The picker stays open and reflects the reloaded course: Physics is now
    // the selected subject.
    await user.click(screen.getByRole("combobox", { name: "Search subject" }));
    const physicsOption = await screen.findByRole("option", { name: /Physics/ });
    expect(physicsOption).toHaveAttribute("aria-selected", "true");
    // Closing the picker reveals the latest subject in the title.
    await user.keyboard("{Escape}");
    expect(screen.getByRole("button", { name: "Edit subject" })).toHaveTextContent("Physics");
  });

  it("re-seeds the teacher editor from the reloaded course after stale_edit", async () => {
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
      return baseImpl(path, init);
    });

    const user = userEvent.setup();
    renderCourseDetail();
    await openProperties(user);
    await startTeachersEdit(user);

    // Stale draft state before the conflict: no primary selected, whereas
    // Teacher Two is primary in `latest`.
    await user.click(screen.getByRole("radio", { name: /No primary teacher/ }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await screen.findByText("Another user changed this course. The latest version has been loaded.");
    expect(screen.getByRole("radio", { name: /Teacher Two/ })).toBeChecked();
    expect(screen.getByRole("radio", { name: /Teacher One/ })).not.toBeChecked();
    expect(screen.getByRole("radio", { name: /No primary teacher/ })).not.toBeChecked();
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
      return baseImpl(path, init);
    });

    const user = userEvent.setup();
    renderCourseDetail();
    await openProperties(user);
    await startTeachersEdit(user);

    await user.click(screen.getByRole("button", { name: "Remove Teacher Two" }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await screen.findByText(/Teacher Two cannot be removed\. They are assigned to 8 future sessions\. Earliest affected session: 5 Aug 2026, 16:00\. Review or reassign those sessions before removing this teacher\./);
    // The editor stays open so the user can review the assignment.
    expect(screen.getByRole("button", { name: "Save" })).toBeInTheDocument();
  });

  it("echoes legacy_course_id from the PATCH response on the next edit", async () => {
    // C1 regression: the PATCH response carries the full overview shape, so
    // the course state the frontend holds after a save still has its
    // legacy_course_id, and the next edit sends it back instead of null.
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/api/v1/courses/course-1" && init?.method === "PATCH") {
        return Promise.resolve(makeCourse({ legacy_course_id: "LEG-7090", name: "Math Renamed" }));
      }
      return baseImpl(path, init);
    });

    const user = userEvent.setup();
    renderCourseDetail();
    await openProperties(user);

    // First edit through the Name property row: no legacy link yet.
    await user.click(await screen.findByRole("button", { name: "Math" }));
    const nameInput = await screen.findByRole("textbox", { name: "Name" });
    await user.clear(nameInput);
    await user.type(nameInput, "Math Renamed");
    await user.keyboard("{Enter}");
    await screen.findByText("Course updated");
    expect(patchBody().legacy_course_id).toBe(null);

    // Second edit: the PATCH response (with legacy_course_id) replaced state.
    await user.click(screen.getByRole("button", { name: "Math Renamed" }));
    const againInput = await screen.findByRole("textbox", { name: "Name" });
    await user.type(againInput, " Again");
    await user.keyboard("{Enter}");
    await waitFor(() => expect(patchCallCount()).toBe(2));
    await waitFor(() => expect(patchBody().legacy_course_id).toBe("LEG-7090"));
  });

  it("keeps the editor open on other API validation errors", async () => {
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/api/v1/courses/course-1" && init?.method === "PATCH") {
        return Promise.reject(new ApiRequestError("invalid teacher", { code: "invalid_teacher", status: 400 }));
      }
      return baseImpl(path, init);
    });

    const user = userEvent.setup();
    renderCourseDetail();
    await openProperties(user);

    await user.click(await screen.findByRole("button", { name: "Math" }));
    const input = await screen.findByRole("textbox", { name: "Name" });
    await user.clear(input);
    await user.type(input, "Algebra");
    await user.keyboard("{Enter}");

    await screen.findByText("invalid teacher");
    // Draft is preserved so the user can correct and retry.
    expect(screen.getByDisplayValue("Algebra")).toBeInTheDocument();
  });

  it("prevents duplicate submissions while saving", async () => {
    let resolvePatch: (value: unknown) => void = () => {};
    const patchPromise = new Promise((resolve) => { resolvePatch = resolve; });
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/api/v1/courses/course-1" && init?.method === "PATCH") return patchPromise;
      return baseImpl(path, init);
    });

    const user = userEvent.setup();
    renderCourseDetail();
    await openProperties(user);
    await startTeachersEdit(user);

    const saveButton = screen.getByRole("button", { name: "Save" });
    await user.click(saveButton);
    await waitFor(() => expect(saveButton).toBeDisabled());

    // A second click while saving must not fire another PATCH.
    fireEvent.click(saveButton);
    expect(patchCallCount()).toBe(1);

    await act(async () => { resolvePatch(makeCourse()); });
    await screen.findByText("Course updated");
    expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
  });

  it("edits the subject through the searchable subject popover", async () => {
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/api/v1/courses/course-1" && init?.method === "PATCH") {
        // The PATCH response carries the full overview shape, including the
        // resolved subject name.
        return Promise.resolve(makeCourse({ subject_id: "subject-2", subject_name: "Physics" }));
      }
      return baseImpl(path, init);
    });

    const user = userEvent.setup();
    renderCourseDetail();
    await openProperties(user);

    // Resting state shows the placeholder until a subject is set.
    await user.click(await screen.findByRole("button", { name: "No subject" }));

    const search = await screen.findByRole("combobox", { name: "Search subject" });
    await user.type(search, "phys");
    // Filtering hides non-matching subjects.
    expect(screen.queryByRole("option", { name: /Mathematics/ })).not.toBeInTheDocument();
    await user.click(await screen.findByRole("option", { name: /Physics/ }));

    await screen.findByText("Course updated");
    const body = patchBody();
    expect(body.subject_id).toBe("subject-2");
    // The row now shows the chosen subject.
    expect(screen.getByRole("button", { name: /Physics/ })).toBeInTheDocument();
  });

  it("edits the course type through the type popover", async () => {
    const user = userEvent.setup();
    renderCourseDetail();
    await openProperties(user);

    await user.click(await screen.findByRole("button", { name: "No type" }));
    await user.click(await screen.findByRole("option", { name: "Group" }));

    await screen.findByText("Course updated");
    expect(patchBody().course_type).toBe("Group");
  });

  it("edits year, hour and student count through number popovers", async () => {
    const user = userEvent.setup();
    renderCourseDetail();
    await openProperties(user);

    const noneButtons = await screen.findAllByRole("button", { name: "None" });
    expect(noneButtons).toHaveLength(3);

    // Year
    await user.click(noneButtons[0]);
    const yearInput = await screen.findByRole("textbox", { name: "Year" });
    await user.type(yearInput, "25");
    await user.keyboard("{Enter}");
    await screen.findByText("Course updated");
    expect(patchBody().year).toBe(25);

    // Hour
    await user.click((await screen.findAllByRole("button", { name: "None" }))[1]);
    const hourInput = await screen.findByRole("textbox", { name: "Hour" });
    await user.type(hourInput, "120");
    await user.keyboard("{Enter}");
    await waitFor(() => expect(patchBody().hour).toBe(120));

    // Students
    await user.click((await screen.findAllByRole("button", { name: "None" }))[2]);
    const studentsInput = await screen.findByRole("textbox", { name: "Students" });
    await user.type(studentsInput, "30");
    await user.keyboard("{Enter}");
    await waitFor(() => expect(patchBody().student_count).toBe(30));
  });

  it("cancels a number edit with Escape without saving", async () => {
    const user = userEvent.setup();
    renderCourseDetail();
    await openProperties(user);

    await user.click((await screen.findAllByRole("button", { name: "None" }))[0]);
    const yearInput = await screen.findByRole("textbox", { name: "Year" });
    await user.clear(yearInput);
    await user.type(yearInput, "25");
    await user.keyboard("{Escape}");

    expect(patchCallCount()).toBe(0);
    expect(screen.queryByRole("textbox", { name: "Year" })).not.toBeInTheDocument();
  });

  it("closes the teachers popover on an outside click without saving", async () => {
    const user = userEvent.setup();
    renderCourseDetail();
    await openProperties(user);
    await startTeachersEdit(user);

    await user.click(screen.getByText("Code"));

    expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
    expect(patchCallCount()).toBe(0);
    // The click landed inside the properties popover, so only the nested row
    // editor closed — the properties popover itself stays open.
    expect(screen.getByRole("button", { name: "Math" })).toBeInTheDocument();
  });

  it("closes a row editor with Escape and keeps the properties popover open", async () => {
    const user = userEvent.setup();
    renderCourseDetail();
    await openProperties(user);

    await user.click(await screen.findByRole("button", { name: "Math" }));
    const input = await screen.findByRole("textbox", { name: "Name" });
    await user.type(input, "Algebra");
    await user.keyboard("{Escape}");

    expect(patchCallCount()).toBe(0);
    expect(screen.queryByRole("textbox", { name: "Name" })).not.toBeInTheDocument();
    // The properties popover remains open with its rows intact.
    expect(screen.getByRole("button", { name: "Math" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "No type" })).toBeInTheDocument();
  });

  it("closes the properties popover with Escape when no row editor is open", async () => {
    const user = userEvent.setup();
    renderCourseDetail();
    await openProperties(user);

    expect(await screen.findByRole("button", { name: "Math" })).toBeInTheDocument();
    await user.keyboard("{Escape}");

    expect(screen.queryByRole("button", { name: "Math" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Edit course properties" })).toHaveAttribute("aria-expanded", "false");
  });

  it("deletes the course from the header overflow menu", async () => {
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/api/v1/courses/course-1" && init?.method === "DELETE") return Promise.resolve();
      return baseImpl(path, init);
    });
    const user = userEvent.setup();
    renderCourseDetail();

    await user.click(await screen.findByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete course" }));

    const confirm = await screen.findByRole("dialog");
    expect(confirm).toHaveTextContent("Delete Course");
    await user.click(screen.getByRole("button", { name: "Delete" }));

    await screen.findByText("Course deleted");
  });
});