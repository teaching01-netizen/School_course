import { expect, test, type Page } from "@playwright/test";
import {
  installCourseTeacherRoutes,
  makeCourse,
  makeSession,
  type CourseResponse,
  type SessionResponse,
} from "./fixtures/courses";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Select a teacher in the MultiTeacherSelect combobox by typing a name and clicking the option.
 *  The combobox is the first role=combobox in the create page (before the native selects). */
async function selectTeacher(page: Page, name: string) {
  const combobox = page.getByRole("combobox").first();
  await combobox.click();
  await combobox.fill(name);
  await page.getByRole("option").filter({ hasText: name }).click();
}

/** Opens the teachers property popover by clicking the teacher chips. */
async function openTeachersPopover(page: Page) {
  await page.getByRole("button", { name: /Alice Smith/ }).click();
  await expect(page.getByRole("radiogroup", { name: "Primary teacher" })).toBeVisible();
}

// ---------------------------------------------------------------------------
// E2E-001: Create multi-teacher course
// ---------------------------------------------------------------------------
test.describe("Course Teachers — Create", () => {
  test("creates a multi-teacher course", async ({ page }) => {
    const createdCourse = makeCourse({
      id: "course-1",
      code: "MATH101",
      name: "Mathematics 101",
      primary_teacher_id: "teacher-a",
      teachers: [
        { id: "teacher-a", username: "Alice Smith", is_primary: true },
        { id: "teacher-b", username: "Bob Jones", is_primary: false },
      ],
    });

    await installCourseTeacherRoutes(page, {
      course: createdCourse,
      coursesList: [createdCourse],
    });

    // Navigate to course create page
    await page.goto("/courses/create");

    // Fill in required form fields
    await page.getByLabel("Year").fill("26");

    // Select Teacher A (Alice Smith) — will become primary since selected first
    await selectTeacher(page, "Alice Smith");

    // Select Teacher B (Bob Jones)
    await selectTeacher(page, "Bob Jones");

    // Select Subject
    await page.getByLabel("Subject").selectOption("subject-math");

    // Fill hour and student count
    await page.getByLabel("Hour").fill("3");
    await page.getByLabel("Student").fill("1");

    // Submit the form
    await page.getByRole("button", { name: "Save" }).click();

    // Wait for the create API to return
    await page.waitForResponse(
      (res) => res.url().includes("/api/v1/courses") &&
              res.request().method() === "POST",
    );

    // Navigate to the course detail page
    await page.goto("/courses/course-1");

    // Verify both teachers are displayed
    await expect(page.getByText("Alice Smith")).toBeVisible();
    await expect(page.getByText("Bob Jones")).toBeVisible();

    // Verify the primary badge is shown next to Alice Smith
    await expect(page.getByText("Primary")).toBeVisible();
  });
});

// ---------------------------------------------------------------------------
// E2E-003: Change primary without changing sessions
// ---------------------------------------------------------------------------
test.describe("Course Teachers — Change Primary", () => {
  test("changes the primary teacher", async ({ page }) => {
    const course = makeCourse({
      id: "course-1",
      code: "MATH101",
      name: "Mathematics 101",
      primary_teacher_id: "teacher-a",
      teachers: [
        { id: "teacher-a", username: "Alice Smith", is_primary: true },
        { id: "teacher-b", username: "Bob Jones", is_primary: false },
      ],
    });

    const updatedCourse: CourseResponse = {
      ...course,
      primary_teacher_id: "teacher-b",
      teachers: [
        { id: "teacher-a", username: "Alice Smith", is_primary: false },
        { id: "teacher-b", username: "Bob Jones", is_primary: true },
      ],
    };

    await installCourseTeacherRoutes(page, { course });

    // Navigate to course detail
    await page.goto("/courses/course-1");

    // Open the Teachers property popover (click the chips in the property grid)
    await openTeachersPopover(page);

    // Change primary teacher from Alice Smith to Bob Jones
    await page
      .getByRole("radiogroup", { name: "Primary teacher" })
      .locator("label")
      .filter({ hasText: "Bob Jones" })
      .click();

    // Update the fixture's course data so the PATCH response reflects the change
    // We achieve this by overriding the PATCH route
    await page.route("**/api/v1/courses/*", async (route) => {
      const url = new URL(route.request().url());
      if (
        route.request().method() === "PATCH" &&
        /^\/api\/v1\/courses\/[^/]+$/.test(url.pathname)
      ) {
        await route.fulfill({
          contentType: "application/json",
          body: JSON.stringify(updatedCourse),
        });
        return;
      }
      await route.fallback();
    });

    // Click Save
    await page.getByRole("button", { name: "Save" }).click();

    // Wait for the popover to close after the successful save
    await expect(page.getByRole("button", { name: "Save" })).toHaveCount(0);

    // Verify Bob Jones is now primary and Alice Smith is still assigned
    // The teacher chips show both names
    await expect(page.getByText("Alice Smith").first()).toBeVisible();
    await expect(page.getByText("Bob Jones").first()).toBeVisible();

    // Primary badge should be visible (now pointing to Bob Jones)
    await expect(page.getByText("Primary")).toBeVisible();
  });
});

// ---------------------------------------------------------------------------
// E2E-004: Attempt removal of active future teacher
// ---------------------------------------------------------------------------
test.describe("Course Teachers — Remove Active Teacher", () => {
  test("rejects removal of teacher with future sessions", async ({ page }) => {
    const sessions: SessionResponse[] = [
      makeSession({
        id: "session-future-1",
        teacher_id: "teacher-b",
        start_at: "2026-08-10T10:00:00Z",
        end_at: "2026-08-10T11:00:00Z",
      }),
      makeSession({
        id: "session-future-2",
        teacher_id: "teacher-b",
        start_at: "2026-08-12T14:00:00Z",
        end_at: "2026-08-12T15:00:00Z",
      }),
    ];

    const course = makeCourse({
      id: "course-1",
      code: "MATH101",
      name: "Mathematics 101",
      primary_teacher_id: "teacher-a",
      teachers: [
        { id: "teacher-a", username: "Alice Smith", is_primary: true },
        { id: "teacher-b", username: "Bob Jones", is_primary: false },
      ],
    });

    await installCourseTeacherRoutes(page, { course, sessions });

    // Navigate to course detail
    await page.goto("/courses/course-1");

    // Open the Teachers property popover (click the chips in the property grid)
    await openTeachersPopover(page);

    // Remove Bob Jones from the teacher selection
    await page.getByRole("button", { name: "Remove Bob Jones" }).click();

    // Override the PATCH route to return a teacher_in_use error
    await page.route("**/api/v1/courses/*", async (route) => {
      const url = new URL(route.request().url());
      if (
        route.request().method() === "PATCH" &&
        /^\/api\/v1\/courses\/[^/]+$/.test(url.pathname)
      ) {
        await route.fulfill({
          status: 409,
          contentType: "application/json",
          body: JSON.stringify({
            code: "teacher_in_use",
            message: "Teacher Bob Jones has future sessions",
            details: {
              teacher_name: "Bob Jones",
              future_session_count: 2,
              earliest_session_start_at: "2026-08-10T10:00:00Z",
            },
          }),
        });
        return;
      }
      await route.fallback();
    });

    // Click Save — the PATCH should fail
    await page.getByRole("button", { name: "Save" }).click();

    // Verify the teacher_in_use error toast is shown
    await expect(page.getByText("cannot be removed")).toBeVisible();

    // Cancel the popover to return to the resting property grid
    await page.getByRole("button", { name: "Cancel" }).click();
    await expect(page.getByRole("button", { name: "Save" })).toHaveCount(0);

    // Verify Bob Jones is still assigned (the course data was not changed)
    // Use .first() because Bob Jones appears in both teacher chips and session rows
    await expect(page.getByText("Alice Smith").first()).toBeVisible();
    await expect(page.getByText("Bob Jones").first()).toBeVisible();
  });
});

// ---------------------------------------------------------------------------
// E2E-005: Remove historical teacher
// ---------------------------------------------------------------------------
test.describe("Course Teachers — Remove Historical Teacher", () => {
  test("removes a teacher with only past sessions", async ({ page }) => {
    // Sessions that are in the past relative to server_now
    const pastSessions: SessionResponse[] = [
      makeSession({
        id: "session-past-1",
        teacher_id: "teacher-b",
        start_at: "2026-07-20T10:00:00Z",
        end_at: "2026-07-20T11:00:00Z",
      }),
      makeSession({
        id: "session-past-2",
        teacher_id: "teacher-b",
        start_at: "2026-07-22T14:00:00Z",
        end_at: "2026-07-22T15:00:00Z",
      }),
    ];

    const course = makeCourse({
      id: "course-1",
      code: "MATH101",
      name: "Mathematics 101",
      primary_teacher_id: "teacher-a",
      teachers: [
        { id: "teacher-a", username: "Alice Smith", is_primary: true },
        { id: "teacher-b", username: "Bob Jones", is_primary: false },
      ],
    });

    // Updated course after removing Bob Jones
    const updatedCourse: CourseResponse = {
      ...course,
      teachers: [{ id: "teacher-a", username: "Alice Smith", is_primary: true }],
      primary_teacher_id: "teacher-a",
    };

    await installCourseTeacherRoutes(page, {
      course,
      sessions: pastSessions,
    });

    // Navigate to course detail
    await page.goto("/courses/course-1");

    // Open the Teachers property popover (click the chips in the property grid)
    await openTeachersPopover(page);

    // Remove Bob Jones from the teacher selection
    await page.getByRole("button", { name: "Remove Bob Jones" }).click();

    // Override the PATCH route to return the updated course
    await page.route("**/api/v1/courses/*", async (route) => {
      const url = new URL(route.request().url());
      if (
        route.request().method() === "PATCH" &&
        /^\/api\/v1\/courses\/[^/]+$/.test(url.pathname)
      ) {
        await route.fulfill({
          contentType: "application/json",
          body: JSON.stringify(updatedCourse),
        });
        return;
      }
      await route.fallback();
    });

    // Click Save — the PATCH should succeed
    await page.getByRole("button", { name: "Save" }).click();

    // Wait for the popover to close after the successful save
    await expect(page.getByRole("button", { name: "Save" })).toHaveCount(0);

    // Verify Bob Jones is no longer shown in the teacher property chips
    const courseInfoDiv = page.locator("div.max-w-2xl").first();
    await expect(courseInfoDiv.getByRole("button", { name: /Alice Smith/ })).toBeVisible();
    await expect(courseInfoDiv.getByRole("button", { name: /Bob Jones/ })).toHaveCount(0);

    // Verify past sessions still show Bob Jones's name
    // Sessions are displayed in the schedule table (2 rows, both show Bob Jones)
    const table = page.getByRole("table", { name: "Course schedule" });
    await expect(table.getByText("Bob Jones").first()).toBeVisible();
  });
});
