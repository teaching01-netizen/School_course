import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page, type Request } from "@playwright/test";
import {
  installSchedulingRoutes,
  makeSchedulingConflict,
  makeSchedulingCourse,
  makeSchedulingSession,
  type SchedulingConflictDetails,
  type SchedulingSession,
  type TeacherDashboardFixture,
} from "./fixtures/scheduling";

async function expectAccessiblePage(page: Page): Promise<void> {
  const results = await new AxeBuilder({ page }).analyze();
  const blocking = results.violations.filter(
    (violation) => violation.impact === "critical" || violation.impact === "serious",
  );
  expect(blocking, blocking.map(({ id, help }) => `${id}: ${help}`).join("\n")).toEqual([]);
}

function requestBody(request: Request): Record<string, unknown> {
  const parsed: unknown = JSON.parse(request.postData() ?? "{}");
  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
    throw new Error("Expected a JSON object request body");
  }
  const record: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(parsed)) record[key] = value;
  return record;
}

function staleEditResponse(current: SchedulingSession) {
  return {
    code: "stale_edit",
    message: "The session was changed by another user.",
    details: { current },
  };
}

test.describe("Scheduling user stories", () => {
  test("standalone create saves the selected course, teacher, room, and interval", async ({ page }) => {
    const course = makeSchedulingCourse({ version: 7 });
    const fixture = await installSchedulingRoutes(page, { course, sessions: [] });

    await page.goto(`/courses/${course.id}`);
    await expect(page.getByRole("heading", { name: course.code })).toBeVisible();
    await page.getByRole("button", { name: "Add…", exact: true }).click();
    await page.getByRole("tab", { name: "One-off session" }).click();

    await page.getByLabel("Room").selectOption("room-1");
    await page.getByLabel("Start (local time)").fill("2026-08-10T09:00");
    await page.getByLabel("End (local time)").fill("2026-08-10T10:30");
    await expect(page.getByText("Available", { exact: true })).toBeVisible();

    const createRequestPromise = page.waitForRequest(
      (request) => request.url().endsWith("/api/v1/sessions") && request.method() === "POST",
    );
    await page.getByRole("button", { name: "Create session", exact: true }).click();
    const createRequest = await createRequestPromise;
    const body = requestBody(createRequest);

    expect(body).toMatchObject({
      course_id: course.id,
      room_id: "room-1",
      teacher_id: "teacher-a",
      start_at: "2026-08-10T02:00:00.000Z",
      end_at: "2026-08-10T03:30:00.000Z",
    });
    await expect(page.getByText("Session created", { exact: true })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Add to Schedule" })).toHaveCount(0);
    await expect.poll(() => fixture.getSessions()).toHaveLength(1);
    expect(fixture.getSessions()[0]?.version).toBe(1);
  });

  test("blocked standalone create recovers after the administrator changes the slot", async ({ page }) => {
    const course = makeSchedulingCourse();
    const conflict = makeSchedulingConflict();
    let preflightCalls = 0;
    await installSchedulingRoutes(page, { course, preflight: { status: "available" } });
    await page.route("**/api/v1/scheduling/preflight", async (route) => {
      preflightCalls += 1;
      if (preflightCalls === 1) {
        await route.fulfill({
          status: 409,
          contentType: "application/json",
          body: JSON.stringify({ code: "schedule_conflict", message: "Room overlap", details: conflict }),
        });
        return;
      }
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ status: "available" }),
      });
    });

    await page.goto(`/courses/${course.id}`);
    await page.getByRole("button", { name: "Add…", exact: true }).click();
    await page.getByRole("tab", { name: "One-off session" }).click();
    await page.getByLabel("Start (local time)").fill("2026-08-10T09:00");
    await page.getByLabel("End (local time)").fill("2026-08-10T10:00");

    await expect(page.getByText("Blocked", { exact: true })).toBeVisible();
    await expect(page.getByTestId("conflict-group")).toBeVisible();
    await expect(page.getByRole("button", { name: /^Blocked/ })).toBeDisabled();

    await page.getByLabel("Start (local time)").fill("2026-08-10T11:00");
    await page.getByLabel("End (local time)").fill("2026-08-10T12:00");
    await expect(page.getByText("Available", { exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Create session", exact: true })).toBeEnabled();
    await page.getByRole("button", { name: "Create session", exact: true }).click();
    await expect(page.getByText("Session created", { exact: true })).toBeVisible();
    expect(preflightCalls).toBeGreaterThan(1);
  });

  test("series conflict blocks submission without a partial session result", async ({ page }) => {
    const course = makeSchedulingCourse();
    const existingSession = makeSchedulingSession({ id: "existing-session" });
    const conflict = makeSchedulingConflict({
      requested: { ...makeSchedulingConflict().requested, series_id: "requested-series" },
    });
    let seriesPosts = 0;
    const fixture = await installSchedulingRoutes(page, {
      course,
      sessions: [existingSession],
      seriesPreflight: { status: "blocked", details: conflict },
    });
    page.on("request", (request) => {
      if (request.url().endsWith("/api/v1/series") && request.method() === "POST") seriesPosts += 1;
    });

    await page.goto(`/courses/${course.id}`);
    await page.getByRole("button", { name: "Add…", exact: true }).click();
    const weekdayLabels = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
    const startDate = await page.getByLabel("Start date").inputValue();
    const weekdayLabel = weekdayLabels[new Date(`${startDate}T00:00:00Z`).getUTCDay()];
    if (!weekdayLabel) throw new Error(`Could not derive a weekday for ${startDate}`);
    await page.getByLabel(weekdayLabel).check();
    await expect(page.getByText("Blocked", { exact: true })).toBeVisible();
    await expect(page.getByTestId("conflict-group")).toBeVisible();
    await expect(page.getByRole("button", { name: /^Blocked/ })).toBeDisabled();
    await expect(page.getByText("09:00", { exact: true })).toBeVisible();
    await expect(page.getByText("10:00", { exact: true })).toBeVisible();

    expect(seriesPosts).toBe(0);
    expect(fixture.getSessions()).toEqual([existingSession]);
    await expect(page.getByText("Series created", { exact: true })).toHaveCount(0);
  });

  test("stale session edit reloads the latest visible occurrence", async ({ page }) => {
    const course = makeSchedulingCourse();
    const initialSession = makeSchedulingSession({ id: "stale-session", version: 1 });
    const latestSession = makeSchedulingSession({
      id: initialSession.id,
      version: 2,
      start_at: "2026-08-10T11:00:00Z",
      end_at: "2026-08-10T12:00:00Z",
    });
    const fixture = await installSchedulingRoutes(page, {
      course,
      sessions: [initialSession],
    });
    await page.route(`**/api/v1/sessions/${initialSession.id}`, async (route) => {
      if (route.request().method() !== "PATCH") {
        await route.fallback();
        return;
      }
      fixture.setSessions([latestSession]);
      await route.fulfill({
        status: 409,
        contentType: "application/json",
        body: JSON.stringify(staleEditResponse(latestSession)),
      });
    });

    await page.goto(`/courses/${course.id}`);
    const row = page.locator("tbody tr").first();
    await row.getByRole("button", { name: "Edit" }).click();
    await row.locator('input[type="time"]').nth(0).fill("10:00");
    await row.locator('input[type="time"]').nth(1).fill("11:00");
    await expect(row.getByRole("button", { name: "Save", exact: true })).toBeEnabled();
    await row.getByRole("button", { name: "Save", exact: true }).click();

    await expect(page.getByText("Stale edit: reloaded latest session. Please edit again.", { exact: true })).toBeVisible();
    await expect(page.getByText("18:00", { exact: true })).toBeVisible();
    await expect(page.getByText("19:00", { exact: true })).toBeVisible();
    expect(fixture.getSessions()[0]?.version).toBe(2);
  });

  test("teacher dashboard exposes sessions without administrator scheduling controls", async ({ page }) => {
    const course = makeSchedulingCourse();
    const dashboard: TeacherDashboardFixture = {
      week_start: "2026-08-02",
      week_end: "2026-08-08",
      teacher: { id: "teacher-a", username: "Alice Smith" },
      sessions: [{
        id: "teacher-session",
        course_id: course.id,
        course_code: course.code,
        course_name: course.name,
        subject_name: null,
        start_at: "2026-08-05T02:00:00Z",
        end_at: "2026-08-05T03:00:00Z",
        room_name: "Room 101",
        absent_count: 0,
        absent_students: [],
        sit_in_visitors: [],
      }],
      summary: { total_sessions: 1, total_absences: 0, total_sit_ins: 0 },
      pending_absence_requests: [],
    };
    await installSchedulingRoutes(page, { role: "Teacher", course, teacherDashboard: dashboard });

    await page.goto("/schedule");
    await expect(page).toHaveURL(/\/teacher-dashboard$/);
    await expect(page.getByRole("heading", { name: "Dashboard" })).toBeVisible();
    await expect(page.getByText(course.name, { exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: /^Add/ })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Edit", exact: true })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Cancel", exact: true })).toHaveCount(0);
  });

  test("preflight idle, provisional, available, and blocked states remain accessible", async ({ page }) => {
    const course = makeSchedulingCourse();
    const conflict: SchedulingConflictDetails = makeSchedulingConflict();
    let status: "provisional" | "available" | "blocked" = "provisional";
    await installSchedulingRoutes(page, { course, preflight: { status: "available" } });
    await page.route("**/api/v1/scheduling/preflight", async (route) => {
      if (status === "blocked") {
        await route.fulfill({
          status: 409,
          contentType: "application/json",
          body: JSON.stringify({ code: "schedule_conflict", message: "Room overlap", details: conflict }),
        });
        return;
      }
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ status }),
      });
    });

    await page.goto(`/courses/${course.id}`);
    await page.getByRole("button", { name: "Add…", exact: true }).click();
    await page.getByRole("tab", { name: "One-off session" }).click();
    await expectAccessiblePage(page);

    await page.getByLabel("Start (local time)").fill("2026-08-10T09:00");
    await page.getByLabel("End (local time)").fill("2026-08-10T10:00");
    await expect(page.getByText("Provisional", { exact: true })).toBeVisible();
    await expect(page.getByTestId("provisional-checklist")).toBeVisible();
    await expectAccessiblePage(page);

    status = "available";
    await page.getByLabel("Room").selectOption("room-1");
    await expect(page.getByText("Available", { exact: true })).toBeVisible();
    await expectAccessiblePage(page);

    status = "blocked";
    await page.getByLabel("End (local time)").fill("2026-08-10T11:00");
    await expect(page.getByText("Blocked", { exact: true })).toBeVisible();
    await expectAccessiblePage(page);
  });
});
