import { expect, test } from "@playwright/test";
import { installAbsenceRoutes, studentLookup, type SubmittedPayload } from "./fixtures/absence";
import { acceptResumePrompt, completeToReview } from "./helpers/absenceFlow";

const verificationStorageKey = "warwick-absence-parent-verification-v1";

test("quota rejection keeps the reviewed request available for correction", async ({ page }) => {
  const submitted: SubmittedPayload[] = [];
  let requestCount = 0;
  await installAbsenceRoutes(page, submitted);
  await page.route("**/api/v1/absences/batch", async (route) => {
    requestCount += 1;
    await route.fulfill({
      status: 403,
      contentType: "application/json",
      body: JSON.stringify({ code: "absence_limit_exceeded", message: "Absence limit exceeded" }),
    });
  });

  await completeToReview(page, "Quota recovery");
  await page.getByRole("button", { name: "Submit absence" }).click();

  await expect(page.getByRole("status").filter({ hasText: /reached the absence limit/i })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Review your absence" })).toBeVisible();
  await expect(page.getByText("Quota recovery")).toBeVisible();
  expect(requestCount).toBe(1);
});

test("a missing make-up cannot be skipped into review", async ({ page }) => {
  const submitted: SubmittedPayload[] = [];
  await page.addInitScript((draft) => {
    window.sessionStorage.setItem("warwick.absence.draft.v1", JSON.stringify(draft));
  }, {
    schemaVersion: 1,
    updatedAt: Date.now(),
    wcode: studentLookup.wcode,
    collectedEmail: "student@example.com",
    step: 2,
    selectedSubjectIds: ["subject-math"],
    selectedSessionIds: ["missed-boundary"],
    sitInSelections: {},
    sitInPriorityLevels: {},
    reason: "Saved reason",
  });
  await installAbsenceRoutes(page, submitted);

  await page.goto("/absence");
  await acceptResumePrompt(page);
  await page.getByRole("button", { name: "Yes, continue" }).click();
  await page.getByRole("button", { name: /^(send code|resend code)$/i }).click();
  await page.locator('input[aria-label="Confirmation code"]').fill("123456", { force: true });
  await page.getByRole("heading", { name: "Which class will you miss?" }).waitFor();
  // The restored draft requires reviewing the current classes first.
  await page.getByRole("button", { name: "Review updated classes" }).click();
  await page.getByRole("button", { name: "Continue" }).click();

  // The physical make-up must be chosen — review stays unreachable.
  await expect(page.getByRole("heading", { name: "Your make-up" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Use this class" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Review your absence" })).toHaveCount(0);
});

test("a server-invalid saved verification returns the student to verification", async ({ page }) => {
  await page.addInitScript(({ key }) => {
    sessionStorage.setItem(key, JSON.stringify({ token: "expired-token", expiresAt: Date.now() + 60_000 }));
  }, { key: verificationStorageKey });
  const submitted: SubmittedPayload[] = [];
  await installAbsenceRoutes(page, submitted);
  // Registered after the fixture so this handler takes precedence (Playwright routes are LIFO).
  await page.route("**/api/v1/absences/parent-verification/status", async (route) => {
    await route.fulfill({
      status: 410,
      contentType: "application/json",
      body: JSON.stringify({ code: "otp_expired", message: "Verification token expired" }),
    });
  });

  await page.goto("/absence");
  await page.getByPlaceholder("e.g. W250389").fill("W250389");
  await page.getByRole("button", { name: "Continue" }).click();

  await expect(page.getByRole("heading", { name: "Is this you?" })).toBeVisible();
  await page.getByRole("button", { name: "Yes, continue" }).click();

  // The stale token is rejected server-side and the student can simply resend.
  await expect(page.getByRole("heading", { name: /confirm with a parent/i })).toBeVisible();
  await expect(page.getByRole("button", { name: /^(send code|resend code)$/i })).toBeEnabled();
});