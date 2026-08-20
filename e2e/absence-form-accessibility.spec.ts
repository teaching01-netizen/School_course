import { expect, test, type Page } from "@playwright/test";
import {
  boundarySitInSession,
  installAbsenceRoutes,
  studentLookup,
  type SubmittedPayload,
} from "./fixtures/absence";
import {
  expectAccessiblePage,
  expectAnimationsSettled,
  expectCurrentStep,
  expectNoHorizontalOverflow,
  expectOtpBoxesInsideViewport,
  expectAdaptiveTabletClassesLayout,
} from "./helpers/absenceAssertions";
import { selectAbsenceCheckbox } from "./helpers/absenceFlow";


async function chooseMakeUpClass(page: Page) {
  const select = page.getByRole("combobox");
  if (await select.isVisible()) {
    await select.selectOption(boundarySitInSession.id);
  } else {
    await page.getByRole("button", { name: /choose a make-up class/i }).click();
    const dialog = page.getByRole("dialog", { name: /choose a make-up class/i });
    await dialog.getByRole("radio").first().check();
    await dialog.getByRole("button", { name: "Confirm make-up class" }).click();
  }
  await expect(page.locator("#sit-in-missed-boundary")).toHaveValue(boundarySitInSession.id);
}
test.use({
  timezoneId: "Asia/Bangkok",
});
test.setTimeout(120_000);

test("public absence form stays accessible and contained through every supported viewport", async ({
  page,
}) => {
  const submitted: SubmittedPayload[] = [];
  await installAbsenceRoutes(page, submitted);
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/absence");
  await expect(page.locator('meta[name="viewport"]')).toHaveAttribute("content", /viewport-fit=cover/);

  await expectCurrentStep(page, 1, "Student");
  await expectAccessiblePage(page);
  await expectNoHorizontalOverflow(page);

  await page.getByPlaceholder("e.g. W250389").fill(studentLookup.wcode);
  await page.getByRole("button", { name: /search/i }).click();
  await page.getByLabel(/your email address/i).fill("student@example.com");
  await page.getByRole("button", { name: "Continue to verification" }).click();
  await expectCurrentStep(page, 2, "Verify");
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
  await expectCurrentStep(page, 3, "Classes");
  await expectAccessiblePage(page);
  await expectAdaptiveTabletClassesLayout(page);
  await selectAbsenceCheckbox(page, "subject-subject-math");
  await selectAbsenceCheckbox(page, "session-missed-boundary");
  await chooseMakeUpClass(page);
  await page.getByLabel(/reason for absence/i).fill("Accessibility regression test");
  await page.getByRole("button", { name: "Review absence" }).click();
  await expectCurrentStep(page, 4, "Review");
  await expectAccessiblePage(page);
  await expectNoHorizontalOverflow(page);
  await page.getByRole("button", { name: "Submit absence" }).click();

  await expect(page.getByRole("heading", { name: "Absence submitted" })).toBeVisible();
  await expect.poll(() => submitted.length).toBe(1);
  await expectAccessiblePage(page);
  await expectNoHorizontalOverflow(page);
});
