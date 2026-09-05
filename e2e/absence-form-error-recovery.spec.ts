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

  await expect(page.getByRole("alert").filter({ hasText: /reached the absence limit/i })).toBeVisible();
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
  await expect(page.getByRole("heading", { name: "Is this you?" })).toBeVisible();
  await page.getByRole("button", { name: "Yes, continue" }).click();
  await page.getByRole("button", { name: /^(send code|resend code)$/i }).click();
  await page.locator('input[aria-label="Confirmation code"]').fill("123456", { force: true });
  // The saved report is offered for resume only after verification.
  await acceptResumePrompt(page);
  // The restored draft requires reviewing the current classes first.
  await page.getByRole("button", { name: /i've reviewed/i }).click();
  await page.getByRole("button", { name: "Continue" }).click();

  // The physical make-up must be chosen — review stays unreachable.
  await expect(page.getByRole("heading", { name: "Your make-up" })).toBeVisible();
  await expect(page.getByRole("button", { name: /choose a time|change time/i })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Review your absence" })).toHaveCount(0);
});

test("a rejected code is never auto-retried; a corrected code verifies exactly once", async ({ page }) => {
  const verifyCodes: string[] = [];
  await installAbsenceRoutes(page, []);
  // Registered after the fixture so this handler takes precedence (LIFO).
  await page.route("**/api/v1/absences/parent-verification/verify", async (route) => {
    const body = route.request().postDataJSON() as { code: string };
    verifyCodes.push(body.code);
    if (body.code !== "123456") {
      await route.fulfill({
        status: 400,
        contentType: "application/json",
        body: JSON.stringify({ code: "invalid_code", message: "Invalid code" }),
      });
      return;
    }
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        token: "verification-token",
        status: "verified",
        wcode: studentLookup.wcode,
        parent_phone: "+66812345678",
        delivery_status: "accepted",
        otp_code_expires_at: new Date(Date.now() + 10 * 60_000).toISOString(),
        expires_at: new Date(Date.now() + 60 * 60_000).toISOString(),
      }),
    });
  });

  await page.goto("/absence");
  await page.getByPlaceholder("e.g. W250389").fill(studentLookup.wcode);
  await page.getByRole("button", { name: "Continue" }).click();
  await expect(page.getByRole("heading", { name: "Is this you?" })).toBeVisible();
  await page.getByRole("button", { name: "Yes, continue" }).click();
  await expect(page.getByRole("heading", { name: /confirm with a parent/i })).toBeVisible();
  await page.getByRole("button", { name: /^(send code|resend code)$/i }).click();
  const codeInput = page.locator('input[aria-label="Confirmation code"]');

  // A wrong code is rejected once — and never auto-retried while it stays on
  // screen (a regression here used to loop /verify until the code was edited).
  await codeInput.fill("111111");
  await expect(page.getByText(/that code isn't right/i)).toBeVisible();
  await page.waitForTimeout(900);
  expect(verifyCodes).toEqual(["111111"]);

  // The corrected code triggers exactly one new request and verifies.
  await codeInput.fill("123456");
  await expect(page.getByRole("heading", { name: "Confirmed" })).toBeVisible();
  await page.waitForTimeout(600);
  expect(verifyCodes).toEqual(["111111", "123456"]);
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