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

async function installRoutes(page: Page) {
  await page.route("**/api/v1/crm/cycles", (route) =>
    route.fulfill({ contentType: "application/json", body: "[]" }),
  );
  await installCourseTeacherRoutes(page, { course });
}

async function openProperties(page: Page): Promise<Locator> {
  const trigger = page.getByRole("button", { name: "Edit course properties" });
  const viewport = page.viewportSize();
  const box = await trigger.boundingBox();
  // At phone width the course header itself can extend past the viewport. Aim
  // at the visible part of the trigger instead of its off-screen center; the
  // nested-overlay regression below is about the panel, not that header.
  const position =
    viewport && box && box.x + box.width > viewport.width
      ? { x: Math.max(1, viewport.width - box.x - 4), y: box.height / 2 }
      : undefined;
  await trigger.click(position ? { position } : {});
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

async function expectInsideViewport(locator: Locator, page: Page) {
  const box = await locator.boundingBox();
  const viewport = page.viewportSize();
  const metrics = await page.evaluate(() => ({
    innerWidth: window.innerWidth,
    innerHeight: window.innerHeight,
    visualWidth: window.visualViewport?.width,
    visualHeight: window.visualViewport?.height,
  }));
  // A mobile layout viewport can be wider than what is actually visible.
  const visibleWidth = Math.min(viewport?.width ?? metrics.innerWidth, metrics.visualWidth ?? metrics.innerWidth);
  const visibleHeight = Math.min(viewport?.height ?? metrics.innerHeight, metrics.visualHeight ?? metrics.innerHeight);
  expect(box).not.toBeNull();
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.y).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(visibleWidth + 1);
  expect(box!.y + box!.height).toBeLessThanOrEqual(visibleHeight + 1);
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
});
