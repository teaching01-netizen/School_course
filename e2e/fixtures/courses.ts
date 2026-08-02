import type { Page, Route } from "@playwright/test";

// ---------------------------------------------------------------------------
// Types matching the frontend's expected API response shapes
// ---------------------------------------------------------------------------
export type CourseTeacherResponse = {
  id: string;
  username: string;
  is_primary: boolean;
};

export type CourseResponse = {
  id: string;
  version: number;
  code: string;
  name: string;
  primary_teacher_id: string | null;
  teachers: CourseTeacherResponse[];
};

export type EditableTeacher = {
  teacher_id: string;
  is_primary: boolean;
};

export type UserResponse = {
  id: string;
  username: string;
  role: string;
};

export type SessionResponse = {
  id: string;
  course_id: string;
  start_at: string;
  end_at: string;
  teacher_id: string;
  room_id: string | null;
  version: number;
  status: string;
};

// ---------------------------------------------------------------------------
// Default data
// ---------------------------------------------------------------------------
const defaultTeachers: UserResponse[] = [
  { id: "teacher-a", username: "Alice Smith", role: "Teacher" },
  { id: "teacher-b", username: "Bob Jones", role: "Teacher" },
];

const defaultSubjects = [
  { id: "subject-math", code: "MATH", name: "Mathematics" },
  { id: "subject-eng", code: "ENG", name: "English" },
];

const defaultRooms = [
  { id: "room-1", name: "Room 101" },
];

// ---------------------------------------------------------------------------
// Factory helpers
// ---------------------------------------------------------------------------
export function makeTeacherUsers(
  overrides?: Partial<UserResponse>[],
): UserResponse[] {
  if (overrides && overrides.length > 0) {
    return overrides.map((o, i) => ({
      ...(defaultTeachers[i] ?? {
        id: `teacher-${i}`,
        username: `Teacher ${i}`,
        role: "Teacher",
      }),
      ...o,
    }));
  }
  return [...defaultTeachers];
}

export function makeCourse(
  overrides?: Partial<CourseResponse>,
): CourseResponse {
  return {
    id: "course-1",
    version: 1,
    code: "MATH101",
    name: "Mathematics 101",
    primary_teacher_id: "teacher-a",
    teachers: [
      { id: "teacher-a", username: "Alice Smith", is_primary: true },
    ],
    ...overrides,
  };
}

export function makeSession(
  overrides: Partial<SessionResponse> & { id: string },
): SessionResponse {
  return {
    course_id: overrides.course_id ?? "course-1",
    start_at: overrides.start_at ?? "2026-08-10T10:00:00Z",
    end_at: overrides.end_at ?? "2026-08-10T11:00:00Z",
    teacher_id: overrides.teacher_id ?? "teacher-b",
    room_id: overrides.room_id ?? "room-1",
    version: overrides.version ?? 1,
    status: overrides.status ?? "active",
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Route installer options
// ---------------------------------------------------------------------------
export type InstallCourseRoutesOptions = {
  course?: CourseResponse;
  teachers?: UserResponse[];
  subjects?: { id: string; code: string; name: string }[];
  rooms?: { id: string; name: string }[];
  sessions?: SessionResponse[];
  students?: unknown[];
  coursesList?: unknown[];
};

// ---------------------------------------------------------------------------
// Route installer
// ---------------------------------------------------------------------------
export async function installCourseTeacherRoutes(
  page: Page,
  options: InstallCourseRoutesOptions = {},
) {
  const {
    course = makeCourse(),
    teachers = defaultTeachers,
    subjects = defaultSubjects,
    rooms = defaultRooms,
    sessions = [],
    students = [],
    coursesList = [],
  } = options;

  // Authenticated admin user
  await page.route("**/api/v1/me", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        id: "admin-1",
        username: "Admin User",
        role: "Admin",
      }),
    }),
  );

  // Teacher users list: use regex to match query parameter
  await page.route(/\/api\/v1\/users\?role=Teacher/, (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify(teachers),
    }),
  );

  // Subjects
  await page.route("**/api/v1/subjects", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify(subjects),
    }),
  );

  // Institute time meta
  await page.route("**/api/v1/meta/time", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        institute_tz: "Asia/Bangkok",
        server_now: "2026-08-02T10:00:00Z",
      }),
    }),
  );

  // Rooms
  await page.route("**/api/v1/rooms", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify(rooms),
    }),
  );

  // Courses routes — catch-all for /api/v1/courses and /api/v1/courses/*
  await page.route(/\/api\/v1\/courses/, (route) => {
    const url = new URL(route.request().url());
    const pathname = url.pathname;
    const method = route.request().method();

    // POST /api/v1/courses — create
    if (method === "POST" && pathname === "/api/v1/courses") {
      route.fulfill({
        contentType: "application/json",
        status: 201,
        body: JSON.stringify(course),
      });
      return;
    }

    // GET /api/v1/courses — list
    if (method === "GET" && pathname === "/api/v1/courses") {
      route.fulfill({
        contentType: "application/json",
        body: JSON.stringify(coursesList),
      });
      return;
    }

    // GET /api/v1/courses/:id — course detail
    if (method === "GET" && /^\/api\/v1\/courses\/[^/]+$/.test(pathname)) {
      route.fulfill({
        contentType: "application/json",
        body: JSON.stringify(course),
      });
      return;
    }

    // PATCH /api/v1/courses/:id — course update
    if (method === "PATCH" && /^\/api\/v1\/courses\/[^/]+$/.test(pathname)) {
      route.fulfill({
        contentType: "application/json",
        body: JSON.stringify(course),
      });
      return;
    }

    // GET /api/v1/courses/:id/sessions
    if (method === "GET" && /^\/api\/v1\/courses\/[^/]+\/sessions$/.test(pathname)) {
      route.fulfill({
        contentType: "application/json",
        body: JSON.stringify(sessions),
      });
      return;
    }

    // GET /api/v1/courses/:id/students
    if (method === "GET" && /^\/api\/v1\/courses\/[^/]+\/students$/.test(pathname)) {
      route.fulfill({
        contentType: "application/json",
        body: JSON.stringify(students),
      });
      return;
    }

    // GET /api/v1/courses/:id/crm-filter
    if (method === "GET" && /^\/api\/v1\/courses\/[^/]+\/crm-filter$/.test(pathname)) {
      route.fulfill({
        contentType: "application/json",
        status: 404,
        body: JSON.stringify({ code: "not_found", message: "CRM not configured" }),
      });
      return;
    }

    // DELETE /api/v1/courses/:id
    if (method === "DELETE" && /^\/api\/v1\/courses\/[^/]+$/.test(pathname)) {
      route.fulfill({
        contentType: "application/json",
        status: 204,
        body: "",
      });
      return;
    }

    route.fallback();
  });

  // Schedule issues summary
  await page.route("**/api/v1/operations/schedule-issues/summary", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ sessions: {} }),
    }),
  );
}
