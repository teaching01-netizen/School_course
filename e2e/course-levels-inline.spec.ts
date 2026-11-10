import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { expectNoHorizontalOverflow } from "./helpers/absenceAssertions";

const COURSES = [
  { id: "c1", code: "MATH-101", name: "Algebra I", subject_id: "s1", subject_code: "MATH", subject_name: "Mathematics", cycle_id: "cy1", cycle_label: "Cycle 2025-01", level: 1, root_course_group_id: "g1", root_course_group_name: "SAT Math" },
  { id: "c2", code: "MATH-201", name: "Algebra II", subject_id: "s1", subject_code: "MATH", subject_name: "Mathematics", cycle_id: "cy1", cycle_label: "Cycle 2025-01", level: 2, root_course_group_id: "g1", root_course_group_name: "SAT Math" },
  { id: "c3", code: "MATH-301", name: "Algebra III", subject_id: "s1", subject_code: "MATH", subject_name: "Mathematics", cycle_id: "cy1", cycle_label: "Cycle 2025-01", level: null, root_course_group_id: "g1", root_course_group_name: "SAT Math" },
];

const GROUPS = [{ id: "g1", name: "SAT Math", course_count: 3, sit_in_rule_id: "rule-level-ladder" }];
const RULES = [{ id: "rule-level-ladder", name: "Level Ladder", type: "level_ladder", predicate: {}, description: "Use the next eligible course level.", created_at: "", updated_at: "" }];

async function installCourseLevelRoutes(page: Page) {
  const groups: { id: string; name: string; course_count: number; sit_in_rule_id: string | null }[] = GROUPS.map((group) => ({ ...group }));
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (request.method() === "GET" && path === "/api/v1/me") {
      await route.fulfill({ contentType: "application/json", body: JSON.stringify({ id: "admin-id", username: "Admin", role: "Admin" }) });
      return;
    }
    if (request.method() === "GET" && path === "/api/v1/admin/course-levels") {
      await route.fulfill({ contentType: "application/json", body: JSON.stringify({ items: COURSES, total: COURSES.length, limit: 100, offset: 0 }) });
      return;
    }
    if (request.method() === "GET" && path === "/api/v1/admin/root-course-groups") {
      await route.fulfill({ contentType: "application/json", body: JSON.stringify(groups) });
      return;
    }
    if (request.method() === "POST" && path === "/api/v1/admin/root-course-groups") {
      const body = JSON.parse(request.postData() ?? "{}") as { name?: string };
      const created = { id: "g-mobile", name: body.name ?? "Untitled", course_count: 0, sit_in_rule_id: null };
      groups.push(created);
      await route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify(created) });
      return;
    }
    const groupMatch = path.match(/^\/api\/v1\/admin\/root-course-groups\/([^/]+)$/);
    if (groupMatch && request.method() === "PUT") {
      const body = JSON.parse(request.postData() ?? "{}") as { name?: string };
      const group = groups.find((item) => item.id === groupMatch[1]);
      if (group && body.name) group.name = body.name;
      await route.fulfill({ contentType: "application/json", body: JSON.stringify({ status: "ok" }) });
      return;
    }
    if (groupMatch && request.method() === "DELETE") {
      const index = groups.findIndex((item) => item.id === groupMatch[1]);
      if (index >= 0) groups.splice(index, 1);
      await route.fulfill({ contentType: "application/json", body: JSON.stringify({ status: "ok" }) });
      return;
    }
    if (request.method() === "GET" && path === "/api/v1/admin/sit-in-rules") {
      await route.fulfill({ contentType: "application/json", body: JSON.stringify(RULES) });
      return;
    }
    if (request.method() === "GET" && path === "/api/v1/admin/course-merge-groups") {
      await route.fulfill({ contentType: "application/json", body: "[]" });
      return;
    }
    if (request.method() === "GET" && path === "/api/v1/admin/course-link-suggestions") {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ enabled: true, mode: "discovery", confirmation_enabled: false, items: [], has_more: false, next_cursor: null, evaluated_at: "2026-01-01T00:00:00Z", detector_version: "v1", institute_timezone: "Asia/Bangkok" }),
      });
      return;
    }
    await route.fulfill({ contentType: "application/json", body: "[]" });
  });
}

async function expectManagerAccessible(page: Page) {
  const results = await new AxeBuilder({ page }).include("#main").analyze();
  const blocking = results.violations.filter((violation) => violation.impact === "critical" || violation.impact === "serious");
  expect(blocking, blocking.map(({ id, help }) => `${id}: ${help}`).join("\n")).toEqual([]);
}

test("course levels manager is inline, mobile-friendly, and accessible", async ({ page }) => {
  await installCourseLevelRoutes(page);
  await page.goto("/course-levels");

  await expect(page.getByRole("heading", { name: "Course Levels" })).toBeVisible();
  const manager = page.getByRole("region", { name: "Course level manager" });
  await expect(manager).toBeVisible();
  await expect(manager.getByRole("heading", { name: "Course level manager" })).toBeVisible();
  await expect(manager.getByRole("button", { name: "Save level for MATH-101" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Manage levels" })).toHaveCount(0);

  await expectNoHorizontalOverflow(page);
  await expectManagerAccessible(page);

  if (await page.evaluate(() => window.innerWidth < 1024)) {
    await expect(page.getByRole("combobox", { name: "Course group" })).toBeVisible();
    await expect(page.locator('aside[aria-label="Course groups"]')).toBeHidden();
    const row = page.getByRole("row").filter({ hasText: "MATH-101" });
    await expect(row.getByRole("spinbutton", { name: "Level for MATH-101" })).toBeVisible();
    await expect(row.getByRole("button", { name: "Save level for MATH-101" })).toBeVisible();

    const settings = page.getByRole("region", { name: "Course group settings" });
    await expect(settings).toBeVisible();
    await settings.getByLabel("New course group").fill("Mobile Group");
    await settings.getByRole("button", { name: "Add course group" }).click();
    await expect(manager.getByRole("heading", { name: "Mobile Group" })).toBeVisible();

    await settings.getByRole("button", { name: "Rename selected group Mobile Group" }).click();
    await settings.getByRole("textbox", { name: "Rename selected group Mobile Group" }).fill("Mobile Renamed");
    await settings.getByRole("button", { name: "Save name for selected group Mobile Group" }).click();
    await expect(manager.getByRole("heading", { name: "Mobile Renamed" })).toBeVisible();

    await settings.getByRole("button", { name: "Delete selected group Mobile Renamed" }).click();
    const dialog = page.getByRole("dialog", { name: "Delete course group?" });
    await dialog.getByRole("button", { name: "Delete group" }).click();
    await expect(manager.getByRole("heading", { name: "Unassigned" })).toBeVisible();
    await expect(page.getByRole("option", { name: /Mobile Renamed/ })).toHaveCount(0);
    await expectManagerAccessible(page);
  } else {
    await expect(manager.getByRole("columnheader", { name: "Status" })).toBeVisible();
  }
});
