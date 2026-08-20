import { expect, test } from "@playwright/test";
import { installAbsenceRoutes, type SubmittedPayload } from "./fixtures/absence";
import { completeToClasses, completeToReview, selectAbsenceCheckbox } from "./helpers/absenceFlow";

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

  await expect(page.getByRole("alert").filter({ hasText: /maximum absences allowed/i })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Review your absence" })).toBeVisible();
  await expect(page.getByText("Quota recovery")).toBeVisible();
  expect(requestCount).toBe(1);
});

test("missing make-up selection blocks review and focuses a visible picker", async ({ page }) => {
  const submitted: SubmittedPayload[] = [];
  await installAbsenceRoutes(page, submitted);
  await completeToClasses(page);
  await selectAbsenceCheckbox(page, "subject-subject-math");
  await selectAbsenceCheckbox(page, "session-missed-boundary");
  await page.getByRole("button", { name: "Review absence" }).click();

  await expect(page.getByRole("alert").filter({ hasText: /pick a make-up class/i })).toBeVisible();
  await expect
    .poll(() =>
      page.evaluate(() => {
        const active = document.activeElement;
        if (!(active instanceof HTMLElement)) return false;
        const styles = getComputedStyle(active);
        return active.matches("[data-make-up-trigger], select[aria-label*='make-up' i], select")
          && styles.display !== "none"
          && styles.visibility !== "hidden"
          && active.getClientRects().length > 0;
      }),
    )
    .toBe(true);
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
  await page.getByRole("button", { name: /search/i }).click();
  await page.getByLabel(/your email address/i).fill("student@example.com");
  await page.getByRole("button", { name: "Continue to verification" }).click();

  await expect(page.getByRole("heading", { name: "Parent verification" })).toBeVisible();
  await expect(page.getByRole("button", { name: /send code/i })).toBeEnabled();
  await expect(page.getByText(/verification has expired|verification phone/i).first()).toBeVisible();
});
