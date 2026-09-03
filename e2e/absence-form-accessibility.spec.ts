import { expect, test } from "@playwright/test";
import {
  installAbsenceRoutes,
  type SubmittedPayload,
} from "./fixtures/absence";
import {
  expectAccessiblePage,
  expectAnimationsSettled,
  expectNoHorizontalOverflow,
  expectOtpBoxesInsideViewport,
  expectProgress,
} from "./helpers/absenceAssertions";
import { completeToReview } from "./helpers/absenceFlow";

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

  await expectProgress(page, 0, 8);
  await expectAccessiblePage(page);
  await expectNoHorizontalOverflow(page);

  await page.getByPlaceholder("e.g. W250389").fill("W250389");
  await page.getByRole("button", { name: "Continue" }).click();
  await expect(page.getByRole("heading", { name: "Is this you?" })).toBeVisible();
  await expectAccessiblePage(page);
  await expectNoHorizontalOverflow(page);
  await page.getByRole("button", { name: "Yes, continue" }).click();
  await expectProgress(page, 20, 40);
  await expect(page.getByRole("button", { name: /^(send code|resend code)$/i })).toBeVisible();
  await expectAccessiblePage(page);
  await expectNoHorizontalOverflow(page);
  await page.getByRole("button", { name: /^(send code|resend code)$/i }).click();
  await expect(page.locator('input[aria-label="Confirmation code"]')).toBeAttached();
  await expectAnimationsSettled(page);
  await expectNoHorizontalOverflow(page);
  await expectOtpBoxesInsideViewport(page);
  await expectAccessiblePage(page);
  await page.locator('input[aria-label="Confirmation code"]').fill("123456", { force: true });
  await expect(page.getByRole("heading", { name: "Where should we send updates?" })).toBeVisible();
  await page.getByLabel(/^email$/i).fill("student@example.com");
  await page.getByRole("button", { name: "Continue" }).click();
  await expectProgress(page, 40, 65);
  await expect(page.getByRole("heading", { name: "Which class will you miss?" })).toBeVisible();
  await expectAccessiblePage(page);
  await expectNoHorizontalOverflow(page);

  await completeToReview(page, "Accessibility regression test", "student@example.com");

  await expectProgress(page, 80, 100);
  await expectAccessiblePage(page);
  await expectNoHorizontalOverflow(page);
  await page.getByRole("button", { name: "Submit absence" }).click();

  await expect(page.getByRole("heading", { name: "Absence submitted" })).toBeVisible();
  await expect.poll(() => submitted.length).toBe(1);
  await expectAccessiblePage(page);
  await expectNoHorizontalOverflow(page);
});