import { expect, test, type Locator, type Page } from "@playwright/test";
import { installCourseTeacherRoutes, makeCourse } from "./fixtures/courses";

const course = makeCourse({
  name: "Mathematics 101",
  code: "MATH101",
  teachers: [
    { id: "teacher-a", username: "Alice Smith", is_primary: true },
    { id: "teacher-b", username: "Bob Jones", is_primary: false },
  ],
});

type CourseFixture = ReturnType<typeof makeCourse>;
type CycleFixture = {
  id: string;
  label: string;
  display_name?: string | null;
  start_date?: string | null;
  end_date?: string | null;
};
type RouteOptions = {
  course?: CourseFixture;
  cycles?: CycleFixture[];
  subjects?: { id: string; code: string; name: string }[];
};

async function installRoutes(page: Page, options: RouteOptions = {}) {
  const { course: activeCourse = course, cycles = [], subjects } = options;
  await page.route("**/api/v1/crm/cycles", (route) =>
    route.fulfill({ contentType: "application/json", body: JSON.stringify(cycles) }),
  );
  await installCourseTeacherRoutes(page, { course: activeCourse, subjects });
}

/** A mobile layout viewport can be wider than what is actually visible. */
async function visibleViewport(page: Page) {
  const viewport = page.viewportSize();
  const metrics = await page.evaluate(() => ({
    innerWidth: window.innerWidth,
    innerHeight: window.innerHeight,
    visualWidth: window.visualViewport?.width,
    visualHeight: window.visualViewport?.height,
  }));
  return {
    width: Math.min(viewport?.width ?? metrics.innerWidth, metrics.visualWidth ?? metrics.innerWidth),
    height: Math.min(viewport?.height ?? metrics.innerHeight, metrics.visualHeight ?? metrics.innerHeight),
  };
}

async function expectInsideViewport(locator: Locator, page: Page) {
  const box = await locator.boundingBox();
  const visible = await visibleViewport(page);
  expect(box).not.toBeNull();
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.y).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(visible.width + 1);
  expect(box!.y + box!.height).toBeLessThanOrEqual(visible.height + 1);
}

/** The viewport cap must compose with the caller's content bound, so a wide
 *  option label cannot grow past either the caller's 28rem-style bound or the
 *  visible viewport. */
async function expectWithinWidthCap(locator: Locator, page: Page, contentCap: number) {
  const visible = await visibleViewport(page);
  const expectedMax = Math.min(contentCap, visible.width - 8);
  const box = await locator.boundingBox();
  expect(box).not.toBeNull();
  expect(box!.width).toBeLessThanOrEqual(expectedMax + 1);
}

async function openProperties(page: Page): Promise<Locator> {
  const trigger = page.getByRole("button", { name: "Edit course properties" });
  // The trigger must be fully on screen and normally clickable — a partially
  // clipped header must not need a coordinate workaround.
  await expectInsideViewport(trigger, page);
  await trigger.click();
  const panel = page.getByRole("dialog", { name: "Course properties" });
  await expect(panel).toBeVisible();
  return panel;
}

async function panelScroll(panel: Locator) {
  return panel.evaluate((element) => ({
    scrollLeft: element.scrollLeft,
    scrollTop: element.scrollTop,
    overflowX: element.scrollWidth - element.clientWidth,
    overflowY: element.scrollHeight - element.clientHeight,
  }));
}

test.describe("Course properties nested editors", () => {
  test("Teachers editor stays usable without scrolling the properties panel", async ({ page }) => {
    await installRoutes(page);
    await page.goto("/courses/course-1");

    const panel = await openProperties(page);
    expect((await panelScroll(panel)).overflowX).toBeLessThanOrEqual(0);

    await panel.getByRole("button", { name: /Alice Smith/ }).click();

    const editor = page.getByRole("dialog", { name: "Edit Teachers" });
    await expect(editor).toBeVisible();
    await expect(editor.getByRole("combobox", { name: "Select teachers…" })).toBeVisible();
    await expect(editor.getByRole("button", { name: "Save" })).toBeVisible();

    // The nested panel used to be fixed-positioned inside the animated,
    // scrollable properties panel, which stretched that panel in both axes and
    // scrolled its labels out of view.
    const scroll = await panelScroll(panel);
    expect(scroll.scrollLeft).toBe(0);
    expect(scroll.overflowX).toBeLessThanOrEqual(0);

    const panelBox = await panel.boundingBox();
    const codeBox = await panel.getByText("Code", { exact: true }).boundingBox();
    expect(panelBox).not.toBeNull();
    expect(codeBox).not.toBeNull();
    expect(codeBox!.x).toBeGreaterThanOrEqual(panelBox!.x - 1);
    expect(codeBox!.x + codeBox!.width).toBeLessThanOrEqual(panelBox!.x + panelBox!.width + 1);

    await expectInsideViewport(editor, page);

    // Escape dismisses only the nested editor; the properties panel survives.
    await page.keyboard.press("Escape");
    await expect(editor).toBeHidden();
    await expect(panel).toBeVisible();
    await expect(panel.getByRole("button", { name: /Alice Smith/ })).toBeFocused();

    await page.keyboard.press("Escape");
    await expect(panel).toBeHidden();
    await expect(page.getByRole("button", { name: "Edit course properties" })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
  });

  test("every property row editor opens without horizontal panel overflow", async ({ page }) => {
    await installRoutes(page);
    await page.goto("/courses/course-1");

    const panel = await openProperties(page);
    const rows: Array<{ trigger: () => Locator; dialog: string }> = [
      { trigger: () => panel.getByRole("button", { name: "Math" }), dialog: "Edit Name" },
      { trigger: () => panel.getByRole("button", { name: "No subject" }), dialog: "Edit Subject" },
      { trigger: () => panel.getByRole("button", { name: "No type" }), dialog: "Edit Type" },
      { trigger: () => panel.getByRole("button", { name: "None" }).nth(0), dialog: "Edit Year" },
      { trigger: () => panel.getByRole("button", { name: "None" }).nth(1), dialog: "Edit Hour" },
      { trigger: () => panel.getByRole("button", { name: "None" }).nth(2), dialog: "Edit Students" },
      { trigger: () => panel.getByRole("button", { name: "No cycle" }), dialog: "Edit Cycle" },
      { trigger: () => panel.getByRole("button", { name: "No expiration" }), dialog: "Edit Expires" },
    ];

    for (const row of rows) {
      await row.trigger().click();
      const editor = page.getByRole("dialog", { name: row.dialog });
      await expect(editor, `${row.dialog} should open`).toBeVisible();

      const scroll = await panelScroll(panel);
      expect(scroll.scrollLeft, `${row.dialog} should not scroll the panel sideways`).toBe(0);
      expect(scroll.overflowX, `${row.dialog} should not widen the panel`).toBeLessThanOrEqual(0);

      // Close without relying on Escape: SearchableSelect intentionally closes
      // its parent property editor, so a click back on the panel is the common
      // dismiss path for every row.
      await panel.getByText("Code", { exact: true }).click();
      await expect(editor, `${row.dialog} should close`).toBeHidden();
      await expect(panel).toBeVisible();
    }
  });

  test("long Cycle labels stay within the shared 28rem content bound", async ({ page }) => {
    const longCycle: CycleFixture = {
      id: "cycle-long",
      label: "Extended cycle",
      display_name:
        "2026 International Baccalaureate Extended Preparation Cycle — Autumn Intake",
      start_date: "2026-01-01",
      end_date: "2026-12-31",
    };
    await installRoutes(page, { cycles: [longCycle] });
    await page.goto("/courses/course-1");

    const panel = await openProperties(page);
    await panel.getByRole("button", { name: "No cycle" }).click();

    const editor = page.getByRole("dialog", { name: "Edit Cycle" });
    await expect(editor).toBeVisible();
    await expect(editor.getByText(longCycle.display_name!)).toBeVisible();
    await expectWithinWidthCap(editor, page, 28 * 16);
    await expectInsideViewport(editor, page);
  });

  test("long SearchableSelect labels respect the content bound and visible viewport", async ({ page }) => {
    const longSubject = {
      id: "subject-long",
      code: "MATH",
      name: "Advanced Mathematics for the International Baccalaureate Extended Programme 2026",
    };
    await installRoutes(page, { subjects: [longSubject] });
    await page.goto("/courses/course-1");

    const panel = await openProperties(page);
    await panel.getByRole("button", { name: "No subject" }).click();

    const editor = page.getByRole("dialog", { name: "Edit Subject" });
    await expect(editor).toBeVisible();
    await editor.getByRole("combobox").click();

    const listbox = page.getByRole("dialog", { name: "Search subject…" });
    await expect(listbox).toBeVisible();
    await expect(listbox.getByRole("option", { name: /Advanced Mathematics/ })).toBeVisible();
    await expectWithinWidthCap(listbox, page, 28 * 16);
    await expectInsideViewport(listbox, page);

    // The list is portaled outside the property editor and must not widen the
    // properties panel either.
    const scroll = await panelScroll(panel);
    expect(scroll.scrollLeft).toBe(0);
    expect(scroll.overflowX).toBeLessThanOrEqual(0);
  });
});

test.describe("Course properties on a narrow viewport", () => {
  test.use({ viewport: { width: 320, height: 844 } });

  test("properties and nested Teachers editor stay inside the viewport", async ({ page }) => {
    await installRoutes(page);
    await page.goto("/courses/course-1");

    const panel = await openProperties(page);
    await expectInsideViewport(panel, page);

    await panel.getByRole("button", { name: /Alice Smith/ }).click();
    const editor = page.getByRole("dialog", { name: "Edit Teachers" });
    await expect(editor).toBeVisible();
    await expectInsideViewport(editor, page);

    const scroll = await panelScroll(panel);
    expect(scroll.scrollLeft).toBe(0);
    expect(scroll.overflowX).toBeLessThanOrEqual(0);
  });

  test("properties trigger stays visible with a long course title", async ({ page }) => {
    const longTitleCourse = makeCourse({
      name: "Advanced Mathematics for International Baccalaureate Students 2026",
      code: "MATH101",
      teachers: [{ id: "teacher-a", username: "Alice Smith", is_primary: true }],
    });
    await installRoutes(page, { course: longTitleCourse });
    await page.goto("/courses/course-1");

    const documentWidth = await page.evaluate(() => ({
      scrollWidth: document.documentElement.scrollWidth,
      clientWidth: document.documentElement.clientWidth,
    }));
    expect(documentWidth.scrollWidth).toBeLessThanOrEqual(documentWidth.clientWidth + 1);

    // openProperties asserts the trigger is fully inside the visible viewport
    // and clicks its normal center.
    await openProperties(page);
  });
});
