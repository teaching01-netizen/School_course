import type { Page, Route } from "@playwright/test";

export type SchedulingRole = "Admin" | "Teacher";

export type SchedulingTeacher = {
  id: string;
  username: string;
  role: "Teacher";
};

export type SchedulingCourseTeacher = {
  id: string;
  username: string;
  is_primary: boolean;
};

export type SchedulingCourse = {
  id: string;
  version: number;
  code: string;
  name: string;
  primary_teacher_id: string | null;
  teachers: SchedulingCourseTeacher[];
};

export type SchedulingSession = {
  id: string;
  series_id?: string | null;
  course_id: string;
  room_id: string | null;
  teacher_id: string;
  start_at: string;
  end_at: string;
  version: number;
};

export type SchedulingRoom = {
  id: string;
  name: string;
  capacity: number | null;
};

export type SchedulingConflict = {
  session_id: string;
  series_id?: string | null;
  course_id: string;
  room_id: string | null;
  teacher_id: string;
  start_at: string;
  end_at: string;
};

export type SchedulingConflictDetails = {
  kind: string;
  requested: {
    start_at: string;
    end_at: string;
    course_id: string;
    room_id: string | null;
    teacher_id: string;
    series_id?: string | null;
  };
  conflicts: SchedulingConflict[] | null;
  total_conflicts?: number;
};

export type SchedulingPreflight =
  | { status: "available" | "provisional"; occurrences_planned?: number }
  | { status: "blocked"; details: SchedulingConflictDetails; message?: string }
  | { status: "error"; message?: string };

export type TeacherDashboardSession = {
  id: string;
  course_id: string;
  course_code: string;
  course_name: string;
  subject_name: string | null;
  start_at: string;
  end_at: string;
  room_name: string | null;
  absent_count: number;
  absent_students: [];
  sit_in_visitors: [];
};

export type TeacherDashboardFixture = {
  week_start: string;
  week_end: string;
  teacher: { id: string; username: string };
  sessions: TeacherDashboardSession[];
  summary: {
    total_sessions: number;
    total_absences: number;
    total_sit_ins: number;
  };
  pending_absence_requests: [];
};

export type SchedulingRouteOptions = {
  role?: SchedulingRole;
  course?: SchedulingCourse;
  teachers?: SchedulingTeacher[];
  rooms?: SchedulingRoom[];
  sessions?: SchedulingSession[];
  preflight?: SchedulingPreflight;
  seriesPreflight?: SchedulingPreflight;
  teacherDashboard?: TeacherDashboardFixture;
};

export type SchedulingFixtureController = {
  getSessions: () => SchedulingSession[];
  setSessions: (sessions: SchedulingSession[]) => void;
};

const defaultTeachers: SchedulingTeacher[] = [
  { id: "teacher-a", username: "Alice Smith", role: "Teacher" },
  { id: "teacher-b", username: "Bob Jones", role: "Teacher" },
];

const defaultRooms: SchedulingRoom[] = [
  { id: "room-1", name: "Room 101", capacity: 24 },
];

export function makeSchedulingCourse(
  overrides: Partial<SchedulingCourse> = {},
): SchedulingCourse {
  return {
    id: "course-1",
    version: 4,
    code: "MATH101",
    name: "Mathematics 101",
    primary_teacher_id: "teacher-a",
    teachers: [
      { id: "teacher-a", username: "Alice Smith", is_primary: true },
    ],
    ...overrides,
  };
}

export function makeSchedulingSession(
  overrides: Partial<SchedulingSession> & Pick<SchedulingSession, "id">,
): SchedulingSession {
  const { id, ...rest } = overrides;
  return {
    id,
    course_id: "course-1",
    room_id: "room-1",
    teacher_id: "teacher-a",
    start_at: "2026-08-10T02:00:00Z",
    end_at: "2026-08-10T03:00:00Z",
    version: 1,
    ...rest,
  };
}

export function makeSchedulingConflict(
  overrides: Partial<SchedulingConflictDetails> = {},
): SchedulingConflictDetails {
  return {
    kind: "room_overlap",
    requested: {
      start_at: "2026-08-10T02:00:00Z",
      end_at: "2026-08-10T03:00:00Z",
      course_id: "course-1",
      room_id: "room-1",
      teacher_id: "teacher-a",
    },
    conflicts: [
      {
        session_id: "conflicting-session",
        course_id: "course-2",
        room_id: "room-1",
        teacher_id: "teacher-b",
        start_at: "2026-08-10T02:00:00Z",
        end_at: "2026-08-10T03:00:00Z",
      },
    ],
    total_conflicts: 1,
    ...overrides,
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function readString(record: Record<string, unknown>, key: string): string | null {
  const value = record[key];
  return typeof value === "string" ? value : null;
}

async function fulfillJson(route: Route, body: unknown, status = 200): Promise<void> {
  await route.fulfill({
    status,
    contentType: "application/json",
    body: JSON.stringify(body),
  });
}

async function fulfillPreflight(route: Route, fixture: SchedulingPreflight): Promise<void> {
  if (fixture.status === "blocked") {
    await fulfillJson(route, {
      code: "schedule_conflict",
      message: fixture.message ?? "The requested schedule conflicts with another session.",
      details: fixture.details,
    }, 409);
    return;
  }

  if (fixture.status === "error") {
    await fulfillJson(route, {
      code: "preflight_unavailable",
      message: fixture.message ?? "Schedule preflight is unavailable.",
    }, 503);
    return;
  }

  await fulfillJson(route, fixture);
}

export async function installSchedulingRoutes(
  page: Page,
  options: SchedulingRouteOptions = {},
): Promise<SchedulingFixtureController> {
  const role = options.role ?? "Admin";
  const course = options.course ?? makeSchedulingCourse();
  const teachers = options.teachers ?? defaultTeachers;
  const rooms = options.rooms ?? defaultRooms;
  let sessions = [...(options.sessions ?? [])];
  const preflight = options.preflight ?? { status: "available" as const };
  const seriesPreflight = options.seriesPreflight ?? { status: "available" as const, occurrences_planned: 1 };
  const dashboard = options.teacherDashboard ?? {
    week_start: "2026-08-02",
    week_end: "2026-08-08",
    teacher: { id: "teacher-a", username: "Alice Smith" },
    sessions: [],
    summary: { total_sessions: 0, total_absences: 0, total_sit_ins: 0 },
    pending_absence_requests: [],
  } satisfies TeacherDashboardFixture;

  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const method = request.method();
    const { pathname } = url;

    if (method === "GET" && pathname === "/api/v1/me") {
      await fulfillJson(route, {
        id: role === "Admin" ? "admin-1" : "teacher-a",
        username: role === "Admin" ? "Admin User" : "Alice Smith",
        role,
      });
      return;
    }

    if (method === "GET" && pathname === "/api/v1/meta/time") {
      await fulfillJson(route, {
        institute_tz: "Asia/Bangkok",
        server_now: "2026-08-02T10:00:00Z",
      });
      return;
    }

    if (method === "GET" && pathname === "/api/v1/rooms") {
      await fulfillJson(route, rooms);
      return;
    }

    if (method === "GET" && pathname === "/api/v1/users" && url.searchParams.get("role") === "Teacher") {
      await fulfillJson(route, teachers);
      return;
    }

    if (method === "GET" && pathname === "/api/v1/courses") {
      await fulfillJson(route, [course]);
      return;
    }

    if (method === "GET" && pathname === `/api/v1/courses/${course.id}`) {
      await fulfillJson(route, course);
      return;
    }

    if (method === "GET" && pathname === `/api/v1/courses/${course.id}/students`) {
      await fulfillJson(route, []);
      return;
    }

    if (method === "GET" && pathname === `/api/v1/courses/${course.id}/crm-filter`) {
      await fulfillJson(route, { enabled: false, locked: false, filter: null });
      return;
    }

    if (method === "GET" && pathname === `/api/v1/courses/${course.id}/sessions`) {
      await fulfillJson(route, sessions);
      return;
    }

    if (method === "GET" && pathname === "/api/v1/sessions") {
      const ids = url.searchParams.get("ids");
      const requestedIds = ids ? new Set(ids.split(",")) : null;
      await fulfillJson(route, requestedIds ? sessions.filter((session) => requestedIds.has(session.id)) : sessions);
      return;
    }

    if (method === "GET" && pathname === "/api/v1/teacher/dashboard") {
      await fulfillJson(route, dashboard);
      return;
    }

    if (method === "POST" && pathname === "/api/v1/operations/schedule-issues/summary") {
      await fulfillJson(route, { sessions: {} });
      return;
    }

    if (method === "POST" && pathname === "/api/v1/scheduling/preflight") {
      await fulfillPreflight(route, preflight);
      return;
    }

    if (method === "POST" && pathname === "/api/v1/scheduling/preflight_series") {
      await fulfillPreflight(route, seriesPreflight);
      return;
    }

    if (method === "POST" && pathname === "/api/v1/sessions") {
      const parsed: unknown = JSON.parse(request.postData() ?? "{}");
      const body = isRecord(parsed) ? parsed : {};
      const createdSession: SchedulingSession = {
        id: `created-session-${sessions.length + 1}`,
        course_id: readString(body, "course_id") ?? course.id,
        room_id: readString(body, "room_id"),
        teacher_id: readString(body, "teacher_id") ?? teachers[0]?.id ?? "teacher-a",
        start_at: readString(body, "start_at") ?? "2026-08-10T02:00:00Z",
        end_at: readString(body, "end_at") ?? "2026-08-10T03:00:00Z",
        version: 1,
      };
      sessions = [...sessions, createdSession];
      await fulfillJson(route, { session: createdSession }, 201);
      return;
    }

    if (method === "POST" && pathname === "/api/v1/series") {
      await fulfillJson(route, { id: "created-series-1", occurrences_planned: 1 }, 201);
      return;
    }

    if (method === "POST" && pathname.endsWith("/change-preview")) {
      await fulfillJson(route, { requires_acknowledgement: false });
      return;
    }

    if (method === "PATCH" && /^\/api\/v1\/sessions\/[^/]+$/.test(pathname)) {
      const sessionId = pathname.split("/").at(-1);
      const current = sessions.find((session) => session.id === sessionId);
      await fulfillJson(route, { session: current ?? sessions[0] ?? null });
      return;
    }

    if (method === "GET" && pathname.endsWith("/attendance")) {
      await fulfillJson(route, []);
      return;
    }

    await route.fallback();
  });

  return {
    getSessions: () => [...sessions],
    setSessions: (nextSessions) => {
      sessions = [...nextSessions];
    },
  };
}
