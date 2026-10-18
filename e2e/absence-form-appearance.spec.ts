import { expect, test } from "@playwright/test";
import { installAbsenceRoutes, type SubmittedPayload } from "./fixtures/absence";
import { chooseMakeUpClass, completeToClasses, completeToReview, selectAbsenceCheckbox } from "./helpers/absenceFlow";
import { expectAccessiblePage, expectNoHorizontalOverflow } from "./helpers/absenceAssertions";

test.use({ timezoneId: "Asia/Bangkok" });

test("stays in light appearance when the device uses dark mode", async ({ page }, testInfo) => {
  await installAbsenceRoutes(page, []);
  await page.emulateMedia({ colorScheme: "dark", reducedMotion: "reduce" });
  await page.goto("/absence");
  await expect(page.getByRole("heading", { name: "Who will be absent?" })).toBeVisible();
  await expect(page.locator(".absence-app-shell")).toHaveCSS("color-scheme", "light");
  await expect(page.locator(".absence-student-lookup")).toHaveCSS("background-color", "rgb(255, 255, 255)");
  await expectAccessiblePage(page);
  await page.screenshot({ path: testInfo.outputPath("light-student.png") });

  await page.getByLabel("Student ID (W-Code)").fill("W250389");
  await page.getByRole("button", { name: "Find student", exact: true }).click();
  const email = page.getByLabel(/email address/i);
  await email.fill("unfinished");
  await expect(email).not.toHaveAttribute("aria-invalid", "true");
  await page.getByRole("heading", { name: "Johnny", exact: true }).focus();
  await expect(email).toHaveAttribute("aria-invalid", "true");
  await expect(email).toHaveAccessibleDescription("Enter a valid email address.");
  await email.fill("student@example.com");
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Parent verification" })).toBeFocused();
  await page.getByRole("button", { name: "Send code", exact: true }).click();
  await expectAccessiblePage(page);
  await page.screenshot({ path: testInfo.outputPath("light-verification.png") });
  await page.getByLabel("Verification code", { exact: true }).fill("123456");

  await selectAbsenceCheckbox(page, "subject-subject:subject-math");
  await selectAbsenceCheckbox(page, "session-missed-boundary");
  if (await page.locator("[data-make-up-trigger]").isVisible()) {
    await page.locator("[data-make-up-trigger]").click();
    await expect(page.getByRole("dialog")).toHaveCSS("color-scheme", "light");
    await expectAccessiblePage(page);
    await page.screenshot({ path: testInfo.outputPath("light-make-up-sheet.png") });
    await page.getByRole("button", { name: "Cancel", exact: true }).click();
  }
  await chooseMakeUpClass(page);
  await page.getByLabel(/reason for absence/i).fill("Medical appointment");
  await expectAccessiblePage(page);
  await page.screenshot({ path: testInfo.outputPath("light-classes.png") });
  await page.getByRole("button", { name: "Review absence" }).click();
  await expectAccessiblePage(page);
  await expectNoHorizontalOverflow(page);
  await page.screenshot({ path: testInfo.outputPath("light-review.png") });
  await page.getByRole("button", { name: "Submit absence" }).click();
  await expect(page.getByRole("heading", { name: "Absence submitted" })).toBeVisible();
  await expect(page.locator(".absence-receipt")).toHaveCSS("color-scheme", "light");
  await expectAccessiblePage(page);
  await page.screenshot({ path: testInfo.outputPath("light-receipt.png") });
});

test("keeps the reviewed details visible and prevents navigation while submitting", async ({ page }) => {
  const submitted: SubmittedPayload[] = [];
  await installAbsenceRoutes(page, submitted);
  let releaseSubmission!: () => void;
  const pendingSubmission = new Promise<void>((resolve) => { releaseSubmission = resolve; });
  await page.route("**/api/v1/absences/batch", async (route) => {
    await pendingSubmission;
    await route.fallback();
  });
  await completeToReview(page);
  await page.getByRole("button", { name: "Submit absence" }).click();
  try {
    await expect(page.getByRole("status").filter({ hasText: /Submitting your absence/ })).toBeVisible();
    await expect(page.getByRole("button", { name: "Back", exact: true })).toBeDisabled();
    await expect(page.getByRole("button", { name: /Submitting/ })).toBeDisabled();
    await expect(page.getByRole("button", { name: "Student - completed" })).toHaveAttribute("tabindex", "-1");
    await expect(page.getByText("Medical appointment")).toBeVisible();
    await expect(page.getByRole("heading", { name: "Absence submitted" })).toHaveCount(0);
  } finally {
    releaseSubmission();
  }
  await expect(page.getByRole("heading", { name: "Absence submitted" })).toBeVisible();
  expect(submitted).toHaveLength(1);
});

test("shows an actionable class-loading error before any course has been selected", async ({ page }) => {
  await installAbsenceRoutes(page, []);
  let classesAvailable = false;
  await page.route("**/api/v1/absence-self-service/sessions**", (route) => classesAvailable
    ? route.fallback()
    : route.fulfill({ status: 503, json: { code: "unavailable", message: "Class dates are temporarily unavailable. Try again." } }));
  await completeToClasses(page);
  await expect(page.getByRole("alert").filter({ hasText: "Class dates are temporarily unavailable" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Review absence" })).toBeDisabled();
  await expect(page.getByText("No courses available.")).toHaveCount(0);
  classesAvailable = true;
  await page.getByRole("button", { name: "Retry loading classes" }).click();
  await expect(page.getByRole("checkbox", { name: "Mathematics", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Review absence" })).toBeEnabled();
});
