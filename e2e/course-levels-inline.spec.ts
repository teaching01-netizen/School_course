import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import type { CourseLevelItem, CourseMergeGroupConfig, RootCourseGroupInfo } from "../src/utils/levels";
import { expectNoHorizontalOverflow } from "./helpers/absenceAssertions";

const COURSES = [
  { id: "c1", code: "MATH-101", name: "Algebra I", subject_id: "s1", subject_code: "MATH", subject_name: "Mathematics", cycle_id: "cy1", cycle_label: "Cycle 2025-01", level: 1, root_course_group_id: "g1", root_course_group_name: "SAT Math" },
  { id: "c2", code: "MATH-201", name: "Algebra II", subject_id: "s1", subject_code: "MATH", subject_name: "Mathematics", cycle_id: "cy1", cycle_label: "Cycle 2025-01", level: 2, root_course_group_id: "g1", root_course_group_name: "SAT Math" },
  { id: "c3", code: "MATH-301", name: "Algebra III", subject_id: "s1", subject_code: "MATH", subject_name: "Mathematics", cycle_id: "cy1", cycle_label: "Cycle 2025-01", level: null, root_course_group_id: "g1", root_course_group_name: "SAT Math" },
];

const GROUPS = [{ id: "g1", name: "SAT Math", course_count: 3, sit_in_rule_id: "rule-level-ladder" }];
const RULES = [{ id: "rule-level-ladder", name: "Level Ladder", type: "level_ladder", predicate: {}, description: "Use the next eligible course level.", created_at: "", updated_at: "" }];

async function installCourseLevelRoutes(page: Page, fixture: {
  courses?: CourseLevelItem[];
  groups?: RootCourseGroupInfo[];
  mergeGroups?: CourseMergeGroupConfig[];
} = {}) {
  const courses = (fixture.courses ?? COURSES).map((course) => ({ ...course }));
  const groups: RootCourseGroupInfo[] = (fixture.groups ?? GROUPS).map((group) => ({ ...group }));
  const assignmentState = { failNext: false };
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (request.method() === "GET" && path === "/api/v1/me") {
      await route.fulfill({ contentType: "application/json", body: JSON.stringify({ id: "admin-id", username: "Admin", role: "Admin" }) });
      return;
    }
    if (request.method() === "GET" && path === "/api/v1/admin/course-levels") {
      await route.fulfill({ json: { items: courses, total: courses.length, limit: 100, offset: 0 } });
      return;
    }
    const assignment = path.match(/^\/api\/v1\/admin\/courses\/([^/]+)\/root-course-group$/);
    if (request.method() === "PUT" && assignment) {
      if (assignmentState.failNext) {
        assignmentState.failNext = false;
        await route.fulfill({ status: 503, json: { message: "Assignment temporarily failed" } });
        return;
      }
      const course = courses.find((item) => item.id === assignment[1]);
      const group = groups.find((item) => item.id === request.postDataJSON().root_course_group_id);
      if (!course || !group) {
        await route.fulfill({ status: 400, json: { message: "Invalid course or group" } });
        return;
      }
      course.root_course_group_id = group.id;
      course.root_course_group_name = group.name;
      await route.fulfill({ json: { status: "ok" } });
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
      const body = JSON.parse(request.postData() ?? "{}") as { name?: string; sit_in_rule_id?: string | null };
      const group = groups.find((item) => item.id === groupMatch[1]);
      if (group && body.name) group.name = body.name;
      if (group && "sit_in_rule_id" in body) group.sit_in_rule_id = body.sit_in_rule_id ?? null;
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
      await route.fulfill({ json: fixture.mergeGroups ?? [] });
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
  return { failNextAssignment: () => { assignmentState.failNext = true; } };
}

const C4_COURSES: CourseLevelItem[] = [
  { ...COURSES[0], code: "26453296000", name: "SAT Math: Beginner C4" },
  { ...COURSES[2], code: "262073290700", name: "SAT Math: Beginner C4 (split)", root_course_group_id: null, root_course_group_name: null },
];
const C4_GROUP: RootCourseGroupInfo = { id: "g2", name: "SAT MATH C4/2026", course_count: 0, sit_in_rule_id: RULES[0].id };
const C4_LINK: CourseMergeGroupConfig = {
  id: "mg1", name: "SAT Math: Beginner C4", level: 1, cycle_id: "cy1", cycle_label: "Cycle 2025-01",
  sit_in_rule_id: RULES[0].id, course_codes: ["26453296000", "262073290700"],
  course_names: ["SAT Math: Beginner C4", "SAT Math: Beginner C4 (split)"], rule_source_course_code: "26453296000",
};

test("adding a continuation by either course code places both codes in the target group", async ({ page }) => {
  const routes = await installCourseLevelRoutes(page, { courses: [...C4_COURSES, COURSES[1]], groups: [GROUPS[0]], mergeGroups: [C4_LINK] });
  await page.goto("/course-levels");

  const manager = page.getByRole("region", { name: "Course level manager" });
  await expect(manager).toBeVisible();
  const settings = page.getByRole("region", { name: "Course group settings" });
  await settings.getByLabel("New course group").fill("SAT MATH C4/2026");
  await settings.getByRole("button", { name: "Add course group" }).click();
  await expect(manager.getByRole("heading", { name: "SAT MATH C4/2026" })).toBeVisible();

  const picker = manager.getByRole("combobox", { name: "Add course to SAT MATH C4/2026" });
  await picker.fill("262073290700");
  await page.getByRole("option", { name: /262073290700/ }).click();
  await expect(page.getByRole("alert")).toContainText("Assign a sit-in rule before adding a levelled course to this group");
  const table = manager.getByRole("region", { name: "Course levels table" });
  await expect(table.getByText("26453296000", { exact: true })).toHaveCount(0);

  await page.getByRole("combobox", { name: "Sit-in rule for SAT MATH C4/2026" }).selectOption(RULES[0].id);
  routes.failNextAssignment();
  await picker.fill("262073290700");
  await page.getByRole("option", { name: /262073290700/ }).click();
  await expect(page.getByRole("alert")).toContainText("Assignment temporarily failed");
  await expect(table.getByText("26453296000", { exact: true })).toHaveCount(0);

  await picker.fill("26453296000");
  await expect(page.getByRole("option", { name: /26453296000/ })).toBeVisible();
  await picker.fill("262073290700");
  await page.getByRole("option", { name: /262073290700/ }).click();

  await expect(table.getByText("26453296000", { exact: true })).toBeVisible();
  await expect(table.getByText("262073290700", { exact: true })).toBeVisible();

  await page.reload();
  if (await page.evaluate(() => window.innerWidth < 1024)) {
    await page.getByRole("combobox", { name: "Course group" }).selectOption("g-mobile");
  } else {
    await page.getByRole("button", { name: "SAT MATH C4/2026", exact: true }).click();
  }
  await expect(table.getByText("26453296000", { exact: true })).toBeVisible();
  await expect(table.getByText("262073290700", { exact: true })).toBeVisible();
  await expect(table.getByText("Rule source (linked)", { exact: true })).toBeVisible();
  await expect(table.getByText("Follows 26453296000", { exact: true })).toBeVisible();
  await expect(page.getByRole("region", { name: "SAT MATH C4/2026", exact: true }).getByText("Ready", { exact: true })).toBeVisible();
});

test("adding a schedule-only merge member places only the selected code in the target group", async ({ page }) => {
  await installCourseLevelRoutes(page, {
    courses: [...C4_COURSES, COURSES[1]],
    groups: [C4_GROUP, GROUPS[0]],
    mergeGroups: [{ ...C4_LINK, rule_source_course_code: null }],
  });
  await page.goto("/course-levels");

  await page.getByRole("combobox", { name: "Add course to SAT MATH C4/2026" }).fill("262073290700");
  await page.getByRole("option", { name: /262073290700/ }).click();
  await expect(page.getByRole("region", { name: "Course levels table" }).getByText("262073290700", { exact: true })).toBeVisible();

  await page.reload();
  const table = page.getByRole("region", { name: "Course levels table" });
  await expect(table.getByText("262073290700", { exact: true })).toBeVisible();
  await expect(table.getByText("26453296000", { exact: true })).toHaveCount(0);
  await expect(table.getByText("Merged course", { exact: true })).toBeVisible();
  await expect(page.getByRole("region", { name: "SAT MATH C4/2026", exact: true }).getByText("Ready", { exact: true })).toBeVisible();
});

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
