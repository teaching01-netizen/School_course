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
import { selectClass } from "./helpers/absenceFlow";

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

  // Stage 1 — Student: identify.
  await expectProgress(page, 0, 8);
  await expectAccessiblePage(page);
  await expectNoHorizontalOverflow(page);

  await page.getByPlaceholder("e.g. W250389").fill("W250389");
  await page.getByRole("button", { name: "Continue" }).click();
  await expect(page.getByRole("heading", { name: "Is this you?" })).toBeVisible();
  await expectAccessiblePage(page);
  await expectNoHorizontalOverflow(page);

  // Stage 1 — Student: parent confirmation (OTP entry stays contained).
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

  // Verification completes on the student's own tap (WCAG 3.2.1), then lands
  // on Classes — the update email is collected in Details.
  await page.locator('input[aria-label="Confirmation code"]').fill("123456", { force: true });
  await expect(page.getByRole("heading", { name: "Confirmed" })).toBeVisible();
  await page.getByRole("status").getByRole("button", { name: "Continue" }).click();
  await expect(page.getByRole("heading", { name: "Which class will you miss?" })).toBeVisible();
  await expectProgress(page, 40, 65);
  await expectAccessiblePage(page);
  await expectNoHorizontalOverflow(page);

  // Stage 2 — Classes.
  await selectClass(page, "Mathematics");
  await page.getByRole("button", { name: "Continue" }).click();

  // Stage 3 — Make-up: one calm plan, each class paired with its arrangement.
  await expect(page.getByRole("heading", { name: "Your make-up" })).toBeVisible();
  await expectProgress(page, 70, 95);
  await expectAccessiblePage(page);
  await expectNoHorizontalOverflow(page);
  await page.getByRole("button", { name: /choose a time|change time/i }).click();
  await page.getByRole("dialog").getByRole("radio").first().click();
  await page.getByRole("button", { name: "Use this time" }).click();
  await page.getByRole("button", { name: "Continue", exact: true }).click();

  // Stage 4 — Details: reason and required email together.
  await expect(page.getByRole("heading", { name: "Why will you be away?" })).toBeVisible();
  await expectProgress(page, 85, 95);
  await expectAccessiblePage(page);
  await expectNoHorizontalOverflow(page);
  await page.getByRole("radio", { name: "Other" }).click();
  await page.getByLabel(/tell us a little more/i).fill("Accessibility regression test");
  await page.getByRole("textbox", { name: /^email$/i }).fill("student@example.com");
  await page.getByRole("button", { name: "Continue" }).click();

  // Stage 5 — Review, then the receipt.
  await expect(page.getByRole("heading", { name: "Review your absence" })).toBeVisible();
  await expectProgress(page, 80, 100);
  await expectAccessiblePage(page);
  await expectNoHorizontalOverflow(page);
  await page.getByRole("button", { name: "Submit absence" }).click();

  await expect(page.getByRole("heading", { name: "Absence report submitted" })).toBeVisible();
  await expect.poll(() => submitted.length).toBe(1);
  await expectAccessiblePage(page);
  await expectNoHorizontalOverflow(page);
});
