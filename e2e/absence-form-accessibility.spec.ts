import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import {
  boundarySitInSession,
  installAbsenceRoutes,
  studentLookup,
  type SubmittedPayload,
} from "./fixtures/absence";

async function expectAccessiblePage(page: Page) {
  const results = await new AxeBuilder({ page }).analyze();
  const blocking = results.violations.filter(
    (violation) => violation.impact === "critical" || violation.impact === "serious",
  );
  expect(blocking, blocking.map(({ id, help }) => `${id}: ${help}`).join("\n")).toEqual([]);
}

async function expectNoHorizontalOverflow(page: Page) {
  await expect
    .poll(() =>
      page.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    )
    .toBe(true);
}

async function expectAnimationsSettled(page: Page) {
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          [...document.querySelectorAll<HTMLElement>('[style*="opacity"]')].filter((element) => {
            const opacity = Number.parseFloat(getComputedStyle(element).opacity);
            return element.getClientRects().length > 0 && opacity < 0.999;
          }).length,
      ),
    )
    .toBe(0);
}

async function expectOtpBoxesInsideViewport(page: Page) {
  const boxes = page
    .locator('input[aria-label="Verification code"]')
    .locator("xpath=..")
    .locator(':scope > div[aria-hidden="true"]');
  await expect(boxes).toHaveCount(6);
  const viewportWidth = await page.evaluate(() => window.innerWidth);
  for (const box of await boxes.all()) {
    const bounds = await box.boundingBox();
    expect(bounds).not.toBeNull();
    expect(bounds!.x).toBeGreaterThanOrEqual(0);
    expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(viewportWidth);
  }
}

test("public absence form stays accessible and contained through every mobile step", async ({
  browser,
}) => {
  const context = await browser.newContext({
    timezoneId: "Asia/Bangkok",
    viewport: { width: 320, height: 844 },
    reducedMotion: "reduce",
  });
  const page = await context.newPage();
  const submitted: SubmittedPayload[] = [];
  await installAbsenceRoutes(page, submitted);

  await page.goto("/absence");
  await expect(page.getByText("Step 1 of 4: Student")).toBeVisible();
  await expectAccessiblePage(page);
  await expectNoHorizontalOverflow(page);

  await page.getByPlaceholder("e.g. W250389").fill(studentLookup.wcode);
  await page.getByRole("button", { name: /search/i }).click();
  await page.getByLabel(/your email address/i).fill("student@example.com");
  await page.getByRole("button", { name: "Continue to verification" }).click();

  await expect(page.getByText("Step 2 of 4: Verify")).toBeVisible();
  await expect(page.getByRole("button", { name: /send code/i }).locator("span")).toHaveCSS(
    "opacity",
    "1",
  );
  await expectAccessiblePage(page);
  await expectNoHorizontalOverflow(page);
  await page.getByRole("button", { name: /send code/i }).click();
  await expect(page.locator('input[aria-label="Verification code"]')).toBeAttached();
  await expectAnimationsSettled(page);
  await expectNoHorizontalOverflow(page);
  await expectOtpBoxesInsideViewport(page);
  await expectAccessiblePage(page);
  await page.locator('input[aria-label="Verification code"]').fill("123456", { force: true });

  await expect(page.getByText("Step 3 of 4: Classes")).toBeVisible();
  await expectAccessiblePage(page);
  await expectNoHorizontalOverflow(page);
  await page.locator("#subject-subject-math").check({ force: true });
  await page.locator("#session-missed-boundary").check();
  await page.getByRole("combobox").selectOption(boundarySitInSession.id);
  await page.getByLabel(/reason for absence/i).fill("Accessibility regression test");
  await page.getByRole("button", { name: "Review absence" }).click();

  await expect(page.getByText("Step 4 of 4: Review")).toBeVisible();
  await expectAccessiblePage(page);
  await expectNoHorizontalOverflow(page);
  await page.getByRole("button", { name: "Submit absence" }).click();

  await expect(page.getByRole("heading", { name: "Absence submitted" })).toBeVisible();
  await expect.poll(() => submitted.length).toBe(1);
  await expectAccessiblePage(page);
  await expectNoHorizontalOverflow(page);
  await context.close();
});
